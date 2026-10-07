-- Copyright (C) 2026 Deijose <tech@deijose.dev>
-- SPDX-License-Identifier: AGPL-3.0-only
-- name: GetWorkspaceByID :one
SELECT id, slug, name, owner_id, created_at
FROM workspaces
WHERE id = ?;

-- name: GetDocByID :one
SELECT id, workspace_id, path, title, content_hash, updated_at
FROM docs
WHERE id = ? AND workspace_id = ? AND deleted_at IS NULL;

-- name: GetDocByPath :one
SELECT id, title, content_hash, updated_at
FROM docs
WHERE workspace_id = ? AND path = ? AND deleted_at IS NULL;

-- name: UpsertDoc :one
INSERT INTO docs (id, workspace_id, path, title, content_hash, created_by)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(workspace_id, path) DO UPDATE SET
    title = excluded.title,
    content_hash = excluded.content_hash,
    updated_at = datetime('now'),
    deleted_at = NULL
RETURNING id;

-- name: GetTaskByID :one
SELECT * FROM tasks
WHERE id = ? AND workspace_id = ?;
