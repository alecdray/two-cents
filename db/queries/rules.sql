-- name: CreateRule :one
INSERT INTO rules (
    id,
    merchant_substring,
    classification,
    category_id
) VALUES (
    ?, ?, ?, ?
)
RETURNING *;

-- name: GetRule :one
SELECT * FROM rules
WHERE id = ?;

-- name: ListRules :many
SELECT * FROM rules
ORDER BY updated_at DESC, id DESC;

-- name: UpdateRule :one
UPDATE rules
SET merchant_substring = ?,
    classification     = ?,
    category_id        = ?,
    updated_at         = CURRENT_TIMESTAMP
WHERE id = ?
RETURNING *;

-- name: DeleteRule :exec
DELETE FROM rules
WHERE id = ?;
