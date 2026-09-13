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
, mask TEXT NOT NULL DEFAULT '', custom_name TEXT, statement_balance REAL, statement_issued_at TIMESTAMP, statement_due_at TIMESTAMP, payment_schedule_mode TEXT NOT NULL DEFAULT 'due_date'
    CHECK (payment_schedule_mode IN ('due_date', 'statement_plus_days')), payment_schedule_offset_days INTEGER NOT NULL DEFAULT 0, statements_unavailable INTEGER NOT NULL DEFAULT 0);
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
, classification TEXT NOT NULL DEFAULT '', category_id TEXT REFERENCES categories (id), categorization_overridden INTEGER NOT NULL DEFAULT 0, transfer_destination_account_id TEXT REFERENCES accounts (id), transfer_subtype TEXT NOT NULL DEFAULT '', transfer_destination_overridden INTEGER NOT NULL DEFAULT 0, description TEXT NOT NULL DEFAULT '', merchant_entity_id TEXT NOT NULL DEFAULT '', logo_url TEXT NOT NULL DEFAULT '', website TEXT NOT NULL DEFAULT '', payment_channel TEXT NOT NULL DEFAULT '', category_confidence TEXT NOT NULL DEFAULT '', authorized_date TIMESTAMP, datetime TIMESTAMP, authorized_datetime TIMESTAMP, counterparties TEXT NOT NULL DEFAULT '[]');
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
CREATE TABLE budget (
    id            TEXT PRIMARY KEY,
    income_target REAL NOT NULL DEFAULT 0,
    savings_target REAL NOT NULL DEFAULT 0,
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE budget_category_limits (
    category_id  TEXT PRIMARY KEY REFERENCES categories(id),
    limit_amount REAL NOT NULL
);
CREATE TABLE users (
    id            TEXT PRIMARY KEY,
    username      TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE merchant_logo_cache (
    logo_key     TEXT PRIMARY KEY,
    logo_url     TEXT NOT NULL,
    content_type TEXT NOT NULL DEFAULT '',
    image_bytes  BLOB,
    created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
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
