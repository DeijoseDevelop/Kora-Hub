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

// addUser crea un usuario directamente en el índice (sin pasar por
// /auth/register, que es la superficie limitada por rate).
func addUser(t *testing.T, srv *Server, role, wsID string) (id, token string) {
	t.Helper()
	ctx := context.Background()
	id = ulid.Make().String()
	if err := srv.queries.CreateUser(ctx, db.CreateUserParams{
		ID: id, Email: id + "@m.dev", PasswordHash: "x", DisplayName: "m-" + id[:6],
	}); err != nil {
		t.Fatal(err)
	}
	if wsID != "" {
		if err := srv.queries.AddMembership(ctx, db.AddMembershipParams{
			UserID: id, WorkspaceID: wsID, Role: role,
		}); err != nil {
			t.Fatal(err)
		}
	}
	token, _ = srv.authSvc.AccessToken(id, nil)
	return id, token
}

func doJSON(t *testing.T, srv *Server, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rdr *bytes.Reader
	if body == nil {
		rdr = bytes.NewReader(nil)
	} else {
		rdr = bytes.NewReader(mustJSON(t, body))
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	srv.Router(nil).ServeHTTP(w, req)
	return w
}

// TestMembersLifecycle cubre invitar, cambiar rol y expulsar miembros
// con la protección del último owner.
func TestMembersLifecycle(t *testing.T) {
	srv, ownerID := testServer(t)
	ctx := context.Background()
	wsID := srv.mustFirstWorkspace(ctx, ownerID)
	ownerToken, _ := srv.authSvc.AccessToken(ownerID, nil)

	// editor existente que luego invitaremos
	editorID, _ := addUser(t, srv, "", "")
	editor, err := srv.queries.GetUserByID(ctx, editorID)
	if err != nil {
		t.Fatal(err)
	}

	// GET /workspaces/:id
	w := doJSON(t, srv, http.MethodGet, "/api/v1/workspaces/"+wsID, ownerToken, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET workspace = %d: %s", w.Code, w.Body.String())
	}
	var detail map[string]any
	json.Unmarshal(w.Body.Bytes(), &detail)
	if detail["role"] != "owner" || detail["slug"] != "ws" {
		t.Errorf("detail = %+v", detail)
	}

	// PATCH nombre (owner)
	w = doJSON(t, srv, http.MethodPatch, "/api/v1/workspaces/"+wsID, ownerToken,
		map[string]string{"name": "Renombrado"})
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH workspace = %d: %s", w.Code, w.Body.String())
	}

	// POST member: email inexistente -> 404
	w = doJSON(t, srv, http.MethodPost, "/api/v1/workspaces/"+wsID+"/members", ownerToken,
		map[string]string{"email": "nadie@x.dev", "role": "editor"})
	if w.Code != http.StatusNotFound {
		t.Errorf("add member inexistente = %d, esperaba 404", w.Code)
	}

	// POST member ok
	w = doJSON(t, srv, http.MethodPost, "/api/v1/workspaces/"+wsID+"/members", ownerToken,
		map[string]string{"email": editor.Email, "role": "editor"})
	if w.Code != http.StatusCreated {
		t.Fatalf("add member = %d: %s", w.Code, w.Body.String())
	}

	// duplicado -> 409
	w = doJSON(t, srv, http.MethodPost, "/api/v1/workspaces/"+wsID+"/members", ownerToken,
		map[string]string{"email": editor.Email, "role": "viewer"})
	if w.Code != http.StatusConflict {
		t.Errorf("add member duplicado = %d, esperaba 409", w.Code)
	}

	// el editor ya puede listar miembros y mutar docs en ese workspace
	editorToken, _ := srv.authSvc.AccessToken(editorID, nil)
	w = doJSON(t, srv, http.MethodGet, "/api/v1/workspaces/"+wsID+"/members", editorToken, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list members (editor) = %d", w.Code)
	}
	var members struct {
		Members []struct {
			UserID string `json:"user_id"`
			Role   string `json:"role"`
		} `json:"members"`
	}
	json.Unmarshal(w.Body.Bytes(), &members)
	if len(members.Members) != 2 {
		t.Fatalf("members = %+v", members.Members)
	}

	// el editor NO puede administrar miembros
	w = doJSON(t, srv, http.MethodPost, "/api/v1/workspaces/"+wsID+"/members", editorToken,
		map[string]string{"email": "otro@x.dev", "role": "viewer"})
	if w.Code != http.StatusForbidden {
		t.Errorf("add member por editor = %d, esperaba 403", w.Code)
	}

	// degradar al último owner -> 409
	w = doJSON(t, srv, http.MethodPatch, "/api/v1/workspaces/"+wsID+"/members/"+ownerID, ownerToken,
		map[string]string{"role": "viewer"})
	if w.Code != http.StatusConflict {
		t.Errorf("degradar último owner = %d, esperaba 409", w.Code)
	}

	// promover editor a owner y luego sí se puede degradar al primero
	w = doJSON(t, srv, http.MethodPatch, "/api/v1/workspaces/"+wsID+"/members/"+editorID, ownerToken,
		map[string]string{"role": "owner"})
	if w.Code != http.StatusOK {
		t.Fatalf("promover a owner = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodPatch, "/api/v1/workspaces/"+wsID+"/members/"+ownerID, ownerToken,
		map[string]string{"role": "viewer"})
	if w.Code != http.StatusOK {
		t.Errorf("degradar owner habiendo otro = %d, esperaba 200", w.Code)
	}

	// expulsar al ex-owner (ya viewer)
	w = doJSON(t, srv, http.MethodDelete, "/api/v1/workspaces/"+wsID+"/members/"+ownerID, editorToken, nil)
	if w.Code != http.StatusNoContent {
		t.Errorf("remove member = %d, esperaba 204", w.Code)
	}
}

// TestDeleteWorkspace: borrar un workspace elimina índice y archivos.
func TestDeleteWorkspace(t *testing.T) {
	srv, ownerID := testServer(t)
	ctx := context.Background()
	wsID := srv.mustFirstWorkspace(ctx, ownerID)
	ownerToken, _ := srv.authSvc.AccessToken(ownerID, nil)

	// siembro un doc canónico
	if err := srv.store.Write("ws", "doc.md", []byte("# doc\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.indexer.ReindexWorkspace(ctx, wsID, "ws"); err != nil {
		t.Fatal(err)
	}

	// un viewer no puede borrar
	_, viewerToken := addUser(t, srv, "viewer", wsID)
	w := doJSON(t, srv, http.MethodDelete, "/api/v1/workspaces/"+wsID, viewerToken, nil)
	if w.Code != http.StatusForbidden {
		t.Errorf("delete por viewer = %d, esperaba 403", w.Code)
	}

	w = doJSON(t, srv, http.MethodDelete, "/api/v1/workspaces/"+wsID, ownerToken, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete workspace = %d: %s", w.Code, w.Body.String())
	}

	if _, err := srv.queries.GetWorkspaceByID(ctx, wsID); err == nil {
		t.Error("el workspace sigue en el índice")
	}
	if files, err := srv.store.List("ws"); err != nil || len(files) != 0 {
		t.Errorf("archivos restantes = %v err=%v", files, err)
	}
}
