-- +goose Up
-- +goose StatementBegin
ALTER TABLE accounts ADD COLUMN mask TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE accounts DROP COLUMN mask;
-- +goose StatementEnd
