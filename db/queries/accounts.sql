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
