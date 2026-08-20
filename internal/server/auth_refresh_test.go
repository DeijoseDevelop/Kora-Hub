package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oklog/ulid/v2"
	"github.com/DeijoseDevelop/Kora-Hub/internal/auth"
	"github.com/DeijoseDevelop/Kora-Hub/internal/db"
)

func registerUser(t *testing.T, srv *Server) (string, map[string]any) {
	t.Helper()
	body := mustJSON(t, registerRequest{
		Email: ulid.Make().String() + "@t.dev",
		Password: "supersecret1",
		DisplayName: "Test",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router(nil).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("register = %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	return resp["access_token"].(string), resp
}

func TestRegisterCreatesPersonalWorkspace(t *testing.T) {
	srv, _ := testServer(t)
	ctx := context.Background()
	access, resp := registerUser(t, srv)

	token, _ := srv.authSvc.ParseAccess(access)
	// el workspace "Personal" existe y el usuario es owner
	ws, err := srv.queries.GetWorkspaceByID(ctx, srv.mustFirstWorkspace(ctx, token.Subject))
	if err != nil {
		t.Fatal(err)
	}
	if ws.Slug != "personal" || ws.Name != "Personal" {
		t.Errorf("workspace = %s/%s", ws.Slug, ws.Name)
	}
	member, err := srv.queries.GetMembership(ctx, db.GetMembershipParams{
		UserID: token.Subject, WorkspaceID: ws.ID,
	})
	if err != nil || member.Role != "owner" {
		t.Errorf("membresía = %+v err=%v", member, err)
	}
	_ = resp
}

func TestRefreshRotatesToken(t *testing.T) {
	srv, _ := testServer(t)
	_, resp := registerUser(t, srv)

	// refresh válido → rota: par nuevo + el viejo queda revocado
	body, _ := json.Marshal(map[string]string{"refresh_token": resp["refresh_token"].(string)})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router(nil).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("refresh = %d: %s", w.Code, w.Body.String())
	}
	var second map[string]any
	json.Unmarshal(w.Body.Bytes(), &second)
	if second["access_token"] == resp["access_token"] {
		t.Error("el access token debería ser nuevo")
	}

	// el refresh viejo ya no sirve
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	srv.Router(nil).ServeHTTP(w2, req2)
	if w2.Code != http.StatusUnauthorized {
		t.Errorf("refresh con token revocado = %d, esperaba 401", w2.Code)
	}

	// y el nuevo sí funciona
	body3, _ := json.Marshal(map[string]string{"refresh_token": second["refresh_token"].(string)})
	req3 := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader(body3))
	req3.Header.Set("Content-Type", "application/json")
	w3 := httptest.NewRecorder()
	srv.Router(nil).ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK {
		t.Errorf("refresh con token nuevo = %d", w3.Code)
	}
}

func TestRefreshRejectsGarbage(t *testing.T) {
	srv, _ := testServer(t)
	body, _ := json.Marshal(map[string]string{"refresh_token": "no-es-un-jwt"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router(nil).ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("refresh basura = %d, esperaba 401", w.Code)
	}
}

func TestLogoutRevokesRefresh(t *testing.T) {
	srv, _ := testServer(t)
	access, resp := registerUser(t, srv)

	body, _ := json.Marshal(map[string]string{"refresh_token": resp["refresh_token"].(string)})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+access)
	w := httptest.NewRecorder()
	srv.Router(nil).ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("logout = %d", w.Code)
	}

	// refresh tras logout → 401
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	srv.Router(nil).ServeHTTP(w2, req2)
	if w2.Code != http.StatusUnauthorized {
		t.Errorf("refresh tras logout = %d, esperaba 401", w2.Code)
	}
}

func TestAuthStatusAndMe(t *testing.T) {
	srv, _ := testServer(t)

	// el testServer siembra un usuario para workspaces; /auth/status debe
	// reflejar la existencia de usuarios en cualquier caso
	access, _ := registerUser(t, srv)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/status", nil)
	w := httptest.NewRecorder()
	srv.Router(nil).ServeHTTP(w, req)
	var status map[string]bool
	json.Unmarshal(w.Body.Bytes(), &status)
	if !status["has_users"] {
		t.Error("después del registro debería haber usuarios")
	}

	// /auth/me autenticado
	req = httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+access)
	w = httptest.NewRecorder()
	srv.Router(nil).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("me = %d", w.Code)
	}
	var me map[string]string
	json.Unmarshal(w.Body.Bytes(), &me)
	if me["email"] == "" || me["display_name"] != "Test" {
		t.Errorf("me = %+v", me)
	}

	// /auth/me sin token → 401
	req = httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	w = httptest.NewRecorder()
	srv.Router(nil).ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("me sin token = %d, esperaba 401", w.Code)
	}
}

var _ = auth.HashToken
