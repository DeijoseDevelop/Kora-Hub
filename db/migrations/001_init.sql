-- Copyright (C) 2026 Deijose <tech@deijose.dev>
-- SPDX-License-Identifier: AGPL-3.0-only
-- 001_init.sql — Esquema del índice (P1: la DB es índice reconstruible).
-- Documento Técnico de Arquitectura, sección 5.1.

CREATE TABLE IF NOT EXISTS users (
    id            TEXT PRIMARY KEY,             -- ULID
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    display_name  TEXT NOT NULL,
    created_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS workspaces (
    id         TEXT PRIMARY KEY,
    slug       TEXT NOT NULL UNIQUE,
    name       TEXT NOT NULL,
    owner_id   TEXT NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS memberships (
    user_id      TEXT NOT NULL REFERENCES users(id),
    workspace_id TEXT NOT NULL REFERENCES workspaces(id),
    role         TEXT NOT NULL CHECK (role IN ('owner', 'editor', 'viewer')),
    PRIMARY KEY (user_id, workspace_id)
);

CREATE TABLE IF NOT EXISTS refresh_tokens (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id),
    token_hash TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    revoked_at TEXT
);

CREATE TABLE IF NOT EXISTS docs (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id),
    path         TEXT NOT NULL,
    title        TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    created_by   TEXT NOT NULL REFERENCES users(id),
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at   TEXT NOT NULL DEFAULT (datetime('now')),
    deleted_at   TEXT,
    UNIQUE (workspace_id, path)
);

CREATE TABLE IF NOT EXISTS doc_versions (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    doc_id       TEXT NOT NULL REFERENCES docs(id),
    content_hash TEXT NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    created_by   TEXT NOT NULL REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS tasks (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id),
    doc_id       TEXT NOT NULL REFERENCES docs(id),
    line_no      INTEGER NOT NULL,
    title        TEXT NOT NULL,
    due_date     TEXT,
    project      TEXT,
    priority     TEXT CHECK (priority IN ('baja', 'media', 'alta')),
    assignee     TEXT,
    done         INTEGER NOT NULL DEFAULT 0 CHECK (done IN (0, 1)),
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at   TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (doc_id, line_no)
);

CREATE TABLE IF NOT EXISTS backlinks (
    src_doc_id  TEXT NOT NULL REFERENCES docs(id),
    dst_doc_id  TEXT NOT NULL REFERENCES docs(id),
    anchor_text TEXT NOT NULL,
    PRIMARY KEY (src_doc_id, dst_doc_id)
);

CREATE TABLE IF NOT EXISTS attachments (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id),
    doc_id       TEXT REFERENCES docs(id),
    filename     TEXT NOT NULL,
    mime         TEXT NOT NULL,
    size_bytes   INTEGER NOT NULL,
    storage_key  TEXT NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS webhooks (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id),
    url          TEXT NOT NULL,
    secret       TEXT NOT NULL,
    events       TEXT NOT NULL,                 -- JSON array
    active       INTEGER NOT NULL DEFAULT 1,
    created_at   TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS audit_log (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    workspace_id TEXT REFERENCES workspaces(id),
    actor_id     TEXT REFERENCES users(id),
    action       TEXT NOT NULL,
    entity       TEXT NOT NULL,
    entity_id    TEXT NOT NULL,
    meta_json    TEXT,
    created_at   TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Sync delta: cursor monótono por workspace (sección 9.1)
CREATE TABLE IF NOT EXISTS change_log (
    seq          INTEGER PRIMARY KEY AUTOINCREMENT,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id),
    entity       TEXT NOT NULL,
    entity_id    TEXT NOT NULL,
    op           TEXT NOT NULL,                 -- upsert | delete
    created_at   TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_change_log_ws ON change_log(workspace_id, seq);

-- FTS5 (sección 5.1): índice de búsqueda full-text.
-- doc_id es UNINDEXED: el rowid interno lo gestiona FTS5 y el doc_id
-- de aplicación se conserva como columna de unión con docs.
CREATE VIRTUAL TABLE IF NOT EXISTS docs_fts USING fts5(
    doc_id UNINDEXED, title, content, tokenize='unicode61'
);
