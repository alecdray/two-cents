-- name: CreateConnection :one
INSERT INTO connections (
    id, item_id, access_token, state
) VALUES (
    ?, ?, ?, ?
)
RETURNING *;

-- name: GetConnection :one
SELECT * FROM connections
WHERE id = ?;

-- name: GetConnectionByItemID :one
SELECT * FROM connections
WHERE item_id = ?;

-- name: ListConnections :many
SELECT * FROM connections
ORDER BY created_at;

-- name: UpdateConnection :one
UPDATE connections
SET item_id      = ?,
    access_token = ?,
    state        = ?,
    updated_at   = CURRENT_TIMESTAMP
WHERE id = ?
RETURNING *;
