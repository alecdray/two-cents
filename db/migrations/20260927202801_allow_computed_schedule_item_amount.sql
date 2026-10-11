-- +goose Up
-- A declared item's amount may now be left unset entirely: null means the
-- figure is computed live from the item's own matched transactions rather
-- than typed, the same way a monthly item's day_of_month may already be null
-- for "no fixed date" (ADR-0031). SQLite cannot alter a column's NOT NULL in
-- place, so rebuild the table, copy the rows, and restore the primary key.
-- +goose StatementBegin
CREATE TABLE schedule_items_new (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL,
    direction    TEXT NOT NULL CHECK (direction IN ('out', 'in')),
    amount       REAL,
    cadence      TEXT NOT NULL CHECK (cadence IN ('monthly', 'biweekly')),
    day_of_month INTEGER,
    anchor_date  TIMESTAMP,
    active       INTEGER NOT NULL DEFAULT 1,
    created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (
        (cadence = 'monthly'  AND (day_of_month IS NULL OR day_of_month BETWEEN 1 AND 31))
        OR
        (cadence = 'biweekly' AND anchor_date IS NOT NULL)
    )
);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO schedule_items_new SELECT * FROM schedule_items;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE schedule_items;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE schedule_items_new RENAME TO schedule_items;
-- +goose StatementEnd

-- +goose Down
-- Restore NOT NULL by rebuilding again. A row with no declared amount would
-- violate it, so give it back the conservative floor of one cent rather than
-- dropping the item — there is no meaningful "real" amount to reconstruct.
-- +goose StatementBegin
UPDATE schedule_items SET amount = 0.01 WHERE amount IS NULL;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE schedule_items_old (
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
        (cadence = 'monthly'  AND (day_of_month IS NULL OR day_of_month BETWEEN 1 AND 31))
        OR
        (cadence = 'biweekly' AND anchor_date IS NOT NULL)
    )
);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO schedule_items_old SELECT * FROM schedule_items;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE schedule_items;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE schedule_items_old RENAME TO schedule_items;
-- +goose StatementEnd
