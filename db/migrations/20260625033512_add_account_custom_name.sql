-- +goose Up
-- +goose StatementBegin
ALTER TABLE accounts ADD COLUMN custom_name TEXT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE accounts DROP COLUMN custom_name;
-- +goose StatementEnd
