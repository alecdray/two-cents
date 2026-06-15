-- +goose Up
-- +goose StatementBegin
CREATE TABLE budget (
    id            TEXT PRIMARY KEY,
    income_target REAL NOT NULL DEFAULT 0,
    savings_target REAL NOT NULL DEFAULT 0,
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE budget_category_limits (
    category_id  TEXT PRIMARY KEY REFERENCES categories(id),
    limit_amount REAL NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE budget_category_limits;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE budget;
-- +goose StatementEnd
