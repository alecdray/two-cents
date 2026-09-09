-- +goose Up
-- Turn the single upserted sweep recommendation into an append-only history of
-- point-in-time snapshots (ADR-0022).
--
-- The table already keys on `id`; what changes is that `id` stops being the fixed
-- literal 'default' and becomes a generated per-snapshot key, and rows are
-- inserted rather than upserted. The schema change that needs is an explicit
-- `computed_at`: the instant the run measured against, which is both the ordering
-- key and the on-screen label. It cannot keep deriving from `updated_at`, because
-- that is the moment the row was written — close enough when one row was
-- overwritten monthly, wrong once a label has to match the month-to-date window
-- its snapshot measured, and once several snapshots can land in one day.
--
-- The table is rebuilt rather than ALTERed because `computed_at` must be NOT NULL
-- and SQLite cannot add a NOT NULL column with a non-constant default. At most one
-- row exists here (the old 'default' upsert), so the copy carries it forward as the
-- first historical snapshot with the best instant we have for it — its write time.
-- Its id stays 'default': the value is opaque from here on, and inventing a new one
-- would gain nothing.
-- +goose StatementBegin
CREATE TABLE sweep_recommendation_new (
    id                      TEXT PRIMARY KEY,
    kind                    TEXT NOT NULL CHECK (kind IN ('numeric', 'needs_attention')),
    current_checking        REAL,
    current_savings         REAL,
    savings_unknown         INTEGER NOT NULL DEFAULT 0,
    total_spending_budget   REAL NOT NULL DEFAULT 0,
    mtd_spending            REAL NOT NULL DEFAULT 0,
    savings_target          REAL NOT NULL DEFAULT 0,
    mtd_savings_contributed REAL NOT NULL DEFAULT 0,
    reserve                 REAL NOT NULL DEFAULT 0,
    fixed_safety_margin     REAL NOT NULL DEFAULT 0,
    suggested_sweep         REAL NOT NULL DEFAULT 0,
    direction               TEXT NOT NULL DEFAULT '',
    reasons                 TEXT NOT NULL DEFAULT '[]',
    computed_at             TIMESTAMP NOT NULL,
    created_at              TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at              TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO sweep_recommendation_new (
    id, kind, current_checking, current_savings, savings_unknown,
    total_spending_budget, mtd_spending, savings_target, mtd_savings_contributed,
    reserve, fixed_safety_margin, suggested_sweep, direction, reasons,
    computed_at, created_at, updated_at
)
SELECT
    id, kind, current_checking, current_savings, savings_unknown,
    total_spending_budget, mtd_spending, savings_target, mtd_savings_contributed,
    reserve, fixed_safety_margin, suggested_sweep, direction, reasons,
    updated_at, created_at, updated_at
FROM sweep_recommendation;

DROP TABLE sweep_recommendation;

ALTER TABLE sweep_recommendation_new RENAME TO sweep_recommendation;

CREATE INDEX idx_sweep_recommendation_computed_at
    ON sweep_recommendation (computed_at DESC);
-- +goose StatementEnd

-- +goose Down
-- Collapse back to a single row: keep only the newest snapshot, since the old
-- shape had nowhere to put the rest.
-- +goose StatementBegin
DROP INDEX idx_sweep_recommendation_computed_at;

CREATE TABLE sweep_recommendation_old (
    id                      TEXT PRIMARY KEY,
    kind                    TEXT NOT NULL CHECK (kind IN ('numeric', 'needs_attention')),
    current_checking        REAL,
    current_savings         REAL,
    savings_unknown         INTEGER NOT NULL DEFAULT 0,
    total_spending_budget   REAL NOT NULL DEFAULT 0,
    mtd_spending            REAL NOT NULL DEFAULT 0,
    savings_target          REAL NOT NULL DEFAULT 0,
    mtd_savings_contributed REAL NOT NULL DEFAULT 0,
    reserve                 REAL NOT NULL DEFAULT 0,
    fixed_safety_margin     REAL NOT NULL DEFAULT 0,
    suggested_sweep         REAL NOT NULL DEFAULT 0,
    direction               TEXT NOT NULL DEFAULT '',
    reasons                 TEXT NOT NULL DEFAULT '[]',
    created_at              TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at              TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO sweep_recommendation_old (
    id, kind, current_checking, current_savings, savings_unknown,
    total_spending_budget, mtd_spending, savings_target, mtd_savings_contributed,
    reserve, fixed_safety_margin, suggested_sweep, direction, reasons,
    created_at, updated_at
)
SELECT
    'default', kind, current_checking, current_savings, savings_unknown,
    total_spending_budget, mtd_spending, savings_target, mtd_savings_contributed,
    reserve, fixed_safety_margin, suggested_sweep, direction, reasons,
    created_at, updated_at
FROM sweep_recommendation
ORDER BY computed_at DESC
LIMIT 1;

DROP TABLE sweep_recommendation;

ALTER TABLE sweep_recommendation_old RENAME TO sweep_recommendation;
-- +goose StatementEnd
