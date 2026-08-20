-- Copyright (C) 2026 Deijose <tech@deijose.dev>
-- SPDX-License-Identifier: AGPL-3.0-only
-- name: ListTasksByWorkspace :many
SELECT * FROM tasks
WHERE workspace_id = ? AND done = ?
ORDER BY done ASC, in_progress DESC, due_date ASC;

-- name: ListTasksByProject :many
SELECT * FROM tasks
WHERE workspace_id = ? AND project = ?
ORDER BY due_date ASC;

-- name: ListTasksDueToday :many
SELECT * FROM tasks
WHERE workspace_id = ? AND assignee = ?
  AND due_date <= date('now') AND done = 0
ORDER BY due_date ASC;

-- name: UpsertTask :exec
INSERT INTO tasks (id, workspace_id, doc_id, line_no, title, due_date,
                   project, priority, assignee, done, in_progress)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (doc_id, line_no) DO UPDATE SET
    title = excluded.title,
    due_date = excluded.due_date,
    project = excluded.project,
    priority = excluded.priority,
    assignee = excluded.assignee,
    done = excluded.done,
    in_progress = excluded.in_progress,
    updated_at = datetime('now');

-- name: DeleteTasksForDoc :exec
DELETE FROM tasks WHERE doc_id = ?;

-- name: SetTaskDone :exec
UPDATE tasks SET done = ?, in_progress = 0, updated_at = datetime('now')
WHERE id = ? AND workspace_id = ?;

-- name: ListTasksByDateRange :many
SELECT * FROM tasks
WHERE workspace_id = ? AND due_date >= ? AND due_date <= ?
ORDER BY due_date ASC, done ASC;

-- name: ListTasksMineToday :many
SELECT * FROM tasks
WHERE workspace_id = ? AND assignee = ? AND due_date <= date('now') AND done = 0
ORDER BY due_date ASC;
