-- Copyright (C) 2026 Deijose <tech@deijose.dev>
-- SPDX-License-Identifier: AGPL-3.0-only
-- 004_doc_versions.sql — Los snapshots de versiones viven como archivos
-- en <workspace>/.versions/ (P1: el filesystem es la verdad); la fila
-- solo registra metadatos + la ruta del snapshot.
ALTER TABLE doc_versions ADD COLUMN storage_path TEXT NOT NULL DEFAULT '';
