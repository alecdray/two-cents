-- name: CreateCategory :one
INSERT INTO categories (
    id,
    name,
    builtin,
    archived
) VALUES (
    ?, ?, ?, ?
)
RETURNING *;

-- name: GetCategory :one
SELECT * FROM categories
WHERE id = ?;

-- name: ListCategories :many
SELECT * FROM categories
ORDER BY name COLLATE NOCASE;

-- name: UpdateCategory :one
UPDATE categories
SET name       = ?,
    archived   = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?
RETURNING *;
