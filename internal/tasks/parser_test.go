// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
package tasks

import (
	"strings"
	"testing"
)

func TestParseLineBasic(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		wantOK  bool
		done    bool
		inProg  bool
		text    string
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
	rewritten := RoundTrip(task, true, "2026-08-20", "zekrost", "alta", "deiver")
	want := "- [x] Preparar propuesta comercial #2026-08-20 @zekrost !alta ~deiver +ventas"
	if rewritten != want {
		t.Errorf("round-trip = %q\nwant         %q", rewritten, want)
	}
}

func TestRoundTripPreservesUnknownTokens(t *testing.T) {
	original := "- [ ] Llamar al cliente ^importante #hoy"
	task, ok := ParseLine(original)
	if !ok {
		t.Fatal("no parseó")
	}
	rewritten := RoundTrip(task, true, "", "", "", "")
	// los tokens desconocidos se conservan; la fecha relativa se conserva tal cual
	if !contains(rewritten, "^importante") {
		t.Errorf("token desconocido perdido: %q", rewritten)
	}
	if !contains(rewritten, "#hoy") {
		t.Errorf("fecha relativa perdida: %q", rewritten)
	}
}

func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}
