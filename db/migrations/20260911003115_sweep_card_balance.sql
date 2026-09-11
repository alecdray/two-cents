-- +goose Up
-- Record the card balance a sweep snapshot reserved against (ADR-0023).
--
-- The reserve gains an uncovered-card-debt term, and a snapshot has to carry every
-- figure that produced its number — the breakdown reconstructs from the snapshot
-- alone, never from what the cards happen to say when it is read back.
--
-- Existing snapshots default to 0, which is what they were computed with: they
-- predate the term, so nothing about their arithmetic is misrepresented.
-- +goose StatementBegin
ALTER TABLE sweep_recommendation ADD COLUMN card_balance REAL NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE sweep_recommendation DROP COLUMN card_balance;
-- +goose StatementEnd
