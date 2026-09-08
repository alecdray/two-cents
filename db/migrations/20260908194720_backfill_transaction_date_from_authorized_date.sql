-- +goose Up
-- Repoint stored transaction dates at the authorized date.
--
-- `transactions.date` is the transaction date the domain buckets months by
-- (docs/architecture/data-model.md). The Plaid adapter used to fill it from
-- Plaid's `date`, which on a posted transaction is the date it *posted* — days
-- after the purchase, and sometimes in the following month. Rows synced before
-- that fix carry the posted date; `authorized_date` holds the true transaction
-- date for the rows the bank reported one for.
--
-- Rows with no `authorized_date` (ACH, direct deposits, most transfers) are
-- already correct — the bank reports no separate authorization for them, so
-- Plaid's `date` is the transaction date. They are left untouched.
--
-- Without this the fix only reaches transactions the bank happens to resend:
-- Plaid stops reporting a posted transaction as `modified` once it settles, so
-- existing rows would keep their posted date indefinitely and their months would
-- never reconcile.
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
