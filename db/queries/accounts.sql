-- name: CreateAccount :one
INSERT INTO accounts (
    id,
    connection_id,
    provider_account_id,
    name,
    bank_type,
    mask,
    kind,
    kind_overridden,
    counts_as_savings,
    savings_overridden,
    balance_amount,
    balance_currency,
    balance_known,
    state,
    last_synced_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
)
RETURNING *;

-- name: GetAccount :one
SELECT * FROM accounts
WHERE id = ?;

-- name: ListAccounts :many
SELECT * FROM accounts
ORDER BY created_at;

-- name: ListAccountsByConnection :many
SELECT * FROM accounts
WHERE connection_id = ?
ORDER BY created_at;

-- name: DeleteAccountsByConnection :exec
DELETE FROM accounts
WHERE connection_id = ?;

-- name: UpdateAccount :one
UPDATE accounts
SET name               = ?,
    bank_type          = ?,
    mask               = ?,
    kind               = ?,
    kind_overridden    = ?,
    counts_as_savings  = ?,
    savings_overridden = ?,
    balance_amount     = ?,
    balance_currency   = ?,
    balance_known      = ?,
    state              = ?,
    last_synced_at     = ?,
    updated_at         = CURRENT_TIMESTAMP
WHERE id = ?
RETURNING *;

-- name: UpdateAccountCustomName :one
UPDATE accounts
SET custom_name = ?,
    updated_at  = CURRENT_TIMESTAMP
WHERE id = ?
RETURNING *;

-- name: UpdateAccountStatement :one
-- Sync-owned: the billing-cycle facts the bank reported. Deliberately separate
-- from UpdateAccount so a sync can never write the user's payment schedule, and
-- from the schedule update so a user edit can never write statement figures.
UPDATE accounts
SET statement_balance   = ?,
    statement_issued_at = ?,
    statement_due_at    = ?,
    updated_at          = CURRENT_TIMESTAMP
WHERE id = ?
RETURNING *;

-- name: UpdateAccountPaymentSchedule :one
-- User-owned: when this card is paid. Sync never calls this.
UPDATE accounts
SET payment_schedule_mode        = ?,
    payment_schedule_offset_days = ?,
    updated_at                   = CURRENT_TIMESTAMP
WHERE id = ?
RETURNING *;
