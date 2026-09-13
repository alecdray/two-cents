-- name: UpsertManualScheduleOccurrenceMatch :exec
-- The user's own decision about an occurrence: an association, or the assertion
-- that nothing satisfied it. It overwrites whatever stood before, including an
-- automatic match it is correcting.
INSERT INTO schedule_occurrence_matches (item_id, occurrence_date, transaction_id, source)
VALUES (?, ?, ?, 'manual')
ON CONFLICT (item_id, occurrence_date) DO UPDATE
SET transaction_id = excluded.transaction_id,
    source         = excluded.source,
    updated_at     = CURRENT_TIMESTAMP;

-- name: InsertAutoScheduleOccurrenceMatch :exec
-- Resolution's decision about an occurrence nothing has been decided about yet.
-- DO NOTHING rather than DO UPDATE: an existing decision is never overwritten
-- here, whatever its source.
INSERT INTO schedule_occurrence_matches (item_id, occurrence_date, transaction_id, source)
VALUES (?, ?, ?, 'auto')
ON CONFLICT (item_id, occurrence_date) DO NOTHING;

-- name: UpdateAutoScheduleOccurrenceMatch :exec
-- Supersedes an earlier *automatic* decision. The source = 'auto' guard is what
-- makes "a manual decision is never overwritten by an automatic one" a property
-- of the write rather than a rule each caller has to remember. An earlier
-- automatic decision is freely replaced, because resolution re-resolves its
-- window from scratch every pass and the later candidate is better informed.
UPDATE schedule_occurrence_matches
SET transaction_id = ?,
    updated_at     = CURRENT_TIMESTAMP
WHERE item_id = ? AND occurrence_date = ? AND source = 'auto';

-- name: ListScheduleOccurrenceMatchesInRange :many
-- Every decision recorded for an occurrence falling in the window, inclusive.
-- Dates compare as text because they are stored as 'YYYY-MM-DD', which sorts
-- chronologically.
SELECT item_id, occurrence_date, transaction_id, source
FROM schedule_occurrence_matches
WHERE occurrence_date BETWEEN ? AND ?
ORDER BY item_id, occurrence_date;

-- name: DeleteScheduleOccurrenceMatchesForItem :exec
-- Declared FKs are not enforced on this connection, so a deleted item's
-- decisions are removed explicitly rather than left to the cascade.
DELETE FROM schedule_occurrence_matches WHERE item_id = ?;
