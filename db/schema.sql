CREATE TABLE goose_db_version (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		version_id INTEGER NOT NULL,
		is_applied INTEGER NOT NULL,
		tstamp TIMESTAMP DEFAULT (datetime('now'))
	);
CREATE TABLE sqlite_sequence(name,seq);
CREATE TABLE connections (
    id           TEXT PRIMARY KEY,
    item_id      TEXT NOT NULL,
    access_token TEXT NOT NULL,
    state        TEXT NOT NULL CHECK (state IN ('active', 'needs_reconnect', 'disconnected')),
    created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS "accounts" (
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
CREATE INDEX idx_accounts_connection_id ON accounts (connection_id);
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
, classification TEXT NOT NULL DEFAULT '', category_id TEXT REFERENCES categories (id), categorization_overridden INTEGER NOT NULL DEFAULT 0);
CREATE INDEX idx_transactions_date ON transactions (date DESC);
CREATE INDEX idx_transactions_account_id ON transactions (account_id);
CREATE TABLE transaction_sync_state (
    connection_id TEXT PRIMARY KEY REFERENCES connections (id),
    cursor        TEXT NOT NULL DEFAULT '',
    updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE categories (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    builtin    INTEGER NOT NULL DEFAULT 0,
    archived   INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE rules (
    id                 TEXT PRIMARY KEY,
    merchant_substring TEXT NOT NULL,
    classification     TEXT NOT NULL CHECK (classification IN ('income', 'spending', 'transfer')),
    category_id        TEXT REFERENCES categories (id),
    created_at         TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at         TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_rules_updated_at ON rules (updated_at DESC);
