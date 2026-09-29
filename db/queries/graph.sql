-- name: ListDocsForGraph :many
SELECT id, title FROM docs
WHERE workspace_id = ? AND deleted_at IS NULL;

-- name: ListBacklinksForGraph :many
SELECT src_doc_id, dst_doc_id, anchor_text
FROM backlinks
WHERE src_doc_id IN (SELECT id FROM docs WHERE workspace_id = ? AND deleted_at IS NULL);

-- name: ListBacklinksTo :many
SELECT src.id, src.path, src.title, b.anchor_text
FROM backlinks b
JOIN docs src ON src.id = b.src_doc_id AND src.deleted_at IS NULL
JOIN docs dst ON dst.id = ? AND dst.workspace_id = ?
WHERE LOWER(dst.title) = LOWER(b.dst_doc_id)
   OR LOWER(dst.path) = LOWER(b.dst_doc_id)
   OR LOWER(dst.path) = LOWER(b.dst_doc_id) || '.md';
