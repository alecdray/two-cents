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
SELECT sqlc.embed(t), a.name AS account_name, c.name AS category_name, da.name AS destination_account_name
FROM transactions t
JOIN accounts a ON a.id = t.account_id
LEFT JOIN categories c ON c.id = t.category_id
LEFT JOIN accounts da ON da.id = t.transfer_destination_account_id
ORDER BY t.date DESC, t.id DESC
LIMIT ?;

-- name: GetRecentTransaction :one
SELECT sqlc.embed(t), a.name AS account_name, c.name AS category_name, da.name AS destination_account_name
FROM transactions t
JOIN accounts a ON a.id = t.account_id
LEFT JOIN categories c ON c.id = t.category_id
LEFT JOIN accounts da ON da.id = t.transfer_destination_account_id
WHERE t.id = ?;

-- name: ListSpendingTransactionsInRange :many
-- The Spending transactions whose date falls in [start, end), newest-first, with
-- the same display joins as ListRecentTransactions. Scoping to one month's
-- Spending is exactly the set the wrap's spend-by-Category aggregates, so the
-- drill-down list reconciles to the figure it was reached from.
SELECT sqlc.embed(t), a.name AS account_name, c.name AS category_name, da.name AS destination_account_name
FROM transactions t
JOIN accounts a ON a.id = t.account_id
LEFT JOIN categories c ON c.id = t.category_id
LEFT JOIN accounts da ON da.id = t.transfer_destination_account_id
WHERE t.classification = 'spending' AND t.date >= ? AND t.date < ?
ORDER BY t.date DESC, t.id DESC;

-- name: TransactionsInRange :many
SELECT id,
       date,
       amount_amount,
       amount_currency,
       classification,
       category_id,
       transfer_subtype,
       status
FROM transactions
WHERE date >= ? AND date < ?
ORDER BY date, id;

-- name: EarliestTransactionDate :one
SELECT date FROM transactions
ORDER BY date ASC, id ASC
LIMIT 1;

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

-- name: SetTransactionTransferDestination :exec
-- Write the auto-paired transfer destination + subtype for a transaction. It
-- never touches the override flag, so a row's sticky transfer facet is preserved;
-- callers pre-skip overridden rows.
UPDATE transactions
SET transfer_destination_account_id = ?,
    transfer_subtype                = ?,
    updated_at                      = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: OverrideTransactionTransferDestination :exec
-- Write a manual transfer-destination choice and mark the row's transfer facet
-- overridden, so it beats auto-pairing and survives re-sync. Independent of the
-- categorization override.
UPDATE transactions
SET transfer_destination_account_id = ?,
    transfer_subtype                = ?,
    transfer_destination_overridden = 1,
    updated_at                      = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: GetTransactionTransferDestination :one
-- The stored transfer facet of one transaction: the destination account, the
-- subtype, and the override flag.
SELECT transfer_destination_account_id,
       transfer_subtype,
       transfer_destination_overridden
FROM transactions
WHERE id = ?;

-- name: ListTransferLegs :many
-- Every stored Transfer leg on a connected (non-closed) account, with the fields
-- the auto-pairing pass needs: the caller filters outflow source legs and the
-- inflow candidates from this set, pairs them by amount + date window, and skips
-- legs whose transfer facet is overridden.
SELECT t.id,
       t.account_id,
       t.amount_amount,
       t.date,
       t.classification,
       t.transfer_destination_overridden
FROM transactions t
JOIN accounts a ON a.id = t.account_id
WHERE t.classification = 'transfer'
  AND a.state != 'closed';
