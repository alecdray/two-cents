package accounts

import (
	"context"
	"database/sql"
	"time"

	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/core/db/sqlc"
)

// Repo is the accounts module's data access layer. It is the only file in
// package accounts that imports core/db/sqlc; its methods take and return
// domain types (Connection, Account) — never sqlc.* shapes.
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

func connectionFromModel(m sqlc.Connection) Connection {
	return Connection{
		ID:             m.ID,
		ProviderItemID: m.ItemID,
		State:          ConnectionState(m.State),
		CreatedAt:      m.CreatedAt,
	}
}

func accountFromModel(m sqlc.Account) Account {
	a := Account{
		ID:                m.ID,
		ConnectionID:      m.ConnectionID,
		ProviderAccountID: m.ProviderAccountID,
		Name:              m.Name,
		BankType:          m.BankType,
		Kind:              banking.AccountKind(m.Kind),
		KindOverridden:    m.KindOverridden != 0,
		CountsAsSavings:   m.CountsAsSavings != 0,
		SavingsOverridden: m.SavingsOverridden != 0,
		Balance: banking.Balance{
			AccountID: m.ProviderAccountID,
			Known:     m.BalanceKnown != 0,
			Money: banking.Money{
				Amount:   m.BalanceAmount,
				Currency: m.BalanceCurrency,
			},
		},
		State: AccountState(m.State),
	}
	if m.LastSyncedAt.Valid {
		t := m.LastSyncedAt.Time
		a.LastSyncedAt = &t
	}
	return a
}

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// --- Connection queries ---

// CreateConnection persists a new connection and returns it as a domain entity.
func (r *Repo) CreateConnection(ctx context.Context, c Connection, encryptedToken string) (Connection, error) {
	model, err := r.q.CreateConnection(ctx, sqlc.CreateConnectionParams{
		ID:          c.ID,
		ItemID:      c.ProviderItemID,
		AccessToken: encryptedToken,
		State:       string(c.State),
	})
	if err != nil {
		return Connection{}, err
	}
	return connectionFromModel(model), nil
}

// ListConnections returns every connection.
func (r *Repo) ListConnections(ctx context.Context) ([]Connection, error) {
	models, err := r.q.ListConnections(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Connection, len(models))
	for i, m := range models {
		out[i] = connectionFromModel(m)
	}
	return out, nil
}

// GetConnection returns a single connection as a domain entity.
func (r *Repo) GetConnection(ctx context.Context, connectionID string) (Connection, error) {
	model, err := r.q.GetConnection(ctx, connectionID)
	if err != nil {
		return Connection{}, err
	}
	return connectionFromModel(model), nil
}

// GetEncryptedToken returns the connection's stored (encrypted) access token.
func (r *Repo) GetEncryptedToken(ctx context.Context, connectionID string) (string, error) {
	model, err := r.q.GetConnection(ctx, connectionID)
	if err != nil {
		return "", err
	}
	return model.AccessToken, nil
}

// SetConnectionState updates only a connection's state, leaving its item id and
// stored token untouched.
func (r *Repo) SetConnectionState(ctx context.Context, connectionID string, state ConnectionState) error {
	model, err := r.q.GetConnection(ctx, connectionID)
	if err != nil {
		return err
	}
	_, err = r.q.UpdateConnection(ctx, sqlc.UpdateConnectionParams{
		ID:          model.ID,
		ItemID:      model.ItemID,
		AccessToken: model.AccessToken,
		State:       string(state),
	})
	return err
}

// DeleteConnection removes a connection row. Its accounts must be removed first
// (see DeleteAccountsByConnection) to respect the connection_id foreign key.
func (r *Repo) DeleteConnection(ctx context.Context, connectionID string) error {
	return r.q.DeleteConnection(ctx, connectionID)
}

// --- Account queries ---

// CreateAccount persists a new account and returns it as a domain entity.
func (r *Repo) CreateAccount(ctx context.Context, a Account) (Account, error) {
	model, err := r.q.CreateAccount(ctx, sqlc.CreateAccountParams{
		ID:                a.ID,
		ConnectionID:      a.ConnectionID,
		ProviderAccountID: a.ProviderAccountID,
		Name:              a.Name,
		BankType:          a.BankType,
		Kind:              string(a.Kind),
		KindOverridden:    boolToInt(a.KindOverridden),
		CountsAsSavings:   boolToInt(a.CountsAsSavings),
		SavingsOverridden: boolToInt(a.SavingsOverridden),
		BalanceAmount:     a.Balance.Money.Amount,
		BalanceCurrency:   a.Balance.Money.Currency,
		BalanceKnown:      boolToInt(a.Balance.Known),
		State:             string(a.State),
		LastSyncedAt:      nullTime(a.LastSyncedAt),
	})
	if err != nil {
		return Account{}, err
	}
	return accountFromModel(model), nil
}

// UpdateAccount writes the account's mutable fields back.
func (r *Repo) UpdateAccount(ctx context.Context, a Account) (Account, error) {
	model, err := r.q.UpdateAccount(ctx, sqlc.UpdateAccountParams{
		ID:                a.ID,
		Name:              a.Name,
		BankType:          a.BankType,
		Kind:              string(a.Kind),
		KindOverridden:    boolToInt(a.KindOverridden),
		CountsAsSavings:   boolToInt(a.CountsAsSavings),
		SavingsOverridden: boolToInt(a.SavingsOverridden),
		BalanceAmount:     a.Balance.Money.Amount,
		BalanceCurrency:   a.Balance.Money.Currency,
		BalanceKnown:      boolToInt(a.Balance.Known),
		State:             string(a.State),
		LastSyncedAt:      nullTime(a.LastSyncedAt),
	})
	if err != nil {
		return Account{}, err
	}
	return accountFromModel(model), nil
}

// ListAccounts returns every account.
func (r *Repo) ListAccounts(ctx context.Context) ([]Account, error) {
	models, err := r.q.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Account, len(models))
	for i, m := range models {
		out[i] = accountFromModel(m)
	}
	return out, nil
}

// ListAccountsByConnection returns the accounts under one connection.
func (r *Repo) ListAccountsByConnection(ctx context.Context, connectionID string) ([]Account, error) {
	models, err := r.q.ListAccountsByConnection(ctx, connectionID)
	if err != nil {
		return nil, err
	}
	out := make([]Account, len(models))
	for i, m := range models {
		out[i] = accountFromModel(m)
	}
	return out, nil
}

// DeleteAccountsByConnection removes every account hanging off a connection.
func (r *Repo) DeleteAccountsByConnection(ctx context.Context, connectionID string) error {
	return r.q.DeleteAccountsByConnection(ctx, connectionID)
}

func nullTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}
