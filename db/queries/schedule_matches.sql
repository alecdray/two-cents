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

-- name: DeleteScheduleOccurrenceMatch :exec
-- Drops one decision, whatever its source. Resolution uses it for a match whose
-- transaction the ledger no longer holds: the decision was about a fact that is
-- gone, so the occurrence returns to the timeline rather than staying hidden
-- behind a row that no longer exists.
DELETE FROM schedule_occurrence_matches
WHERE item_id = ? AND occurrence_date = ?;

-- name: ListLatestManualScheduleOccurrenceMatches :many
-- The most recent occurrence each item has had settled by hand, which is what
-- teaches that item its merchant. Only a manual decision teaches: an automatic
-- match is evidence the resolver produced, and feeding it back would let one
-- weak match widen the test that made it.
--
-- SQLite resolves the bare transaction_id against the row MAX() selected, which
-- is what makes this one row per item rather than an arbitrary pairing.
SELECT item_id, MAX(occurrence_date) AS occurrence_date, transaction_id
FROM schedule_occurrence_matches
WHERE source = 'manual' AND transaction_id IS NOT NULL
GROUP BY item_id;

-- name: ListSettledScheduleOccurrenceTransactionIDs :many
-- Every transaction already spoken for by a decision. Resolution seeds its
-- claimed set from this so it never offers one row to a second occurrence and
-- learns that from the partial unique index mid-pass.
SELECT transaction_id
FROM schedule_occurrence_matches
WHERE transaction_id IS NOT NULL;
