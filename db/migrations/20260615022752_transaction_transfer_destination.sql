-- +goose Up
-- The transfer-destination/subtype facet of a transaction, owned and written only
-- by the transactions module (the bank-sync upsert never touches these columns).
-- transfer_destination_account_id is the paired or user-marked destination account
-- (NULL = unknown); transfer_subtype is '' (non-transfer / unresolved),
-- 'savings_contribution', or 'plain', set only on outflow Transfer legs;
-- transfer_destination_overridden is the second sticky manual-override facet,
-- independent of categorization_overridden, that beats auto-pairing and survives
-- re-sync. The FK is nullable and un-cascaded, matching the existing category_id.
-- +goose StatementBegin
ALTER TABLE transactions ADD COLUMN transfer_destination_account_id TEXT REFERENCES accounts (id);
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE transactions ADD COLUMN transfer_subtype TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE transactions ADD COLUMN transfer_destination_overridden INTEGER NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE transactions DROP COLUMN transfer_destination_overridden;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE transactions DROP COLUMN transfer_subtype;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE transactions DROP COLUMN transfer_destination_account_id;
-- +goose StatementEnd
