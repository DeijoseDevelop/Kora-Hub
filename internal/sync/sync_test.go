// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
package sync

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/DeijoseDevelop/Kora-Hub/internal/db"
	"github.com/DeijoseDevelop/Kora-Hub/internal/docs"
	"github.com/DeijoseDevelop/Kora-Hub/internal/indexer"
)

func setup(t *testing.T) (*Engine, *db.Queries, *sql.DB, string, string) {
	t.Helper()
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
	_ = queries.CreateUser(ctx, db.CreateUserParams{
		ID: userID, Email: userID + "@t.dev", PasswordHash: "x", DisplayName: "t",
	})
	wsID := ulid.Make().String()
	_ = queries.CreateWorkspace(ctx, db.CreateWorkspaceParams{
		ID: wsID, Slug: "ws", Name: "ws", OwnerID: userID,
	})

	store := docs.NewStore(dir)
	idx := indexer.New(store, queries, conn, logger)
	return NewEngine(queries, conn, store, idx), queries, conn, wsID, "ws"
}

func TestPushAppliesAndRecordsChange(t *testing.T) {
	eng, queries, _, wsID, slug := setup(t)
	ctx := context.Background()

	res, err := eng.Push(ctx, wsID, slug, []PushCommand{{
		IdempotencyKey: ulid.Make().String(),
		Op:             "doc.upsert",
		Path:           "inbox.md",
		Content:        "# Inbox\n\n- [ ] tarea #hoy\n",
		UpdatedAt:      "2026-08-15 10:00:00",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Changes) != 1 || res.Changes[0].Op != "upsert" {
		t.Fatalf("delta = %+v", res.Changes)
	}
	if res.Changes[0].Doc == nil || res.Changes[0].Doc.Content == "" {
		t.Fatal("el delta debe incluir el snapshot del documento")
	}
	if res.Cursor == 0 {
		t.Fatal("cursor no avanzó")
	}

	// el índice quedó construido: la tarea se extrajo del Markdown
	open, err := queries.ListTasksByWorkspace(ctx, db.ListTasksByWorkspaceParams{WorkspaceID: wsID, Done: 0})
	if err != nil || len(open) != 1 {
		t.Fatalf("tareas = %v err=%v", len(open), err)
	}
}

func TestPushDeduplicatesReplays(t *testing.T) {
	eng, _, _, wsID, slug := setup(t)
	ctx := context.Background()

	cmd := PushCommand{
		IdempotencyKey: "clave-fija",
		Op:             "doc.upsert",
		Path:           "a.md",
		Content:        "- [ ] única\n",
		UpdatedAt:      "2026-08-15 10:00:00",
	}
	first, err := eng.Push(ctx, wsID, slug, []PushCommand{cmd})
	if err != nil {
		t.Fatal(err)
	}
	replay, err := eng.Push(ctx, wsID, slug, []PushCommand{cmd})
	if err != nil {
		t.Fatal(err)
	}
	if len(replay.Changes) != 0 {
		t.Fatalf("el replay no debe generar cambios nuevos: %+v", replay.Changes)
	}
	if replay.Cursor != first.Cursor {
		t.Fatalf("cursor cambió en replay: %d -> %d", first.Cursor, replay.Cursor)
	}
}

func TestLWWKeepsNewerVersionAndPreservesLoser(t *testing.T) {
	eng, queries, _, wsID, slug := setup(t)
	ctx := context.Background()

	// El servidor almacena updated_at con datetime('now') en UTC; los
	// comandos del cliente deben usar el mismo formato/zonificación.
	now := time.Now().UTC()
	serverTime := now.Add(time.Hour).Format("2006-01-02 15:04:05")
	olderTime := now.Add(-time.Hour).Format("2006-01-02 15:04:05")

	// 1) el servidor recibe primero una versión más reciente
	_, err := eng.Push(ctx, wsID, slug, []PushCommand{{
		IdempotencyKey: "k1",
		Op:             "doc.upsert",
		Path:           "conflicto.md",
		Content:        "- [ ] versión servidor\n",
		UpdatedAt:      serverTime,
	}})
	if err != nil {
		t.Fatal(err)
	}

	// 2) llega un push viejo (misma ruta, timestamp anterior): LWW descarta
	res, err := eng.Push(ctx, wsID, slug, []PushCommand{{
		IdempotencyKey: "k2",
		Op:             "doc.upsert",
		Path:           "conflicto.md",
		Content:        "- [ ] versión cliente vieja\n",
		UpdatedAt:      olderTime,
	}})
	if err != nil {
		t.Fatal(err)
	}

	// el push viejo no genera cambios (LWW: nada que propagar)
	if len(res.Changes) != 0 {
		t.Fatalf("el push LWW no debe propagar cambios: %+v", res.Changes)
	}

	// el estado final conserva el contenido del servidor (ganador)
	all, err := eng.Pull(ctx, wsID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	var winner *Change
	for i := range all.Changes {
		if all.Changes[i].Doc != nil && all.Changes[i].Doc.Path == "conflicto.md" {
			winner = &all.Changes[i]
		}
	}
	if winner == nil || winner.Doc.Content != "- [ ] versión servidor\n" {
		t.Errorf("LWW perdió: %+v", winner)
	}

	// la versión perdedora se preservó en doc_versions (recuperable)
	doc, err := queries.GetDocByPath(ctx, db.GetDocByPathParams{WorkspaceID: wsID, Path: "conflicto.md"})
	if err != nil {
		t.Fatal(err)
	}
	versions, err := queries.GetDocVersions(ctx, db.GetDocVersionsParams{DocID: doc.ID, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 {
		t.Fatalf("doc_versions = %d, esperaba 1", len(versions))
	}
}

func TestPullIsIncremental(t *testing.T) {
	eng, _, _, wsID, slug := setup(t)
	ctx := context.Background()

	_, err := eng.Push(ctx, wsID, slug, []PushCommand{{
		IdempotencyKey: "p1", Op: "doc.upsert", Path: "a.md",
		Content: "- [ ] a\n", UpdatedAt: "2026-08-15 10:00:00",
	}})
	if err != nil {
		t.Fatal(err)
	}

	first, err := eng.Pull(ctx, wsID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if first.Cursor == 0 || len(first.Changes) != 1 {
		t.Fatalf("primer pull: cursor=%d changes=%d", first.Cursor, len(first.Changes))
	}

	// el segundo push genera un cambio nuevo
	_, err = eng.Push(ctx, wsID, slug, []PushCommand{{
		IdempotencyKey: "p2", Op: "doc.upsert", Path: "b.md",
		Content: "- [ ] b\n", UpdatedAt: "2026-08-15 10:05:00",
	}})
	if err != nil {
		t.Fatal(err)
	}

	// pull desde el cursor anterior: solo el cambio nuevo
	second, err := eng.Pull(ctx, wsID, first.Cursor, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Changes) != 1 || second.Changes[0].Doc.Path != "b.md" {
		t.Fatalf("delta incremental incorrecto: %+v", second.Changes)
	}
}

// TestPullCursorDoesNotSkipOnLimit: el cursor del pull debe ser el seq
// de la última fila ENTREGADA, no el máximo global — si no, los
// cambios que no caben en un page se pierden para siempre.
func TestPullCursorDoesNotSkipOnLimit(t *testing.T) {
	eng, _, _, wsID, slug := setup(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		_, err := eng.Push(ctx, wsID, slug, []PushCommand{{
			IdempotencyKey: ulid.Make().String(),
			Op:             "doc.upsert",
			Path:           "doc" + string(rune('a'+i)) + ".md",
			Content:        "- [ ] t\n",
			UpdatedAt:      "2026-08-15 10:0" + string(rune('0'+i)) + ":00",
		}})
		if err != nil {
			t.Fatal(err)
		}
	}

	page1, err := eng.Pull(ctx, wsID, 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page1.Changes) != 2 {
		t.Fatalf("page1 = %d cambios", len(page1.Changes))
	}

	// continuar desde el cursor del page1: los 3 restantes, ninguno perdido
	page2, err := eng.Pull(ctx, wsID, page1.Cursor, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(page2.Changes) != 3 {
		t.Fatalf("page2 = %d cambios (esperaba 3) — el cursor se saltó filas", len(page2.Changes))
	}
}

// TestPushWithoutUpdatedAtPreservesLoser: sin updated_at el comando
// gana (compat) pero la versión perdedora se preserva en doc_versions
// (§9: la versión perdedora NUNCA se pierde).
func TestPushWithoutUpdatedAtPreservesLoser(t *testing.T) {
	eng, queries, _, wsID, slug := setup(t)
	ctx := context.Background()

	_, err := eng.Push(ctx, wsID, slug, []PushCommand{{
		IdempotencyKey: "u1", Op: "doc.upsert", Path: "lww.md",
		Content: "- [ ] contenido original del servidor\n",
		UpdatedAt: "2026-08-15 10:00:00",
	}})
	if err != nil {
		t.Fatal(err)
	}

	// push sin updated_at: gana, pero el contenido previo se preserva
	_, err = eng.Push(ctx, wsID, slug, []PushCommand{{
		IdempotencyKey: "u2", Op: "doc.upsert", Path: "lww.md",
		Content: "- [ ] contenido nuevo del cliente\n",
	}})
	if err != nil {
		t.Fatal(err)
	}

	doc, err := queries.GetDocByPath(ctx, db.GetDocByPathParams{WorkspaceID: wsID, Path: "lww.md"})
	if err != nil {
		t.Fatal(err)
	}
	versions, err := queries.GetDocVersions(ctx, db.GetDocVersionsParams{DocID: doc.ID, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) == 0 {
		t.Fatal("la versión perdedora debe quedar en doc_versions")
	}
	// el hash del snapshot debe ser el del contenido original perdedor
	wantHash := docs.ContentHash([]byte("- [ ] contenido original del servidor\n"))
	found := false
	for _, v := range versions {
		if v.ContentHash == wantHash {
			found = true
		}
	}
	if !found {
		t.Fatal("el snapshot de la versión perdedora no contiene el contenido original")
	}
}

// TestDocDeleteIsIdempotent: repetir doc.delete no deja el batch clavado.
func TestDocDeleteIsIdempotent(t *testing.T) {
	eng, _, _, wsID, slug := setup(t)
	ctx := context.Background()

	_, err := eng.Push(ctx, wsID, slug, []PushCommand{{
		IdempotencyKey: "d1", Op: "doc.upsert", Path: "borrar.md",
		Content: "- [ ] t\n", UpdatedAt: "2026-08-15 10:00:00",
	}})
	if err != nil {
		t.Fatal(err)
	}
	docList, err := eng.Pull(ctx, wsID, 0, 10)
	if err != nil || len(docList.Changes) == 0 {
		t.Fatalf("pull: %v %+v", err, docList)
	}
	docID := docList.Changes[0].EntityID

	for i := 0; i < 2; i++ {
		_, err := eng.Push(ctx, wsID, slug, []PushCommand{{
			IdempotencyKey: "del" + string(rune('0'+i)), Op: "doc.delete", DocID: docID,
		}})
		if err != nil {
			t.Fatalf("doc.delete #%d: %v", i, err)
		}
	}
}
