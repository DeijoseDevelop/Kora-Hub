-- Copyright (C) 2026 Deijose <tech@deijose.dev>
-- SPDX-License-Identifier: AGPL-3.0-only
-- 005_backlinks_fk.sql — dst_doc_id guarda el TEXTO del [[wikilink]]
-- (se resuelve a doc por titulo/path en query-time), no un id de doc:
-- el REFERENCES hacia fallar silenciosamente cada insert con
-- foreign_keys(1) y la tabla quedaba siempre vacia.
CREATE TABLE IF NOT EXISTS backlinks_new (
    src_doc_id  TEXT NOT NULL REFERENCES docs(id),
    dst_doc_id  TEXT NOT NULL,
    anchor_text TEXT NOT NULL,
    PRIMARY KEY (src_doc_id, dst_doc_id)
);
INSERT OR IGNORE INTO backlinks_new SELECT * FROM backlinks;
DROP TABLE backlinks;
ALTER TABLE backlinks_new RENAME TO backlinks;
