// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestSavedViewsLifecycle cubre crear, listar, aplicar filtros y borrar
// vistas guardadas, incluyendo la restriccion de rol para mutaciones.
func TestSavedViewsLifecycle(t *testing.T) {
	srv, ownerID := testServer(t)
	token, _ := srv.authSvc.AccessToken(ownerID, nil)
	wsID := workspaceID(t, srv, ownerID)

	// crear
	w := doJSON(t, srv, http.MethodPost,
		"/api/v1/views?workspace="+wsID, token,
		map[string]any{"name": "Hoy", "filters": map[string]any{"vista": "kanban", "proyecto": "kora"}})
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", w.Code, w.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil || created.ID == "" {
		t.Fatalf("id vacio en la respuesta: %s", w.Body.String())
	}

	// nombre duplicado en el mismo workspace -> conflicto
	w = doJSON(t, srv, http.MethodPost,
		"/api/v1/views?workspace="+wsID, token,
		map[string]any{"name": "Hoy", "filters": map[string]any{"vista": "tabla"}})
	if w.Code != http.StatusConflict {
		t.Fatalf("duplicado status = %d, esperado 409", w.Code)
	}

	// filters invalido -> 400
	w = doJSON(t, srv, http.MethodPost,
		"/api/v1/views?workspace="+wsID, token,
		map[string]any{"name": "rota", "filters": "no-es-json"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("filters invalido status = %d, esperado 400", w.Code)
	}

	// listar
	w = doJSON(t, srv, http.MethodGet, "/api/v1/views?workspace="+wsID, token, nil)
	var list struct {
		Views []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Filters string `json:"filters"`
		} `json:"views"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Views) != 1 || list.Views[0].Name != "Hoy" {
		t.Fatalf("views = %+v", list.Views)
	}
	var filters map[string]string
	if err := json.Unmarshal([]byte(list.Views[0].Filters), &filters); err != nil || filters["vista"] != "kanban" {
		t.Fatalf("filters no round-trippean: %s", list.Views[0].Filters)
	}

	// un viewer puede listar pero no borrar ni crear
	_, viewerToken := addUser(t, srv, "viewer", wsID)
	w = doJSON(t, srv, http.MethodGet, "/api/v1/views?workspace="+wsID, viewerToken, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("viewer list status = %d", w.Code)
	}
	w = doJSON(t, srv, http.MethodPost,
		"/api/v1/views?workspace="+wsID, viewerToken,
		map[string]any{"name": "v", "filters": map[string]any{}})
	if w.Code != http.StatusForbidden {
		t.Fatalf("viewer create status = %d, esperado 403", w.Code)
	}
	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/views/"+created.ID+"?workspace="+wsID, nil)
	req.Header.Set("Authorization", "Bearer "+viewerToken)
	wr := httptest.NewRecorder()
	srv.Router(nil).ServeHTTP(wr, req)
	if wr.Code != http.StatusForbidden {
		t.Fatalf("viewer delete status = %d, esperado 403", wr.Code)
	}

	// borrar como owner
	req = httptest.NewRequest(http.MethodDelete,
		"/api/v1/views/"+created.ID+"?workspace="+wsID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	wr = httptest.NewRecorder()
	srv.Router(nil).ServeHTTP(wr, req)
	if wr.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, esperado 204", wr.Code)
	}
}
