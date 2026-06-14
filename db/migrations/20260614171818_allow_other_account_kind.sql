-- +goose Up
-- Widen the accounts.kind CHECK to admit the 'other' bucket (loans,
-- investments, …) alongside cash and credit. SQLite cannot alter a CHECK
-- constraint in place, so rebuild the table, copy the rows, and restore the
-- index.
-- +goose StatementBegin
CREATE TABLE accounts_new (
    id                  TEXT PRIMARY KEY,
    connection_id       TEXT NOT NULL REFERENCES connections (id),
    provider_account_id TEXT NOT NULL,
    name                TEXT NOT NULL,
    bank_type           TEXT NOT NULL,
    kind                TEXT NOT NULL CHECK (kind IN ('cash', 'credit', 'other')),
    kind_overridden     INTEGER NOT NULL DEFAULT 0,
    counts_as_savings   INTEGER NOT NULL DEFAULT 0,
    savings_overridden  INTEGER NOT NULL DEFAULT 0,
    balance_amount      REAL NOT NULL DEFAULT 0,
    balance_currency    TEXT NOT NULL DEFAULT '',
    balance_known       INTEGER NOT NULL DEFAULT 0,
    state               TEXT NOT NULL CHECK (state IN ('active', 'hidden', 'closed')),
    last_synced_at      TIMESTAMP,
    created_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO accounts_new SELECT * FROM accounts;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE accounts;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE accounts_new RENAME TO accounts;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_accounts_connection_id ON accounts (connection_id);
-- +goose StatementEnd

-- +goose Down
-- Restore the narrower cash/credit CHECK by rebuilding again. Rows whose kind
-- is 'other' would violate the restored constraint, so drop them first.
-- +goose StatementBegin
DELETE FROM accounts WHERE kind = 'other';
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE accounts_old (
    id                  TEXT PRIMARY KEY,
    connection_id       TEXT NOT NULL REFERENCES connections (id),
    provider_account_id TEXT NOT NULL,
    name                TEXT NOT NULL,
    bank_type           TEXT NOT NULL,
    kind                TEXT NOT NULL CHECK (kind IN ('cash', 'credit')),
    kind_overridden     INTEGER NOT NULL DEFAULT 0,
    counts_as_savings   INTEGER NOT NULL DEFAULT 0,
    savings_overridden  INTEGER NOT NULL DEFAULT 0,
    balance_amount      REAL NOT NULL DEFAULT 0,
    balance_currency    TEXT NOT NULL DEFAULT '',
    balance_known       INTEGER NOT NULL DEFAULT 0,
    state               TEXT NOT NULL CHECK (state IN ('active', 'hidden', 'closed')),
    last_synced_at      TIMESTAMP,
    created_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO accounts_old SELECT * FROM accounts;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE accounts;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE accounts_old RENAME TO accounts;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_accounts_connection_id ON accounts (connection_id);
-- +goose StatementEnd
