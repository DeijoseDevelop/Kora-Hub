-- Copyright (C) 2026 Deijose <tech@deijose.dev>
-- SPDX-License-Identifier: AGPL-3.0-only
-- 007_tasks_v2.sql — Extension de gramatica aprobada (D2, 2026-09-29,
-- doc tecnico §6.5): recurrencia *every:<n>d|w|m|y, identidad estable
-- ^id: y dependencias ^blocked-by:.
-- task_uid NO es UNIQUE: duplicados se reportan como warning (regla
-- tolerante §6.2), nunca se rompe el indexado.
ALTER TABLE tasks ADD COLUMN task_uid   TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN recur      TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN blocked_by TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_tasks_uid ON tasks (workspace_id, task_uid);
