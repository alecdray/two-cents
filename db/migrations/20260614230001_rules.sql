-- +goose Up
-- +goose StatementBegin
CREATE TABLE rules (
    id                 TEXT PRIMARY KEY,
    merchant_substring TEXT NOT NULL,
    classification     TEXT NOT NULL CHECK (classification IN ('income', 'spending', 'transfer')),
    category_id        TEXT REFERENCES categories (id),
    created_at         TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at         TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_rules_updated_at ON rules (updated_at DESC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX idx_rules_updated_at;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE rules;
-- +goose StatementEnd
