-- name: GetUser :one
SELECT * FROM users
WHERE id = ?;

-- name: UpsertUser :one
INSERT INTO users (
    id,
    username,
    password_hash
) VALUES (
    ?, ?, ?
)
ON CONFLICT(id) DO UPDATE SET
    username      = excluded.username,
    password_hash = excluded.password_hash,
    updated_at    = CURRENT_TIMESTAMP
RETURNING *;
