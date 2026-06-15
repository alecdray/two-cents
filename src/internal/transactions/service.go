package transactions

import (
	"errors"
	"fmt"
	"strings"

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
}

// NewService builds a transactions Service over the database, the bank provider
// seam, the accounts service it orchestrates each sync around, and the
// categorization service it consults to classify each synced row.
func NewService(d *db.DB, provider banking.BankProvider, accountsSvc *accounts.Service, categorizationSvc *categorization.Service) *Service {
	return &Service{
		db:             d,
		provider:       provider,
		accounts:       accountsSvc,
		categorization: categorizationSvc,
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
	return nil
}

// syncConnection pulls one connection's changes from its stored cursor, applies
// them, and persists the returned cursor — all the row writes plus the cursor
// advance in a single transaction so a partial apply never leaves the cursor
// ahead of the data. A re-auth signal is swallowed (the connection is skipped
// and its cursor untouched) so the remaining connections still sync.
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

	var affected []string
	if err := s.db.WithTx(func(tx *db.DB) error {
		repo := NewRepo(tx.Queries())
		upserted, err := applyChanges(ctx, repo, target.AccountIDByProvider, changes)
		if err != nil {
			return err
		}
		affected = upserted
		if err := repo.SetSyncCursor(ctx, target.ConnectionID, changes.Cursor); err != nil {
			return fmt.Errorf("failed to persist sync cursor: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	// Categorize the rows this pull touched, after the row writes have committed:
	// the categorization read (rules + taxonomy) and the per-row write run on the
	// global handle, outside the upsert transaction.
	return s.categorizeUncategorized(ctx, affected)
}

// applyChanges persists one pull's delta: added/modified rows upsert in place by
// provider id and removed ids delete by id. A transaction whose provider account
// id is not among the connection's known accounts is skipped — it has no account
// row to attribute to. It returns the ids of the rows it upserted (added or
// modified, attributed), so the caller can auto-categorize them.
func applyChanges(ctx contextx.ContextX, repo *Repo, accountIDByProvider map[string]string, changes banking.TransactionChanges) ([]string, error) {
	var upserted []string
	upsert := func(bt banking.Transaction) error {
		accountID, ok := accountIDByProvider[bt.AccountID]
		if !ok {
			return nil
		}
		if err := repo.UpsertTransaction(ctx, transactionFromBanking(bt, accountID)); err != nil {
			return fmt.Errorf("failed to upsert transaction: %w", err)
		}
		upserted = append(upserted, bt.ID)
		return nil
	}

	for _, bt := range changes.Added {
		if err := upsert(bt); err != nil {
			return nil, err
		}
	}
	for _, bt := range changes.Modified {
		if err := upsert(bt); err != nil {
			return nil, err
		}
	}
	for _, id := range changes.RemovedIDs {
		if err := repo.DeleteTransaction(ctx, id); err != nil {
			return nil, fmt.Errorf("failed to delete transaction: %w", err)
		}
	}
	return upserted, nil
}

// categorizeUncategorized auto-categorizes the given rows that are new or still
// uncategorized and not overridden — the sync's step-4 auto-categorize. An
// overridden row keeps its sticky facet; an already-classified row (a
// pending→posted modify that was categorized on its first sight) is left as-is.
func (s *Service) categorizeUncategorized(ctx contextx.ContextX, ids []string) error {
	for _, id := range ids {
		row, err := s.repo().GetCategorizationRow(ctx, id)
		if err != nil {
			return fmt.Errorf("failed to load transaction for categorization: %w", err)
		}
		if row.Overridden || row.Classification != "" {
			continue
		}
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

// RecentTransactions returns at most limit transactions across all accounts,
// most recent first (date desc, then id desc), each carrying its account's
// display name. It reads stored rows only and never calls the provider.
func (s *Service) RecentTransactions(ctx contextx.ContextX, limit int) ([]RecentTransaction, error) {
	recent, err := s.repo().ListRecentTransactions(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list recent transactions: %w", err)
	}
	return recent, nil
}

// RecentTransaction returns a single transaction as the recent-activity read
// model — the per-row read the re-categorize handler re-renders after a mutation.
func (s *Service) RecentTransaction(ctx contextx.ContextX, id string) (RecentTransaction, error) {
	row, err := s.repo().GetRecentTransaction(ctx, id)
	if err != nil {
		return RecentTransaction{}, fmt.Errorf("failed to load transaction: %w", err)
	}
	return row, nil
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
