-- +goose Up
-- The internal categorization facet of a transaction, owned and written only by
-- the transactions module (the bank-sync upsert never touches these columns).
-- classification is '' transiently until auto-categorization resolves it;
-- category_id is set only for a spending classification; categorization_overridden
-- is the sticky manual-override facet that beats auto-resolution and survives
-- re-sync.
-- +goose StatementBegin
ALTER TABLE transactions ADD COLUMN classification TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE transactions ADD COLUMN category_id TEXT REFERENCES categories (id);
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE transactions ADD COLUMN categorization_overridden INTEGER NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE transactions DROP COLUMN categorization_overridden;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE transactions DROP COLUMN category_id;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE transactions DROP COLUMN classification;
-- +goose StatementEnd
