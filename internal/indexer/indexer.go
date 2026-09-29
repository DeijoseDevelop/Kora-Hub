// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
// Package indexer sincroniza el filesystem canónico con el índice
// SQLite (P1): recorre los archivos Markdown, regenera los índices de
// documentos, tareas, backlinks y FTS, y descarta la caché anterior.
// Borrar la base de datos es una operación segura (sección 4.4).
package indexer

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"path"
	"strings"

	"github.com/DeijoseDevelop/Kora-Hub/internal/db"
	"github.com/DeijoseDevelop/Kora-Hub/internal/docs"
	"github.com/DeijoseDevelop/Kora-Hub/internal/graph"
	"github.com/DeijoseDevelop/Kora-Hub/internal/tasks"
	"github.com/oklog/ulid/v2"
)

// Indexer reindexa un workspace desde su árbol canónico.
type Indexer struct {
	store   *docs.Store
	queries *db.Queries
	conn    *sql.DB
	logger  *slog.Logger
}

func New(store *docs.Store, queries *db.Queries, conn *sql.DB, logger *slog.Logger) *Indexer {
	return &Indexer{store: store, queries: queries, conn: conn, logger: logger}
}

// ReindexWorkspace recorre todos los archivos del workspace y regenera
// el índice. Incremental: solo se reparsean archivos cuyo content_hash
// cambió (sección 6.2).
func (ix *Indexer) ReindexWorkspace(ctx context.Context, workspaceID, slug string) (int, error) {
	ws, err := ix.queries.GetWorkspaceByID(ctx, workspaceID)
	if err != nil {
		return 0, err
	}
	ownerID := ws.OwnerID

	files, err := ix.store.List(slug)
	if err != nil {
		return 0, err
	}

	indexed := 0
	for _, rel := range files {
		changed, err := ix.reindexFile(ctx, workspaceID, slug, rel, ownerID)
		if err != nil {
			ix.logger.Warn("reindexar archivo", "path", rel, "err", err)
			continue
		}
		if changed {
			indexed++
		}
	}
	return indexed, nil
}

// reindexFile indexa un único documento si su contenido cambió.
func (ix *Indexer) reindexFile(ctx context.Context, workspaceID, slug, rel, ownerID string) (bool, error) {
	content, err := ix.store.Read(slug, rel)
	if err != nil {
		return false, err
	}
	hash := docs.ContentHash(content)

	existing, err := ix.queries.GetDocByPath(ctx, db.GetDocByPathParams{
		WorkspaceID: workspaceID,
		Path:        rel,
	})
	if err == nil && existing.ContentHash == hash {
		return false, nil // sin drift: no se reindexa
	}

	docID := existing.ID
	if docID == "" {
		docID = ulid.Make().String()
	}
	title := deriveTitle(rel, content)
	if _, err := ix.queries.UpsertDoc(ctx, db.UpsertDocParams{
		ID:          docID,
		WorkspaceID: workspaceID,
		Path:        rel,
		Title:       title,
		ContentHash: hash,
		CreatedBy:   ownerID, // el autor real se resuelve por membresía en sync
	}); err != nil {
		return false, err
	}

	// el cambio queda registrado para el sync delta (sección 9.1)
	if _, err := ix.queries.InsertChange(ctx, db.InsertChangeParams{
		WorkspaceID: workspaceID,
		Entity:      "doc",
		EntityID:    docID,
		Op:          "upsert",
	}); err != nil {
		return false, err
	}

	if err := ix.rebuildTasks(ctx, workspaceID, docID, rel, content); err != nil {
		return false, err
	}
	if err := ix.rebuildBacklinks(ctx, docID, content); err != nil {
		return false, err
	}
	if err := ix.rebuildFTS(ctx, docID, title, content); err != nil {
		return false, err
	}
	return true, nil
}

// rebuildTasks reemplaza las tareas del documento en el índice
// (clave: doc_id + line_no; idempotente por diseño).
func (ix *Indexer) rebuildTasks(ctx context.Context, workspaceID, docID, rel string, content []byte) error {
	if err := ix.queries.DeleteTasksForDoc(ctx, docID); err != nil {
		return err
	}
	for _, t := range tasks.Parse(string(content)) {
		// el project por defecto es la carpeta del documento
		project := t.Project
		if project == "" {
			project = defaultProject(rel)
		}
		if err := ix.queries.UpsertTask(ctx, db.UpsertTaskParams{
			ID:          ulid.Make().String(),
			WorkspaceID: workspaceID,
			DocID:       docID,
			LineNo:      int64(t.Line),
			Title:       t.Text,
			DueDate:     sql.NullString{String: t.DueDate, Valid: t.DueDate != ""},
			Project:     sql.NullString{String: project, Valid: project != ""},
			Priority:    sql.NullString{String: t.Priority, Valid: t.Priority != ""},
			Assignee:    sql.NullString{String: t.Assignee, Valid: t.Assignee != ""},
			Done:        boolToInt64(t.Done),
			InProgress:  boolToInt64(t.InProgress),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (ix *Indexer) rebuildBacklinks(ctx context.Context, docID string, content []byte) error {
	// los wikilinks editados fuera quedarían como residuos sin el DELETE
	if _, err := ix.conn.ExecContext(ctx, `DELETE FROM backlinks WHERE src_doc_id = ?`, docID); err != nil {
		return err
	}
	for _, link := range graph.ExtractBacklinks(string(content)) {
		// el destino es el texto del [[wikilink]]; el ancla #seccion no
		// forma parte del nombre del documento destino
		dest := link.Dest
		if i := strings.Index(dest, "#"); i >= 0 {
			dest = dest[:i]
		}
		dest = strings.TrimSpace(dest)
		if dest == "" {
			continue
		}
		if _, err := ix.conn.ExecContext(ctx,
			`INSERT OR REPLACE INTO backlinks (src_doc_id, dst_doc_id, anchor_text) VALUES (?, ?, ?)`,
			docID, dest, link.Anchor); err != nil {
			return err
		}
	}
	return nil
}

func (ix *Indexer) rebuildFTS(ctx context.Context, docID, title string, content []byte) error {
	// FTS5 autoincrementa su rowid interno: para evitar duplicados se
	// borra la fila anterior del documento antes de insertar.
	if _, err := ix.conn.ExecContext(ctx, `DELETE FROM docs_fts WHERE doc_id = ?`, docID); err != nil {
		return err
	}
	_, err := ix.conn.ExecContext(ctx,
		`INSERT INTO docs_fts (doc_id, title, content) VALUES (?, ?, ?)`,
		docID, title, string(content))
	return err
}

// DeleteDoc borra el documento del filesystem canónico y del índice,
// registrando la operación para el sync delta.
func (ix *Indexer) DeleteDoc(ctx context.Context, workspaceID, slug, docID string) error {
	doc, err := ix.queries.GetDocByID(ctx, db.GetDocByIDParams{ID: docID, WorkspaceID: workspaceID})
	if err != nil {
		return err
	}
	if err := ix.store.Delete(slug, doc.Path); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := ix.queries.SoftDeleteDoc(ctx, db.SoftDeleteDocParams{ID: docID, WorkspaceID: workspaceID}); err != nil {
		return err
	}
	for _, stmt := range []string{
		`DELETE FROM docs_fts WHERE doc_id = ?`,
		`DELETE FROM backlinks WHERE src_doc_id = ?`,
	} {
		if _, err := ix.conn.ExecContext(ctx, stmt, docID); err != nil {
			return err
		}
	}
	if err := ix.queries.DeleteTasksForDoc(ctx, docID); err != nil {
		return err
	}
	_, err = ix.queries.InsertChange(ctx, db.InsertChangeParams{
		WorkspaceID: workspaceID, Entity: "doc", EntityID: docID, Op: "delete",
	})
	return err
}

// deriveTitle extrae el título del primer encabezado H1 o usa el nombre
// del archivo.
func deriveTitle(rel string, content []byte) string {
	for _, line := range strings.Split(string(content), "\n") {
		if t := strings.TrimSpace(line); strings.HasPrefix(t, "# ") {
			return strings.TrimSpace(t[2:])
		}
	}
	name := path.Base(rel)
	return strings.TrimSuffix(name, path.Ext(name))
}

func defaultProject(rel string) string {
	dir := path.Dir(rel)
	if dir == "." {
		return "inbox"
	}
	return dir
}

func boolToInt64(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
