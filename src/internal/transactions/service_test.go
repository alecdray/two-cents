package transactions

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/alecdray/two-cents/src/internal/accounts"
	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/categorization"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/db"

	"github.com/pressly/goose/v3"

	_ "github.com/mattn/go-sqlite3"
)

// newCategorization builds a categorization Service over the test database (the
// migrations seed the built-in taxonomy), with no re-categorization seam — the
// transactions sync only reads it to resolve each row.
func newCategorization(database *db.DB) *categorization.Service {
	return categorization.NewService(database, nil)
}

// testKey is a valid 32-byte (AES-256) hex key for cryptox in tests; the
// accounts service encrypts stored tokens under it.
const testKey = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"

// stubProvider is an in-package banking.BankProvider stand-in for the
// transactions tests. It serves account listings/balances per access token (so
// a real accounts.Service can register and sync against it) and scripts each
// connection's transaction pull as a function of the presented cursor — the
// realistic cursor model where an empty cursor backfills and the returned cursor
// resumes. It records the call order and the cursors presented so tests can
// assert accounts-first ordering and cursor resume.
type stubProvider struct {
	accountsByToken map[string][]banking.Account
	// syncByToken maps an access token to its pull behaviour: given the presented
	// cursor it returns the changes for that connection. Absent → no changes.
	syncByToken map[string]func(cursor string) (banking.TransactionChanges, error)
	// reauthSyncTokens marks tokens whose pull must report ErrReauthRequired.
	reauthSyncTokens map[string]bool

	callOrder      []string
	cursorsByToken map[string][]string
	syncWasCalled  bool
}

func newStub() *stubProvider {
	return &stubProvider{
		accountsByToken:  map[string][]banking.Account{},
		syncByToken:      map[string]func(string) (banking.TransactionChanges, error){},
		reauthSyncTokens: map[string]bool{},
		cursorsByToken:   map[string][]string{},
	}
}

func (s *stubProvider) ListAccounts(_ contextx.ContextX, accessToken string) ([]banking.Account, error) {
	s.callOrder = append(s.callOrder, "list:"+accessToken)
	return s.accountsByToken[accessToken], nil
}

func (s *stubProvider) GetBalances(_ contextx.ContextX, accessToken string) ([]banking.Balance, error) {
	s.callOrder = append(s.callOrder, "balances:"+accessToken)
	accts := s.accountsByToken[accessToken]
	out := make([]banking.Balance, len(accts))
	for i, a := range accts {
		out[i] = a.Balance
	}
	return out, nil
}

func (s *stubProvider) SyncTransactions(_ contextx.ContextX, accessToken, cursor string) (banking.TransactionChanges, error) {
	s.callOrder = append(s.callOrder, "sync:"+accessToken)
	s.syncWasCalled = true
	s.cursorsByToken[accessToken] = append(s.cursorsByToken[accessToken], cursor)
	if s.reauthSyncTokens[accessToken] {
		return banking.TransactionChanges{}, banking.ErrReauthRequired
	}
	if fn, ok := s.syncByToken[accessToken]; ok {
		return fn(cursor)
	}
	return banking.TransactionChanges{Cursor: cursor}, nil
}

func (s *stubProvider) CreateLinkToken(_ contextx.ContextX, _ banking.LinkOptions) (banking.LinkToken, error) {
	return banking.LinkToken{}, nil
}

func (s *stubProvider) ExchangePublicToken(_ contextx.ContextX, _ string) (banking.Item, error) {
	return banking.Item{}, nil
}

func (s *stubProvider) RemoveItem(_ contextx.ContextX, _ string) error { return nil }

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

func cashAccount(providerID, name string) banking.Account {
	return banking.Account{
		ID:      providerID,
		Name:    name,
		Kind:    banking.KindCash,
		Type:    "depository",
		Subtype: "checking",
		Balance: banking.Balance{AccountID: providerID, Known: true, Money: banking.Money{Amount: 100, Currency: "USD"}},
	}
}

func txnDate(day int) time.Time {
	return time.Date(2026, time.June, day, 0, 0, 0, 0, time.UTC)
}

// bankTxn builds a seam transaction with the documented sign convention: a
// positive amount is an outflow, a negative amount an inflow.
func bankTxn(id, providerAccountID string, day int, amount float64, pending bool) banking.Transaction {
	return banking.Transaction{
		ID:           id,
		AccountID:    providerAccountID,
		Date:         txnDate(day),
		Amount:       banking.Money{Amount: amount, Currency: "USD"},
		Merchant:     "Merchant " + id,
		Counterparty: "RAW " + id,
		Category:     banking.Category{Primary: "GENERAL_MERCHANDISE", Detailed: "GENERAL_MERCHANDISE_OTHER"},
		Pending:      pending,
	}
}

// registerConnection wires a connection (with its accounts) into the accounts
// service the way a real connect flow would, so the transactions sync has
// something to pull for. It returns the connection id.
func registerConnection(t *testing.T, svc *accounts.Service, accessToken, itemID string) string {
	t.Helper()
	conn, err := svc.RegisterConnection(testCtx(), accessToken, itemID)
	if err != nil {
		t.Fatalf("RegisterConnection: %v", err)
	}
	return conn.ID
}

// providerToInternal maps the registered accounts' provider ids to their stored
// internal ids, so a test can read back the rows the sync attributed to them.
func providerToInternal(t *testing.T, accountsSvc *accounts.Service) map[string]string {
	t.Helper()
	targets, err := accountsSvc.ConnectionsToSync(testCtx())
	if err != nil {
		t.Fatalf("ConnectionsToSync: %v", err)
	}
	merged := map[string]string{}
	for _, tg := range targets {
		for provID, internalID := range tg.AccountIDByProvider {
			merged[provID] = internalID
		}
	}
	return merged
}

func countTransactions(t *testing.T, database *db.DB) int {
	t.Helper()
	var n int
	if err := database.Sql().QueryRow("SELECT COUNT(*) FROM transactions").Scan(&n); err != nil {
		t.Fatalf("count transactions: %v", err)
	}
	return n
}

// --- A. Apply an incremental pull's changes, keyed by provider id ---

func TestSyncPersistsPulledChanges(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	const token = "tok-a"
	provider := newStub()
	provider.accountsByToken[token] = []banking.Account{
		cashAccount("p-check", "Everyday Checking"),
		cashAccount("p-card", "Rewards Card"),
	}
	provider.syncByToken[token] = func(cursor string) (banking.TransactionChanges, error) {
		if cursor == "" {
			return banking.TransactionChanges{
				Added: []banking.Transaction{
					bankTxn("t-outflow", "p-check", 1, 84.32, false),   // posted outflow
					bankTxn("t-inflow", "p-check", 2, -2400.00, false), // posted inflow
					bankTxn("t-pending", "p-card", 3, 5.75, true),      // pending outflow
				},
				Cursor: "c1",
			}, nil
		}
		return banking.TransactionChanges{Cursor: cursor}, nil
	}

	accountsSvc := accounts.NewService(database, provider, testKey)
	registerConnection(t, accountsSvc, token, "item-a")
	svc := NewService(database, provider, accountsSvc, newCategorization(database))

	if err := svc.SyncTransactions(ctx); err != nil {
		t.Fatalf("SyncTransactions: %v", err)
	}

	if got := countTransactions(t, database); got != 3 {
		t.Fatalf("stored %d transactions, want 3", got)
	}

	internal := providerToInternal(t, accountsSvc)

	t.Run("a row is attributed to its account's internal id with the signed amount and status", func(t *testing.T) {
		var accountID, status string
		var amount float64
		row := database.Sql().QueryRow("SELECT account_id, amount_amount, status FROM transactions WHERE id = ?", "t-inflow")
		if err := row.Scan(&accountID, &amount, &status); err != nil {
			t.Fatalf("read row: %v", err)
		}
		if accountID != internal["p-check"] {
			t.Errorf("account_id = %q, want the internal id %q for p-check", accountID, internal["p-check"])
		}
		if amount != -2400.00 {
			t.Errorf("amount = %v, want -2400 (inflow stored signed as-is)", amount)
		}
		if status != "posted" {
			t.Errorf("status = %q, want posted", status)
		}
	})

	t.Run("a pending row stores pending status", func(t *testing.T) {
		var status string
		row := database.Sql().QueryRow("SELECT status FROM transactions WHERE id = ?", "t-pending")
		if err := row.Scan(&status); err != nil {
			t.Fatalf("read row: %v", err)
		}
		if status != "pending" {
			t.Errorf("status = %q, want pending", status)
		}
	})
}

func TestSyncIsIdempotent(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	const token = "tok-a"
	provider := newStub()
	provider.accountsByToken[token] = []banking.Account{cashAccount("p-check", "Checking")}
	provider.syncByToken[token] = func(cursor string) (banking.TransactionChanges, error) {
		if cursor == "" {
			return banking.TransactionChanges{
				Added:  []banking.Transaction{bankTxn("t1", "p-check", 1, 10, false), bankTxn("t2", "p-check", 2, 20, false)},
				Cursor: "c1",
			}, nil
		}
		// No provider change on resume.
		return banking.TransactionChanges{Cursor: cursor}, nil
	}

	accountsSvc := accounts.NewService(database, provider, testKey)
	registerConnection(t, accountsSvc, token, "item-a")
	svc := NewService(database, provider, accountsSvc, newCategorization(database))

	if err := svc.SyncTransactions(ctx); err != nil {
		t.Fatalf("SyncTransactions (first): %v", err)
	}
	if err := svc.SyncTransactions(ctx); err != nil {
		t.Fatalf("SyncTransactions (second): %v", err)
	}

	if got := countTransactions(t, database); got != 2 {
		t.Errorf("stored %d transactions after re-sync, want 2 (no duplicates)", got)
	}
}

func TestSyncModifiedUpdatesInPlace(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	const token = "tok-a"
	provider := newStub()
	provider.accountsByToken[token] = []banking.Account{cashAccount("p-check", "Checking")}
	provider.syncByToken[token] = func(cursor string) (banking.TransactionChanges, error) {
		switch cursor {
		case "":
			return banking.TransactionChanges{
				Added:  []banking.Transaction{bankTxn("t1", "p-check", 1, 10, true)}, // pending
				Cursor: "c1",
			}, nil
		case "c1":
			modified := bankTxn("t1", "p-check", 1, 12.50, false) // same id, now posted, amount shifted
			return banking.TransactionChanges{Modified: []banking.Transaction{modified}, Cursor: "c2"}, nil
		default:
			return banking.TransactionChanges{Cursor: cursor}, nil
		}
	}

	accountsSvc := accounts.NewService(database, provider, testKey)
	registerConnection(t, accountsSvc, token, "item-a")
	svc := NewService(database, provider, accountsSvc, newCategorization(database))

	if err := svc.SyncTransactions(ctx); err != nil {
		t.Fatalf("SyncTransactions (first): %v", err)
	}
	if err := svc.SyncTransactions(ctx); err != nil {
		t.Fatalf("SyncTransactions (second): %v", err)
	}

	if got := countTransactions(t, database); got != 1 {
		t.Fatalf("stored %d transactions, want 1 (update in place, no duplicate)", got)
	}
	var status string
	var amount float64
	row := database.Sql().QueryRow("SELECT status, amount_amount FROM transactions WHERE id = ?", "t1")
	if err := row.Scan(&status, &amount); err != nil {
		t.Fatalf("read row: %v", err)
	}
	if status != "posted" {
		t.Errorf("status = %q, want posted after the modified pull", status)
	}
	if amount != 12.50 {
		t.Errorf("amount = %v, want 12.50 after the modified pull", amount)
	}
}

func TestSyncRemovesDeletedIDs(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	const token = "tok-a"
	provider := newStub()
	provider.accountsByToken[token] = []banking.Account{cashAccount("p-check", "Checking")}
	provider.syncByToken[token] = func(cursor string) (banking.TransactionChanges, error) {
		switch cursor {
		case "":
			return banking.TransactionChanges{
				Added:  []banking.Transaction{bankTxn("t1", "p-check", 1, 10, true), bankTxn("t2", "p-check", 2, 20, false)},
				Cursor: "c1",
			}, nil
		case "c1":
			return banking.TransactionChanges{RemovedIDs: []string{"t1"}, Cursor: "c2"}, nil
		default:
			return banking.TransactionChanges{Cursor: cursor}, nil
		}
	}

	accountsSvc := accounts.NewService(database, provider, testKey)
	registerConnection(t, accountsSvc, token, "item-a")
	svc := NewService(database, provider, accountsSvc, newCategorization(database))

	if err := svc.SyncTransactions(ctx); err != nil {
		t.Fatalf("SyncTransactions (first): %v", err)
	}
	if err := svc.SyncTransactions(ctx); err != nil {
		t.Fatalf("SyncTransactions (second): %v", err)
	}

	if got := countTransactions(t, database); got != 1 {
		t.Fatalf("stored %d transactions, want 1 (t1 removed)", got)
	}
	var remaining string
	if err := database.Sql().QueryRow("SELECT id FROM transactions").Scan(&remaining); err != nil {
		t.Fatalf("read remaining: %v", err)
	}
	if remaining != "t2" {
		t.Errorf("remaining transaction = %q, want t2 (t1 should be deleted)", remaining)
	}
}

func TestSyncWithNoConnectionsStoresNothing(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	provider := newStub()
	accountsSvc := accounts.NewService(database, provider, testKey)
	svc := NewService(database, provider, accountsSvc, newCategorization(database))

	if err := svc.SyncTransactions(ctx); err != nil {
		t.Fatalf("SyncTransactions: %v", err)
	}
	if got := countTransactions(t, database); got != 0 {
		t.Errorf("stored %d transactions with no connections, want 0", got)
	}
}

// --- B. Per-connection cursor persistence & resume ---

func TestCursorPersistsAndResumes(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	const token = "tok-a"
	provider := newStub()
	provider.accountsByToken[token] = []banking.Account{cashAccount("p-check", "Checking")}
	provider.syncByToken[token] = func(cursor string) (banking.TransactionChanges, error) {
		if cursor == "" {
			return banking.TransactionChanges{
				Added:  []banking.Transaction{bankTxn("t1", "p-check", 1, 10, false)},
				Cursor: "cursor-after-backfill",
			}, nil
		}
		return banking.TransactionChanges{Cursor: cursor}, nil
	}

	accountsSvc := accounts.NewService(database, provider, testKey)
	connID := registerConnection(t, accountsSvc, token, "item-a")
	svc := NewService(database, provider, accountsSvc, newCategorization(database))

	// First invocation backfills from an empty cursor.
	if err := svc.SyncTransactions(ctx); err != nil {
		t.Fatalf("SyncTransactions (first): %v", err)
	}

	t.Run("the returned cursor is persisted per connection", func(t *testing.T) {
		var stored string
		row := database.Sql().QueryRow("SELECT cursor FROM transaction_sync_state WHERE connection_id = ?", connID)
		if err := row.Scan(&stored); err != nil {
			t.Fatalf("read stored cursor: %v", err)
		}
		if stored != "cursor-after-backfill" {
			t.Errorf("stored cursor = %q, want cursor-after-backfill", stored)
		}
	})

	// A separate invocation must resume from the stored cursor.
	if err := svc.SyncTransactions(ctx); err != nil {
		t.Fatalf("SyncTransactions (second): %v", err)
	}

	t.Run("the next sync presents the stored cursor, not an empty one", func(t *testing.T) {
		presented := provider.cursorsByToken[token]
		if len(presented) != 2 {
			t.Fatalf("provider was pulled %d times, want 2", len(presented))
		}
		if presented[0] != "" {
			t.Errorf("first pull presented cursor %q, want empty (full backfill)", presented[0])
		}
		if presented[1] != "cursor-after-backfill" {
			t.Errorf("second pull presented cursor %q, want the resumed cursor-after-backfill", presented[1])
		}
	})
}

// --- C. Accounts-first ordering + per-connection failure isolation ---

func TestSyncAccountsRunFirst(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	const token = "tok-a"
	provider := newStub()
	provider.accountsByToken[token] = []banking.Account{cashAccount("p-check", "Checking")}

	accountsSvc := accounts.NewService(database, provider, testKey)
	registerConnection(t, accountsSvc, token, "item-a")
	// Reset the call log so only the sync pass is observed (registration also
	// calls ListAccounts).
	provider.callOrder = nil

	svc := NewService(database, provider, accountsSvc, newCategorization(database))
	if err := svc.SyncTransactions(ctx); err != nil {
		t.Fatalf("SyncTransactions: %v", err)
	}

	// The accounts refresh (ListAccounts + GetBalances) must precede the first
	// transaction pull.
	firstSync := -1
	firstBalances := -1
	for i, call := range provider.callOrder {
		if firstBalances == -1 && call == "balances:"+token {
			firstBalances = i
		}
		if firstSync == -1 && call == "sync:"+token {
			firstSync = i
		}
	}
	if firstBalances == -1 {
		t.Fatalf("accounts refresh (GetBalances) was never called; call order: %v", provider.callOrder)
	}
	if firstSync == -1 {
		t.Fatalf("transaction pull was never called; call order: %v", provider.callOrder)
	}
	if firstBalances > firstSync {
		t.Errorf("accounts refresh ran after the transaction pull (order: %v); SyncAccounts must run first", provider.callOrder)
	}
}

func TestReauthConnectionSkippedOthersContinue(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	const tokenBad = "tok-bad"
	const tokenGood = "tok-good"
	provider := newStub()
	provider.accountsByToken[tokenBad] = []banking.Account{cashAccount("bad-check", "Bad Checking")}
	provider.accountsByToken[tokenGood] = []banking.Account{cashAccount("good-check", "Good Checking")}
	provider.reauthSyncTokens[tokenBad] = true
	provider.syncByToken[tokenGood] = func(cursor string) (banking.TransactionChanges, error) {
		if cursor == "" {
			return banking.TransactionChanges{
				Added:  []banking.Transaction{bankTxn("g1", "good-check", 1, 10, false)},
				Cursor: "good-c1",
			}, nil
		}
		return banking.TransactionChanges{Cursor: cursor}, nil
	}

	accountsSvc := accounts.NewService(database, provider, testKey)
	badConnID := registerConnection(t, accountsSvc, tokenBad, "item-bad")
	registerConnection(t, accountsSvc, tokenGood, "item-good")
	svc := NewService(database, provider, accountsSvc, newCategorization(database))

	// The re-auth on the bad connection must not fail the whole pass.
	if err := svc.SyncTransactions(ctx); err != nil {
		t.Fatalf("SyncTransactions should not error when a connection needs re-auth: %v", err)
	}

	t.Run("the healthy connection's rows are stored", func(t *testing.T) {
		if got := countTransactions(t, database); got != 1 {
			t.Errorf("stored %d transactions, want 1 (only the healthy connection)", got)
		}
	})

	t.Run("the re-auth connection's cursor is left unchanged", func(t *testing.T) {
		var n int
		if err := database.Sql().QueryRow("SELECT COUNT(*) FROM transaction_sync_state WHERE connection_id = ?", badConnID).Scan(&n); err != nil {
			t.Fatalf("count cursor rows: %v", err)
		}
		if n != 0 {
			t.Errorf("re-auth connection has %d cursor rows, want 0 (cursor must be left untouched)", n)
		}
	})
}

// --- D. Recent-transactions read ---

func TestRecentTransactionsOrderedWithAccountName(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	const token = "tok-a"
	provider := newStub()
	provider.accountsByToken[token] = []banking.Account{
		cashAccount("p-check", "Everyday Checking"),
		cashAccount("p-card", "Rewards Card"),
	}
	provider.syncByToken[token] = func(cursor string) (banking.TransactionChanges, error) {
		if cursor == "" {
			return banking.TransactionChanges{
				Added: []banking.Transaction{
					bankTxn("t-old", "p-check", 1, 10, false),
					bankTxn("t-mid", "p-card", 2, -50, false),
					bankTxn("t-new", "p-check", 3, 30, true),
				},
				Cursor: "c1",
			}, nil
		}
		return banking.TransactionChanges{Cursor: cursor}, nil
	}

	accountsSvc := accounts.NewService(database, provider, testKey)
	registerConnection(t, accountsSvc, token, "item-a")
	svc := NewService(database, provider, accountsSvc, newCategorization(database))
	if err := svc.SyncTransactions(ctx); err != nil {
		t.Fatalf("SyncTransactions: %v", err)
	}

	t.Run("returns rows newest first, joined to the account display name", func(t *testing.T) {
		recent, err := svc.RecentTransactions(ctx, 10)
		if err != nil {
			t.Fatalf("RecentTransactions: %v", err)
		}
		if len(recent) != 3 {
			t.Fatalf("got %d rows, want 3", len(recent))
		}
		// Newest (day 3) first, oldest (day 1) last.
		if !recent[0].Date.Equal(txnDate(3)) || !recent[2].Date.Equal(txnDate(1)) {
			t.Errorf("rows are not date-desc ordered: %v, %v, %v", recent[0].Date, recent[1].Date, recent[2].Date)
		}
		if recent[0].AccountName != "Everyday Checking" {
			t.Errorf("newest row account name = %q, want Everyday Checking", recent[0].AccountName)
		}
		if recent[1].AccountName != "Rewards Card" {
			t.Errorf("middle row account name = %q, want Rewards Card", recent[1].AccountName)
		}
		if !recent[0].Pending {
			t.Errorf("newest row should be pending")
		}
		if recent[1].Amount.Amount != -50 {
			t.Errorf("middle row amount = %v, want -50 (signed inflow as-is)", recent[1].Amount.Amount)
		}
	})

	t.Run("respects the limit", func(t *testing.T) {
		recent, err := svc.RecentTransactions(ctx, 2)
		if err != nil {
			t.Fatalf("RecentTransactions: %v", err)
		}
		if len(recent) != 2 {
			t.Errorf("got %d rows, want at most 2", len(recent))
		}
	})

	t.Run("does not call the provider", func(t *testing.T) {
		provider.syncWasCalled = false
		if _, err := svc.RecentTransactions(ctx, 10); err != nil {
			t.Fatalf("RecentTransactions: %v", err)
		}
		if provider.syncWasCalled {
			t.Errorf("RecentTransactions called the provider; it must read stored rows only")
		}
	})
}

// TestRecentTransactionsOrderIsFullyDetermined proves the recent list's order is
// settled entirely by (date desc, then provider id desc): when several rows share
// a date the tie breaks on the provider id, descending — so the order is stable
// and never depends on insertion order or any other field. The provider hands the
// rows back in a deliberately jumbled order to make the point.
func TestRecentTransactionsOrderIsFullyDetermined(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	const token = "tok-a"
	provider := newStub()
	provider.accountsByToken[token] = []banking.Account{cashAccount("p-check", "Checking")}
	provider.syncByToken[token] = func(cursor string) (banking.TransactionChanges, error) {
		if cursor == "" {
			return banking.TransactionChanges{
				Added: []banking.Transaction{
					// Three rows share day 5; their ids must order them desc:
					// id-c, id-b, id-a. A fourth, older row (day 4) sorts last.
					bankTxn("id-a", "p-check", 5, 10, false),
					bankTxn("id-c", "p-check", 5, 10, false),
					bankTxn("zz-old", "p-check", 4, 10, false),
					bankTxn("id-b", "p-check", 5, 10, false),
				},
				Cursor: "c1",
			}, nil
		}
		return banking.TransactionChanges{Cursor: cursor}, nil
	}

	accountsSvc := accounts.NewService(database, provider, testKey)
	registerConnection(t, accountsSvc, token, "item-a")
	svc := NewService(database, provider, accountsSvc, newCategorization(database))
	if err := svc.SyncTransactions(ctx); err != nil {
		t.Fatalf("SyncTransactions: %v", err)
	}

	recent, err := svc.RecentTransactions(ctx, 100)
	if err != nil {
		t.Fatalf("RecentTransactions: %v", err)
	}
	wantMerchants := []string{"Merchant id-c", "Merchant id-b", "Merchant id-a", "Merchant zz-old"}
	if len(recent) != len(wantMerchants) {
		t.Fatalf("got %d rows, want %d", len(recent), len(wantMerchants))
	}
	for i, want := range wantMerchants {
		if recent[i].Merchant != want {
			t.Errorf("row %d merchant = %q, want %q (date desc, then id desc)", i, recent[i].Merchant, want)
		}
	}
}

// TestRecentTransactionsLimitTakesTheMostRecent proves "the most recent N" is
// exactly the top of the (date desc, id desc) order: with more rows than the
// limit, the limited read returns the newest rows in order and drops the oldest.
func TestRecentTransactionsLimitTakesTheMostRecent(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	const token = "tok-a"
	provider := newStub()
	provider.accountsByToken[token] = []banking.Account{cashAccount("p-check", "Checking")}
	provider.syncByToken[token] = func(cursor string) (banking.TransactionChanges, error) {
		if cursor == "" {
			// Five rows on distinct, ascending days 1..5.
			added := []banking.Transaction{
				bankTxn("d1", "p-check", 1, 10, false),
				bankTxn("d2", "p-check", 2, 10, false),
				bankTxn("d3", "p-check", 3, 10, false),
				bankTxn("d4", "p-check", 4, 10, false),
				bankTxn("d5", "p-check", 5, 10, false),
			}
			return banking.TransactionChanges{Added: added, Cursor: "c1"}, nil
		}
		return banking.TransactionChanges{Cursor: cursor}, nil
	}

	accountsSvc := accounts.NewService(database, provider, testKey)
	registerConnection(t, accountsSvc, token, "item-a")
	svc := NewService(database, provider, accountsSvc, newCategorization(database))
	if err := svc.SyncTransactions(ctx); err != nil {
		t.Fatalf("SyncTransactions: %v", err)
	}

	recent, err := svc.RecentTransactions(ctx, 3)
	if err != nil {
		t.Fatalf("RecentTransactions: %v", err)
	}
	if len(recent) != 3 {
		t.Fatalf("got %d rows, want 3 (the limit)", len(recent))
	}
	// The three most recent are days 5, 4, 3 — the older two are dropped.
	if !recent[0].Date.Equal(txnDate(5)) || !recent[1].Date.Equal(txnDate(4)) || !recent[2].Date.Equal(txnDate(3)) {
		t.Errorf("limited read returned the wrong window: %v, %v, %v; want days 5,4,3", recent[0].Date, recent[1].Date, recent[2].Date)
	}
}
