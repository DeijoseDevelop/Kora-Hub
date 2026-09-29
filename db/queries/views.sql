-- Copyright (C) 2026 Deijose <tech@deijose.dev>
-- SPDX-License-Identifier: AGPL-3.0-only
-- Saved views: filtros serializados por workspace (D1).

-- name: CreateSavedView :exec
INSERT INTO saved_views (id, workspace_id, name, filters, created_by)
VALUES (?, ?, ?, ?, ?);

-- name: ListSavedViews :many
SELECT id, workspace_id, name, filters, created_by, created_at
FROM saved_views
WHERE workspace_id = ?
ORDER BY name;

-- name: GetSavedView :one
SELECT id, workspace_id, name, filters, created_by, created_at
FROM saved_views
WHERE id = ? AND workspace_id = ?;

-- name: DeleteSavedView :exec
DELETE FROM saved_views WHERE id = ? AND workspace_id = ?;
