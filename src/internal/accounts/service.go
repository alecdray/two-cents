package accounts

import (
	"errors"
	"fmt"
	"time"

	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/cryptox"
	"github.com/alecdray/two-cents/src/internal/core/db"

	"github.com/google/uuid"
)

// Service owns Connections and the Accounts beneath them. It talks to the bank
// only through the injected banking.BankProvider seam and protects access
// tokens at rest with cryptox under the configured encryption key.
type Service struct {
	db            *db.DB
	provider      banking.BankProvider
	encryptionKey string
}

// NewService builds an accounts Service over the database, the bank provider
// seam, and the symmetric encryption key used to protect stored access tokens.
func NewService(d *db.DB, provider banking.BankProvider, encryptionKey string) *Service {
	return &Service{
		db:            d,
		provider:      provider,
		encryptionKey: encryptionKey,
	}
}

// RegisterConnection records a freshly enrolled bank login: it stores the access
// token encrypted alongside the provider item id, lists the login's accounts via
// the provider, and creates one active account per provider account — seeding
// each account's kind and counts-as-savings flag and loading its initial
// balance. The whole write happens in a single transaction.
func (s *Service) RegisterConnection(ctx contextx.ContextX, accessToken, providerItemID string) (Connection, error) {
	providerAccounts, err := s.provider.ListAccounts(ctx, accessToken)
	if err != nil {
		return Connection{}, fmt.Errorf("failed to list provider accounts: %w", err)
	}

	encryptedToken, err := cryptox.SymmetricEncrypt(accessToken, s.encryptionKey)
	if err != nil {
		return Connection{}, fmt.Errorf("failed to encrypt access token: %w", err)
	}

	connection := Connection{
		ID:             uuid.NewString(),
		ProviderItemID: providerItemID,
		State:          ConnectionActive,
	}

	err = s.db.WithTx(func(tx *db.DB) error {
		repo := NewRepo(tx.Queries())
		created, err := repo.CreateConnection(ctx, connection, encryptedToken)
		if err != nil {
			return fmt.Errorf("failed to create connection: %w", err)
		}
		connection = created

		now := time.Now()
		for _, pa := range providerAccounts {
			if _, err := repo.CreateAccount(ctx, newAccount(connection.ID, pa, now)); err != nil {
				return fmt.Errorf("failed to create account: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return Connection{}, err
	}
	return connection, nil
}

// SyncAccounts refreshes every active or needs-reconnect connection: it decrypts
// the connection's token, discovers its current accounts and reads their live
// balances, updates each existing account's balance and last-synced timestamp,
// and creates+seeds any newly appearing account. It does not duplicate an
// existing account, nor reseed an account's kind or counts-as-savings.
//
// A provider call that surfaces banking.ErrReauthRequired flips the connection
// to needs-reconnect (its accounts and history are retained) and the connection
// is skipped; a later clean sync returns it to active.
func (s *Service) SyncAccounts(ctx contextx.ContextX) error {
	connections, err := s.repo().ListConnections(ctx)
	if err != nil {
		return fmt.Errorf("failed to list connections: %w", err)
	}

	for _, conn := range connections {
		if conn.State != ConnectionActive && conn.State != ConnectionNeedsReconnect {
			continue
		}
		if err := s.syncConnection(ctx, conn); err != nil {
			return err
		}
	}
	return nil
}

// syncConnection refreshes a single connection. A re-auth signal from the
// provider transitions it to needs-reconnect and returns without error so the
// remaining connections still sync.
func (s *Service) syncConnection(ctx contextx.ContextX, conn Connection) error {
	encryptedToken, err := s.repo().GetEncryptedToken(ctx, conn.ID)
	if err != nil {
		return fmt.Errorf("failed to load access token: %w", err)
	}
	accessToken, err := cryptox.SymmetricDecrypt(encryptedToken, s.encryptionKey)
	if err != nil {
		return fmt.Errorf("failed to decrypt access token: %w", err)
	}

	providerAccounts, err := s.provider.ListAccounts(ctx, accessToken)
	if err != nil {
		if errors.Is(err, banking.ErrReauthRequired) {
			return s.repo().SetConnectionState(ctx, conn.ID, ConnectionNeedsReconnect)
		}
		return fmt.Errorf("failed to list provider accounts: %w", err)
	}

	balances, err := s.provider.GetBalances(ctx, accessToken)
	if err != nil {
		if errors.Is(err, banking.ErrReauthRequired) {
			return s.repo().SetConnectionState(ctx, conn.ID, ConnectionNeedsReconnect)
		}
		return fmt.Errorf("failed to get provider balances: %w", err)
	}

	balanceByID := make(map[string]banking.Balance, len(balances))
	for _, b := range balances {
		balanceByID[b.AccountID] = b
	}

	return s.db.WithTx(func(tx *db.DB) error {
		repo := NewRepo(tx.Queries())

		existing, err := repo.ListAccountsByConnection(ctx, conn.ID)
		if err != nil {
			return fmt.Errorf("failed to list existing accounts: %w", err)
		}
		existingByProviderID := make(map[string]Account, len(existing))
		for _, a := range existing {
			existingByProviderID[a.ProviderAccountID] = a
		}

		now := time.Now()
		for _, pa := range providerAccounts {
			balance := pa.Balance
			if b, ok := balanceByID[pa.ID]; ok {
				balance = b
			}

			if current, ok := existingByProviderID[pa.ID]; ok {
				current.Balance = balance
				current.LastSyncedAt = &now
				if _, err := repo.UpdateAccount(ctx, current); err != nil {
					return fmt.Errorf("failed to update account: %w", err)
				}
				continue
			}

			account := newAccount(conn.ID, pa, now)
			account.Balance = balance
			if _, err := repo.CreateAccount(ctx, account); err != nil {
				return fmt.Errorf("failed to create account: %w", err)
			}
		}

		if conn.State != ConnectionActive {
			if err := repo.SetConnectionState(ctx, conn.ID, ConnectionActive); err != nil {
				return fmt.Errorf("failed to reactivate connection: %w", err)
			}
		}
		return nil
	})
}

// Overview derives the cash/credit position across the active, non-hidden,
// non-closed accounts: total cash (savings included), total credit debt, and
// net cash (cash − debt). An account whose balance is unknown is excluded from
// the totals — never counted as zero — as is any account in the other bucket.
func (s *Service) Overview(ctx contextx.ContextX) (Overview, error) {
	accounts, err := s.repo().ListAccounts(ctx)
	if err != nil {
		return Overview{}, fmt.Errorf("failed to list accounts: %w", err)
	}
	return computeOverview(accounts), nil
}

// computeOverview sums the overview totals over the eligible accounts. Pure, so
// it is exercised directly by tests.
func computeOverview(accounts []Account) Overview {
	var overview Overview
	for _, a := range accounts {
		if a.State != AccountActive {
			continue
		}
		if !a.Balance.Known {
			continue
		}
		switch a.Kind {
		case banking.KindCash:
			if overview.Currency == "" {
				overview.Currency = a.Balance.Money.Currency
			}
			overview.TotalCash += a.Balance.Money.Amount
		case banking.KindCredit:
			if overview.Currency == "" {
				overview.Currency = a.Balance.Money.Currency
			}
			overview.TotalDebt += a.Balance.Money.Amount
		default:
			// Other-bucket accounts (loans, investments, …) sit outside the
			// cash/debt position and are skipped entirely.
			continue
		}
	}
	overview.NetCash = overview.TotalCash - overview.TotalDebt
	return overview
}

// newAccount builds a fresh active Account from a provider account, taking the
// seam's already-bucketed kind and counts-as-savings defaults, recording the
// bank's reported subtype as the display label, and stamping the sync time.
func newAccount(connectionID string, pa banking.Account, syncedAt time.Time) Account {
	return Account{
		ID:                uuid.NewString(),
		ConnectionID:      connectionID,
		ProviderAccountID: pa.ID,
		Name:              pa.Name,
		BankType:          pa.Subtype,
		Kind:              pa.Kind,
		CountsAsSavings:   pa.CountsAsSavings,
		Balance:           pa.Balance,
		State:             AccountActive,
		LastSyncedAt:      &syncedAt,
	}
}

// repo binds a Repo to the global (non-transactional) query handle.
func (s *Service) repo() *Repo {
	return NewRepo(s.db.Queries())
}
