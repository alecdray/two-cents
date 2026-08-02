package transactions

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/alecdray/two-cents/src/internal/accounts"
	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/categorization"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/db"
)

// ValidationError is a recoverable, user-facing input error on a manual
// re-categorization (a Spending choice with no Category). Adapters surface its
// Message inline beside the picker rather than treating it as a server failure.
type ValidationError struct {
	Message string
}

func (e ValidationError) Error() string { return e.Message }

// IsValidationError reports whether err is (or wraps) a ValidationError, so
// adapters can render its message inline instead of returning a server error.
func IsValidationError(err error) (ValidationError, bool) {
	var ve ValidationError
	if errors.As(err, &ve) {
		return ve, true
	}
	return ValidationError{}, false
}

// Service owns the Transaction rows and the per-connection sync cursors, and is
// the only writer of a transaction's categorization facet. It reaches the bank
// only through the injected banking.BankProvider seam and drives Accounts
// (balances + connection health) first on every sync pass; it asks the injected
// categorization Service to decide each row's classification (the decider owns
// the policy, transactions owns the write). The module graph stays acyclic:
// transactions depends on accounts and categorization, neither the reverse.
type Service struct {
	db             *db.DB
	provider       banking.BankProvider
	accounts       *accounts.Service
	categorization *categorization.Service
	// logoFetcher is the SSRF-constrained seam the post-sync cache-warm step fetches
	// merchant logos through. It may be nil (no logos are warmed then), so the warm
	// step guards against it.
	logoFetcher LogoFetcher
}

// NewService builds a transactions Service over the database, the bank provider
// seam, the accounts service it orchestrates each sync around, the categorization
// service it consults to classify each synced row, and the logo fetcher the post-sync
// cache-warm step pulls merchant logos through (nil to warm no logos).
func NewService(d *db.DB, provider banking.BankProvider, accountsSvc *accounts.Service, categorizationSvc *categorization.Service, logoFetcher LogoFetcher) *Service {
	return &Service{
		db:             d,
		provider:       provider,
		accounts:       accountsSvc,
		categorization: categorizationSvc,
		logoFetcher:    logoFetcher,
	}
}

// SyncTransactions runs a full sync pass. It refreshes Accounts first (balances
// and connection health), then pulls each syncable connection's incremental
// transaction changes through the provider seam, applies them keyed by provider
// id, and advances the connection's resume cursor.
//
// Per-connection failures are isolated: a connection whose pull reports
// banking.ErrReauthRequired is skipped with its cursor left untouched and the
// pass continues — the overall sync does not error for that case, mirroring how
// Accounts handles a re-auth signal.
func (s *Service) SyncTransactions(ctx contextx.ContextX) error {
	// Accounts first — refresh balances and connection health (and flag any
	// connection the provider now reports as needing re-auth) before writing any
	// transaction rows.
	if err := s.accounts.SyncAccounts(ctx); err != nil {
		return fmt.Errorf("failed to sync accounts: %w", err)
	}

	targets, err := s.accounts.ConnectionsToSync(ctx)
	if err != nil {
		return fmt.Errorf("failed to list connections to sync: %w", err)
	}

	for _, target := range targets {
		if err := s.syncConnection(ctx, target); err != nil {
			return err
		}
	}

	// With every connection's rows pulled, categorize every still-uncategorized,
	// non-overridden row — not just this pass's delta. Running the sweep over the
	// stored set (like the transfer-pairing step below) makes categorization
	// self-healing: a row a prior sync left at classification='' — synced before
	// categorization ran, or after a categorize error that still advanced the
	// cursor — resolves on the next sync instead of needing a full re-backfill.
	if err := s.categorizeUncategorizedSweep(ctx); err != nil {
		return err
	}

	// With every connection's rows pulled and categorized, resolve each outflow
	// Transfer leg's destination + subtype against the stored set — once, so a
	// pairing can span accounts on different connections and an inflow synced on
	// any connection can resolve an earlier-synced outflow.
	if err := s.resolveTransferDestinations(ctx); err != nil {
		return err
	}

	// Warm the merchant-logo cache as a best-effort final step, decoupled from sync
	// correctness the same way the categorize sweep is decoupled from the cursor
	// advance: it runs only after every row write and cursor advance is committed,
	// fetches through the SSRF-constrained seam, and its failures are logged and
	// swallowed here. So a fetch that errors, times out, or is refused can never fail
	// the sync or roll back a committed transaction row or balance.
	if err := s.warmMerchantLogoCache(ctx); err != nil {
		slog.ErrorContext(ctx, "failed to warm merchant logo cache", "error", err)
	}
	return nil
}

// maxLogosWarmedPerSync bounds how many distinct un-cached merchant logos one sync
// pass fetches, so a large backlog drains over successive syncs rather than in a
// single fetch storm; the most-recent-first ordering warms the current month first.
const maxLogosWarmedPerSync = 40

// warmMerchantLogoCache fetches and caches merchant logos that are not yet cached,
// across the whole stored transaction set rather than only this pull's delta (so a
// pre-existing merchant heals without a cursor clear), most-recent-first and bounded
// to one batch per pass. Each un-cached logo URL is fetched once through the injected
// SSRF-constrained seam: a usable raster image is stored as a positive entry, and a
// URL that is absent, fails, or is refused by the fetch policy is stored as a negative
// entry so it is never attempted again. A merchant already cached (positive OR
// negative) is skipped, so a later sync re-fetches nothing already attempted. The
// step is best-effort: a fetch failure becomes a negative entry rather than an error,
// and the caller logs and swallows any error this returns, so sync correctness is
// never coupled to a logo fetch.
func (s *Service) warmMerchantLogoCache(ctx contextx.ContextX) error {
	if s.logoFetcher == nil {
		return nil
	}

	urls, err := s.repo().ListMerchantLogoURLsByRecency(ctx)
	if err != nil {
		return fmt.Errorf("failed to list merchant logo urls: %w", err)
	}
	cachedKeys, err := s.repo().ListCachedLogoKeys(ctx)
	if err != nil {
		return fmt.Errorf("failed to list cached logo keys: %w", err)
	}
	cached := make(map[string]bool, len(cachedKeys))
	for _, k := range cachedKeys {
		cached[k] = true
	}

	warmed := 0
	for _, logoURL := range urls {
		if warmed >= maxLogosWarmedPerSync {
			break
		}
		key := merchantLogoKey(logoURL)
		if cached[key] {
			continue // already fetched once (positive or negative); never re-attempt.
		}
		cached[key] = true
		warmed++

		imageBytes, contentType, ferr := s.logoFetcher.FetchLogo(ctx, logoURL)
		if ferr != nil || len(imageBytes) == 0 || contentType == "" {
			// No usable logo: negative-cache the key so the next sync skips it. A fetch
			// error is treated exactly like a policy refusal — recorded, never propagated.
			if err := s.repo().PutMerchantLogoMiss(ctx, key, logoURL); err != nil {
				return fmt.Errorf("failed to record merchant logo miss: %w", err)
			}
			continue
		}
		if err := s.repo().PutMerchantLogo(ctx, key, logoURL, contentType, imageBytes); err != nil {
			return fmt.Errorf("failed to cache merchant logo: %w", err)
		}
	}
	return nil
}

// CachedLogo is a positively cached merchant logo ready to serve from our origin: the
// image bytes and their stored content type.
type CachedLogo struct {
	ContentType string
	Bytes       []byte
}

// MerchantLogo returns the positively cached logo for a key, or ok=false when the key
// is absent or negative-cached. It is a pure cache read — it performs no outbound
// fetch — so the image endpoint that serves it never contacts a third party.
func (s *Service) MerchantLogo(ctx contextx.ContextX, key string) (CachedLogo, bool, error) {
	contentType, imageBytes, ok, err := s.repo().GetMerchantLogo(ctx, key)
	if err != nil {
		return CachedLogo{}, false, fmt.Errorf("failed to read cached merchant logo: %w", err)
	}
	if !ok {
		return CachedLogo{}, false, nil
	}
	return CachedLogo{ContentType: contentType, Bytes: imageBytes}, true, nil
}

// transferPairingWindowDays is the inclusive ±N calendar-day window an outflow
// Transfer leg's matching inflow may fall within when the pairing pass learns its
// destination (ADR-0003's conservative ±3 days).
const transferPairingWindowDays = 3

// resolveTransferDestinations re-resolves every non-overridden outflow Transfer
// leg's destination + subtype from scratch, the sync's transfer-pairing step. It
// reads the connected-account facets and every stored Transfer leg, builds the
// inflow-candidate list (legs on other connected accounts, each annotated with
// its account's counts-as-savings flag), and asks the pure
// categorization.ResolveTransferSubtype engine to pair each outflow source leg —
// writing the resolved destination + subtype. Overridden legs keep their sticky
// manual choice and are skipped; only outflow legs carry a subtype, so the inflow
// mirror legs are never labeled. It re-runs every sync (a later-synced inflow can
// resolve an earlier unknown outflow), mirroring ApplyCategorization's
// re-resolve-from-scratch stance.
func (s *Service) resolveTransferDestinations(ctx contextx.ContextX) error {
	facets, err := s.accounts.ConnectedAccountFacets(ctx)
	if err != nil {
		return fmt.Errorf("failed to load account facets for transfer pairing: %w", err)
	}
	connected := make(map[string]bool, len(facets))
	savings := make(map[string]bool, len(facets))
	for _, f := range facets {
		connected[f.ID] = true
		savings[f.ID] = f.CountsAsSavings
	}

	legs, err := s.repo().ListTransferLegs(ctx)
	if err != nil {
		return fmt.Errorf("failed to load transfer legs: %w", err)
	}

	// The inflow candidates: every inflow Transfer leg on a connected account,
	// annotated with that account's counts-as-savings flag. The engine itself
	// excludes same-account legs and applies the amount + date-window match.
	var candidates []categorization.TransferLeg
	for _, leg := range legs {
		if leg.Amount >= 0 || !connected[leg.AccountID] {
			continue
		}
		candidates = append(candidates, categorization.TransferLeg{
			TransactionID:   leg.ID,
			AccountID:       leg.AccountID,
			AmountCents:     categorization.AmountCents(leg.Amount),
			Date:            leg.Date,
			CountsAsSavings: savings[leg.AccountID],
		})
	}

	for _, leg := range legs {
		// Only outflow source legs carry a subtype; a manually-marked leg is
		// sticky and never reverted by the auto pass.
		if leg.Amount <= 0 || leg.Overridden || !connected[leg.AccountID] {
			continue
		}
		decision := categorization.ResolveTransferSubtype(categorization.TransferSubtypeInput{
			SourceAccountID: leg.AccountID,
			AmountCents:     categorization.AmountCents(leg.Amount),
			Date:            leg.Date,
			Candidates:      candidates,
			WindowDays:      transferPairingWindowDays,
		})
		if err := s.repo().SetTransferDestination(ctx, leg.ID, decision.DestinationAccountID, decision.Subtype); err != nil {
			return fmt.Errorf("failed to write transfer destination: %w", err)
		}
	}
	return nil
}

// syncConnection pulls one connection's changes from its stored cursor, applies
// them, and persists the returned cursor — all the row writes plus the cursor
// advance in a single transaction so a partial apply never leaves the cursor
// ahead of the data. A re-auth signal is swallowed (the connection is skipped
// and its cursor untouched) so the remaining connections still sync. It does not
// categorize: the post-pull sweep in SyncTransactions resolves the new rows along
// with any earlier stragglers, so a categorize step is never coupled to the cursor
// advance.
func (s *Service) syncConnection(ctx contextx.ContextX, target accounts.ConnectionSyncTarget) error {
	cursor, err := s.repo().GetSyncCursor(ctx, target.ConnectionID)
	if err != nil {
		return fmt.Errorf("failed to load sync cursor: %w", err)
	}

	changes, err := s.provider.SyncTransactions(ctx, target.AccessToken, cursor)
	if err != nil {
		if errors.Is(err, banking.ErrReauthRequired) {
			// Skip this connection — leave its cursor unchanged so the next pass
			// resumes from the same point once the login is restored.
			return nil
		}
		return fmt.Errorf("failed to pull transactions: %w", err)
	}

	return s.db.WithTx(func(tx *db.DB) error {
		repo := NewRepo(tx.Queries())
		if err := applyChanges(ctx, repo, target.AccountIDByProvider, changes); err != nil {
			return err
		}
		if err := repo.SetSyncCursor(ctx, target.ConnectionID, changes.Cursor); err != nil {
			return fmt.Errorf("failed to persist sync cursor: %w", err)
		}
		return nil
	})
}

// applyChanges persists one pull's delta: added/modified rows upsert in place by
// provider id and removed ids delete by id. A transaction whose provider account
// id is not among the connection's known accounts is skipped — it has no account
// row to attribute to. Categorization is not done here; the post-pull sweep in
// SyncTransactions resolves every still-uncategorized row.
func applyChanges(ctx contextx.ContextX, repo *Repo, accountIDByProvider map[string]string, changes banking.TransactionChanges) error {
	upsert := func(bt banking.Transaction) error {
		accountID, ok := accountIDByProvider[bt.AccountID]
		if !ok {
			return nil
		}
		if err := repo.UpsertTransaction(ctx, transactionFromBanking(bt, accountID)); err != nil {
			return fmt.Errorf("failed to upsert transaction: %w", err)
		}
		return nil
	}

	for _, bt := range changes.Added {
		if err := upsert(bt); err != nil {
			return err
		}
	}
	for _, bt := range changes.Modified {
		if err := upsert(bt); err != nil {
			return err
		}
	}
	for _, id := range changes.RemovedIDs {
		if err := repo.DeleteTransaction(ctx, id); err != nil {
			return fmt.Errorf("failed to delete transaction: %w", err)
		}
	}
	return nil
}

// categorizeUncategorizedSweep resolves every non-overridden transaction still at
// the uncategorized default (classification=""), across the whole stored set rather
// than only the current pull's delta — the categorization analogue of
// resolveTransferDestinations' full re-scan. This is what makes categorization
// self-healing: a row a prior sync left uncategorized (synced before categorization
// ran, or after a categorize error that still advanced the cursor) is resolved on
// the next sync instead of staying stuck until a full DB re-backfill. Already-
// classified rows are out of scope (the query excludes them), so once a row is
// resolved it is never re-swept; overridden rows are excluded too.
func (s *Service) categorizeUncategorizedSweep(ctx contextx.ContextX) error {
	rows, err := s.repo().ListUncategorizedRows(ctx)
	if err != nil {
		return fmt.Errorf("failed to list uncategorized transactions: %w", err)
	}
	for _, row := range rows {
		if _, err := s.resolveAndWrite(ctx, row); err != nil {
			return err
		}
	}
	return nil
}

// resolveAndWrite asks the categorization module to decide a row's bucket and
// writes the result when it differs from what is stored, reporting whether it
// changed. It never marks the row overridden — that is the manual path only.
func (s *Service) resolveAndWrite(ctx contextx.ContextX, row categorizationRow) (bool, error) {
	decision, err := s.categorization.Resolve(ctx, row.Category, row.Merchant, row.Counterparty, row.Amount)
	if err != nil {
		return false, fmt.Errorf("failed to resolve categorization: %w", err)
	}
	if decision.Classification == row.Classification && equalStringPtr(decision.CategoryID, row.CategoryID) {
		return false, nil
	}
	if err := s.repo().SetCategorization(ctx, row.ID, decision.Classification, decision.CategoryID); err != nil {
		return false, fmt.Errorf("failed to write categorization: %w", err)
	}
	return true, nil
}

// equalStringPtr reports whether two optional strings hold the same value (both
// nil, or both set to the same string).
func equalStringPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// resolveAccountNames fills each row's AccountName and TransferDestinationName from
// the accounts module — the owner of the display-name precedence (custom name else
// bank name, ADR-0017). The read model carries only account ids; the names are
// resolved here rather than joined in SQL so the naming policy lives in one place
// (a cross-module read flows through the owning service, never raw SQL — see
// docs/architecture/data-model.md). A destination id whose account has since been
// removed resolves to no name and is left blank, matching the removed JOIN's
// behaviour (see docs/architecture/known-gaps.md).
func (s *Service) resolveAccountNames(ctx contextx.ContextX, rows []RecentTransaction) error {
	if len(rows) == 0 {
		return nil
	}
	ids := make([]string, 0, len(rows)*2)
	for _, r := range rows {
		ids = append(ids, r.AccountID)
		if r.TransferDestinationAccountID != nil {
			ids = append(ids, *r.TransferDestinationAccountID)
		}
	}
	names, err := s.accounts.DisplayNames(ctx, ids)
	if err != nil {
		return fmt.Errorf("failed to resolve account display names: %w", err)
	}
	for i := range rows {
		rows[i].AccountName = names[rows[i].AccountID]
		if id := rows[i].TransferDestinationAccountID; id != nil {
			rows[i].TransferDestinationName = names[*id]
		}
	}
	return nil
}

// decorate fills the read model's derived, cross-cutting fields: each row's account
// display names (resolved through accounts) and the served URL of its cached merchant
// logo. Every read that returns RecentTransactions runs it, so those fields are
// populated consistently on every surface.
func (s *Service) decorate(ctx contextx.ContextX, rows []RecentTransaction) error {
	if err := s.resolveAccountNames(ctx, rows); err != nil {
		return err
	}
	return s.resolveMerchantLogos(ctx, rows)
}

// resolveMerchantLogos fills each row's MerchantLogoURL with our origin's served URL
// for its logo — but only when that logo is positively cached — so a row references a
// locally served image and never the bank CDN. A row whose logo is absent or negative-
// cached is left blank (the caller shows its category glyph instead). It reads the
// cache only and never fetches.
func (s *Service) resolveMerchantLogos(ctx contextx.ContextX, rows []RecentTransaction) error {
	if len(rows) == 0 {
		return nil
	}
	positiveKeys, err := s.repo().ListPositiveLogoKeys(ctx)
	if err != nil {
		return fmt.Errorf("failed to load cached merchant logos: %w", err)
	}
	positive := make(map[string]bool, len(positiveKeys))
	for _, k := range positiveKeys {
		positive[k] = true
	}
	for i := range rows {
		if rows[i].LogoURL == "" {
			continue
		}
		key := merchantLogoKey(rows[i].LogoURL)
		if positive[key] {
			rows[i].MerchantLogoURL = merchantLogoServedURL(key)
		}
	}
	return nil
}

// RecentTransactions returns at most limit transactions across all accounts,
// most recent first (date desc, then id desc), each carrying its account's
// display name. It reads stored rows only and never calls the provider.
func (s *Service) RecentTransactions(ctx contextx.ContextX, limit int) ([]RecentTransaction, error) {
	recent, err := s.repo().ListRecentTransactions(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list recent transactions: %w", err)
	}
	if err := s.decorate(ctx, recent); err != nil {
		return nil, err
	}
	return recent, nil
}

// FilteredTransactions returns the full-history activity rows matching the filter,
// newest-first, each carrying its account display name (and Category / transfer
// destination names). Unlike RecentTransactions it applies no recent cap: callers
// use it only when a filter is active (search and/or needs-attention) so the view
// can find matches beyond the recent window. It reads stored rows only and never
// calls the provider.
func (s *Service) FilteredTransactions(ctx contextx.ContextX, f Filter) ([]RecentTransaction, error) {
	var merchant *string
	if f.Merchant != "" {
		m := f.Merchant
		merchant = &m
	}
	rows, err := s.repo().ListTransactionsFiltered(ctx, merchant, f.NeedsAttention)
	if err != nil {
		return nil, fmt.Errorf("failed to list filtered transactions: %w", err)
	}
	if err := s.decorate(ctx, rows); err != nil {
		return nil, err
	}
	return rows, nil
}

// RecentTransaction returns a single transaction as the recent-activity read
// model — the per-row read the re-categorize handler re-renders after a mutation.
func (s *Service) RecentTransaction(ctx contextx.ContextX, id string) (RecentTransaction, error) {
	row, err := s.repo().GetRecentTransaction(ctx, id)
	if err != nil {
		return RecentTransaction{}, fmt.Errorf("failed to load transaction: %w", err)
	}
	rows := []RecentTransaction{row}
	if err := s.decorate(ctx, rows); err != nil {
		return RecentTransaction{}, err
	}
	return rows[0], nil
}

// SpendingTransactionsInRange returns the Spending transactions whose date falls
// in the half-open [start, end) range, newest-first, each carrying its account
// and Category display names — the source set the spend drill-down buckets and
// lists. It counts every transaction in the range regardless of its account's
// hidden/closed state and reads stored rows only, never calling the provider.
func (s *Service) SpendingTransactionsInRange(ctx contextx.ContextX, start, end time.Time) ([]RecentTransaction, error) {
	rows, err := s.repo().ListSpendingTransactionsInRange(ctx, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to list spending transactions in range: %w", err)
	}
	if err := s.decorate(ctx, rows); err != nil {
		return nil, err
	}
	return rows, nil
}

// IncomeTransactionsInRange returns the Income legs whose date falls in the
// half-open [start, end) range, newest-first — the source set the wrap's gross
// income figure sums and the income drill-down lists. Reads stored rows only.
func (s *Service) IncomeTransactionsInRange(ctx contextx.ContextX, start, end time.Time) ([]RecentTransaction, error) {
	rows, err := s.repo().ListIncomeTransactionsInRange(ctx, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to list income transactions in range: %w", err)
	}
	if err := s.decorate(ctx, rows); err != nil {
		return nil, err
	}
	return rows, nil
}

// SavingsContributionsInRange returns the savings-contribution source legs whose
// date falls in the half-open [start, end) range, newest-first — the source set the
// wrap's savings-contributed figure sums and the savings drill-down lists. Reads
// stored rows only.
func (s *Service) SavingsContributionsInRange(ctx contextx.ContextX, start, end time.Time) ([]RecentTransaction, error) {
	rows, err := s.repo().ListSavingsContributionsInRange(ctx, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to list savings contributions in range: %w", err)
	}
	if err := s.decorate(ctx, rows); err != nil {
		return nil, err
	}
	return rows, nil
}

// MonthTransactions returns every transaction (any classification) whose date falls
// in the half-open [start, end) range, newest-first — the wrap's inline full-month
// list. Reads stored rows only, never calling the provider.
func (s *Service) MonthTransactions(ctx contextx.ContextX, start, end time.Time) ([]RecentTransaction, error) {
	rows, err := s.repo().ListAllTransactionsInRange(ctx, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to list month transactions: %w", err)
	}
	if err := s.decorate(ctx, rows); err != nil {
		return nil, err
	}
	return rows, nil
}

// SpendingByAccountInRange returns the Spending transactions on one specific
// account whose date falls in [start, end), newest-first. Refund inflows
// (negative amounts) are included so the signed net correctly accounts for them.
// Used by the sweep computation to derive MTD spending from the checking account.
func (s *Service) SpendingByAccountInRange(ctx contextx.ContextX, accountID string, start, end time.Time) ([]RecentTransaction, error) {
	rows, err := s.repo().ListSpendingByAccountInRange(ctx, accountID, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to list spending by account in range: %w", err)
	}
	if err := s.decorate(ctx, rows); err != nil {
		return nil, err
	}
	return rows, nil
}

// SavingsContributionsByAccountInRange returns the savings-contribution source
// legs on one specific account whose date falls in [start, end), newest-first.
// Used by the sweep computation to derive how much has already been moved from
// checking to savings this month.
func (s *Service) SavingsContributionsByAccountInRange(ctx contextx.ContextX, accountID string, start, end time.Time) ([]RecentTransaction, error) {
	rows, err := s.repo().ListSavingsContributionsByAccountInRange(ctx, accountID, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to list savings contributions by account in range: %w", err)
	}
	if err := s.decorate(ctx, rows); err != nil {
		return nil, err
	}
	return rows, nil
}

// EarliestTransactionDate returns the earliest stored transaction date. The bool
// is false when there are no transactions — an empty table is a normal state
// (the wraps list collapses to the current month), not an error.
func (s *Service) EarliestTransactionDate(ctx contextx.ContextX) (time.Time, bool, error) {
	date, ok, err := s.repo().EarliestTransactionDate(ctx)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("failed to read earliest transaction date: %w", err)
	}
	return date, ok, nil
}

// ReCategorize records a manual re-categorization of one transaction and marks
// it overridden, so the choice beats auto-resolution and survives re-sync. It
// enforces the Classification/Category coupling: a Spending outcome requires a
// Category; Income/Transfer/needs-review clear it. A coupling violation is a
// recoverable ValidationError the adapter renders inline.
func (s *Service) ReCategorize(ctx contextx.ContextX, txnID string, classification categorization.Classification, categoryID *string) error {
	classification, categoryID, err := coupleChoice(classification, categoryID)
	if err != nil {
		return err
	}
	if err := s.repo().OverrideCategorization(ctx, txnID, classification, categoryID); err != nil {
		return fmt.Errorf("failed to re-categorize transaction: %w", err)
	}
	return nil
}

// MarkTransferDestination records a manual transfer-destination choice for one
// outflow Transfer leg and marks its transfer facet overridden, so the choice
// beats auto-pairing and survives re-sync — independent of the categorization
// facet (it never touches classification / category_id / categorization_overridden).
// A nil destination is allowed: the user can attribute a transfer to a subtype
// (e.g. savings) without recording a connected destination account. The target
// row must be an outflow Transfer (amount > 0, classification transfer) and the
// subtype one of the allowed values, else a recoverable ValidationError the
// adapter renders inline.
func (s *Service) MarkTransferDestination(ctx contextx.ContextX, txnID string, destinationAccountID *string, subtype categorization.TransferSubtype) error {
	row, err := s.repo().GetCategorizationRow(ctx, txnID)
	if err != nil {
		return fmt.Errorf("failed to load transaction for transfer destination: %w", err)
	}
	if err := validateTransferMark(row, subtype); err != nil {
		return err
	}
	if err := s.repo().OverrideTransferDestination(ctx, txnID, destinationAccountID, subtype); err != nil {
		return fmt.Errorf("failed to mark transfer destination: %w", err)
	}
	return nil
}

// validateTransferMark guards a manual transfer-destination mark: the target row
// must be an outflow Transfer leg (amount > 0, classification transfer) — the only
// leg that carries a subtype — and the subtype must be one of the allowed values
// (a savings contribution or a plain transfer). A violation is a recoverable
// ValidationError the adapter renders inline.
func validateTransferMark(row categorizationRow, subtype categorization.TransferSubtype) error {
	if row.Classification != categorization.Transfer || row.Amount.Amount <= 0 {
		return ValidationError{Message: "Only an outflow transfer can have its destination marked."}
	}
	switch subtype {
	case categorization.SubtypeSavingsContribution, categorization.SubtypePlain:
		return nil
	default:
		return ValidationError{Message: "Choose a valid transfer subtype."}
	}
}

// coupleChoice enforces the Classification/Category coupling and normalizes the
// Category by outcome: a Spending choice must carry a Category; Income, Transfer,
// and needs-review carry none, so any supplied Category is cleared.
func coupleChoice(classification categorization.Classification, categoryID *string) (categorization.Classification, *string, error) {
	switch classification {
	case categorization.Spending:
		if categoryID == nil || strings.TrimSpace(*categoryID) == "" {
			return "", nil, ValidationError{Message: "Choose a category for a spending transaction."}
		}
		return classification, categoryID, nil
	case categorization.Income, categorization.Transfer, categorization.NeedsReview:
		return classification, nil, nil
	default:
		return "", nil, ValidationError{Message: "Choose a valid categorization."}
	}
}

// ApplyCategorization re-runs categorization over the non-overridden transactions
// whose cleaned merchant matches any of the given substrings (case-insensitive),
// returning how many actually changed. It is the runtime side of the rule-change
// seam: a Rule create/edit/delete drives it through the server-wired closure.
// Overridden rows are skipped so a sticky manual choice is never reverted.
func (s *Service) ApplyCategorization(ctx contextx.ContextX, substrings []string) (int, error) {
	rows, err := s.repo().ListCategorizationRows(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to list transactions for re-categorization: %w", err)
	}

	changed := 0
	for _, row := range rows {
		if row.Overridden {
			continue
		}
		clean := categorization.CleanMerchantName(row.Merchant, row.Counterparty)
		if !matchesAnySubstring(clean, substrings) {
			continue
		}
		didChange, err := s.resolveAndWrite(ctx, row)
		if err != nil {
			return changed, err
		}
		if didChange {
			changed++
		}
	}
	return changed, nil
}

// RepairTransferSubtypes re-pairs every non-overridden Transfer leg from the
// stored data (no provider call), the runtime side of the accounts kind/savings
// override seam: a counts-as-savings change drives it through the server-wired
// closure so the change applies immediately instead of waiting for the next sync.
// It is the same re-resolution the sync runs as its final step; manually-marked
// Transfers are skipped, so a sticky destination choice is never reverted.
func (s *Service) RepairTransferSubtypes(ctx contextx.ContextX) error {
	return s.resolveTransferDestinations(ctx)
}

// matchesAnySubstring reports whether the cleaned merchant contains any of the
// substrings, case-insensitively — the same match the engine's rule step uses.
func matchesAnySubstring(cleanMerchant string, substrings []string) bool {
	lower := strings.ToLower(cleanMerchant)
	for _, sub := range substrings {
		sub = strings.TrimSpace(sub)
		if sub == "" {
			continue
		}
		if strings.Contains(lower, strings.ToLower(sub)) {
			return true
		}
	}
	return false
}

// repo binds a Repo to the global (non-transactional) query handle.
func (s *Service) repo() *Repo {
	return NewRepo(s.db.Queries())
}
