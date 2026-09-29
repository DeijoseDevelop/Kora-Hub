-- Copyright (C) 2026 Deijose <tech@deijose.dev>
-- SPDX-License-Identifier: AGPL-3.0-only
-- Adjuntos (seccion 10): metadatos en el indice, bytes en el backend
-- de almacenamiento (local o S3-compatible).

-- name: CreateAttachment :exec
INSERT INTO attachments (id, workspace_id, doc_id, filename, mime, size_bytes, storage_key)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: GetAttachment :one
SELECT id, workspace_id, doc_id, filename, mime, size_bytes, storage_key, created_at
FROM attachments
WHERE id = ? AND workspace_id = ?;

-- name: ListAttachmentsByWorkspace :many
SELECT id, workspace_id, doc_id, filename, mime, size_bytes, storage_key, created_at
FROM attachments
WHERE workspace_id = ?
ORDER BY created_at DESC;

-- name: DeleteAttachment :exec
DELETE FROM attachments WHERE id = ? AND workspace_id = ?;

-- name: SearchAttachmentsByName :many
SELECT id, workspace_id, doc_id, filename, mime, size_bytes, storage_key, created_at
FROM attachments
WHERE workspace_id = sqlc.arg(workspace_id) AND filename LIKE '%' || sqlc.arg(term) || '%'
ORDER BY created_at DESC
LIMIT sqlc.arg(max_results);
