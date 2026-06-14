package transactions

import (
	"errors"
	"fmt"

	"github.com/alecdray/two-cents/src/internal/accounts"
	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/db"
)

// Service owns the Transaction rows and the per-connection sync cursors. It
// reaches the bank only through the injected banking.BankProvider seam and
// drives Accounts (balances + connection health) first on every sync pass, so
// the module graph stays acyclic: transactions depends on accounts, never the
// reverse.
type Service struct {
	db       *db.DB
	provider banking.BankProvider
	accounts *accounts.Service
}

// NewService builds a transactions Service over the database, the bank provider
// seam, and the accounts service it orchestrates each sync around.
func NewService(d *db.DB, provider banking.BankProvider, accountsSvc *accounts.Service) *Service {
	return &Service{
		db:       d,
		provider: provider,
		accounts: accountsSvc,
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
// row to attribute to.
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

// repo binds a Repo to the global (non-transactional) query handle.
func (s *Service) repo() *Repo {
	return NewRepo(s.db.Queries())
}
