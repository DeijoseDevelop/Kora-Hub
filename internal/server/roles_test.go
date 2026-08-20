// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oklog/ulid/v2"
	"github.com/DeijoseDevelop/Kora-Hub/internal/db"
)

// TestRoleEnforcement: un viewer puede leer pero las mutaciones dan 403.
func TestRoleEnforcement(t *testing.T) {
	srv, ownerID := testServer(t)
	ctx := context.Background()

	wsID := srv.mustFirstWorkspace(ctx, ownerID)

	viewerID := ulid.Make().String()
	if err := srv.queries.CreateUser(ctx, db.CreateUserParams{
		ID: viewerID, Email: viewerID + "@v.dev", PasswordHash: "x", DisplayName: "v",
	}); err != nil {
		t.Fatal(err)
	}
	if err := srv.queries.AddMembership(ctx, db.AddMembershipParams{
		UserID: viewerID, WorkspaceID: wsID, Role: "viewer",
	}); err != nil {
		t.Fatal(err)
	}
	viewerToken, _ := srv.authSvc.AccessToken(viewerID, nil)

	// lectura permitida
	req := httptest.NewRequest(http.MethodGet, "/api/v1/docs", nil)
	req.Header.Set("Authorization", "Bearer "+viewerToken)
	w := httptest.NewRecorder()
	srv.Router(nil).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /docs viewer = %d", w.Code)
	}

	// mutaciones bloqueadas
	body, _ := json.Marshal(docRequest{Path: "a.md", Title: "A", Content: "# A\n"})
	mut := []struct {
		method, path string
		body         []byte
	}{
		{http.MethodPost, "/api/v1/docs", body},
		{http.MethodPost, "/api/v1/tasks", mustJSON(t, quickAddRequest{Text: "tarea"})},
		{http.MethodPost, "/api/v1/sync/push", mustJSON(t, map[string]any{"commands": []syncCmd{{IdempotencyKey: "k", Op: "doc.upsert", Path: "b.md", Content: "x"}}})},
	}
	for _, m := range mut {
		req := httptest.NewRequest(m.method, m.path, bytes.NewReader(m.body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+viewerToken)
		w := httptest.NewRecorder()
		srv.Router(nil).ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Errorf("%s %s viewer = %d, esperaba 403", m.method, m.path, w.Code)
		}
	}

	// reindex: solo owner
	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/reindex", nil)
	req.Header.Set("Authorization", "Bearer "+viewerToken)
	w = httptest.NewRecorder()
	srv.Router(nil).ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("reindex viewer = %d, esperaba 403", w.Code)
	}

	// el owner sí puede mutar
	ownerToken, _ := srv.authSvc.AccessToken(ownerID, nil)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/docs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ownerToken)
	w = httptest.NewRecorder()
	srv.Router(nil).ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Errorf("POST /docs owner = %d, esperaba 201", w.Code)
	}
}

type syncCmd struct {
	IdempotencyKey string `json:"idempotency_key"`
	Op             string `json:"op"`
	Path           string `json:"path"`
	Content        string `json:"content"`
}

// TestProjections: las vistas tabla/calendario/mis-tareas-hoy son
// consultas del índice (sección 6.3).
func TestProjections(t *testing.T) {
	srv, ownerID := testServer(t)
	ctx := context.Background()
	wsID := srv.mustFirstWorkspace(ctx, ownerID)

	// doc con tareas para sembrar el índice
	if err := srv.store.Write("ws", "proyecto.md", []byte(
		"# Proyecto\n\n- [ ] A vencer #2026-09-01 @proyecto !alta ~alice\n- [ ] B vencida #2020-01-01 @proyecto ~bob\n- [x] C hecha #2026-09-05\n",
	)); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.indexer.ReindexWorkspace(ctx, wsID, "ws"); err != nil {
		t.Fatal(err)
	}

	token, _ := srv.authSvc.AccessToken(ownerID, nil)

	get := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		srv.Router(nil).ServeHTTP(w, req)
		return w
	}

	// kanban: abiertas
	w := get("/api/v1/tasks")
	if w.Code != 200 {
		t.Fatalf("kanban = %d", w.Code)
	}
	var open struct {
		Tasks []db.Task `json:"tasks"`
	}
	json.Unmarshal(w.Body.Bytes(), &open)
	if len(open.Tasks) != 2 {
		t.Fatalf("abiertas = %d, esperaba 2", len(open.Tasks))
	}

	// tabla con filtro de proyecto (A y B tienen @proyecto; C no)
	w = get("/api/v1/tasks?vista=tabla&proyecto=proyecto")
	json.Unmarshal(w.Body.Bytes(), &open)
	if len(open.Tasks) != 2 {
		t.Fatalf("tabla proyecto = %d, esperaba 2", len(open.Tasks))
	}

	// calendario por rango
	w = get("/api/v1/tasks?vista=calendario&desde=2026-09-01&hasta=2026-09-30")
	json.Unmarshal(w.Body.Bytes(), &open)
	if len(open.Tasks) != 2 {
		t.Fatalf("calendario = %d, esperaba 2 (A y C)", len(open.Tasks))
	}

	// mis tareas hoy: ninguna con due <= hoy sin done (B está vencida pero ~bob)
	w = get("/api/v1/tasks?vista=mis-tareas-hoy&asignado=alice")
	json.Unmarshal(w.Body.Bytes(), &open)
	if len(open.Tasks) != 0 {
		t.Fatalf("mis-tareas-hoy alice = %d, esperaba 0", len(open.Tasks))
	}
	w = get("/api/v1/tasks?vista=mis-tareas-hoy&asignado=bob")
	json.Unmarshal(w.Body.Bytes(), &open)
	if len(open.Tasks) != 1 || open.Tasks[0].Title != "B vencida" {
		t.Fatalf("mis-tareas-hoy bob = %+v", open.Tasks)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestTaskJSONShape: los campos nullable se serializan como string|null,
// nunca como el objeto interno de sql.NullString.
func TestTaskJSONShape(t *testing.T) {
	srv, ownerID := testServer(t)
	ctx := context.Background()
	wsID := srv.mustFirstWorkspace(ctx, ownerID)

	if err := srv.store.Write("ws", "t.md", []byte(
		"- [ ] tarea con todo #2026-09-01 @proyecto !alta ~deiver\n- [ ] tarea sin metadatos\n",
	)); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.indexer.ReindexWorkspace(ctx, wsID, "ws"); err != nil {
		t.Fatal(err)
	}

	token, _ := srv.authSvc.AccessToken(ownerID, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router(nil).ServeHTTP(w, req)

	var resp struct {
		Tasks []map[string]any `json:"tasks"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Tasks) != 2 {
		t.Fatalf("tasks = %d", len(resp.Tasks))
	}
	byTitle := map[string]map[string]any{}
	for _, tk := range resp.Tasks {
		byTitle[tk["title"].(string)] = tk
	}
	full := byTitle["tarea con todo"]
	if v, ok := full["project"].(string); !ok || v != "proyecto" {
		t.Errorf("project debe ser string, got %#v", full["project"])
	}
	if v, ok := full["due_date"].(string); !ok || v != "2026-09-01" {
		t.Errorf("due_date debe ser string, got %#v", full["due_date"])
	}
	empty := byTitle["tarea sin metadatos"]
	if empty["priority"] != nil || empty["assignee"] != nil {
		t.Errorf("priority/assignee sin valor deben ser null, got %#v / %#v", empty["priority"], empty["assignee"])
	}
}
