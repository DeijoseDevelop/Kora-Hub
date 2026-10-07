// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/DeijoseDevelop/Kora-Hub/internal/db"
	"github.com/DeijoseDevelop/Kora-Hub/internal/docs"
)

// TestWorkspaceSlugRejectsTraversal: el slug se usa como directorio del
// filesystem canónico — "..", "/" y compañía no pueden colarse (antes
// cualquier usuario podía escribir fuera de data/workspaces).
func TestWorkspaceSlugRejectsTraversal(t *testing.T) {
	for _, slug := range []string{"../evil", "..", "a/b", "a\\b", "-lead", "trail-", "a--b", "UPPER"} {
		if docs.ValidSlug(slug) {
			t.Errorf("ValidSlug(%q) = true, debería ser false", slug)
		}
	}
	for _, slug := range []string{"personal", "kora-hub", "a", "ws-1", "0abc"} {
		if !docs.ValidSlug(slug) {
			t.Errorf("ValidSlug(%q) = false, debería ser true", slug)
		}
	}

	srv, ownerID := testServer(t)
	token, _ := srv.authSvc.AccessToken(ownerID, nil)
	body, _ := json.Marshal(workspaceRequest{Slug: "../evil", Name: "Evil"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router(nil).ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("slug traversal = %d %s, esperaba 400", w.Code, w.Body.String())
	}
}

// TestRefreshTokenRejectedAsAccess: la confusión de tipos de token daba
// 30 días de acceso con un refresh filtrado.
func TestRefreshTokenRejectedAsAccess(t *testing.T) {
	srv, ownerID := testServer(t)
	refresh, _, err := srv.authSvc.RefreshToken(ownerID)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+refresh)
	w := httptest.NewRecorder()
	srv.Router(nil).ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("refresh como access = %d, esperaba 401", w.Code)
	}
}

// TestDeleteWorkspaceWithTasksAndVersions: DELETE /workspaces fallaba
// con 500 si el workspace tenía tareas/versiones/shares (FK).
func TestDeleteWorkspaceWithTasksAndVersions(t *testing.T) {
	srv, ownerID := testServer(t)
	ctx := context.Background()
	wsID := srv.mustFirstWorkspace(ctx, ownerID)
	token, _ := srv.authSvc.AccessToken(ownerID, nil)

	// doc con tarea + un PATCH (genera snapshot en doc_versions)
	body, _ := json.Marshal(docRequest{Path: "con-tareas.md", Title: "Con tareas", Content: "# Con tareas\n\n- [ ] tarea pendiente\n"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/docs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router(nil).ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create doc = %d %s", w.Code, w.Body.String())
	}

	// PATCH de la tarea (round-trip en el markdown + snapshot de versión)
	tasks := srv.mustTasks(ctx, wsID)
	if len(tasks) == 0 {
		t.Fatal("la tarea no se indexó")
	}
	patch, _ := json.Marshal(patchTaskRequest{Done: boolPtr(true)})
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/tasks/"+tasks[0].ID, bytes.NewReader(patch))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	srv.Router(nil).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("patch task = %d %s", w.Code, w.Body.String())
	}

	// share-link (doc_shares)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/docs/"+tasks[0].DocID+"/share", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	srv.Router(nil).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("share = %d %s", w.Code, w.Body.String())
	}

	// DELETE del workspace: antes 500 por FK
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/workspaces/"+wsID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	srv.Router(nil).ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete workspace = %d %s", w.Code, w.Body.String())
	}
}

// TestSearchEscapesFTSQuery: las comillas y los operadores FTS no deben
// romper la búsqueda con un 500.
func TestSearchEscapesFTSQuery(t *testing.T) {
	srv, ownerID := testServer(t)
	token, _ := srv.authSvc.AccessToken(ownerID, nil)
	for _, q := range []string{`"comillas"`, `AND`, `a OR b`, `foo*`, `-`} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q="+url.QueryEscape(q), nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		srv.Router(nil).ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("search %q = %d %s", q, w.Code, w.Body.String())
		}
	}
}

// TestPatchTaskPreservesID: el id devuelto por el PATCH debe seguir
// existiendo tras el reindex (antes el reindex regeneraba todos los ids).
func TestPatchTaskPreservesID(t *testing.T) {
	srv, ownerID := testServer(t)
	ctx := context.Background()
	wsID := srv.mustFirstWorkspace(ctx, ownerID)
	token, _ := srv.authSvc.AccessToken(ownerID, nil)

	body, _ := json.Marshal(docRequest{Path: "ids.md", Title: "IDs", Content: "# IDs\n\n- [ ] una\n- [ ] dos\n"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/docs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router(nil).ServeHTTP(w, req)

	before := srv.mustTasks(ctx, wsID)
	if len(before) < 2 {
		t.Fatalf("tareas = %d", len(before))
	}

	patch, _ := json.Marshal(patchTaskRequest{Done: boolPtr(true)})
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/tasks/"+before[0].ID, bytes.NewReader(patch))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	srv.Router(nil).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("patch = %d %s", w.Code, w.Body.String())
	}

	after := srv.mustTasks(ctx, wsID)
	ids := map[string]bool{}
	for _, tk := range after {
		ids[tk.ID] = true
	}
	for _, tk := range before {
		if !ids[tk.ID] {
			t.Fatalf("el id %s desapareció tras el reindex (line %d title %q)", tk.ID, tk.LineNo, tk.Title)
		}
	}
}

// TestRoundTripCheckedTaskPatch: editar una tarea ya hecha no corrompe
// la línea del markdown canónico.
func TestRoundTripCheckedTaskPatch(t *testing.T) {
	srv, ownerID := testServer(t)
	ctx := context.Background()
	_ = srv.mustFirstWorkspace(ctx, ownerID)
	token, _ := srv.authSvc.AccessToken(ownerID, nil)

	body, _ := json.Marshal(docRequest{Path: "rt.md", Title: "RT", Content: "# RT\n\n- [x] informe semanal @viejo\n"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/docs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router(nil).ServeHTTP(w, req)

	ws, _ := srv.queries.GetWorkspaceByID(ctx, srv.mustFirstWorkspace(ctx, ownerID))
	raw, err := srv.store.Read(ws.Slug, "rt.md")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "- [x] - [x]") || strings.Contains(string(raw), "- [ ] - [x]") {
		t.Fatalf("línea corrompida: %q", raw)
	}
	if !strings.Contains(string(raw), "informe semanal") {
		t.Fatalf("título perdido: %q", raw)
	}
}

func boolPtr(b bool) *bool { return &b }

// mustTasks lista las tareas indexadas del workspace de prueba.
func (s *Server) mustTasks(ctx context.Context, wsID string) []db.Task {
	rows, _ := s.queries.ListTasksByWorkspace(ctx, db.ListTasksByWorkspaceParams{WorkspaceID: wsID, Done: 0})
	rows2, _ := s.queries.ListTasksByWorkspace(ctx, db.ListTasksByWorkspaceParams{WorkspaceID: wsID, Done: 1})
	return append(rows, rows2...)
}
