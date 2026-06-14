package transactions

import (
	"context"
	"database/sql"
	"errors"

	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/core/db/sqlc"
)

// Repo is the transactions module's data access layer. It is the only file in
// package transactions that imports core/db/sqlc; its methods take and return
// this package's domain types (Transaction, RecentTransaction) — never sqlc.*
// shapes.
type Repo struct {
	q *sqlc.Queries
}

// NewRepo binds a Repo to the given Queries. Callers bind to db.Queries() for
// the global handle or to tx.Queries() inside a db.WithTx callback for
// transactional work.
func NewRepo(q *sqlc.Queries) *Repo {
	return &Repo{q: q}
}

// --- conversion helpers (private — only repo.go touches sqlc types) ---

func recentFromRow(r sqlc.ListRecentTransactionsRow) RecentTransaction {
	return RecentTransaction{
		AccountName: r.AccountName,
		Date:        r.Transaction.Date,
		Amount: banking.Money{
			Amount:   r.Transaction.AmountAmount,
			Currency: r.Transaction.AmountCurrency,
		},
		Merchant: r.Transaction.Merchant,
		Pending:  Status(r.Transaction.Status) == StatusPending,
	}
}

// --- Transaction queries ---

// UpsertTransaction inserts a transaction or, when its provider id already
// exists, updates the existing row in place — the keyed-by-provider-id dedupe
// that keeps a re-sync from creating duplicates.
func (r *Repo) UpsertTransaction(ctx context.Context, t Transaction) error {
	return r.q.UpsertTransaction(ctx, sqlc.UpsertTransactionParams{
		ID:               t.ID,
		AccountID:        t.AccountID,
		Date:             t.Date,
		AmountAmount:     t.Amount.Amount,
		AmountCurrency:   t.Amount.Currency,
		Merchant:         t.Merchant,
		Counterparty:     t.Counterparty,
		CategoryPrimary:  t.Category.Primary,
		CategoryDetailed: t.Category.Detailed,
		Status:           string(t.Status),
	})
}

// DeleteTransaction removes a transaction by its provider id — the handling for
// a provider-reported removal (a dropped pending authorization or a removed
// posted transaction).
func (r *Repo) DeleteTransaction(ctx context.Context, id string) error {
	return r.q.DeleteTransaction(ctx, id)
}

// ListRecentTransactions returns up to limit transactions across all accounts,
// most recent first (date desc, then id desc), each carrying its account's
// display name.
func (r *Repo) ListRecentTransactions(ctx context.Context, limit int) ([]RecentTransaction, error) {
	rows, err := r.q.ListRecentTransactions(ctx, int64(limit))
	if err != nil {
		return nil, err
	}
	out := make([]RecentTransaction, len(rows))
	for i, row := range rows {
		out[i] = recentFromRow(row)
	}
	return out, nil
}

// --- Sync cursor queries ---

// GetSyncCursor returns the stored resume cursor for a connection, or the empty
// string when none has been recorded yet (the signal for an initial backfill).
func (r *Repo) GetSyncCursor(ctx context.Context, connectionID string) (string, error) {
	cursor, err := r.q.GetSyncCursor(ctx, connectionID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return cursor, nil
}

// SetSyncCursor records the resume cursor a pull returned for a connection,
// inserting the row on the first sync and updating it thereafter.
func (r *Repo) SetSyncCursor(ctx context.Context, connectionID, cursor string) error {
	return r.q.UpsertSyncCursor(ctx, sqlc.UpsertSyncCursorParams{
		ConnectionID: connectionID,
		Cursor:       cursor,
	})
}
