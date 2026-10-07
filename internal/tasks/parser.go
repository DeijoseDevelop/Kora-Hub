// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
// Package tasks implementa el motor de tareas embebidas: convierte
// Markdown plano en un sistema de gestión sin bases de datos de tareas
// separadas. Es el activo intelectual central del producto.
//
// Gramática (Documento Técnico de Arquitectura, sección 6.1):
//
//	tarea := checkbox WS texto (WS metadato)*
//	checkbox := '- [ ]' | '- [x]' | '- [~]'      -- ~ = en progreso
//	metadato := fecha | proyecto | prioridad | asignado | etiqueta
//	          | recurrencia | identificador | dependencia
//	fecha := '#' (AAAA-MM-DD | 'hoy' | 'mañana' | 'lun'..'dom')
//	proyecto := '@' ident          prioridad := '!' (baja|media|alta|1..3)
//	asignado := '~' ident          etiqueta := '+' ident
//	recurrencia := '*' 'every:' [1-9][0-9]* ('d'|'w'|'m'|'y')
//	identificador := '^id:' ident  dependencia := '^blocked-by:' ident
//	ident := [a-z0-9-]+            -- cualquier valor admite "comillas"
//
// Invariantes: idempotente, tolerante y con round-trip garantizado:
// toda edición desde una vista reescribe la línea original preservando
// el resto del texto al byte (sección 6.2).
package tasks

import (
	"strconv"
	"strings"
	"time"
)

// State de una tarea embebida.
const (
	StateOpen     = " "
	StateDone     = "x"
	StateProgress = "~"
)

// Task es la representación parseada de una línea de checkbox.
// RawLine conserva la línea original para garantizar el round-trip.
type Task struct {
	Line       int      `json:"line_no"` // 1-based dentro del documento
	RawLine    string   `json:"-"`
	Text       string   `json:"title"`
	Done       bool     `json:"done"`
	InProgress bool     `json:"in_progress"`
	DueDate    string   `json:"due_date"` // AAAA-MM-DD
	Project    string   `json:"project"`
	Priority   string   `json:"priority"` // baja | media | alta
	Assignee   string   `json:"assignee"`
	Tags       []string `json:"tags"`
	Recur      string   `json:"recur"`      // *every:<intervalo> (sección 6.5)
	TaskUID    string   `json:"task_uid"`   // ^id: identidad estable opcional
	BlockedBy  string   `json:"blocked_by"` // ^blocked-by: referencia a otro ^id
}

// ParseLine intenta parsear una línea de documento como tarea embebida.
// Devuelve ok=false si la línea no es un checkbox Markdown.
func ParseLine(line string) (Task, bool) {
	trimmed := strings.TrimRight(line, "\r\n ")
	if len(trimmed) < 6 || trimmed[0] != '-' {
		return Task{}, false
	}
	state, rest, ok := parseCheckbox(trimmed)
	if !ok {
		return Task{}, false
	}

	t := Task{
		RawLine:    trimmed,
		Done:       state == StateDone,
		InProgress: state == StateProgress,
	}

	// texto + metadatos: separados por espacios, respetando "comillas"
	parts := splitMeta(rest)
	textParts := make([]string, 0, len(parts))
	for _, p := range parts {
		switch {
		case strings.HasPrefix(p, "#"):
			if d := parseDate(p[1:]); d != "" {
				t.DueDate = d
			} else {
				// #no-fecha es texto del título (un hashtag), no un slot
				textParts = append(textParts, p)
			}
		case strings.HasPrefix(p, "*every:") && isRecurInterval(p[7:]):
			t.Recur = p[7:]
		case strings.HasPrefix(p, "^id:") && isIdentValue(p[4:]):
			t.TaskUID = unquote(p[4:])
		case strings.HasPrefix(p, "^blocked-by:") && isIdentValue(p[12:]):
			t.BlockedBy = unquote(p[12:])
		case strings.HasPrefix(p, "@") && isIdentValue(p[1:]):
			t.Project = unquote(p[1:])
		case strings.HasPrefix(p, "!") && isPriority(unquote(p[1:])):
			t.Priority = normalizePriority(unquote(p[1:]))
		case strings.HasPrefix(p, "~") && isIdentValue(p[1:]):
			t.Assignee = unquote(p[1:])
		case strings.HasPrefix(p, "+") && isIdentValue(p[1:]):
			t.Tags = append(t.Tags, unquote(p[1:]))
		default:
			// tolerante: metadatos desconocidos se conservan como texto
			textParts = append(textParts, p)
		}
	}
	t.Text = strings.Join(textParts, " ")
	return t, true
}

// Parse recorre un documento y extrae todas las tareas embebidas.
// Idempotente: parsear dos veces el mismo documento produce el mismo
// conjunto (clave: doc_id + line_no + hash de línea).
func Parse(content string) []Task {
	var out []Task
	for i, line := range strings.Split(content, "\n") {
		if t, ok := ParseLine(line); ok {
			t.Line = i + 1
			out = append(out, t)
		}
	}
	return out
}

// RoundTrip reescribe la línea original de una tarea tras una mutación,
// preservando al byte el texto que no sea de metadatos. Garantiza que
// completar/reprogramar/reasignar desde una vista nunca corrompa la
// redacción original (sección 6.2).
//
// state es el nuevo estado del checkbox (StateOpen/StateDone/StateProgress).
// Un parámetro de metadato vacío conserva el token original; si trae
// valor se reemplaza la primera aparición válida de ese slot y, si no
// existía, se añade al final. Los tokens que no son metadatos válidos
// (p. ej. #regalo o !foo) son texto y se conservan intactos.
func RoundTrip(t Task, state string, due, project, priority, assignee string) string {
	_, rest, ok := parseCheckbox(t.RawLine)
	if !ok {
		return t.RawLine // no debería pasar: t ya fue parseado
	}
	if state != StateOpen && state != StateDone && state != StateProgress {
		state = stateOf(t, false)
	}
	out := []string{"- [" + state + "]"}
	used := map[string]bool{"due": false, "project": false, "priority": false, "assignee": false}
	for _, p := range splitMeta(rest) {
		switch {
		case strings.HasPrefix(p, "#") && parseDate(p[1:]) != "":
			if due != "" && !used["due"] {
				out = append(out, "#"+quoteValue(due))
				used["due"] = true
			} else {
				out = append(out, p)
			}
		case strings.HasPrefix(p, "@") && isIdentValue(p[1:]):
			if project != "" && !used["project"] {
				out = append(out, "@"+quoteValue(project))
				used["project"] = true
			} else {
				out = append(out, p)
			}
		case strings.HasPrefix(p, "!") && isPriority(unquote(p[1:])):
			if priority != "" && !used["priority"] {
				out = append(out, "!"+quoteValue(priority))
				used["priority"] = true
			} else {
				out = append(out, p)
			}
		case strings.HasPrefix(p, "~") && isIdentValue(p[1:]):
			if assignee != "" && !used["assignee"] {
				out = append(out, "~"+quoteValue(assignee))
				used["assignee"] = true
			} else {
				out = append(out, p)
			}
		default:
			// texto y metadatos no mutables (^id, ^blocked-by, *every, +tag,
			// tokens inválidos): se reescriben tal cual, al byte
			out = append(out, p)
		}
	}
	// slots nuevos: el valor se pidió pero la línea no lo traía
	if due != "" && !used["due"] {
		out = append(out, "#"+quoteValue(due))
	}
	if project != "" && !used["project"] {
		out = append(out, "@"+quoteValue(project))
	}
	if priority != "" && !used["priority"] {
		out = append(out, "!"+quoteValue(priority))
	}
	if assignee != "" && !used["assignee"] {
		out = append(out, "~"+quoteValue(assignee))
	}
	return strings.Join(out, " ")
}

// quoteValue envuelve en comillas un valor que no sea un ident plano
// (§6.5: cualquier valor admite "comillas" para admitir espacios).
func quoteValue(s string) string {
	if isIdent(s) {
		return s
	}
	return `"` + s + `"`
}

func stateOf(t Task, done bool) string {
	if done {
		return StateDone
	}
	if t.InProgress {
		return StateProgress
	}
	return StateOpen
}

// StateOf expone el estado de una tarea como constante de checkbox.
func StateOf(t Task) string {
	switch {
	case t.Done:
		return StateDone
	case t.InProgress:
		return StateProgress
	default:
		return StateOpen
	}
}

func parseCheckbox(line string) (state, rest string, ok bool) {
	// "- [ ]" | "- [x]" | "- [~]" (tolerante: [X] y tabulador tras '-')
	if len(line) < 6 || (line[1] != ' ' && line[1] != '\t') || line[2] != '[' || line[4] != ']' {
		return "", "", false
	}
	c := line[3]
	switch c {
	case ' ', 'x', '~':
	case 'X':
		c = 'x' // estilo GitHub: [X] cuenta como hecha
	default:
		return "", "", false
	}
	if len(line) < 7 || (line[5] != ' ' && line[5] != '\t') {
		return "", "", false // exige espacio tras ']' — sin él no es checkbox
	}
	rest = strings.TrimSpace(line[5:])
	if rest == "" {
		return "", "", false // tarea sin texto: se ignora
	}
	return string(c), rest, true
}

func isIdent(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return true
}

// splitMeta trocea en espacios respetando valores "entre comillas"
// (sección 6.5: cualquier valor de metadato admite comillas dobles).
// Las comillas se conservan en el token para el round-trip; unquote
// las retira al extraer el valor.
func splitMeta(s string) []string {
	var out []string
	var cur strings.Builder
	inQ := false
	for _, c := range s {
		switch {
		case c == '"':
			inQ = !inQ
			cur.WriteRune(c)
		case (c == ' ' || c == '\t') && !inQ:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(c)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// unquote retira las comillas dobles que envuelven un valor.
func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// isIdentValue acepta ident plano ([a-z0-9-]+) o cualquier valor entre
// comillas no vacío (permite espacios: @"proyecto largo").
func isIdentValue(s string) bool {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return len(s) > 2
	}
	return isIdent(s)
}

// isRecurInterval valida el intervalo de *every: [1-9][0-9]*(d|w|m|y).
func isRecurInterval(s string) bool {
	if len(s) < 2 {
		return false
	}
	unit := s[len(s)-1]
	if unit != 'd' && unit != 'w' && unit != 'm' && unit != 'y' {
		return false
	}
	num := s[:len(s)-1]
	if num[0] == '0' {
		return false // sin sentido: every:0d
	}
	for _, r := range num {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// SpawnRecurring construye la línea de la siguiente ocurrencia: la
// misma tarea reabierta ([ ]) con la fecha recalculada. El ^id/ la
// ^blocked-by no se copian — la nueva ocurrencia es una tarea distinta.
func SpawnRecurring(rawLine, nextDate string) string {
	fields := strings.Fields(rawLine)
	out := []string{"- [ ]"}
	rest := fields
	// el checkbox ocupa 2 tokens ([x]/[~]) o 3 ([ ] — el espacio parte)
	switch {
	case len(rest) >= 2 && rest[0] == "-" && strings.HasPrefix(rest[1], "[") && strings.HasSuffix(rest[1], "]"):
		rest = rest[2:]
	case len(rest) >= 3 && rest[0] == "-" && rest[1] == "[" && strings.HasSuffix(rest[2], "]"):
		rest = rest[3:]
	}
	for _, p := range rest {
		switch {
		case strings.HasPrefix(p, "#"):
			out = append(out, "#"+nextDate)
		case strings.HasPrefix(p, "^id:"), strings.HasPrefix(p, "^blocked-by:"):
			// identidad/dependencia no se heredan a la ocurrencia nueva
		default:
			out = append(out, p)
		}
	}
	return strings.Join(out, " ")
}

// NextOccurrence calcula la siguiente fecha de una tarea recurrente
// sumando el intervalo a la fecha base (due_date actual).
func NextOccurrence(dueDate, recur string) string {
	if len(recur) < 2 {
		return ""
	}
	base, err := time.Parse("2006-01-02", dueDate)
	if err != nil {
		return ""
	}
	n, err := strconv.Atoi(recur[:len(recur)-1])
	if err != nil {
		return ""
	}
	switch recur[len(recur)-1] {
	case 'd':
		return base.AddDate(0, 0, n).Format("2006-01-02")
	case 'w':
		return base.AddDate(0, 0, 7*n).Format("2006-01-02")
	case 'm':
		return base.AddDate(0, n, 0).Format("2006-01-02")
	case 'y':
		return base.AddDate(n, 0, 0).Format("2006-01-02")
	}
	return ""
}

func isPriority(s string) bool {
	switch s {
	case "baja", "media", "alta", "1", "2", "3":
		return true
	}
	return false
}

func normalizePriority(s string) string {
	switch s {
	case "1":
		return "alta"
	case "2":
		return "media"
	case "3":
		return "baja"
	}
	return s
}

// parseDate resuelve fechas relativas ('hoy', 'mañana', 'lun'..'dom' y
// sus nombres largos) a AAAA-MM-DD; fechas inválidas devuelven "" sin
// romper el parseo (regla tolerante, sección 6.2).
func parseDate(raw string) string {
	if raw == "" {
		return ""
	}
	if len(raw) == 10 && raw[4] == '-' && raw[7] == '-' {
		return raw // AAAA-MM-DD válida o no: se conserva
	}
	now := time.Now()
	switch strings.ToLower(raw) {
	case "hoy":
		return now.Format("2006-01-02")
	case "mañana":
		return now.AddDate(0, 0, 1).Format("2006-01-02")
	}
	dow := map[string]time.Weekday{
		"lun": time.Monday, "mar": time.Tuesday, "mie": time.Wednesday,
		"jue": time.Thursday, "vie": time.Friday, "sab": time.Saturday,
		"dom": time.Sunday,
		// nombres largos: tolerancia de escritura (misma semántica)
		"lunes": time.Monday, "martes": time.Tuesday, "miércoles": time.Wednesday,
		"miercoles": time.Wednesday, "jueves": time.Thursday, "viernes": time.Friday,
		"sábado": time.Saturday, "sabado": time.Saturday, "domingo": time.Sunday,
	}
	if wd, ok := dow[strings.ToLower(raw)]; ok {
		delta := (int(wd) - int(now.Weekday()) + 7) % 7
		if delta == 0 {
			delta = 7 // el próximo día con ese nombre
		}
		return now.AddDate(0, 0, delta).Format("2006-01-02")
	}
	return "" // fecha inválida: se conserva como texto, nunca rompe el parseo
}
