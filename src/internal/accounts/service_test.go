package accounts

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/cryptox"
	"github.com/alecdray/two-cents/src/internal/core/db"

	"github.com/pressly/goose/v3"

	_ "github.com/mattn/go-sqlite3"
)

// testKey is a valid 32-byte (AES-256) hex key for cryptox in tests.
const testKey = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"

// fakeProvider is an in-package banking.BankProvider stand-in. Each call reads
// the currently configured accounts/balances, and an armed reauth error is
// returned (once) to exercise the needs-reconnect transition.
type fakeProvider struct {
	accounts     []banking.Account
	balances     []banking.Balance
	reauthOnNext bool
	// lastAccessToken records the token the service last called the provider
	// with, so a test can observe the decrypted value flowing through the seam.
	lastAccessToken string
}

func (f *fakeProvider) ListAccounts(_ contextx.ContextX, accessToken string) ([]banking.Account, error) {
	f.lastAccessToken = accessToken
	if f.reauthOnNext {
		f.reauthOnNext = false
		return nil, banking.ErrReauthRequired
	}
	return f.accounts, nil
}

func (f *fakeProvider) GetBalances(_ contextx.ContextX, accessToken string) ([]banking.Balance, error) {
	f.lastAccessToken = accessToken
	if f.balances != nil {
		return f.balances, nil
	}
	out := make([]banking.Balance, len(f.accounts))
	for i, a := range f.accounts {
		out[i] = a.Balance
	}
	return out, nil
}

func (f *fakeProvider) SyncTransactions(_ contextx.ContextX, _, _ string) (banking.TransactionChanges, error) {
	return banking.TransactionChanges{}, nil
}

func newTestDB(t *testing.T) *db.DB {
	t.Helper()

	migrationsDir, err := filepath.Abs("../../../db/migrations")
	if err != nil {
		t.Fatalf("resolve migrations dir: %v", err)
	}

	dbPath := filepath.Join(t.TempDir(), "test.db")
	sqlDB, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("set dialect: %v", err)
	}
	if err := goose.Up(sqlDB, migrationsDir); err != nil {
		t.Fatalf("goose up: %v", err)
	}
	return db.WrapSqlDB(sqlDB)
}

func testCtx() contextx.ContextX {
	return contextx.NewContextX(context.Background())
}

func knownBalance(accountID string, amount float64) banking.Balance {
	return banking.Balance{
		AccountID: accountID,
		Known:     true,
		Money:     banking.Money{Amount: amount, Currency: "USD"},
	}
}

func providerAccount(id, name string, kind banking.AccountKind, savings bool, balance banking.Balance) banking.Account {
	return banking.Account{
		ID:              id,
		Name:            name,
		Kind:            kind,
		CountsAsSavings: savings,
		Balance:         balance,
	}
}

// providerLabelledAccount is providerAccount plus the bank's type/subtype label
// strings, used where a test asserts the subtype flows through as the bank_type
// display label.
func providerLabelledAccount(id, name string, kind banking.AccountKind, bankType, subtype string, savings bool, balance banking.Balance) banking.Account {
	a := providerAccount(id, name, kind, savings, balance)
	a.Type = bankType
	a.Subtype = subtype
	return a
}

// --- RegisterConnection ---

func TestRegisterConnection(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	provider := &fakeProvider{accounts: []banking.Account{
		providerAccount("p-check", "Checking", banking.KindCash, false, knownBalance("p-check", 500)),
		providerAccount("p-save", "Savings", banking.KindCash, true, knownBalance("p-save", 1500)),
		providerAccount("p-card", "Credit Card", banking.KindCredit, false, knownBalance("p-card", 300)),
	}}

	const accessToken = "access-token-plaintext"
	svc := NewService(database, provider, testKey)

	conn, err := svc.RegisterConnection(ctx, accessToken, "item-123")
	if err != nil {
		t.Fatalf("RegisterConnection: %v", err)
	}

	t.Run("one active connection is stored", func(t *testing.T) {
		conns, err := svc.repo().ListConnections(ctx)
		if err != nil {
			t.Fatalf("list connections: %v", err)
		}
		if len(conns) != 1 {
			t.Fatalf("expected 1 connection, got %d", len(conns))
		}
		if conns[0].State != ConnectionActive {
			t.Errorf("state = %q, want active", conns[0].State)
		}
		if conns[0].ProviderItemID != "item-123" {
			t.Errorf("item id = %q, want item-123", conns[0].ProviderItemID)
		}
	})

	t.Run("one account per provider account with seeded kind, savings, balance", func(t *testing.T) {
		accounts, err := svc.repo().ListAccountsByConnection(ctx, conn.ID)
		if err != nil {
			t.Fatalf("list accounts: %v", err)
		}
		if len(accounts) != 3 {
			t.Fatalf("expected 3 accounts, got %d", len(accounts))
		}
		byProvider := map[string]Account{}
		for _, a := range accounts {
			byProvider[a.ProviderAccountID] = a
		}

		card := byProvider["p-card"]
		if card.Kind != banking.KindCredit {
			t.Errorf("card kind = %q, want credit", card.Kind)
		}
		if card.CountsAsSavings {
			t.Errorf("card should not count as savings")
		}

		save := byProvider["p-save"]
		if save.Kind != banking.KindCash {
			t.Errorf("savings kind = %q, want cash", save.Kind)
		}
		if !save.CountsAsSavings {
			t.Errorf("savings should count as savings")
		}
		if !save.Balance.Known || save.Balance.Money.Amount != 1500 {
			t.Errorf("savings balance = %+v, want known 1500", save.Balance)
		}

		check := byProvider["p-check"]
		if check.Kind != banking.KindCash || check.CountsAsSavings {
			t.Errorf("checking should be cash, non-savings; got kind=%q savings=%v", check.Kind, check.CountsAsSavings)
		}
	})

	t.Run("stored token is encrypted and decrypts back to the plaintext", func(t *testing.T) {
		stored, err := svc.repo().GetEncryptedToken(ctx, conn.ID)
		if err != nil {
			t.Fatalf("get token: %v", err)
		}
		if stored == accessToken {
			t.Fatalf("stored token is the plaintext; it must be encrypted")
		}
		got, err := cryptox.SymmetricDecrypt(stored, testKey)
		if err != nil {
			t.Fatalf("decrypt: %v", err)
		}
		if got != accessToken {
			t.Errorf("decrypted token = %q, want %q", got, accessToken)
		}
	})
}

// --- SyncAccounts: refresh + discover, idempotent ---

func TestSyncAccountsRefreshesAndDiscovers(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	provider := &fakeProvider{accounts: []banking.Account{
		providerAccount("p-check", "Checking", banking.KindCash, false, knownBalance("p-check", 500)),
	}}
	svc := NewService(database, provider, testKey)

	conn, err := svc.RegisterConnection(ctx, "tok", "item-123")
	if err != nil {
		t.Fatalf("RegisterConnection: %v", err)
	}

	// User overrides the checking account's kind so we can prove no reseed.
	before, err := svc.repo().ListAccountsByConnection(ctx, conn.ID)
	if err != nil {
		t.Fatalf("list before: %v", err)
	}
	overridden := before[0]
	overridden.Kind = banking.KindCredit
	overridden.KindOverridden = true
	overridden.CountsAsSavings = true
	overridden.SavingsOverridden = true
	if _, err := svc.repo().UpdateAccount(ctx, overridden); err != nil {
		t.Fatalf("override account: %v", err)
	}

	// Next sync: checking balance changed + a new savings account appears.
	provider.accounts = []banking.Account{
		providerAccount("p-check", "Checking", banking.KindCash, false, knownBalance("p-check", 750)),
		providerAccount("p-save", "Savings", banking.KindCash, true, knownBalance("p-save", 2000)),
	}

	if err := svc.SyncAccounts(ctx); err != nil {
		t.Fatalf("SyncAccounts: %v", err)
	}

	accounts, err := svc.repo().ListAccountsByConnection(ctx, conn.ID)
	if err != nil {
		t.Fatalf("list after: %v", err)
	}
	if len(accounts) != 2 {
		t.Fatalf("expected 2 accounts after sync (no duplicates), got %d", len(accounts))
	}
	byProvider := map[string]Account{}
	for _, a := range accounts {
		byProvider[a.ProviderAccountID] = a
	}

	check := byProvider["p-check"]
	if check.Balance.Money.Amount != 750 {
		t.Errorf("checking balance = %v, want refreshed 750", check.Balance.Money.Amount)
	}
	if check.LastSyncedAt == nil {
		t.Errorf("checking last-synced should be set")
	}
	if check.Kind != banking.KindCredit || !check.CountsAsSavings {
		t.Errorf("overridden account was reseeded: kind=%q savings=%v", check.Kind, check.CountsAsSavings)
	}

	save := byProvider["p-save"]
	if save.Kind != banking.KindCash || !save.CountsAsSavings {
		t.Errorf("new savings account not seeded correctly: kind=%q savings=%v", save.Kind, save.CountsAsSavings)
	}
	if save.Balance.Money.Amount != 2000 {
		t.Errorf("new savings balance = %v, want 2000", save.Balance.Money.Amount)
	}
}

// --- needs-reconnect transition ---

func TestSyncFlipsToNeedsReconnectThenBack(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	provider := &fakeProvider{accounts: []banking.Account{
		providerAccount("p-check", "Checking", banking.KindCash, false, knownBalance("p-check", 500)),
	}}
	svc := NewService(database, provider, testKey)

	conn, err := svc.RegisterConnection(ctx, "tok", "item-123")
	if err != nil {
		t.Fatalf("RegisterConnection: %v", err)
	}

	// Provider reports re-auth required on the next sync.
	provider.reauthOnNext = true
	if err := svc.SyncAccounts(ctx); err != nil {
		t.Fatalf("SyncAccounts (reauth): %v", err)
	}

	conns, err := svc.repo().ListConnections(ctx)
	if err != nil {
		t.Fatalf("list connections: %v", err)
	}
	if conns[0].State != ConnectionNeedsReconnect {
		t.Fatalf("state = %q, want needs_reconnect", conns[0].State)
	}
	accounts, err := svc.repo().ListAccountsByConnection(ctx, conn.ID)
	if err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	if len(accounts) != 1 {
		t.Errorf("accounts should be retained through reauth; got %d", len(accounts))
	}

	// A subsequent clean sync returns the connection to active.
	if err := svc.SyncAccounts(ctx); err != nil {
		t.Fatalf("SyncAccounts (clean): %v", err)
	}
	conns, err = svc.repo().ListConnections(ctx)
	if err != nil {
		t.Fatalf("list connections: %v", err)
	}
	if conns[0].State != ConnectionActive {
		t.Errorf("state = %q, want active after clean sync", conns[0].State)
	}
}

// --- Overview totals ---

func TestComputeOverview(t *testing.T) {
	unknown := banking.Balance{AccountID: "x", Known: false}

	accounts := []Account{
		{Kind: banking.KindCash, State: AccountActive, Balance: knownBalance("a", 1000)},    // cash
		{Kind: banking.KindCash, State: AccountActive, Balance: knownBalance("b", 1500)},    // savings cash
		{Kind: banking.KindCredit, State: AccountActive, Balance: knownBalance("c", 400)},   // debt
		{Kind: banking.KindCredit, State: AccountActive, Balance: knownBalance("d", 100)},   // debt
		{Kind: banking.KindOther, State: AccountActive, Balance: knownBalance("g", 250000)}, // excluded (other: loan)
		{Kind: banking.KindOther, State: AccountActive, Balance: knownBalance("h", 32000)},  // excluded (other: investment)
		{Kind: banking.KindCash, State: AccountHidden, Balance: knownBalance("e", 9999)},    // excluded (hidden)
		{Kind: banking.KindCash, State: AccountClosed, Balance: knownBalance("f", 8888)},    // excluded (closed)
		{Kind: banking.KindCash, State: AccountActive, Balance: unknown},                    // excluded (unknown)
		{Kind: banking.KindCredit, State: AccountActive, Balance: unknown},                  // excluded (unknown)
	}

	ov := computeOverview(accounts)

	if ov.TotalCash != 2500 {
		t.Errorf("total cash = %v, want 2500", ov.TotalCash)
	}
	if ov.TotalDebt != 500 {
		t.Errorf("total debt = %v, want 500", ov.TotalDebt)
	}
	if ov.NetCash != 2000 {
		t.Errorf("net cash = %v, want 2000 (2500 - 500); other-bucket accounts must be excluded", ov.NetCash)
	}
}

func TestOverviewExcludesUnknownNotAsZero(t *testing.T) {
	// One known cash account and one unknown — net must reflect only the known.
	accounts := []Account{
		{Kind: banking.KindCash, State: AccountActive, Balance: knownBalance("a", 750)},
		{Kind: banking.KindCash, State: AccountActive, Balance: banking.Balance{Known: false}},
	}
	ov := computeOverview(accounts)
	if ov.TotalCash != 750 || ov.NetCash != 750 {
		t.Errorf("unknown balance must be excluded, not zero; got cash=%v net=%v", ov.TotalCash, ov.NetCash)
	}
}
