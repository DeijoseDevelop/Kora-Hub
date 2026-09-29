-- Copyright (C) 2026 Deijose <tech@deijose.dev>
-- SPDX-License-Identifier: AGPL-3.0-only
-- 006_saved_views.sql — Vistas guardadas por workspace (D1, aprobada
-- 2026-09-29): filtros serializados en JSON que la UI aplica sobre las
-- proyecciones del indice. Son datos de configuracion del workspace,
-- no contenido canonico: viven en SQLite, no en el filesystem.
CREATE TABLE IF NOT EXISTS saved_views (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id),
    name         TEXT NOT NULL,
    filters      TEXT NOT NULL DEFAULT '{}',
    created_by   TEXT,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (workspace_id, name)
);
