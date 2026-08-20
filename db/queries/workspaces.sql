-- Copyright (C) 2026 Deijose <tech@deijose.dev>
-- SPDX-License-Identifier: AGPL-3.0-only
-- name: CreateWorkspace :exec
INSERT INTO workspaces (id, slug, name, owner_id)
VALUES (?, ?, ?, ?);

-- name: AddMembership :exec
INSERT INTO memberships (user_id, workspace_id, role)
VALUES (?, ?, ?);

-- name: GetMembership :one
SELECT user_id, workspace_id, role
FROM memberships
WHERE user_id = ? AND workspace_id = ?;

-- name: ListWorkspacesByUser :many
SELECT w.id, w.slug, w.name, w.owner_id, w.created_at, m.role
FROM workspaces w
JOIN memberships m ON m.workspace_id = w.id
WHERE m.user_id = ?
ORDER BY w.created_at DESC;
