package transactions

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

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
	return recentFrom(r.Transaction, r.AccountMask, r.CategoryName)
}

// recentFrom builds the recent-activity read model from a stored transaction row
// and its joined account mask and category name (empty when the category LEFT JOIN
// missed). It carries the source and destination account *ids*; the display names
// those ids resolve to are filled by the service through the accounts module, which
// owns the display-name precedence (ADR-0017) — the names are deliberately not
// joined in SQL, so the naming policy stays in one place (see data-model.md).
func recentFrom(t sqlc.Transaction, accountMask string, categoryName sql.NullString) RecentTransaction {
	rt := RecentTransaction{
		ID:          t.ID,
		AccountID:   t.AccountID,
		AccountMask: accountMask,
		Date:        t.Date,
		Amount: banking.Money{
			Amount:   t.AmountAmount,
			Currency: t.AmountCurrency,
		},
		Merchant:                      t.Merchant,
		Counterparty:                  t.Counterparty,
		CategoryPrimary:               t.CategoryPrimary,
		CategoryDetailed:              t.CategoryDetailed,
		Description:                   t.Description,
		MerchantEntityID:              t.MerchantEntityID,
		LogoURL:                       t.LogoUrl,
		Website:                       t.Website,
		PaymentChannel:                t.PaymentChannel,
		CategoryConfidence:            t.CategoryConfidence,
		AuthorizedDate:                nullTimeToPtr(t.AuthorizedDate),
		Datetime:                      nullTimeToPtr(t.Datetime),
		AuthorizedDatetime:            nullTimeToPtr(t.AuthorizedDatetime),
		Counterparties:                unmarshalCounterparties(t.Counterparties),
		Pending:                       Status(t.Status) == StatusPending,
		Classification:                categorization.Classification(t.Classification),
		CategorizationOverridden:      t.CategorizationOverridden != 0,
		TransferSubtype:               categorization.TransferSubtype(t.TransferSubtype),
		TransferDestinationOverridden: t.TransferDestinationOverridden != 0,
	}
	if t.CategoryID.Valid {
		id := t.CategoryID.String
		rt.CategoryID = &id
	}
	if categoryName.Valid {
		rt.CategoryName = categoryName.String
	}
	if t.TransferDestinationAccountID.Valid {
		id := t.TransferDestinationAccountID.String
		rt.TransferDestinationAccountID = &id
	}
	// An outflow Transfer leg with no recorded destination and no manual override
	// is the unknown state the UI flags to mark — keyed on the destination column,
	// not the subtype (which can't distinguish unknown from resolved-non-savings).
	rt.TransferDestinationUnknown = categorization.Classification(t.Classification) == categorization.Transfer &&
		t.AmountAmount > 0 &&
		!t.TransferDestinationAccountID.Valid &&
		t.TransferDestinationOverridden == 0
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
		ID:                 t.ID,
		AccountID:          t.AccountID,
		Date:               t.Date,
		AmountAmount:       t.Amount.Amount,
		AmountCurrency:     t.Amount.Currency,
		Merchant:           t.Merchant,
		Counterparty:       t.Counterparty,
		CategoryPrimary:    t.Category.Primary,
		CategoryDetailed:   t.Category.Detailed,
		Status:             string(t.Status),
		Description:        t.Description,
		MerchantEntityID:   t.MerchantEntityID,
		LogoUrl:            t.LogoURL,
		Website:            t.Website,
		PaymentChannel:     t.PaymentChannel,
		CategoryConfidence: t.CategoryConfidence,
		AuthorizedDate:     nullTimeFromPtr(t.AuthorizedDate),
		Datetime:           nullTimeFromPtr(t.Datetime),
		AuthorizedDatetime: nullTimeFromPtr(t.AuthorizedDatetime),
		Counterparties:     marshalCounterparties(t.Counterparties),
	})
}

// DeleteTransaction removes a transaction by its provider id — the handling for
// a provider-reported removal (a dropped pending authorization or a removed
// posted transaction).
func (r *Repo) DeleteTransaction(ctx context.Context, id string) error {
	return r.q.DeleteTransaction(ctx, id)
}

// ListRecentTransactions returns up to limit transactions across all accounts,
// most recent first (date desc, then id desc), each carrying its account id and
// mask (the service resolves the id to the account's display name).
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

// ListTransactionsFiltered returns the full-history activity rows matching the
// given filter — an optional cleaned-merchant substring (nil skips the match) and
// the needs-attention predicate — newest-first (date desc, then id desc), each
// with its account/destination ids and Category name (the service resolves the ids
// to display names). Unlike ListRecentTransactions it applies no recent cap: an
// active filter sees the whole history. The two facets compose.
func (r *Repo) ListTransactionsFiltered(ctx context.Context, merchant *string, needsAttention bool) ([]RecentTransaction, error) {
	params := sqlc.ListTransactionsFilteredParams{NeedsAttention: boolToInt64(needsAttention)}
	if merchant != nil {
		params.Merchant = *merchant
	}
	rows, err := r.q.ListTransactionsFiltered(ctx, params)
	if err != nil {
		return nil, err
	}
	out := make([]RecentTransaction, len(rows))
	for i, row := range rows {
		out[i] = recentFrom(row.Transaction, row.AccountMask, row.CategoryName)
	}
	return out, nil
}

// ListSpendingTransactionsInRange returns the Spending transactions whose date
// falls in [start, end), newest-first (date desc, then id desc), each with its
// account id and Category name (the service resolves the id to a display name) —
// the source rows the spend drill-down buckets and lists.
func (r *Repo) ListSpendingTransactionsInRange(ctx context.Context, start, end time.Time) ([]RecentTransaction, error) {
	rows, err := r.q.ListSpendingTransactionsInRange(ctx, sqlc.ListSpendingTransactionsInRangeParams{
		Date:   start,
		Date_2: end,
	})
	if err != nil {
		return nil, err
	}
	out := make([]RecentTransaction, len(rows))
	for i, row := range rows {
		out[i] = recentFrom(row.Transaction, row.AccountMask, row.CategoryName)
	}
	return out, nil
}

// ListIncomeTransactionsInRange returns the Income legs whose date falls in
// [start, end), newest-first — the rows behind the wrap's gross-income figure and
// its income drill-down.
func (r *Repo) ListIncomeTransactionsInRange(ctx context.Context, start, end time.Time) ([]RecentTransaction, error) {
	rows, err := r.q.ListIncomeTransactionsInRange(ctx, sqlc.ListIncomeTransactionsInRangeParams{
		Date:   start,
		Date_2: end,
	})
	if err != nil {
		return nil, err
	}
	out := make([]RecentTransaction, len(rows))
	for i, row := range rows {
		out[i] = recentFrom(row.Transaction, row.AccountMask, row.CategoryName)
	}
	return out, nil
}

// ListSavingsContributionsInRange returns the savings-contribution source legs
// whose date falls in [start, end), newest-first — the rows behind the wrap's
// savings-contributed figure and its savings drill-down.
func (r *Repo) ListSavingsContributionsInRange(ctx context.Context, start, end time.Time) ([]RecentTransaction, error) {
	rows, err := r.q.ListSavingsContributionsInRange(ctx, sqlc.ListSavingsContributionsInRangeParams{
		Date:   start,
		Date_2: end,
	})
	if err != nil {
		return nil, err
	}
	out := make([]RecentTransaction, len(rows))
	for i, row := range rows {
		out[i] = recentFrom(row.Transaction, row.AccountMask, row.CategoryName)
	}
	return out, nil
}

// ListAllTransactionsInRange returns every transaction (any classification) whose
// date falls in [start, end), newest-first — the rows behind the wrap's inline
// full-month list.
func (r *Repo) ListAllTransactionsInRange(ctx context.Context, start, end time.Time) ([]RecentTransaction, error) {
	rows, err := r.q.ListAllTransactionsInRange(ctx, sqlc.ListAllTransactionsInRangeParams{
		Date:   start,
		Date_2: end,
	})
	if err != nil {
		return nil, err
	}
	out := make([]RecentTransaction, len(rows))
	for i, row := range rows {
		out[i] = recentFrom(row.Transaction, row.AccountMask, row.CategoryName)
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
	return recentFrom(row.Transaction, row.AccountMask, row.CategoryName), nil
}

// TransactionsInRange returns the activity rows whose transaction date falls in
// the half-open [start, end) range, ordered by date then id. It applies no
// account-state filter — every transaction in the range is returned regardless
// of its account's hidden/closed state — and aggregates nothing; the pure month
// projections sum the rows.
func (r *Repo) TransactionsInRange(ctx context.Context, start, end time.Time) ([]ActivityRow, error) {
	rows, err := r.q.TransactionsInRange(ctx, sqlc.TransactionsInRangeParams{
		Date:   start,
		Date_2: end,
	})
	if err != nil {
		return nil, err
	}
	out := make([]ActivityRow, len(rows))
	for i, row := range rows {
		out[i] = activityFromRow(row)
	}
	return out, nil
}

// activityFromRow maps a sqlc range row onto the module's ActivityRow read model,
// preserving the signed amount and the optional Category id.
func activityFromRow(r sqlc.TransactionsInRangeRow) ActivityRow {
	row := ActivityRow{
		ID:   r.ID,
		Date: r.Date,
		Amount: banking.Money{
			Amount:   r.AmountAmount,
			Currency: r.AmountCurrency,
		},
		Classification:  categorization.Classification(r.Classification),
		TransferSubtype: categorization.TransferSubtype(r.TransferSubtype),
		Pending:         Status(r.Status) == StatusPending,
	}
	if r.CategoryID.Valid {
		id := r.CategoryID.String
		row.CategoryID = &id
	}
	return row
}

// EarliestTransactionDate returns the earliest stored transaction date. The bool
// is false (with a zero time and no error) when there are no transactions —
// sql.ErrNoRows is the empty-table signal, not a failure.
func (r *Repo) EarliestTransactionDate(ctx context.Context) (time.Time, bool, error) {
	date, err := r.q.EarliestTransactionDate(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return date, true, nil
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

// ListUncategorizedRows returns the categorization inputs for every transaction
// still at the uncategorized default (classification = "") and not overridden — the
// self-healing sweep's candidate set, the rows a later sync must resolve even when
// they are not in the current pull's delta.
func (r *Repo) ListUncategorizedRows(ctx context.Context) ([]categorizationRow, error) {
	models, err := r.q.ListUncategorizedForCategorization(ctx)
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

// --- Transfer-destination queries ---

// ListTransferLegs returns every stored Transfer leg on a connected (non-closed)
// account — the candidate set the auto-pairing pass filters into outflow source
// legs and inflow candidates and pairs by amount + date window.
func (r *Repo) ListTransferLegs(ctx context.Context) ([]transferLeg, error) {
	rows, err := r.q.ListTransferLegs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]transferLeg, len(rows))
	for i, row := range rows {
		out[i] = transferLeg{
			ID:         row.ID,
			AccountID:  row.AccountID,
			Amount:     row.AmountAmount,
			Date:       row.Date,
			Overridden: row.TransferDestinationOverridden != 0,
		}
	}
	return out, nil
}

// SetTransferDestination writes the auto-paired destination + subtype for a
// transaction, leaving its override flag untouched, so a manually-marked transfer
// facet is preserved; callers pre-skip overridden rows.
func (r *Repo) SetTransferDestination(ctx context.Context, id string, destinationAccountID *string, subtype categorization.TransferSubtype) error {
	return r.q.SetTransactionTransferDestination(ctx, sqlc.SetTransactionTransferDestinationParams{
		TransferDestinationAccountID: nullStringFromPtr(destinationAccountID),
		TransferSubtype:              string(subtype),
		ID:                           id,
	})
}

// OverrideTransferDestination writes a manual transfer-destination choice and
// marks the transfer facet overridden, so it beats auto-pairing and survives
// re-sync — independent of the categorization override.
func (r *Repo) OverrideTransferDestination(ctx context.Context, id string, destinationAccountID *string, subtype categorization.TransferSubtype) error {
	return r.q.OverrideTransactionTransferDestination(ctx, sqlc.OverrideTransactionTransferDestinationParams{
		TransferDestinationAccountID: nullStringFromPtr(destinationAccountID),
		TransferSubtype:              string(subtype),
		ID:                           id,
	})
}

// GetTransferDestination returns the stored transfer facet of one transaction —
// the destination account, the subtype, and the override flag.
func (r *Repo) GetTransferDestination(ctx context.Context, id string) (transferDestination, error) {
	row, err := r.q.GetTransactionTransferDestination(ctx, id)
	if err != nil {
		return transferDestination{}, err
	}
	td := transferDestination{
		Subtype:    categorization.TransferSubtype(row.TransferSubtype),
		Overridden: row.TransferDestinationOverridden != 0,
	}
	if row.TransferDestinationAccountID.Valid {
		id := row.TransferDestinationAccountID.String
		td.DestinationAccountID = &id
	}
	return td, nil
}

// nullStringFromPtr maps an optional string onto a sql.NullString.
func nullStringFromPtr(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}

// nullTimeFromPtr maps an optional timestamp onto a sql.NullTime (nil → invalid),
// for the nullable bank display-detail timestamps the bank often omits.
func nullTimeFromPtr(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

// nullTimeToPtr maps a sql.NullTime back onto an optional timestamp (invalid → nil).
func nullTimeToPtr(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	out := t.Time
	return &out
}

// marshalCounterparties encodes the display-only counterparties list as the JSON
// string the counterparties column stores; a nil/empty slice encodes as "[]".
func marshalCounterparties(cps []banking.Counterparty) string {
	if len(cps) == 0 {
		return "[]"
	}
	b, err := json.Marshal(cps)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// unmarshalCounterparties decodes the stored counterparties JSON back into the
// display list. It tolerates an empty or malformed value by returning an empty
// slice rather than failing the read — the data is read-only display detail, never
// a load-bearing input.
func unmarshalCounterparties(s string) []banking.Counterparty {
	if s == "" || s == "[]" {
		return nil
	}
	var cps []banking.Counterparty
	if err := json.Unmarshal([]byte(s), &cps); err != nil {
		return nil
	}
	return cps
}

// boolToInt64 maps a Go bool onto the 0/1 SQLite stores booleans as — the
// integer the filtered query's needs-attention toggle compares against.
func boolToInt64(b bool) int64 {
	if b {
		return 1
	}
	return 0
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
