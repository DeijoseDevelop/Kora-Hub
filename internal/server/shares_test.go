// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
package server

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// TestShareLinkLifecycle: crear, leer sin sesion, regenerar, revocar.
func TestShareLinkLifecycle(t *testing.T) {
	srv, userID := testServer(t)
	token, _ := srv.authSvc.AccessToken(userID, nil)

	// crear doc
	w := doJSON(t, srv, http.MethodPost, "/api/v1/docs", token, map[string]any{
		"path": "notas/publica.md", "title": "Publica", "content": "# Publica\n",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create doc: %d %s", w.Code, w.Body.String())
	}
	var doc map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &doc)

	// share: GET inicial sin share
	w = doJSON(t, srv, http.MethodGet, "/api/v1/docs/"+doc["id"].(string)+"/share", token, nil)
	var state map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &state)
	if state["token"] != nil {
		t.Fatalf("share inesperado: %v", state)
	}

	// crear share
	w = doJSON(t, srv, http.MethodPost, "/api/v1/docs/"+doc["id"].(string)+"/share", token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("create share: %d %s", w.Code, w.Body.String())
	}
	var share map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &share)
	tok, _ := share["token"].(string)
	if tok == "" {
		t.Fatalf("sin token: %v", share)
	}

	// GET publico SIN sesion
	w = doJSON(t, srv, http.MethodGet, "/api/v1/public/docs/"+tok, "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("public get: %d %s", w.Code, w.Body.String())
	}
	var pub map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &pub)
	if pub["title"] != "Publica" || pub["content"] != "# Publica\n" {
		t.Fatalf("public doc = %v", pub)
	}

	// regenerar invalida el token viejo
	w = doJSON(t, srv, http.MethodPost, "/api/v1/docs/"+doc["id"].(string)+"/share", token, nil)
	var share2 map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &share2)
	if share2["token"] == tok {
		t.Fatalf("el token no roto: %v", share2)
	}
	w = doJSON(t, srv, http.MethodGet, "/api/v1/public/docs/"+tok, "", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("token viejo deberia dar 404, dio %d", w.Code)
	}

	// revocar
	w = doJSON(t, srv, http.MethodDelete, "/api/v1/docs/"+doc["id"].(string)+"/share", token, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("revoke: %d", w.Code)
	}
	w = doJSON(t, srv, http.MethodGet, "/api/v1/public/docs/"+share2["token"].(string), "", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("revocado deberia dar 404, dio %d", w.Code)
	}
}

// TestShareViewerReadOnly: viewer no puede crear ni revocar shares.
func TestShareViewerReadOnly(t *testing.T) {
	srv, userID := testServer(t)
	wsID := srv.mustFirstWorkspace(context.Background(), userID)
	_, viewerToken := addUser(t, srv, "viewer", wsID)
	ownerToken, _ := srv.authSvc.AccessToken(userID, nil)

	w := doJSON(t, srv, http.MethodPost, "/api/v1/docs", ownerToken, map[string]any{
		"path": "x.md", "title": "X", "content": "# X\n",
	})
	var doc map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &doc)

	w = doJSON(t, srv, http.MethodPost, "/api/v1/docs/"+doc["id"].(string)+"/share", viewerToken, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("viewer creando share: %d", w.Code)
	}
}
