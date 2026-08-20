// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
package indexer

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/oklog/ulid/v2"
	"github.com/DeijoseDevelop/Kora-Hub/internal/db"
	"github.com/DeijoseDevelop/Kora-Hub/internal/docs"
	"github.com/DeijoseDevelop/Kora-Hub/internal/search"
)

func setup(t *testing.T) (*Indexer, *db.Queries, *docs.Store) {
	t.Helper()
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	conn, err := db.Open(filepath.Join(dir, "hub.db"), logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })

	store := docs.NewStore(dir)
	queries := db.New(conn)
	return New(store, queries, conn, logger), queries, store
}

// newWorkspace crea un usuario + workspace reales (FK) y devuelve ids.
func newWorkspace(t *testing.T, queries *db.Queries, slug string) (wsID, userID string) {
	t.Helper()
	ctx := context.Background()
	userID = ulid.Make().String()
	if err := queries.CreateUser(ctx, db.CreateUserParams{
		ID: userID, Email: userID + "@test.dev", PasswordHash: "x", DisplayName: "t",
	}); err != nil {
		t.Fatal(err)
	}
	wsID = ulid.Make().String()
	if err := queries.CreateWorkspace(ctx, db.CreateWorkspaceParams{
		ID: wsID, Slug: slug, Name: slug, OwnerID: userID,
	}); err != nil {
		t.Fatal(err)
	}
	return wsID, userID
}

func TestReindexCreatesTasksAndFTS(t *testing.T) {
	ix, queries, store := setup(t)
	ctx := context.Background()

	wsID, _ := newWorkspace(t, queries, "proyecto-a")
	content := "# Proyecto\n\n- [ ] Preparar propuesta #2026-08-20 @zekrost !alta\n- [x] Enviar informe\n"
	if err := store.Write("proyecto-a", "propuestas/2026-08-propuesta.md", []byte(content)); err != nil {
		t.Fatal(err)
	}

	n, err := ix.ReindexWorkspace(ctx, wsID, "proyecto-a")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("reindex = %d, want 1", n)
	}

	docsIdx, err := queries.ListDocsByWorkspace(ctx, wsID)
	if err != nil || len(docsIdx) != 1 {
		t.Fatalf("docs en índice = %v, err=%v", len(docsIdx), err)
	}
	if docsIdx[0].Title != "Proyecto" {
		t.Errorf("título = %q", docsIdx[0].Title)
	}

	open, err := queries.ListTasksByWorkspace(ctx, db.ListTasksByWorkspaceParams{WorkspaceID: wsID, Done: 0})
	if err != nil || len(open) != 1 {
		t.Fatalf("tareas abiertas = %v, err=%v", len(open), err)
	}
	task := open[0]
	if task.Project.String != "zekrost" || task.Priority.String != "alta" || task.DueDate.String != "2026-08-20" {
		t.Errorf("metadatos de tarea incorrectos: %+v", task)
	}

	done, err := queries.ListTasksByWorkspace(ctx, db.ListTasksByWorkspaceParams{WorkspaceID: wsID, Done: 1})
	if err != nil || len(done) != 1 {
		t.Fatalf("tareas cerradas = %v, err=%v", len(done), err)
	}

	// FTS5 sin duplicados tras reindexar dos veces
	if _, err := ix.ReindexWorkspace(ctx, wsID, "proyecto-a"); err != nil {
		t.Fatal(err)
	}
	res, err := search.SearchDocs(ctx, ix.conn, wsID, "propuesta", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].Path != "propuestas/2026-08-propuesta.md" {
		t.Errorf("búsqueda FTS = %+v", res)
	}
}

func TestReindexIsIncremental(t *testing.T) {
	ix, queries, store := setup(t)
	ctx := context.Background()
	wsID, _ := newWorkspace(t, queries, "b")

	if err := store.Write("b", "a.md", []byte("- [ ] tarea")); err != nil {
		t.Fatal(err)
	}
	first, err := ix.ReindexWorkspace(ctx, wsID, "b")
	if err != nil {
		t.Fatal(err)
	}
	second, err := ix.ReindexWorkspace(ctx, wsID, "b")
	if err != nil {
		t.Fatal(err)
	}
	if first != 1 || second != 0 {
		t.Fatalf("incremental: first=%d second=%d", first, second)
	}
}

func TestReindexRoundTripViaEdit(t *testing.T) {
	ix, queries, store := setup(t)
	ctx := context.Background()
	wsID, _ := newWorkspace(t, queries, "c")

	if err := store.Write("c", "inbox.md", []byte("- [ ] tarea #hoy\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := ix.ReindexWorkspace(ctx, wsID, "c"); err != nil {
		t.Fatal(err)
	}

	open, _ := queries.ListTasksByWorkspace(ctx, db.ListTasksByWorkspaceParams{WorkspaceID: wsID, Done: 0})
	if len(open) != 1 {
		t.Fatalf("esperaba 1 tarea, got %d", len(open))
	}
	task := open[0]
	if task.DueDate.String == "" {
		t.Error("la fecha relativa #hoy debió resolverse")
	}
}
