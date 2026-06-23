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
    status,
    description,
    merchant_entity_id,
    logo_url,
    website,
    payment_channel,
    category_confidence,
    authorized_date,
    datetime,
    authorized_datetime,
    counterparties
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
)
ON CONFLICT (id) DO UPDATE SET
    account_id          = excluded.account_id,
    date                = excluded.date,
    amount_amount       = excluded.amount_amount,
    amount_currency     = excluded.amount_currency,
    merchant            = excluded.merchant,
    counterparty        = excluded.counterparty,
    category_primary    = excluded.category_primary,
    category_detailed   = excluded.category_detailed,
    status              = excluded.status,
    description         = excluded.description,
    merchant_entity_id  = excluded.merchant_entity_id,
    logo_url            = excluded.logo_url,
    website             = excluded.website,
    payment_channel     = excluded.payment_channel,
    category_confidence = excluded.category_confidence,
    authorized_date     = excluded.authorized_date,
    datetime            = excluded.datetime,
    authorized_datetime = excluded.authorized_datetime,
    counterparties      = excluded.counterparties,
    updated_at          = CURRENT_TIMESTAMP;
-- The bank display-detail columns ARE refreshed here (description, merchant_entity_id,
-- logo_url, website, payment_channel, category_confidence, the authorized/posted
-- timestamps, and the counterparties JSON): they are bank-sourced and carry no user
-- state, so every sync rewrites them (ADR-0013). The categorization and transfer-facet
-- columns (classification, category_id, categorization_overridden, transfer_*) stay
-- deliberately absent: those are owned separately, so a new row takes the column
-- defaults and an existing row keeps whatever facet it already carries. Keep the two
-- groups straight: facet columns out, display-detail columns in.

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
-- auto-resolution and survives re-sync. Moving the row OFF Transfer also clears
-- its transfer facet (destination, subtype, and the transfer override flag back to
-- their defaults): Reporting counts a savings contribution by its subtype alone,
-- outside the classification switch, so a stale subtype on a now non-Transfer row
-- would double-count the move as both savings and spending. A Transfer to Transfer
-- re-categorize leaves the transfer facet untouched.
UPDATE transactions
SET classification            = @classification,
    category_id               = @category_id,
    categorization_overridden = 1,
    transfer_destination_account_id = CASE WHEN @classification = 'transfer' THEN transfer_destination_account_id ELSE NULL END,
    transfer_subtype                = CASE WHEN @classification = 'transfer' THEN transfer_subtype ELSE '' END,
    transfer_destination_overridden = CASE WHEN @classification = 'transfer' THEN transfer_destination_overridden ELSE 0 END,
    updated_at                = CURRENT_TIMESTAMP
WHERE id = @id;

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

-- name: ListUncategorizedForCategorization :many
-- The categorization inputs for every transaction still at the uncategorized
-- default (classification = '') and not manually overridden. This is the
-- self-healing sweep's candidate set: rows a prior sync left uncategorized (synced
-- before categorization ran, or after a categorize error that still advanced the
-- cursor) that a later sync must resolve even though they are not in the current
-- pull's delta. Categorized rows (any non-empty classification) are out of scope.
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
WHERE classification = '' AND categorization_overridden = 0;

-- name: DeleteTransaction :exec
DELETE FROM transactions
WHERE id = ?;

-- name: ListRecentTransactions :many
SELECT sqlc.embed(t), a.name AS account_name, a.mask AS account_mask, c.name AS category_name, da.name AS destination_account_name
FROM transactions t
JOIN accounts a ON a.id = t.account_id
LEFT JOIN categories c ON c.id = t.category_id
LEFT JOIN accounts da ON da.id = t.transfer_destination_account_id
ORDER BY t.date DESC, t.id DESC
LIMIT ?;

-- name: GetRecentTransaction :one
SELECT sqlc.embed(t), a.name AS account_name, a.mask AS account_mask, c.name AS category_name, da.name AS destination_account_name
FROM transactions t
JOIN accounts a ON a.id = t.account_id
LEFT JOIN categories c ON c.id = t.category_id
LEFT JOIN accounts da ON da.id = t.transfer_destination_account_id
WHERE t.id = ?;

-- name: ListTransactionsFiltered :many
-- The full-history filtered activity read backing the /transactions view search
-- and needs-attention filter. Unlike ListRecentTransactions it applies no recent
-- cap - an active filter sees the whole history. Both filters are optional and
-- compose: a NULL merchant skips the merchant match; a zero needs_attention skips
-- the needs-attention predicate. needs-attention is the union of an unresolved
-- inflow (needs_review), uncategorized Spending (spending with no Category), and
-- an unknown-destination outflow Transfer - the unknown predicate mirrors the
-- recentFrom TransferDestinationUnknown rule exactly (see docs/domain/README.md).
-- Same display joins and ordering as ListRecentTransactions.
SELECT sqlc.embed(t), a.name AS account_name, a.mask AS account_mask, c.name AS category_name, da.name AS destination_account_name
FROM transactions t
JOIN accounts a ON a.id = t.account_id
LEFT JOIN categories c ON c.id = t.category_id
LEFT JOIN accounts da ON da.id = t.transfer_destination_account_id
WHERE (sqlc.narg('merchant') IS NULL OR t.merchant LIKE '%' || sqlc.narg('merchant') || '%')
  AND (
    CAST(sqlc.arg('needs_attention') AS INTEGER) = 0
    OR t.classification = 'needs_review'
    OR (t.classification = 'spending' AND t.category_id IS NULL)
    OR (t.classification = 'transfer'
        AND t.amount_amount > 0
        AND t.transfer_destination_account_id IS NULL
        AND t.transfer_destination_overridden = 0)
  )
ORDER BY t.date DESC, t.id DESC;

-- name: ListSpendingTransactionsInRange :many
-- The Spending transactions whose date falls in [start, end), newest-first, with
-- the same display joins as ListRecentTransactions. Scoping to one month's
-- Spending is exactly the set the wrap's spend-by-Category aggregates, so the
-- drill-down list reconciles to the figure it was reached from.
SELECT sqlc.embed(t), a.name AS account_name, a.mask AS account_mask, c.name AS category_name, da.name AS destination_account_name
FROM transactions t
JOIN accounts a ON a.id = t.account_id
LEFT JOIN categories c ON c.id = t.category_id
LEFT JOIN accounts da ON da.id = t.transfer_destination_account_id
WHERE t.classification = 'spending' AND t.date >= ? AND t.date < ?
ORDER BY t.date DESC, t.id DESC;

-- name: ListIncomeTransactionsInRange :many
-- The Income legs whose date falls in [start, end), newest-first, with the same
-- display joins as ListSpendingTransactionsInRange. This is exactly the set the
-- wrap's gross-income figure sums, so the income drill-down reconciles to it.
SELECT sqlc.embed(t), a.name AS account_name, a.mask AS account_mask, c.name AS category_name, da.name AS destination_account_name
FROM transactions t
JOIN accounts a ON a.id = t.account_id
LEFT JOIN categories c ON c.id = t.category_id
LEFT JOIN accounts da ON da.id = t.transfer_destination_account_id
WHERE t.classification = 'income' AND t.date >= ? AND t.date < ?
ORDER BY t.date DESC, t.id DESC;

-- name: ListSavingsContributionsInRange :many
-- The savings-contribution source legs whose date falls in [start, end),
-- newest-first, with the same display joins. This is exactly the set the wrap's
-- savings-contributed figure sums (source leg only; the mirror inflow carries a
-- different subtype), so the savings drill-down reconciles to it.
SELECT sqlc.embed(t), a.name AS account_name, a.mask AS account_mask, c.name AS category_name, da.name AS destination_account_name
FROM transactions t
JOIN accounts a ON a.id = t.account_id
LEFT JOIN categories c ON c.id = t.category_id
LEFT JOIN accounts da ON da.id = t.transfer_destination_account_id
WHERE t.transfer_subtype = 'savings_contribution' AND t.date >= ? AND t.date < ?
ORDER BY t.date DESC, t.id DESC;

-- name: ListAllTransactionsInRange :many
-- Every transaction (any classification) whose date falls in [start, end),
-- newest-first, with the same display joins as ListSpendingTransactionsInRange.
-- Backs the wrap's inline full-month list, which spans the whole month's activity
-- and is not a reconciling figure.
SELECT sqlc.embed(t), a.name AS account_name, a.mask AS account_mask, c.name AS category_name, da.name AS destination_account_name
FROM transactions t
JOIN accounts a ON a.id = t.account_id
LEFT JOIN categories c ON c.id = t.category_id
LEFT JOIN accounts da ON da.id = t.transfer_destination_account_id
WHERE t.date >= ? AND t.date < ?
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
