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

-- name: UpdateWorkspaceName :exec
UPDATE workspaces SET name = ?
WHERE id = ?;

-- name: ListMembersByWorkspace :many
SELECT u.id AS user_id, u.email, u.display_name, m.role
FROM memberships m
JOIN users u ON u.id = m.user_id
WHERE m.workspace_id = ?
ORDER BY m.role ASC, u.email ASC;

-- name: UpdateMembershipRole :exec
UPDATE memberships SET role = ?
WHERE user_id = ? AND workspace_id = ?;

-- name: RemoveMembership :exec
DELETE FROM memberships
WHERE user_id = ? AND workspace_id = ?;

-- name: DeleteWorkspaceRows :exec
DELETE FROM workspaces WHERE id = ?;

-- name: DeleteMembershipsOfWorkspace :exec
DELETE FROM memberships WHERE workspace_id = ?;

-- name: DeleteWorkspaceIndex :exec
DELETE FROM docs WHERE workspace_id = ?;

-- name: DeleteWorkspaceTasks :exec
DELETE FROM tasks WHERE workspace_id = ?;

-- name: DeleteWorkspaceChanges :exec
DELETE FROM change_log WHERE workspace_id = ?;

-- name: DeleteWorkspaceCommands :exec
DELETE FROM sync_commands WHERE workspace_id = ?;

-- name: DeleteWorkspaceAttachments :exec
DELETE FROM attachments WHERE workspace_id = ?;
