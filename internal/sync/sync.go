// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
// Package sync implementa el motor de sincronización offline-first
// (sección 9): delta por cursor monótono por workspace, push con
// idempotency-key y conflictos last-write-wins con la versión perdedora
// preservada en doc_versions.
package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DeijoseDevelop/Kora-Hub/internal/db"
	"github.com/DeijoseDevelop/Kora-Hub/internal/docs"
	"github.com/DeijoseDevelop/Kora-Hub/internal/indexer"
)

// Change es un evento del change_log con su snapshot.
type Change struct {
	Seq       int64          `json:"seq"`
	Entity    string         `json:"entity"`
	EntityID  string         `json:"entity_id"`
	Op        string         `json:"op"` // upsert | delete
	Doc       *DocSnapshot   `json:"doc,omitempty"`
	CreatedAt string         `json:"created_at"`
}

// DocSnapshot es el estado completo de un documento para aplicar en el
// cliente (incluye el contenido: el servidor jamás omite datos).
type DocSnapshot struct {
	ID          string `json:"id"`
	Path        string `json:"path"`
	Title       string `json:"title"`
	Content     string `json:"content"`
	ContentHash string `json:"content_hash"`
	UpdatedAt   string `json:"updated_at"`
}

// PullResult es la respuesta a GET /sync/changes.
type PullResult struct {
	Cursor  int64    `json:"cursor"`
	Changes []Change `json:"changes"`
}

// PushCommand es un comando del cliente con su idempotency-key.
type PushCommand struct {
	IdempotencyKey string `json:"idempotency_key" binding:"required"`
	Op             string `json:"op" binding:"required"` // doc.upsert | doc.delete
	DocID          string `json:"doc_id,omitempty"`
	Path           string `json:"path,omitempty"`
	Content        string `json:"content,omitempty"`
	UpdatedAt      string `json:"updated_at,omitempty"`
}

// Engine agrupa lo necesario para pull y push.
type Engine struct {
	queries *db.Queries
	conn    *sql.DB
	store   *docs.Store
	indexer *indexer.Indexer
}

func NewEngine(queries *db.Queries, conn *sql.DB, store *docs.Store, idx *indexer.Indexer) *Engine {
	return &Engine{queries: queries, conn: conn, store: store, indexer: idx}
}

// Pull devuelve el delta de cambios desde el cursor y el nuevo cursor.
func (e *Engine) Pull(ctx context.Context, workspaceID string, since int64, limit int) (*PullResult, error) {
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	rows, err := e.queries.GetChangesAfter(ctx, db.GetChangesAfterParams{
		WorkspaceID: workspaceID, Seq: since, Limit: int64(limit),
	})
	if err != nil {
		return nil, err
	}

	out := make([]Change, 0, len(rows))
	for _, r := range rows {
		ch := Change{Seq: r.Seq, Entity: r.Entity, EntityID: r.EntityID, Op: r.Op, CreatedAt: r.CreatedAt}
		if r.Entity == "doc" {
			snap, err := e.docSnapshot(ctx, workspaceID, r.EntityID, r.Op == "delete")
			if err == nil {
				ch.Doc = snap
			}
		}
		out = append(out, ch)
	}

	cursor, err := e.lastSeq(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return &PullResult{Cursor: cursor, Changes: out}, nil
}

// Push aplica comandos offline del cliente. Replays con la misma
// idempotency-key se deduplican; conflictos se resuelven con
// last-write-wins conservando la versión perdedora en doc_versions.
func (e *Engine) Push(ctx context.Context, workspaceID, workspaceSlug string, cmds []PushCommand) (*PullResult, error) {
	cursorBefore, err := e.lastSeq(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	for _, cmd := range cmds {
		if err := e.apply(ctx, workspaceID, workspaceSlug, cmd); err != nil {
			return nil, fmt.Errorf("comando %s: %w", cmd.IdempotencyKey, err)
		}
	}
	// tras aplicar, devuelve solo el delta causado por este push
	return e.Pull(ctx, workspaceID, cursorBefore, 1000)
}

func (e *Engine) apply(ctx context.Context, wsID, wsSlug string, cmd PushCommand) error {
	// deduplicación de replays
	_, err := e.queries.IsCommandProcessed(ctx, db.IsCommandProcessedParams{
		IdempotencyKey: cmd.IdempotencyKey, WorkspaceID: wsID,
	})
	if err == nil {
		return nil // ya aplicado en un push anterior
	}
	if err != sql.ErrNoRows {
		return err
	}

	switch cmd.Op {
	case "doc.upsert":
		if err := e.applyDocUpsert(ctx, wsID, wsSlug, cmd); err != nil {
			return err
		}
	case "doc.delete":
		if cmd.DocID != "" {
			if err := e.indexer.DeleteDoc(ctx, wsID, wsSlug, cmd.DocID); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("op desconocida %q", cmd.Op)
	}

	raw, _ := json.Marshal(cmd)
	return e.queries.MarkCommandProcessed(ctx, db.MarkCommandProcessedParams{
		IdempotencyKey: cmd.IdempotencyKey, WorkspaceID: wsID, CommandJson: string(raw),
	})
}

// applyDocUpsert aplica LWW: si la versión del servidor es más nueva,
// se descarta el push y la versión perdedora se preserva en doc_versions.
func (e *Engine) applyDocUpsert(ctx context.Context, wsID, wsSlug string, cmd PushCommand) error {
	if cmd.Path == "" {
		return fmt.Errorf("doc.upsert sin path")
	}
	if !strings.HasSuffix(strings.ToLower(cmd.Path), ".md") {
		cmd.Path += ".md"
	}

	existing, err := e.queries.GetDocByPath(ctx, db.GetDocByPathParams{WorkspaceID: wsID, Path: cmd.Path})
	if err == nil && cmd.UpdatedAt != "" && existing.UpdatedAt > cmd.UpdatedAt {
		// LWW: el servidor gana; la versión perdedora se conserva en
		// doc_versions (recuperable, nunca destructivo).
		ws, err := e.queries.GetWorkspaceByID(ctx, wsID)
		if err != nil {
			return err
		}
		hash := docs.ContentHash([]byte(cmd.Content))
		if err := e.queries.InsertDocVersion(ctx, db.InsertDocVersionParams{
			DocID: existing.ID, ContentHash: hash, CreatedBy: ws.OwnerID,
		}); err != nil {
			return err
		}
		return nil
	}

	if err := e.store.Write(wsSlug, cmd.Path, []byte(cmd.Content)); err != nil {
		return err
	}
	if _, err := e.indexer.ReindexWorkspace(ctx, wsID, wsSlug); err != nil {
		return err
	}
	// LWW coherente: el reloj del cliente manda. Sin esto, una cola de
	// comandos offline aplicada al reconectar haría que el último cambio
	// se rechazara (el servidor usaba su propio tiempo de aplicación).
	if cmd.UpdatedAt != "" {
		if err := e.queries.SetDocUpdatedAt(ctx, db.SetDocUpdatedAtParams{
			UpdatedAt: cmd.UpdatedAt, WorkspaceID: wsID, Path: cmd.Path,
		}); err != nil {
			return err
		}
	}
	return nil
}

// lastSeq devuelve el cursor actual del workspace (0 si no hay cambios).
func (e *Engine) lastSeq(ctx context.Context, workspaceID string) (int64, error) {
	seq, err := e.queries.LastChangeSeq(ctx, workspaceID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return seq, err
}

func (e *Engine) docSnapshot(ctx context.Context, wsID, docID string, deleted bool) (*DocSnapshot, error) {
	doc, err := e.queries.GetDocByID(ctx, db.GetDocByIDParams{ID: docID, WorkspaceID: wsID})
	if err != nil {
		if err == sql.ErrNoRows && deleted {
			return &DocSnapshot{ID: docID}, nil
		}
		return nil, err
	}
	ws, err := e.queries.GetWorkspaceByID(ctx, wsID)
	if err != nil {
		return nil, err
	}
	content, err := e.store.Read(ws.Slug, doc.Path)
	if err != nil {
		return nil, err
	}
	return &DocSnapshot{
		ID:          doc.ID,
		Path:        doc.Path,
		Title:       doc.Title,
		Content:     string(content),
		ContentHash: doc.ContentHash,
		UpdatedAt:   doc.UpdatedAt,
	}, nil
}
