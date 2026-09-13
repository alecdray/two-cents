-- +goose Up
-- Reshape the sweep snapshot around the cash-flow timeline (ADR-0024).
--
-- The reserve model's figures are gone, not renamed: the budget terms, the
-- month-to-date counters and the card balance were terms in an arithmetic that no
-- longer exists. What replaces them is `required_checking` — the highest point the
-- running total reaches over the horizon — and `timeline`, the dated events it was
-- computed from, stored as JSON so a snapshot explains its own arithmetic without
-- recomputing it (ADR-0022 requires the figures to reconstruct from the row alone).
--
-- **Pre-existing snapshots are dropped rather than carried forward.** They were
-- computed under the superseded model and share no figures with this one, so there
-- is nothing to migrate into these columns; rendering two irreconcilable shapes on
-- one navigable timeline costs more than advisory history with no ongoing use is
-- worth.
-- +goose StatementBegin
DROP INDEX idx_sweep_recommendation_computed_at;
DROP TABLE sweep_recommendation;

CREATE TABLE sweep_recommendation (
    id                  TEXT PRIMARY KEY,
    kind                TEXT NOT NULL CHECK (kind IN ('numeric', 'needs_attention')),
    current_checking    REAL,
    current_savings     REAL,
    savings_unknown     INTEGER NOT NULL DEFAULT 0,
    required_checking   REAL NOT NULL DEFAULT 0,
    fixed_safety_margin REAL NOT NULL DEFAULT 0,
    suggested_sweep     REAL NOT NULL DEFAULT 0,
    direction           TEXT NOT NULL DEFAULT '',
    reasons             TEXT NOT NULL DEFAULT '[]',
    timeline            TEXT NOT NULL DEFAULT '[]',
    computed_at         TIMESTAMP NOT NULL,
    created_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_sweep_recommendation_computed_at
    ON sweep_recommendation (computed_at DESC);
-- +goose StatementEnd

-- +goose Down
-- Restore the reserve-model shape, empty: the timeline snapshots have no figures
-- to project back onto it either.
-- +goose StatementBegin
DROP INDEX idx_sweep_recommendation_computed_at;
DROP TABLE sweep_recommendation;

CREATE TABLE sweep_recommendation (
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
    updated_at              TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    card_balance            REAL NOT NULL DEFAULT 0
);

CREATE INDEX idx_sweep_recommendation_computed_at
    ON sweep_recommendation (computed_at DESC);
-- +goose StatementEnd
