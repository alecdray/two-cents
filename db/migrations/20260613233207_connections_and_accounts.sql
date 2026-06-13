-- +goose Up
-- +goose StatementBegin
CREATE TABLE connections (
    id           TEXT PRIMARY KEY,
    item_id      TEXT NOT NULL,
    access_token TEXT NOT NULL,
    state        TEXT NOT NULL CHECK (state IN ('active', 'needs_reconnect', 'disconnected')),
    created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE accounts (
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
CREATE INDEX idx_accounts_connection_id ON accounts (connection_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX idx_accounts_connection_id;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE accounts;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE connections;
-- +goose StatementEnd
