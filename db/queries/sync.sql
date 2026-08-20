-- Copyright (C) 2026 Deijose <tech@deijose.dev>
-- SPDX-License-Identifier: AGPL-3.0-only
-- Sync delta (seccion 9): cursor monotono por workspace + push con
-- deduplicacion por idempotency-key.

-- name: InsertChange :one
INSERT INTO change_log (workspace_id, entity, entity_id, op)
VALUES (?, ?, ?, ?)
RETURNING seq, created_at;

-- name: GetChangesAfter :many
SELECT seq, workspace_id, entity, entity_id, op, created_at
FROM change_log
WHERE workspace_id = ? AND seq > ?
ORDER BY seq ASC
LIMIT ?;

-- name: LastChangeSeq :one
SELECT seq FROM change_log
WHERE workspace_id = ?
ORDER BY seq DESC
LIMIT 1;

-- name: MarkCommandProcessed :exec
INSERT INTO sync_commands (idempotency_key, workspace_id, command_json)
VALUES (?, ?, ?);

-- name: IsCommandProcessed :one
SELECT 1 FROM sync_commands WHERE idempotency_key = ? AND workspace_id = ?;

-- name: InsertDocVersion :exec
INSERT INTO doc_versions (doc_id, content_hash, created_at, created_by)
VALUES (?, ?, datetime('now'), ?);

-- name: GetDocVersions :many
SELECT doc_id, content_hash, created_at, created_by
FROM doc_versions
WHERE doc_id = ?
ORDER BY created_at DESC
LIMIT ?;

-- name: SetDocUpdatedAt :exec
UPDATE docs SET updated_at = ?
WHERE workspace_id = ? AND path = ?;
