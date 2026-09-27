-- +goose Up
-- A monthly scheduled item may now declare no day_of_month at all: dateless,
-- for a bill paid only when it arrives rather than on a fixed date (ADR-0031).
-- Biweekly is unchanged and still requires anchor_date. SQLite cannot alter a
-- CHECK constraint in place, so rebuild the table, copy the rows, and restore
-- the primary key.
-- +goose StatementBegin
CREATE TABLE schedule_items_new (
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
INSERT INTO schedule_items_new SELECT * FROM schedule_items;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE schedule_items;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE schedule_items_new RENAME TO schedule_items;
-- +goose StatementEnd

-- +goose Down
-- Restore the narrower CHECK by rebuilding again. A dateless row would violate
-- it, so give it back the smallest valid day rather than dropping the item.
-- +goose StatementBegin
UPDATE schedule_items SET day_of_month = 1 WHERE cadence = 'monthly' AND day_of_month IS NULL;
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
        (cadence = 'monthly'  AND day_of_month IS NOT NULL AND day_of_month BETWEEN 1 AND 31)
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
