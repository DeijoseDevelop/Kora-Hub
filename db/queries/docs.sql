-- Copyright (C) 2026 Deijose <tech@deijose.dev>
-- SPDX-License-Identifier: AGPL-3.0-only
-- name: CreateDoc :exec
INSERT INTO docs (id, workspace_id, path, title, content_hash, created_by)
VALUES (?, ?, ?, ?, ?, ?);

-- name: GetDoc :one
SELECT id, workspace_id, path, title, content_hash, created_by,
       created_at, updated_at, deleted_at
FROM docs
WHERE id = ? AND workspace_id = ?;

-- name: UpdateDocContent :exec
UPDATE docs
SET title = ?, content_hash = ?, updated_at = datetime('now')
WHERE id = ? AND workspace_id = ?;

-- name: ListDocsByWorkspace :many
SELECT id, workspace_id, path, title, content_hash, created_by,
       created_at, updated_at, deleted_at
FROM docs
WHERE workspace_id = ? AND deleted_at IS NULL
ORDER BY updated_at DESC;



-- name: SoftDeleteDoc :exec
UPDATE docs SET deleted_at = datetime('now')
WHERE id = ? AND workspace_id = ?;
