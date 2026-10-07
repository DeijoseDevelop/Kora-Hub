// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
// Package search expone la búsqueda full-text FTS5 (sección 4.2).
// Se implementa con SQL directo: sqlc no conoce las tablas virtuales
// de SQLite, y el FTS es índice reconstruible (P1), no modelo de datos.
package search

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// Result es un documento que matcheó la búsqueda.
type Result struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Path      string `json:"path"`
	UpdatedAt string `json:"updated_at"`
}

const query = `
SELECT d.id, d.title, d.path, d.updated_at
FROM docs_fts, docs d
WHERE docs_fts MATCH ?
  AND d.id = docs_fts.doc_id
  AND d.workspace_id = ?
  AND d.deleted_at IS NULL
ORDER BY rank
LIMIT ?`

// SearchDocs busca documentos en un workspace por consulta FTS5.
func SearchDocs(ctx context.Context, conn *sql.DB, workspaceID, q string, limit int) ([]Result, error) {
	if q == "" {
		return nil, nil
	}
	rows, err := conn.QueryContext(ctx, query, escapeFTS(q), workspaceID, limit)
	if err != nil {
		return nil, fmt.Errorf("fts5: %w", err)
	}
	defer rows.Close()

	out := make([]Result, 0, 8)
	for rows.Next() {
		var r Result
		if err := rows.Scan(&r.ID, &r.Title, &r.Path, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// escapeFTS envuelve la consulta en comillas para que sea un literal
// FTS5: sin esto, entradas con comillas o con AND/OR/NOT rompen la
// sintaxis del MATCH y la búsqueda devuelve 500.
func escapeFTS(q string) string {
	q = strings.ReplaceAll(q, `"`, `""`)
	return `"` + q + `"`
}
