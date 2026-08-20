// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/oklog/ulid/v2"
	"github.com/DeijoseDevelop/Kora-Hub/internal/auth"
	"github.com/DeijoseDevelop/Kora-Hub/internal/config"
	"github.com/DeijoseDevelop/Kora-Hub/internal/db"
	"github.com/DeijoseDevelop/Kora-Hub/internal/docs"
	"github.com/DeijoseDevelop/Kora-Hub/internal/indexer"
)

func testServer(t *testing.T) (*Server, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	conn, err := db.Open(filepath.Join(dir, "hub.db"), logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })

	ctx := context.Background()
	queries := db.New(conn)
	userID := ulid.Make().String()
	if err := queries.CreateUser(ctx, db.CreateUserParams{
		ID: userID, Email: userID + "@t.dev", PasswordHash: "x", DisplayName: "t",
	}); err != nil {
		t.Fatal(err)
	}
	wsID := ulid.Make().String()
	if err := queries.CreateWorkspace(ctx, db.CreateWorkspaceParams{
		ID: wsID, Slug: "ws", Name: "ws", OwnerID: userID,
	}); err != nil {
		t.Fatal(err)
	}
	if err := queries.AddMembership(ctx, db.AddMembershipParams{
		UserID: userID, WorkspaceID: wsID, Role: "owner",
	}); err != nil {
		t.Fatal(err)
	}

	store := docs.NewStore(dir)
	idx := indexer.New(store, queries, conn, logger)
	cfg := &config.Config{AccessTTL: 15 * 60 * 1e9, RefreshTTL: 30 * 24 * 3600 * 1e9}
	authSvc := auth.NewService("test-secret", cfg.AccessTTL, cfg.RefreshTTL)
	return New(cfg, queries, conn, logger, authSvc, store, idx), userID
}

func TestQuickAddAppendsToInboxAndIndexes(t *testing.T) {
	srv, userID := testServer(t)
	ctx := context.Background()

	token, _ := srv.authSvc.AccessToken(userID, nil)

	body, _ := json.Marshal(quickAddRequest{Text: "revisar facturas #mañana @zekrost !alta ~deiver"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router(nil).ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}

	var created TaskDTO
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Title != "revisar facturas" {
		t.Errorf("title = %q", created.Title)
	}
	if created.Project == nil || *created.Project != "zekrost" ||
		created.Priority == nil || *created.Priority != "alta" ||
		created.Assignee == nil || *created.Assignee != "deiver" {
		t.Errorf("metadatos = %+v", created)
	}
	if created.DueDate == nil || *created.DueDate == "" {
		t.Error("la fecha relativa #mañana debió resolverse")
	}

	// invariante P1: la tarea vive en Inbox.md canónico
	content, err := srv.store.Read("ws", InboxPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(content, []byte("- [ ] revisar facturas")) {
		t.Errorf("Inbox.md = %q", content)
	}
	// no debe quedar texto residual del quick add (el resto del texto
	// con metadatos parseables se normalizó en la línea)
	if bytes.Contains(content, []byte("#mañana")) {
		t.Errorf("fecha relativa sin resolver en el archivo: %q", content)
	}

	// la tarea existe en el índice del workspace
	wsID := srv.mustFirstWorkspace(ctx, userID)
	open, err := srv.queries.ListTasksByWorkspace(ctx, db.ListTasksByWorkspaceParams{WorkspaceID: wsID, Done: 0})
	if err != nil || len(open) != 1 {
		t.Fatalf("tareas = %v err=%v", len(open), err)
	}
}

func (s *Server) mustFirstWorkspace(ctx context.Context, userID string) string {
	rows, _ := s.queries.ListWorkspacesByUser(ctx, userID)
	return rows[0].ID
}
