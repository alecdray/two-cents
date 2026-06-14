-- name: UpsertTransaction :exec
INSERT INTO transactions (
    id,
    account_id,
    date,
    amount_amount,
    amount_currency,
    merchant,
    counterparty,
    category_primary,
    category_detailed,
    status
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
)
ON CONFLICT (id) DO UPDATE SET
    account_id        = excluded.account_id,
    date              = excluded.date,
    amount_amount     = excluded.amount_amount,
    amount_currency   = excluded.amount_currency,
    merchant          = excluded.merchant,
    counterparty      = excluded.counterparty,
    category_primary  = excluded.category_primary,
    category_detailed = excluded.category_detailed,
    status            = excluded.status,
    updated_at        = CURRENT_TIMESTAMP;

-- name: DeleteTransaction :exec
DELETE FROM transactions
WHERE id = ?;

-- name: ListRecentTransactions :many
SELECT sqlc.embed(t), a.name AS account_name
FROM transactions t
JOIN accounts a ON a.id = t.account_id
ORDER BY t.date DESC, t.id DESC
LIMIT ?;

-- name: GetSyncCursor :one
SELECT cursor FROM transaction_sync_state
WHERE connection_id = ?;

-- name: UpsertSyncCursor :exec
INSERT INTO transaction_sync_state (
    connection_id, cursor
) VALUES (
    ?, ?
)
ON CONFLICT (connection_id) DO UPDATE SET
    cursor     = excluded.cursor,
    updated_at = CURRENT_TIMESTAMP;
