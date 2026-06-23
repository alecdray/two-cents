-- +goose Up
-- The read-only bank display detail of a transaction (ADR-0013): the raw bank
-- descriptor, merchant-identity fields (entity id, logo, website), the payment
-- channel, the bank's category confidence, the authorized/posted timestamps, and
-- a denormalized counterparties JSON array. Unlike the override-facet columns,
-- these carry no user state — they are bank-sourced and refreshed by the sync
-- upsert on every pull, so they live IN UpsertTransaction. The timestamps are
-- nullable (the bank often omits them); the rest take empty/'[]' defaults.
-- +goose StatementBegin
ALTER TABLE transactions ADD COLUMN description TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE transactions ADD COLUMN merchant_entity_id TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE transactions ADD COLUMN logo_url TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE transactions ADD COLUMN website TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE transactions ADD COLUMN payment_channel TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE transactions ADD COLUMN category_confidence TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE transactions ADD COLUMN authorized_date TIMESTAMP;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE transactions ADD COLUMN datetime TIMESTAMP;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE transactions ADD COLUMN authorized_datetime TIMESTAMP;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE transactions ADD COLUMN counterparties TEXT NOT NULL DEFAULT '[]';
-- +goose StatementEnd

-- Backfill existing rows: clear every connection's resume cursor so the next sync
-- re-pulls and re-upserts the full set, populating the new columns. Idempotent by
-- DedupeKey — the same self-healing re-pull stance the categorization sweep takes.
-- +goose StatementBegin
DELETE FROM transaction_sync_state;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE transactions DROP COLUMN counterparties;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE transactions DROP COLUMN authorized_datetime;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE transactions DROP COLUMN datetime;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE transactions DROP COLUMN authorized_date;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE transactions DROP COLUMN category_confidence;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE transactions DROP COLUMN payment_channel;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE transactions DROP COLUMN website;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE transactions DROP COLUMN logo_url;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE transactions DROP COLUMN merchant_entity_id;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE transactions DROP COLUMN description;
-- +goose StatementEnd
