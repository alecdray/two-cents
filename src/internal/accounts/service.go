package accounts

import (
	"errors"
	"fmt"
	"strings"
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

// BeginConnect starts a new bank enrollment by minting a provider link token.
// The front end hands the token to the provider's connect flow; in fake mode the
// token's Mode tells the page the flow is simulated and no provider UI is opened.
func (s *Service) BeginConnect(ctx contextx.ContextX) (banking.LinkToken, error) {
	token, err := s.provider.CreateLinkToken(ctx, banking.LinkOptions{})
	if err != nil {
		return banking.LinkToken{}, fmt.Errorf("failed to create link token: %w", err)
	}
	return token, nil
}

// CompleteConnect finishes an enrollment the connect flow returned a public token
// for: it exchanges the public token for a durable bank login and registers the
// connection, persisting the encrypted access token and seeding one account per
// provider account.
func (s *Service) CompleteConnect(ctx contextx.ContextX, publicToken string) (Connection, error) {
	item, err := s.provider.ExchangePublicToken(ctx, publicToken)
	if err != nil {
		return Connection{}, fmt.Errorf("failed to exchange public token: %w", err)
	}
	return s.RegisterConnection(ctx, item.AccessToken, item.ProviderItemID)
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
	accessToken, err := s.connectionAccessToken(ctx, conn.ID)
	if err != nil {
		return err
	}

	providerAccounts, balances, err := s.fetchAccountsAndBalances(ctx, accessToken)
	if err != nil {
		if errors.Is(err, banking.ErrReauthRequired) {
			return s.repo().SetConnectionState(ctx, conn.ID, ConnectionNeedsReconnect)
		}
		return err
	}

	return s.refreshConnectionAccounts(ctx, conn, providerAccounts, balances)
}

// connectionAccessToken loads a connection's stored token and decrypts it to the
// plaintext the provider seam is called with. The plaintext is never persisted
// unencrypted nor sent to the client; it may be handed to the in-process
// transactions sync via ConnectionsToSync.
func (s *Service) connectionAccessToken(ctx contextx.ContextX, connectionID string) (string, error) {
	encryptedToken, err := s.repo().GetEncryptedToken(ctx, connectionID)
	if err != nil {
		return "", fmt.Errorf("failed to load access token: %w", err)
	}
	accessToken, err := cryptox.SymmetricDecrypt(encryptedToken, s.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt access token: %w", err)
	}
	return accessToken, nil
}

// fetchAccountsAndBalances reads a login's current accounts and balances through
// the provider seam. A re-auth signal is returned as banking.ErrReauthRequired
// (via errors.Is) so callers can decide whether to flag the connection or
// surface the failure.
func (s *Service) fetchAccountsAndBalances(ctx contextx.ContextX, accessToken string) ([]banking.Account, []banking.Balance, error) {
	providerAccounts, err := s.provider.ListAccounts(ctx, accessToken)
	if err != nil {
		if errors.Is(err, banking.ErrReauthRequired) {
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("failed to list provider accounts: %w", err)
	}

	balances, err := s.provider.GetBalances(ctx, accessToken)
	if err != nil {
		if errors.Is(err, banking.ErrReauthRequired) {
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("failed to get provider balances: %w", err)
	}
	return providerAccounts, balances, nil
}

// refreshConnectionAccounts writes a connection's discovered accounts back in
// one transaction: it updates each existing account's balance and last-synced
// stamp, seeds any newly appearing account, and reactivates a connection that
// was not already active. It never duplicates an account nor reseeds an existing
// account's kind/counts-as-savings.
func (s *Service) refreshConnectionAccounts(ctx contextx.ContextX, conn Connection, providerAccounts []banking.Account, balances []banking.Balance) error {
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
				// Backfill the mask for accounts stored before it was captured;
				// it is stable bank data, so refresh it whenever the provider
				// reports one (guarded so an absent mask never clobbers a stored one).
				if pa.Mask != "" {
					current.Mask = pa.Mask
				}
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

// ConnectionSyncTarget names a connection the transactions sync must pull from,
// carrying its id, its decrypted access token, and the map from provider account
// id to internal account id needed to attribute pulled transactions to stored
// accounts. The token is decrypted inside accounts (the field's owner) and
// handed to the in-process sync; it is never persisted unencrypted nor sent to
// the client.
type ConnectionSyncTarget struct {
	ConnectionID        string
	AccessToken         string
	AccountIDByProvider map[string]string
}

// ConnectionsToSync returns the connections the transactions sync should pull
// from — the active and needs-reconnect ones (a disconnected connection is
// terminal and skipped). Each target carries the connection's decrypted access
// token and its provider→internal account id map. A needs-reconnect connection
// is included so the sync can retry it; if the provider still rejects the login
// the pull surfaces banking.ErrReauthRequired and the caller skips it.
func (s *Service) ConnectionsToSync(ctx contextx.ContextX) ([]ConnectionSyncTarget, error) {
	connections, err := s.repo().ListConnections(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list connections: %w", err)
	}

	var targets []ConnectionSyncTarget
	for _, conn := range connections {
		if conn.State != ConnectionActive && conn.State != ConnectionNeedsReconnect {
			continue
		}

		accessToken, err := s.connectionAccessToken(ctx, conn.ID)
		if err != nil {
			return nil, err
		}

		accountsForConn, err := s.repo().ListAccountsByConnection(ctx, conn.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to list connection accounts: %w", err)
		}
		accountIDByProvider := make(map[string]string, len(accountsForConn))
		for _, a := range accountsForConn {
			accountIDByProvider[a.ProviderAccountID] = a.ID
		}

		targets = append(targets, ConnectionSyncTarget{
			ConnectionID:        conn.ID,
			AccessToken:         accessToken,
			AccountIDByProvider: accountIDByProvider,
		})
	}
	return targets, nil
}

// Disconnect removes a linked bank: it decrypts the connection's token and
// severs the login at the provider, then in one transaction deletes the
// connection's accounts and the connection itself. After it returns the bank is
// gone from Two Cents entirely (accounts and history included).
func (s *Service) Disconnect(ctx contextx.ContextX, connectionID string) error {
	accessToken, err := s.connectionAccessToken(ctx, connectionID)
	if err != nil {
		return err
	}

	if err := s.provider.RemoveItem(ctx, accessToken); err != nil {
		return fmt.Errorf("failed to remove provider item: %w", err)
	}

	return s.db.WithTx(func(tx *db.DB) error {
		repo := NewRepo(tx.Queries())
		if err := repo.DeleteAccountsByConnection(ctx, connectionID); err != nil {
			return fmt.Errorf("failed to delete connection accounts: %w", err)
		}
		if err := repo.DeleteConnection(ctx, connectionID); err != nil {
			return fmt.Errorf("failed to delete connection: %w", err)
		}
		return nil
	})
}

// BeginReconnect mints an update-mode link token for a connection whose login
// expired: it decrypts the connection's token and asks the provider for a token
// that reconnects that existing login (rather than enrolling a new bank). Used
// by real mode only — the front end hands the token to the provider's update
// flow.
func (s *Service) BeginReconnect(ctx contextx.ContextX, connectionID string) (banking.LinkToken, error) {
	accessToken, err := s.connectionAccessToken(ctx, connectionID)
	if err != nil {
		return banking.LinkToken{}, err
	}

	token, err := s.provider.CreateLinkToken(ctx, banking.LinkOptions{AccessToken: accessToken})
	if err != nil {
		return banking.LinkToken{}, fmt.Errorf("failed to create relink token: %w", err)
	}
	return token, nil
}

// CompleteReconnect confirms a refreshed login works and clears the
// needs-reconnect flag: it decrypts the connection's token, reads the login's
// accounts and balances through the provider, refreshes the connection's
// accounts, and sets it active. If the provider still rejects the login (any
// error, including banking.ErrReauthRequired), it returns the error and leaves
// the connection needs-reconnect so the badge stays and the failure surfaces.
func (s *Service) CompleteReconnect(ctx contextx.ContextX, connectionID string) error {
	conn, err := s.repo().GetConnection(ctx, connectionID)
	if err != nil {
		return fmt.Errorf("failed to load connection: %w", err)
	}

	accessToken, err := s.connectionAccessToken(ctx, connectionID)
	if err != nil {
		return err
	}

	providerAccounts, balances, err := s.fetchAccountsAndBalances(ctx, accessToken)
	if err != nil {
		return fmt.Errorf("failed to reconnect bank: %w", err)
	}

	return s.refreshConnectionAccounts(ctx, conn, providerAccounts, balances)
}

// ErrInvalidKind is returned when SetAccountKind is given a value outside the
// three buckets. The kind picker only offers valid values, so this guards
// crafted requests; the adapter maps it to a 400.
var ErrInvalidKind = errors.New("invalid account kind")

// ErrSavingsNotApplicable is returned when ToggleCountsAsSavings targets a credit
// account, where the flag is meaningless: a transfer into a credit account is a
// credit-card payment, never a savings contribution (ADR-0008). The toggle is
// withheld from credit rows, so this guards crafted requests.
var ErrSavingsNotApplicable = errors.New("counts-as-savings does not apply to a credit account")

// SetAccountKind overrides an account's spending bucket to the user's choice and
// marks it overridden so a later sync never reseeds it. Choosing credit also
// force-clears counts-as-savings — a credit destination is never a savings
// contribution, and the transfer-subtype engine assumes the flag is false there
// (ADR-0008); that is the one coupling between the otherwise-orthogonal axes. It
// reports whether the effective counts-as-savings value changed, so the adapter
// can re-pair existing transfers without this service reaching into transactions.
func (s *Service) SetAccountKind(ctx contextx.ContextX, accountID string, kind banking.AccountKind) (savingsChanged bool, err error) {
	if kind != banking.KindCash && kind != banking.KindCredit && kind != banking.KindOther {
		return false, ErrInvalidKind
	}

	account, err := s.repo().GetAccount(ctx, accountID)
	if err != nil {
		return false, fmt.Errorf("failed to load account: %w", err)
	}

	account.Kind = kind
	account.KindOverridden = true
	if kind == banking.KindCredit && account.CountsAsSavings {
		account.CountsAsSavings = false
		account.SavingsOverridden = true
		savingsChanged = true
	}

	if _, err := s.repo().UpdateAccount(ctx, account); err != nil {
		return false, fmt.Errorf("failed to update account kind: %w", err)
	}
	return savingsChanged, nil
}

// ToggleCountsAsSavings flips an account's counts-as-savings flag and marks it
// overridden. It rejects a credit account, where the flag is meaningless. The
// flag always changes, so it always reports true; the adapter re-pairs existing
// transfers on the back of that (the flag drives savings-contribution resolution).
func (s *Service) ToggleCountsAsSavings(ctx contextx.ContextX, accountID string) (savingsChanged bool, err error) {
	account, err := s.repo().GetAccount(ctx, accountID)
	if err != nil {
		return false, fmt.Errorf("failed to load account: %w", err)
	}
	if account.Kind == banking.KindCredit {
		return false, ErrSavingsNotApplicable
	}

	account.CountsAsSavings = !account.CountsAsSavings
	account.SavingsOverridden = true
	if _, err := s.repo().UpdateAccount(ctx, account); err != nil {
		return false, fmt.Errorf("failed to toggle counts-as-savings: %w", err)
	}
	return true, nil
}

// maxCustomNameLen caps a user-set account name; longer input is truncated to
// this many characters (silently, no error surfaced).
const maxCustomNameLen = 60

// SetAccountName sets or clears an account's custom display name. The input is
// trimmed and capped at maxCustomNameLen; an empty result clears the override
// (custom_name back to NULL), reverting the account to its bank name. It touches
// neither kind nor counts-as-savings, so it fires no transfer re-pair.
func (s *Service) SetAccountName(ctx contextx.ContextX, accountID, name string) error {
	name = strings.TrimSpace(name)
	if r := []rune(name); len(r) > maxCustomNameLen {
		name = string(r[:maxCustomNameLen])
	}
	var custom *string
	if name != "" {
		custom = &name
	}
	if _, err := s.repo().SetAccountCustomName(ctx, accountID, custom); err != nil {
		return fmt.Errorf("failed to set account name: %w", err)
	}
	return nil
}

// HideAccount drops an account from the overview and the pickers by marking it
// hidden, while its transactions keep counting in the tracker and wraps (hiding
// is a display choice, never a rewrite of money that moved). Reversible via
// UnhideAccount; an account is never hard-deleted by hiding.
func (s *Service) HideAccount(ctx contextx.ContextX, accountID string) error {
	return s.setAccountState(ctx, accountID, AccountHidden)
}

// UnhideAccount returns a hidden account to active, the reverse of HideAccount.
func (s *Service) UnhideAccount(ctx contextx.ContextX, accountID string) error {
	return s.setAccountState(ctx, accountID, AccountActive)
}

// setAccountState loads an account, sets its display state, and persists it. It
// backs the hide/unhide operations, which only move the account between the
// active and hidden display states; closing is driven by disconnect, not here.
func (s *Service) setAccountState(ctx contextx.ContextX, accountID string, state AccountState) error {
	account, err := s.repo().GetAccount(ctx, accountID)
	if err != nil {
		return fmt.Errorf("failed to load account: %w", err)
	}
	account.State = state
	if _, err := s.repo().UpdateAccount(ctx, account); err != nil {
		return fmt.Errorf("failed to update account state: %w", err)
	}
	return nil
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

// ConnectedAccountFacets returns the pairing facets for every active account:
// its internal id, display name, spending bucket, and counts-as-savings flag. It
// is the read-only seam the transactions transfer pairing pass uses to learn a
// transfer's destination account and derive its subtype; accounts owns these
// rows and writes nothing here. Hidden and closed accounts are excluded — they
// leave the overview and the pickers together (a hidden account can't be the
// manual destination of a new transfer).
func (s *Service) ConnectedAccountFacets(ctx contextx.ContextX) ([]AccountFacet, error) {
	accounts, err := s.repo().ListAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list accounts: %w", err)
	}
	facets := make([]AccountFacet, 0, len(accounts))
	for _, a := range accounts {
		if a.State != AccountActive {
			continue
		}
		facets = append(facets, AccountFacet{
			ID:              a.ID,
			Name:            a.DisplayName(),
			Kind:            a.Kind,
			CountsAsSavings: a.CountsAsSavings,
		})
	}
	return facets, nil
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
			if a.CountsAsSavings {
				overview.TotalSavings += a.Balance.Money.Amount
			}
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
	overview.FreeCash = overview.NetCash - overview.TotalSavings
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
		Mask:              pa.Mask,
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
