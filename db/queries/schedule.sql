-- name: ListScheduleItems :many
SELECT * FROM schedule_items
ORDER BY name COLLATE NOCASE, rowid;

-- name: GetScheduleItem :one
SELECT * FROM schedule_items
WHERE id = ?;

-- name: InsertScheduleItem :exec
INSERT INTO schedule_items (
    id,
    name,
    direction,
    amount,
    cadence,
    day_of_month,
    anchor_date,
    active
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?
);

-- name: UpdateScheduleItem :exec
UPDATE schedule_items
SET name         = ?,
    direction    = ?,
    amount       = ?,
    cadence      = ?,
    day_of_month = ?,
    anchor_date  = ?,
    active       = ?,
    updated_at   = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: DeleteScheduleItem :exec
DELETE FROM schedule_items
WHERE id = ?;
