-- +goose Up
-- Repoint stored transaction dates at the authorized date.
--
-- The Plaid adapter used to fill `transactions.date` from Plaid's `date`, which
-- on a posted transaction is the date it *posted*, not the transaction date the
-- domain buckets months by. `transactionDate` in src/internal/plaid/entities.go
-- carries the field semantics and why `authorized_date` is the right source;
-- rows synced before that fix carry the posted date.
--
-- Why a backfill is needed at all: Plaid stops reporting a posted transaction as
-- `modified` once it settles, so the adapter fix alone never reaches these rows —
-- they would keep their posted date indefinitely and their months would never
-- reconcile. Rows with no `authorized_date` are already correct and stay untouched.

-- +goose StatementBegin
UPDATE transactions
SET    date       = authorized_date,
       updated_at = CURRENT_TIMESTAMP
WHERE  authorized_date IS NOT NULL
  AND  date(authorized_date) <> date(date);
-- +goose StatementEnd

-- +goose Down
-- Irreversible by design: Plaid's posted calendar date was only ever stored in
-- `date`, so overwriting it above discards it and nothing here can reconstruct
-- it. Down is a no-op rather than a lie — rolling back leaves the corrected
-- dates in place, which are the dates the schema asks for either way.
-- +goose StatementBegin
SELECT 1;
-- +goose StatementEnd
