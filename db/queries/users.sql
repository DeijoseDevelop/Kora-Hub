-- Copyright (C) 2026 Deijose <tech@deijose.dev>
-- SPDX-License-Identifier: AGPL-3.0-only
-- name: CreateUser :exec
INSERT INTO users (id, email, password_hash, display_name)
VALUES (?, ?, ?, ?);

-- name: GetUserByEmail :one
SELECT id, email, password_hash, display_name, created_at
FROM users
WHERE email = ?;

-- name: GetUserByID :one
SELECT id, email, password_hash, display_name, created_at
FROM users
WHERE id = ?;

-- name: CreateRefreshToken :exec
INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at)
VALUES (?, ?, ?, ?);

-- name: GetRefreshTokenByID :one
SELECT id, user_id, token_hash, expires_at, revoked_at
FROM refresh_tokens
WHERE id = ?;

-- name: RevokeRefreshToken :exec
UPDATE refresh_tokens SET revoked_at = datetime('now')
WHERE id = ?;

-- name: DeleteExpiredRefreshTokens :exec
DELETE FROM refresh_tokens WHERE expires_at < datetime('now');

-- name: CountUsers :one
SELECT COUNT(*) FROM users;

-- name: ListUsers :many
SELECT id, email, display_name, created_at
FROM users
ORDER BY created_at ASC;
