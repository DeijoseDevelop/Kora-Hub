// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
package tasks

import (
	"strings"
	"testing"
)

func TestParseLineBasic(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		wantOK bool
		done   bool
		inProg bool
		text   string
	}{
		{"abierta", "- [ ] Preparar propuesta", true, false, false, "Preparar propuesta"},
		{"hecha", "- [x] Enviar informe", true, true, false, "Enviar informe"},
		{"en progreso", "- [~] Revisar PR", true, false, true, "Revisar PR"},
		{"no es checkbox", "Texto normal", false, false, false, ""},
		{"sin texto", "- [ ]", false, false, false, ""},
		{"no es lista", "- Preparar propuesta", false, false, false, ""},
		{"checkbox de lista normal", "- [x]tarea pegada", false, false, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseLine(tt.line)
			if ok != tt.wantOK {
				t.Fatalf("ParseLine(%q) ok = %v, want %v", tt.line, ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if got.Done != tt.done || got.InProgress != tt.inProg {
				t.Fatalf("estado = done:%v inProgress:%v", got.Done, got.InProgress)
			}
			if got.Text != tt.text {
				t.Fatalf("text = %q, want %q", got.Text, tt.text)
			}
		})
	}
}

// TestParseLineV2 cubre la extension de gramatica aprobada (seccion
// 6.5): recurrencia, identidad, dependencias y valores con comillas.
func TestParseLineV2(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		recur     string
		uid       string
		blockedBy string
		project   string
		assignee  string
		text      string
	}{
		{
			"recurrencia semanal",
			"- [ ] Backup semanal #2026-10-05 *every:1w",
			"1w", "", "", "", "", "Backup semanal",
		},
		{
			"identidad y dependencia",
			"- [ ] Desplegar release ^id:deploy-1 ^blocked-by:qa-review",
			"", "deploy-1", "qa-review", "", "", "Desplegar release",
		},
		{
			"valores con comillas",
			`- [ ] Plan @"kora hub" ~"Jane Doe" +"tag con espacio"`,
			"", "", "", "kora hub", "Jane Doe", "Plan",
		},
		{
			"intervalo invalido se conserva como texto",
			"- [ ] Revisar *every:0d",
			"", "", "", "", "", "Revisar *every:0d",
		},
		{
			"uid invalido se conserva como texto",
			"- [ ] Revisar ^id:Mayuscula",
			"", "", "", "", "", "Revisar ^id:Mayuscula",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task, ok := ParseLine(tt.line)
			if !ok {
				t.Fatalf("ParseLine(%q) no parseo", tt.line)
			}
			if task.Recur != tt.recur {
				t.Errorf("recur = %q, want %q", task.Recur, tt.recur)
			}
			if task.TaskUID != tt.uid {
				t.Errorf("task_uid = %q, want %q", task.TaskUID, tt.uid)
			}
			if task.BlockedBy != tt.blockedBy {
				t.Errorf("blocked_by = %q, want %q", task.BlockedBy, tt.blockedBy)
			}
			if task.Project != tt.project {
				t.Errorf("project = %q, want %q", task.Project, tt.project)
			}
			if task.Assignee != tt.assignee {
				t.Errorf("assignee = %q, want %q", task.Assignee, tt.assignee)
			}
			if task.Text != tt.text {
				t.Errorf("text = %q, want %q", task.Text, tt.text)
			}
		})
	}
}

func TestRecurSpawn(t *testing.T) {
	if got := NextOccurrence("2026-10-05", "2w"); got != "2026-10-19" {
		t.Fatalf("NextOccurrence 2w = %q", got)
	}
	if got := NextOccurrence("2026-01-31", "1m"); got != "2026-03-03" && got != "2026-02-28" {
		// Go AddDate desborda 31-ene -> 3-mar; el valor exacto es
		// implementacion, pero nunca debe fallar
		t.Fatalf("NextOccurrence 1m = %q", got)
	}
	if got := NextOccurrence("2026-10-05", "x9"); got != "" {
		t.Fatalf("intervalo invalido = %q", got)
	}
	line := "- [x] Backup semanal #2026-10-05 *every:1w ~deiver ^id:bak-1 ^blocked-by:otro"
	got := SpawnRecurring(line, "2026-10-12")
	want := "- [ ] Backup semanal #2026-10-12 *every:1w ~deiver"
	if got != want {
		t.Fatalf("SpawnRecurring = %q, want %q", got, want)
	}
}

func TestParseLineMetadata(t *testing.T) {
	line := "- [ ] Preparar propuesta comercial #2026-08-20 @zekrost !alta ~deiver +ventas"
	task, ok := ParseLine(line)
	if !ok {
		t.Fatal("no parseó")
	}
	if task.DueDate != "2026-08-20" {
		t.Errorf("due_date = %q", task.DueDate)
	}
	if task.Project != "zekrost" {
		t.Errorf("project = %q", task.Project)
	}
	if task.Priority != "alta" {
		t.Errorf("priority = %q", task.Priority)
	}
	if task.Assignee != "deiver" {
		t.Errorf("assignee = %q", task.Assignee)
	}
	if len(task.Tags) != 1 || task.Tags[0] != "ventas" {
		t.Errorf("tags = %v", task.Tags)
	}
	if task.Text != "Preparar propuesta comercial" {
		t.Errorf("text = %q", task.Text)
	}
}

func TestPriorityNormalization(t *testing.T) {
	for input, want := range map[string]string{"!1": "alta", "!2": "media", "!3": "baja", "!media": "media"} {
		task, ok := ParseLine("- [ ] tarea " + input)
		if !ok {
			t.Fatalf("no parseó %q", input)
		}
		if task.Priority != want {
			t.Errorf("%q -> priority %q, want %q", input, task.Priority, want)
		}
	}
}

func TestRelativeDates(t *testing.T) {
	task, ok := ParseLine("- [ ] revisar facturas #mañana @zekrost !alta")
	if !ok {
		t.Fatal("no parseó")
	}
	if task.DueDate == "" || len(task.DueDate) != 10 {
		t.Errorf("fecha relativa inválida: %q", task.DueDate)
	}
}

func TestTolerantMetadata(t *testing.T) {
	// metadatos desconocidos se conservan como texto
	task, ok := ParseLine("- [ ] tarea con %raro y texto")
	if !ok {
		t.Fatal("no parseó")
	}
	if task.Text != "tarea con %raro y texto" {
		t.Errorf("text = %q", task.Text)
	}
}

func TestParseDocumentIdempotent(t *testing.T) {
	content := "# Proyecto\n\n- [ ] Tarea A #2026-08-20 @zekrost !alta\n\nTexto suelto\n\n- [x] Tarea B ~deiver\n- [~] Tarea C\n"
	a := Parse(content)
	b := Parse(content)
	if len(a) != 3 || len(b) != 3 {
		t.Fatalf("esperaba 3 tareas, got %d y %d", len(a), len(b))
	}
	for i := range a {
		if a[i].Line != b[i].Line || a[i].RawLine != b[i].RawLine || a[i].Done != b[i].Done {
			t.Fatalf("parseo no idempotente en tarea %d", i)
		}
	}
	if a[0].Line != 3 || a[1].Line != 7 || a[2].Line != 8 {
		t.Errorf("line_no incorrectos: %d, %d, %d", a[0].Line, a[1].Line, a[2].Line)
	}
}

func TestRoundTripPreservesText(t *testing.T) {
	original := "- [ ] Preparar propuesta comercial #2026-08-20 @zekrost !alta ~deiver +ventas"
	task, ok := ParseLine(original)
	if !ok {
		t.Fatal("no parseó")
	}
	rewritten := RoundTrip(task, StateDone, "2026-08-20", "zekrost", "alta", "deiver")
	want := "- [x] Preparar propuesta comercial #2026-08-20 @zekrost !alta ~deiver +ventas"
	if rewritten != want {
		t.Errorf("round-trip = %q\nwant         %q", rewritten, want)
	}
}

// TestRoundTripCheckedLines cubre la regresion critica: mutar una tarea
// [x]/[~] reescribia la linea duplicando el checkbox o borrando el
// titulo (los tests previos solo partian de lineas abiertas).
func TestRoundTripCheckedLines(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		state    string
		due      string
		project  string
		priority string
		assignee string
		want     string
	}{
		{
			"hecha sin cambios conserva la linea",
			"- [x] Enviar informe semanal", StateDone, "", "", "", "",
			"- [x] Enviar informe semanal",
		},
		{
			"reabrir tarea hecha conserva el texto",
			"- [x] Enviar informe semanal", StateOpen, "", "", "", "",
			"- [ ] Enviar informe semanal",
		},
		{
			"en progreso sin cambios",
			"- [~] Revisar PR #2026-01-01", StateProgress, "", "", "", "",
			"- [~] Revisar PR #2026-01-01",
		},
		{
			"completar tarea en progreso",
			"- [~] Revisar PR #2026-01-01", StateDone, "", "", "", "",
			"- [x] Revisar PR #2026-01-01",
		},
		{
			"editar metadatos de tarea hecha",
			"- [x] Informe anual @viejo !baja", StateDone, "2026-05-01", "nuevo", "alta", "deiver",
			// los slots existentes se reemplazan en su posición; los nuevos
			// se añaden al final en orden canónico (# @ ! ~)
			"- [x] Informe anual @nuevo !alta #2026-05-01 ~deiver",
		},
		{
			"solo el primer slot de cada tipo",
			"- [ ] t #2026-01-01 #2026-02-02 @a @b", StateOpen, "2026-03-03", "c", "", "",
			"- [ ] t #2026-03-03 #2026-02-02 @c @b",
		},
		{
			"valores con espacios se re-citan",
			`- [ ] Plan @"kora hub"`, StateOpen, "", "otro proyecto", "", "",
			`- [ ] Plan @"otro proyecto"`,
		},
		{
			"tokens invalidos son texto y no se mutan",
			"- [ ] Comprar #regalo !fuerte", StateDone, "2026-01-01", "p", "alta", "a",
			"- [x] Comprar #regalo !fuerte #2026-01-01 @p !alta ~a",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task, ok := ParseLine(tt.line)
			if !ok {
				t.Fatalf("ParseLine(%q) no parseó", tt.line)
			}
			got := RoundTrip(task, tt.state, tt.due, tt.project, tt.priority, tt.assignee)
			if got != tt.want {
				t.Errorf("RoundTrip = %q\nwant       %q", got, tt.want)
			}
		})
	}
}

// TestRoundTripIdempotent: aplicar RoundTrip sin cambios reproduce la
// misma linea (invariante de seccion 6.2).
func TestRoundTripIdempotent(t *testing.T) {
	lines := []string{
		"- [ ] Preparar propuesta #2026-08-20 @zekrost !alta ~deiver +ventas",
		"- [x] Enviar informe ^id:x1 ^blocked-by:y",
		"- [~] Revisar *every:1w #hoy",
		`- [ ] Plan @"kora hub" ~"Jane Doe"`,
		"- [ ] Comprar #regalo !fuerte",
	}
	for _, line := range lines {
		task, ok := ParseLine(line)
		if !ok {
			t.Fatalf("ParseLine(%q) no parseó", line)
		}
		got := RoundTrip(task, StateOf(task), "", "", "", "")
		if got != task.RawLine {
			t.Errorf("no idempotente:\n got %q\nwant %q", got, task.RawLine)
		}
	}
}

func TestRoundTripPreservesUnknownTokens(t *testing.T) {
	original := "- [ ] Llamar al cliente ^importante #hoy"
	task, ok := ParseLine(original)
	if !ok {
		t.Fatal("no parseó")
	}
	rewritten := RoundTrip(task, StateDone, "", "", "", "")
	// los tokens desconocidos se conservan; la fecha relativa se conserva tal cual
	if !contains(rewritten, "^importante") {
		t.Errorf("token desconocido perdido: %q", rewritten)
	}
	if !contains(rewritten, "#hoy") {
		t.Errorf("fecha relativa perdida: %q", rewritten)
	}
}

// TestParseLineTolerantCheckbox: [X] y tabulador se aceptan (espejo TS).
func TestParseLineTolerantCheckbox(t *testing.T) {
	task, ok := ParseLine("- [X] tarea hecha")
	if !ok || !task.Done {
		t.Fatalf("[X] deberia ser hecha: ok=%v done=%v", ok, task.Done)
	}
	task, ok = ParseLine("-\ttarea con tab")
	if ok {
		t.Fatalf("sin checkbox no es tarea: %v", task)
	}
	task, ok = ParseLine("-\t[x] con tabulador")
	if !ok || !task.Done {
		t.Fatalf("tabulador tras guion deberia valer: ok=%v", ok)
	}
}

// TestParseLineHashtagInTitle: #no-fecha es texto del titulo, no un
// slot (regresión de la divergencia Go↔TS).
func TestParseLineHashtagInTitle(t *testing.T) {
	task, ok := ParseLine("- [ ] Comprar #regalo para ana")
	if !ok {
		t.Fatal("no parseó")
	}
	if task.Text != "Comprar #regalo para ana" {
		t.Errorf("text = %q", task.Text)
	}
	if task.DueDate != "" {
		t.Errorf("due_date = %q, want vacío", task.DueDate)
	}
}

func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}

// TestLooksLikeTask: no se debe duplicar el checkbox si el texto ya lo
// trae (regresión: el quick-add generaba "- [ ] - [x] ...").
func TestLooksLikeTask(t *testing.T) {
	for text, want := range map[string]bool{
		"- [ ] tarea":          true,
		"- [x] tarea hecha":    true,
		"- [~] en progreso":    true,
		"- [X] mayuscula":      true,
		"tarea sin checkbox":   false,
		"- tarea normal":       false,
		"- [ ]":                false, // sin texto
		"":                     false,
		"  - [ ] con espacios": true,
	} {
		if got := LooksLikeTask(text); got != want {
			t.Errorf("LooksLikeTask(%q) = %v, want %v", text, got, want)
		}
	}
}
