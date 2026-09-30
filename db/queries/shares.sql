-- Copyright (C) 2026 Deijose <tech@deijose.dev>
-- SPDX-License-Identifier: AGPL-3.0-only
-- shares.sql - share-links publicos de documentos (ASCII puro: sqlc)

-- name: UpsertDocShare :exec
INSERT INTO doc_shares (doc_id, token, created_by)
VALUES (?, ?, ?)
ON CONFLICT (doc_id) DO UPDATE SET token = excluded.token,
    created_by = excluded.created_by,
    created_at = datetime('now');

-- name: GetDocShare :one
SELECT doc_id, token, created_by, created_at
FROM doc_shares WHERE doc_id = ?;

-- name: GetDocShareByToken :one
SELECT doc_id, token, created_by, created_at
FROM doc_shares WHERE token = ?;

-- name: DeleteDocShare :exec
DELETE FROM doc_shares WHERE doc_id = ?;

-- name: GetPublicDocByToken :one
-- Lookup publico: el token ES la capacidad (no hay workspace explicito).
-- Devuelve slug para leer el archivo canonico del store.
SELECT d.id, d.workspace_id, w.slug, d.path, d.title, d.updated_at
FROM doc_shares s
JOIN docs d ON d.id = s.doc_id
JOIN workspaces w ON w.id = d.workspace_id
WHERE s.token = ? AND d.deleted_at IS NULL;
