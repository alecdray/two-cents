-- +goose Up
-- The stored decision about one occurrence of a scheduled item: the transaction
-- that satisfied it, or the user's assertion that nothing did (ADR-0027).
--
-- An occurrence has no row until something is decided about it, and the absence
-- of a row is not a decision — it simply means the occurrence is still placed on
-- the timeline. `transaction_id` is nullable on purpose: a null with a manual
-- source is the user's recorded rejection, which is a *different* fact from no
-- row at all. Without it, clearing a wrong automatic match would last exactly
-- until the next sync re-made it, because resolution re-resolves from scratch.
--
-- `occurrence_date` is a calendar date ('YYYY-MM-DD'), not a timestamp. An
-- occurrence falls on a day, not at an instant, and storing it as text keeps the
-- key from shifting a day when read back in a zone behind UTC.
-- +goose StatementBegin
CREATE TABLE schedule_occurrence_matches (
    item_id         TEXT NOT NULL REFERENCES schedule_items(id) ON DELETE CASCADE,
    occurrence_date TEXT NOT NULL,
    transaction_id  TEXT,
    source          TEXT NOT NULL CHECK (source IN ('manual', 'auto')),
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (item_id, occurrence_date)
);
-- +goose StatementEnd

-- One transaction settles at most one obligation. Partial, because any number of
-- occurrences may be recorded as deliberately unmatched.
-- +goose StatementBegin
CREATE UNIQUE INDEX schedule_occurrence_matches_transaction
    ON schedule_occurrence_matches (transaction_id)
    WHERE transaction_id IS NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE schedule_occurrence_matches;
-- +goose StatementEnd
