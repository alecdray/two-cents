-- +goose Up
-- +goose StatementBegin
-- Initial migration. Establishes the migration chain; domain tables
-- (accounts, transactions, categories, rules, budgets, ...) are added by
-- their owning modules in subsequent migrations.
SELECT 1;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
SELECT 1;
-- +goose StatementEnd
