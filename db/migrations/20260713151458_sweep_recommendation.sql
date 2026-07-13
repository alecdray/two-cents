-- +goose Up
-- +goose StatementBegin
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
    created_at              TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at              TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE sweep_recommendation;
-- +goose StatementEnd
