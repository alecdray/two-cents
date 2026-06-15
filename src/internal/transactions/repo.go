package transactions

import (
	"context"
	"database/sql"
	"errors"

	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/categorization"
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
	return recentFrom(r.Transaction, r.AccountName, r.CategoryName)
}

// recentFrom builds the recent-activity read model from a stored transaction row
// and its joined account / category names. The category name is empty when the
// row carries no Category (the LEFT JOIN missed).
func recentFrom(t sqlc.Transaction, accountName string, categoryName sql.NullString) RecentTransaction {
	rt := RecentTransaction{
		ID:          t.ID,
		AccountName: accountName,
		Date:        t.Date,
		Amount: banking.Money{
			Amount:   t.AmountAmount,
			Currency: t.AmountCurrency,
		},
		Merchant:       t.Merchant,
		Pending:        Status(t.Status) == StatusPending,
		Classification: categorization.Classification(t.Classification),
	}
	if t.CategoryID.Valid {
		id := t.CategoryID.String
		rt.CategoryID = &id
	}
	if categoryName.Valid {
		rt.CategoryName = categoryName.String
	}
	return rt
}

// categorizationRowFrom maps a sqlc categorization-input row onto the module's
// internal categorizationRow.
func categorizationRowFrom(id, merchant, counterparty, categoryPrimary, categoryDetailed, amountCurrency, classification string, categoryID sql.NullString, amount float64, overridden int64) categorizationRow {
	row := categorizationRow{
		ID:             id,
		Merchant:       merchant,
		Counterparty:   counterparty,
		Category:       banking.Category{Primary: categoryPrimary, Detailed: categoryDetailed},
		Amount:         banking.Money{Amount: amount, Currency: amountCurrency},
		Classification: categorization.Classification(classification),
		Overridden:     overridden != 0,
	}
	if categoryID.Valid {
		id := categoryID.String
		row.CategoryID = &id
	}
	return row
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

// GetRecentTransaction returns a single transaction as the recent-activity read
// model, joined to its account and Category names — the per-row read the
// re-categorize handler re-renders after a mutation.
func (r *Repo) GetRecentTransaction(ctx context.Context, id string) (RecentTransaction, error) {
	row, err := r.q.GetRecentTransaction(ctx, id)
	if err != nil {
		return RecentTransaction{}, err
	}
	return recentFrom(row.Transaction, row.AccountName, row.CategoryName), nil
}

// --- Categorization queries ---

// GetCategorizationRow returns the categorization inputs and current facet for
// one transaction.
func (r *Repo) GetCategorizationRow(ctx context.Context, id string) (categorizationRow, error) {
	m, err := r.q.GetTransactionForCategorization(ctx, id)
	if err != nil {
		return categorizationRow{}, err
	}
	return categorizationRowFrom(m.ID, m.Merchant, m.Counterparty, m.CategoryPrimary, m.CategoryDetailed, m.AmountCurrency, m.Classification, m.CategoryID, m.AmountAmount, m.CategorizationOverridden), nil
}

// ListCategorizationRows returns the categorization inputs and current facet for
// every transaction — the candidate set the rule-change re-categorization pass
// filters and re-resolves.
func (r *Repo) ListCategorizationRows(ctx context.Context) ([]categorizationRow, error) {
	models, err := r.q.ListTransactionsForCategorization(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]categorizationRow, len(models))
	for i, m := range models {
		out[i] = categorizationRowFrom(m.ID, m.Merchant, m.Counterparty, m.CategoryPrimary, m.CategoryDetailed, m.AmountCurrency, m.Classification, m.CategoryID, m.AmountAmount, m.CategorizationOverridden)
	}
	return out, nil
}

// SetCategorization writes the auto-resolved classification + Category for a
// transaction, leaving its override flag untouched.
func (r *Repo) SetCategorization(ctx context.Context, id string, classification categorization.Classification, categoryID *string) error {
	return r.q.SetTransactionCategorization(ctx, sqlc.SetTransactionCategorizationParams{
		Classification: string(classification),
		CategoryID:     nullStringFromPtr(categoryID),
		ID:             id,
	})
}

// OverrideCategorization writes a manual re-categorization and marks the row
// overridden, so it survives re-sync and beats auto-resolution.
func (r *Repo) OverrideCategorization(ctx context.Context, id string, classification categorization.Classification, categoryID *string) error {
	return r.q.OverrideTransactionCategorization(ctx, sqlc.OverrideTransactionCategorizationParams{
		Classification: string(classification),
		CategoryID:     nullStringFromPtr(categoryID),
		ID:             id,
	})
}

// nullStringFromPtr maps an optional string onto a sql.NullString.
func nullStringFromPtr(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
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
