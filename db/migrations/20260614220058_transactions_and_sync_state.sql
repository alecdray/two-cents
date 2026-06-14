-- +goose Up
-- +goose StatementBegin
CREATE TABLE transactions (
    id                TEXT PRIMARY KEY,
    account_id        TEXT NOT NULL REFERENCES accounts (id),
    date              TIMESTAMP NOT NULL,
    amount_amount     REAL NOT NULL,
    amount_currency   TEXT NOT NULL,
    merchant          TEXT NOT NULL,
    counterparty      TEXT NOT NULL,
    category_primary  TEXT NOT NULL,
    category_detailed TEXT NOT NULL,
    status            TEXT NOT NULL CHECK (status IN ('pending', 'posted')),
    created_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_transactions_date ON transactions (date DESC);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_transactions_account_id ON transactions (account_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE transaction_sync_state (
    connection_id TEXT PRIMARY KEY REFERENCES connections (id),
    cursor        TEXT NOT NULL DEFAULT '',
    updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE transaction_sync_state;
-- +goose StatementEnd

-- +goose StatementBegin
DROP INDEX idx_transactions_account_id;
-- +goose StatementEnd

-- +goose StatementBegin
DROP INDEX idx_transactions_date;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE transactions;
-- +goose StatementEnd
