package server_test

// Assembled cross-module tests. These wire more than one module's HTTP handlers
// together (the accounts connect handler, its BackfillTransactions hook, and the
// transactions sync handler) to prove invariants that only hold once the modules
// are composed — so they belong at the composition root, not inside one module's
// adapter test package. A domain module's adapters may not import a peer's
// adapters (docs/architecture/archetypes/domain-module.md), but the composition
// root legitimately reaches into every module's adapters, so this is its home.

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecdray/two-cents/src/internal/accounts"
	accountsAdapters "github.com/alecdray/two-cents/src/internal/accounts/adapters"
	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/categorization"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/db"
	"github.com/alecdray/two-cents/src/internal/transactions"
	txnAdapters "github.com/alecdray/two-cents/src/internal/transactions/adapters"

	"github.com/pressly/goose/v3"

	_ "github.com/mattn/go-sqlite3"
)

// testKey is a valid 32-byte (AES-256) hex key for the encryption seam.
const testKey = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"

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

// getPage drives a GET /transactions through the handler and returns the body.
func getPage(t *testing.T, txnSvc *transactions.Service, accountsSvc *accounts.Service, categorizationSvc *categorization.Service) (int, string) {
	t.Helper()
	handler := txnAdapters.NewHttpHandler(txnSvc, accountsSvc, categorizationSvc)
	req := httptest.NewRequest(http.MethodGet, "/transactions", nil)
	rec := httptest.NewRecorder()
	handler.GetTransactionsPage(rec, req)
	return rec.Code, rec.Body.String()
}

// recordingProvider is a banking.BankProvider stand-in that serves a fixed set
// of accounts and a fixed backfill, and records the order of the calls each
// service makes against it. The call log lets a test assert that the accounts
// refresh (ListAccounts/GetBalances) precedes the transaction pull on every sync
// path, and the per-method counters let a test assert a render touches the bank
// not at all. It can be armed to surface ErrReauthRequired from ListAccounts
// once, to drive a connection into needs-reconnect.
type recordingProvider struct {
	accounts     []banking.Account
	transactions []banking.Transaction

	armReauth bool

	callOrder []string
	calls     int
}

func (p *recordingProvider) ListAccounts(_ contextx.ContextX, _ string) ([]banking.Account, error) {
	p.callOrder = append(p.callOrder, "accounts")
	p.calls++
	if p.armReauth {
		p.armReauth = false
		return nil, banking.ErrReauthRequired
	}
	out := make([]banking.Account, len(p.accounts))
	copy(out, p.accounts)
	return out, nil
}

func (p *recordingProvider) GetBalances(_ contextx.ContextX, _ string) ([]banking.Balance, error) {
	p.callOrder = append(p.callOrder, "accounts")
	p.calls++
	out := make([]banking.Balance, len(p.accounts))
	for i, a := range p.accounts {
		out[i] = a.Balance
	}
	return out, nil
}

func (p *recordingProvider) SyncTransactions(_ contextx.ContextX, _, cursor string) (banking.TransactionChanges, error) {
	p.callOrder = append(p.callOrder, "transactions")
	p.calls++
	if cursor == "" {
		added := make([]banking.Transaction, len(p.transactions))
		copy(added, p.transactions)
		return banking.TransactionChanges{Added: added, Cursor: "cursor-v1"}, nil
	}
	return banking.TransactionChanges{Cursor: cursor}, nil
}

func (p *recordingProvider) CreateLinkToken(_ contextx.ContextX, _ banking.LinkOptions) (banking.LinkToken, error) {
	p.calls++
	return banking.LinkToken{Token: "link", Mode: accountsAdapters.BankModeFake}, nil
}

func (p *recordingProvider) ExchangePublicToken(_ contextx.ContextX, _ string) (banking.Item, error) {
	p.calls++
	return banking.Item{AccessToken: "access-token", ProviderItemID: "item-id"}, nil
}

func (p *recordingProvider) RemoveItem(_ contextx.ContextX, _ string) error {
	p.calls++
	return nil
}

func newRecordingProvider() *recordingProvider {
	return &recordingProvider{
		accounts: []banking.Account{
			cashProviderAccount("p-check", "Everyday Checking"),
		},
		transactions: []banking.Transaction{
			{ID: "t1", AccountID: "p-check", Amount: banking.Money{Amount: 10, Currency: "USD"}, Merchant: "Corner Market"},
			{ID: "t2", AccountID: "p-check", Amount: banking.Money{Amount: -20, Currency: "USD"}, Merchant: "Payroll"},
		},
	}
}

func cashProviderAccount(id, name string) banking.Account {
	return banking.Account{
		ID:      id,
		Name:    name,
		Kind:    banking.KindCash,
		Type:    "depository",
		Subtype: "checking",
		Balance: banking.Balance{AccountID: id, Known: true, Money: banking.Money{Amount: 100, Currency: "USD"}},
	}
}

// assertAccountsBeforeTransactions checks that within one sync pass's recorded
// calls the accounts refresh happened before the first transaction pull — the
// invariant every sync path must hold so balances and connection health are
// fresh before any transaction row is written.
func assertAccountsBeforeTransactions(t *testing.T, label string, order []string) {
	t.Helper()
	firstAccounts, firstTransactions := -1, -1
	for i, c := range order {
		if firstAccounts == -1 && c == "accounts" {
			firstAccounts = i
		}
		if firstTransactions == -1 && c == "transactions" {
			firstTransactions = i
		}
	}
	if firstAccounts == -1 {
		t.Fatalf("%s: accounts were never refreshed; call order: %v", label, order)
	}
	if firstTransactions == -1 {
		t.Fatalf("%s: transactions were never pulled; call order: %v", label, order)
	}
	if firstAccounts > firstTransactions {
		t.Errorf("%s: accounts refreshed after the transaction pull (order: %v); accounts must come first", label, order)
	}
}

// TestEverySyncPathRefreshesAccountsBeforeTransactions drives all four ways a
// sync is triggered — the connect-time backfill, the reconnect-time backfill,
// the recurring background task, and the manual "Sync now" — and asserts each
// refreshes accounts before it writes any transaction rows. They all funnel
// through Service.SyncTransactions, so this proves the one chokepoint holds the
// invariant on every path that reaches it.
func TestEverySyncPathRefreshesAccountsBeforeTransactions(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	provider := newRecordingProvider()
	accountsSvc := accounts.NewService(database, provider, testKey)
	categorizationSvc := categorization.NewService(database, nil)
	txnSvc := transactions.NewService(database, provider, accountsSvc, categorizationSvc, nil)

	backfill := func(c contextx.ContextX) error { return txnSvc.SyncTransactions(c) }
	connectHandler := accountsAdapters.NewHttpHandler(accountsSvc, accountsAdapters.BankModeFake, backfill, nil)
	txnHandler := txnAdapters.NewHttpHandler(txnSvc, accountsSvc, categorizationSvc)

	t.Run("connect-time backfill", func(t *testing.T) {
		provider.callOrder = nil
		form := url.Values{"public_token": {"any"}}
		req := httptest.NewRequest(http.MethodPost, "/accounts/connections", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		connectHandler.PostConnection(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("connect status = %d, want 200", rec.Code)
		}
		assertAccountsBeforeTransactions(t, "connect", provider.callOrder)
	})

	// Drive the connection into needs-reconnect so the reconnect path has work.
	provider.armReauth = true
	if err := accountsSvc.SyncAccounts(ctx); err != nil {
		t.Fatalf("SyncAccounts (arm reauth): %v", err)
	}
	// The assembled triggers register exactly one connection; read its id back.
	var connID string
	if err := database.Sql().QueryRow("SELECT id FROM connections").Scan(&connID); err != nil {
		t.Fatalf("read connection id: %v", err)
	}

	t.Run("reconnect-time backfill", func(t *testing.T) {
		provider.callOrder = nil
		req := httptest.NewRequest(http.MethodPost, "/accounts/connections/"+connID+"/reconnect", nil)
		req.SetPathValue("id", connID)
		rec := httptest.NewRecorder()
		connectHandler.PostReconnect(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("reconnect status = %d, want 200", rec.Code)
		}
		assertAccountsBeforeTransactions(t, "reconnect", provider.callOrder)
	})

	t.Run("background task", func(t *testing.T) {
		provider.callOrder = nil
		if err := transactions.NewSyncTask(txnSvc).Run(ctx); err != nil {
			t.Fatalf("task.Run: %v", err)
		}
		assertAccountsBeforeTransactions(t, "background task", provider.callOrder)
	})

	t.Run("manual sync now", func(t *testing.T) {
		provider.callOrder = nil
		req := httptest.NewRequest(http.MethodPost, "/transactions/sync", nil)
		rec := httptest.NewRecorder()
		txnHandler.PostSync(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("sync status = %d, want 200", rec.Code)
		}
		assertAccountsBeforeTransactions(t, "manual sync", provider.callOrder)
	})
}

// TestTransactionsRenderTouchesNoBank proves rendering /transactions reads local
// storage only — it makes no call to the bank provider. Data is first synced in
// (which does call the provider), the provider's call counter is then reset, and
// the GET render is asserted to leave it at zero.
func TestTransactionsRenderTouchesNoBank(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	provider := newRecordingProvider()
	accountsSvc := accounts.NewService(database, provider, testKey)
	categorizationSvc := categorization.NewService(database, nil)
	txnSvc := transactions.NewService(database, provider, accountsSvc, categorizationSvc, nil)

	if _, err := accountsSvc.RegisterConnection(ctx, "access-token", "item-id"); err != nil {
		t.Fatalf("RegisterConnection: %v", err)
	}
	if err := txnSvc.SyncTransactions(ctx); err != nil {
		t.Fatalf("SyncTransactions: %v", err)
	}

	// Everything from here on must read stored rows only.
	provider.calls = 0

	handler := txnAdapters.NewHttpHandler(txnSvc, accountsSvc, categorizationSvc)
	req := httptest.NewRequest(http.MethodGet, "/transactions", nil)
	rec := httptest.NewRecorder()
	handler.GetTransactionsPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `data-testid="transactions-list"`) {
		t.Fatalf("render did not show the populated list; the read setup is wrong")
	}
	if provider.calls != 0 {
		t.Errorf("rendering the page made %d bank provider call(s); the read must touch local storage only", provider.calls)
	}
}

// TestConnectThenManualSyncIsIdempotent proves the assembled idempotency the
// product rests on: a bank connected through the connect handler backfills its
// transactions, and a later manual "Sync now" over the same unchanged provider
// state produces no duplicate rows — the rendered list holds the same rows
// before and after.
func TestConnectThenManualSyncIsIdempotent(t *testing.T) {
	database := newTestDB(t)

	provider := newRecordingProvider()
	accountsSvc := accounts.NewService(database, provider, testKey)
	categorizationSvc := categorization.NewService(database, nil)
	txnSvc := transactions.NewService(database, provider, accountsSvc, categorizationSvc, nil)

	backfill := func(c contextx.ContextX) error { return txnSvc.SyncTransactions(c) }
	connectHandler := accountsAdapters.NewHttpHandler(accountsSvc, accountsAdapters.BankModeFake, backfill, nil)

	form := url.Values{"public_token": {"any"}}
	req := httptest.NewRequest(http.MethodPost, "/accounts/connections", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	connectHandler.PostConnection(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("connect status = %d, want 200", rec.Code)
	}

	txnHandler := txnAdapters.NewHttpHandler(txnSvc, accountsSvc, categorizationSvc)

	// The connect-time backfill made the rows readable.
	if _, body := getPage(t, txnSvc, accountsSvc, categorizationSvc); strings.Count(body, `data-testid="transactions-row"`) != 2 {
		t.Fatalf("after connect: row count = %d, want 2", strings.Count(body, `data-testid="transactions-row"`))
	}

	// A manual sync over the same unchanged provider state.
	syncReq := httptest.NewRequest(http.MethodPost, "/transactions/sync", nil)
	syncRec := httptest.NewRecorder()
	txnHandler.PostSync(syncRec, syncReq)
	if syncRec.Code != http.StatusOK {
		t.Fatalf("sync status = %d, want 200", syncRec.Code)
	}
	if got := strings.Count(syncRec.Body.String(), `data-testid="transactions-row"`); got != 2 {
		t.Errorf("after manual sync: row count = %d, want 2 (no duplicates)", got)
	}
}
