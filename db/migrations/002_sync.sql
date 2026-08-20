-- Copyright (C) 2026 Deijose <tech@deijose.dev>
-- SPDX-License-Identifier: AGPL-3.0-only
-- 002_sync.sql — Infraestructura del sync delta (sección 9).
-- Deduplicación de replays por idempotency-key: el cliente encola
-- comandos y los reenvía al reconectar; el servidor nunca aplica dos
-- veces el mismo comando.
CREATE TABLE IF NOT EXISTS sync_commands (
    idempotency_key TEXT PRIMARY KEY,
    workspace_id    TEXT NOT NULL REFERENCES workspaces(id),
    command_json    TEXT NOT NULL,
    created_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Índice para resolución de conflictos LWW: al descartar una versión
-- perdedora se conserva una copia previa del documento.
CREATE INDEX IF NOT EXISTS idx_change_log_ws_seq
    ON change_log(workspace_id, seq);
