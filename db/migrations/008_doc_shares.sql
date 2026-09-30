-- Copyright (C) 2026 Deijose <tech@deijose.dev>
-- SPDX-License-Identifier: AGPL-3.0-only
-- 008_doc_shares.sql — Share-links publicos por documento (D5,
-- aprobada 2026-09-29): un token ULID por doc; el GET publico sirve
-- el contenido sin sesion. Un share por doc (PK doc_id); regenerar
-- reemplaza el token e invalida el anterior (revocacion implicita).
CREATE TABLE IF NOT EXISTS doc_shares (
    doc_id     TEXT PRIMARY KEY REFERENCES docs(id),
    token      TEXT NOT NULL UNIQUE,
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
