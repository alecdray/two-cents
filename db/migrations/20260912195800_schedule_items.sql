-- +goose Up
-- The sweep schedule: the user-declared dated checking activity the cash-flow
-- timeline is built from (ADR-0024).
--
-- `amount` is the *conservative* figure — the maximum expected for an outflow, the
-- minimum for an inflow — so the column is named for its meaning rather than its
-- arithmetic. A column called `max_amount` would invite someone to later "fix"
-- income to use an average, which is the one direction that makes the sweep unsafe.
--
-- `day_of_month` and `anchor_date` are each nullable because only one applies:
-- monthly items carry the day, biweekly ones the anchor. A CHECK enforces the
-- pairing so a row can never describe a cadence it has no date for.
-- +goose StatementBegin
CREATE TABLE schedule_items (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL,
    direction    TEXT NOT NULL CHECK (direction IN ('out', 'in')),
    amount       REAL NOT NULL,
    cadence      TEXT NOT NULL CHECK (cadence IN ('monthly', 'biweekly')),
    day_of_month INTEGER,
    anchor_date  TIMESTAMP,
    active       INTEGER NOT NULL DEFAULT 1,
    created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (
        (cadence = 'monthly'  AND day_of_month IS NOT NULL AND day_of_month BETWEEN 1 AND 31)
        OR
        (cadence = 'biweekly' AND anchor_date IS NOT NULL)
    )
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE schedule_items;
-- +goose StatementEnd
