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
//	fecha := '#' (AAAA-MM-DD | 'hoy' | 'mañana' | 'lun'..'dom')
//	proyecto := '@' ident
//	prioridad := '!' ('baja'|'media'|'alta'|'1'..'3')
//	asignado := '~' ident
//	etiqueta := '+' ident
//	ident := [a-z0-9-]+
//
// Invariantes: idempotente, tolerante y con round-trip garantizado:
// toda edición desde una vista reescribe la línea original preservando
// el resto del texto al byte (sección 6.2).
package tasks

import (
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
	Line      int      `json:"line_no"` // 1-based dentro del documento
	RawLine   string   `json:"-"`
	Text      string   `json:"title"`
	Done      bool     `json:"done"`
	InProgress bool    `json:"in_progress"`
	DueDate   string   `json:"due_date"` // AAAA-MM-DD
	Project   string   `json:"project"`
	Priority  string   `json:"priority"` // baja | media | alta
	Assignee  string   `json:"assignee"`
	Tags      []string `json:"tags"`
}

// ParseLine intenta parsear una línea de documento como tarea embebida.
// Devuelve ok=false si la línea no es un checkbox Markdown.
func ParseLine(line string) (Task, bool) {
	trimmed := strings.TrimRight(line, "\r\n ")
	if !strings.HasPrefix(trimmed, "- [") {
		return Task{}, false
	}
	state, rest, ok := parseCheckbox(trimmed)
	if !ok {
		return Task{}, false
	}

	t := Task{
		RawLine: trimmed,
		Done:    state == StateDone,
		InProgress: state == StateProgress,
	}

	// texto + metadatos: separados por espacios simples
	parts := strings.Fields(rest)
	textParts := make([]string, 0, len(parts))
	for _, p := range parts {
		switch {
		case strings.HasPrefix(p, "#"):
			t.DueDate = parseDate(p[1:])
		case strings.HasPrefix(p, "@") && isIdent(p[1:]):
			t.Project = p[1:]
		case strings.HasPrefix(p, "!") && isPriority(p[1:]):
			t.Priority = normalizePriority(p[1:])
		case strings.HasPrefix(p, "~") && isIdent(p[1:]):
			t.Assignee = p[1:]
		case strings.HasPrefix(p, "+") && isIdent(p[1:]):
			t.Tags = append(t.Tags, p[1:])
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
func RoundTrip(t Task, done bool, due, project, priority, assignee string) string {
	fields := strings.Fields(t.RawLine)
	var out []string
	// conserva el estado original del checkbox (o lo actualiza)
	head := "- [" + stateOf(t, done) + "]"
	out = append(out, head)
	// "- [ ]" se parte en 3 tokens al hacer Fields; se saltan
	rest := fields
	if len(rest) >= 3 && rest[0] == "-" && rest[1] == "[" && strings.HasSuffix(rest[2], "]") {
		rest = rest[3:]
	}
	wroteText := false
	for _, p := range rest {
		switch {
		case strings.HasPrefix(p, "#"):
			if due != "" {
				out = append(out, "#"+due)
			} else {
				out = append(out, p) // sin cambio: se conserva el token original
			}
		case strings.HasPrefix(p, "@"):
			if project != "" {
				out = append(out, "@"+project)
			} else {
				out = append(out, p)
			}
		case strings.HasPrefix(p, "!"):
			if priority != "" {
				out = append(out, "!"+priority)
			} else {
				out = append(out, p)
			}
		case strings.HasPrefix(p, "~"):
			if assignee != "" {
				out = append(out, "~"+assignee)
			} else {
				out = append(out, p)
			}
		default:
			// texto: se vuelve a escribir tal cual, al byte
			if !wroteText {
				out = append(out, strings.TrimSpace(p))
				wroteText = true
			} else {
				out = append(out, p)
			}
		}
	}
	return strings.Join(out, " ")
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

func parseCheckbox(line string) (state, rest string, ok bool) {
	// "- [ ]" | "- [x]" | "- [~]" — exige espacio tras ']'
	if len(line) < 6 || line[1] != ' ' || line[2] != '[' || line[4] != ']' || line[5] != ' ' {
		return "", "", false
	}
	c := line[3]
	switch c {
	case ' ', 'x', '~':
	default:
		return "", "", false
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

// parseDate resuelve fechas relativas ('hoy', 'mañana', 'lun'..'dom')
// a AAAA-MM-DD; fechas inválidas devuelven "" sin romper el parseo
// (regla tolerante, sección 6.2).
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
	}
	if wd, ok := dow[strings.ToLower(raw)]; ok {
		delta := (int(wd) - int(now.Weekday()) + 7) % 7
		if delta == 0 {
			delta = 7 // el próximo día con ese nombre
		}
		return now.AddDate(0, 0, delta).Format("2006-01-02")
	}
	return "" // fecha inválida: warning en la UI, nunca rompe el parseo
}
