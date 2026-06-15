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
-- The categorization columns (classification, category_id, categorization_overridden)
-- are deliberately absent from both the insert column list and the ON CONFLICT
-- update: categorization is owned separately, so a new row takes the column
-- defaults and an existing row keeps whatever categorization it already carries.

-- name: SetTransactionCategorization :exec
-- Write the auto-resolved categorization for a transaction. It never touches the
-- override flag, so a row's sticky facet is preserved; callers pre-skip overridden
-- rows.
UPDATE transactions
SET classification = ?,
    category_id    = ?,
    updated_at     = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: OverrideTransactionCategorization :exec
-- Write a manual re-categorization and mark the row overridden, so it beats
-- auto-resolution and survives re-sync.
UPDATE transactions
SET classification            = ?,
    category_id               = ?,
    categorization_overridden = 1,
    updated_at                = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: GetTransactionForCategorization :one
-- The fields the categorization engine needs to (re-)resolve one transaction,
-- plus its current categorization facet so callers can skip overridden / already
-- categorized rows.
SELECT id,
       merchant,
       counterparty,
       category_primary,
       category_detailed,
       amount_amount,
       amount_currency,
       classification,
       category_id,
       categorization_overridden
FROM transactions
WHERE id = ?;

-- name: ListTransactionsForCategorization :many
-- Every transaction's categorization inputs and current facet, for the
-- rule-change re-categorization pass (which filters and re-resolves in Go).
SELECT id,
       merchant,
       counterparty,
       category_primary,
       category_detailed,
       amount_amount,
       amount_currency,
       classification,
       category_id,
       categorization_overridden
FROM transactions;

-- name: DeleteTransaction :exec
DELETE FROM transactions
WHERE id = ?;

-- name: ListRecentTransactions :many
SELECT sqlc.embed(t), a.name AS account_name, c.name AS category_name
FROM transactions t
JOIN accounts a ON a.id = t.account_id
LEFT JOIN categories c ON c.id = t.category_id
ORDER BY t.date DESC, t.id DESC
LIMIT ?;

-- name: GetRecentTransaction :one
SELECT sqlc.embed(t), a.name AS account_name, c.name AS category_name
FROM transactions t
JOIN accounts a ON a.id = t.account_id
LEFT JOIN categories c ON c.id = t.category_id
WHERE t.id = ?;

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
