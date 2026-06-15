package adapters_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecdray/two-cents/src/internal/accounts"
	accountsViews "github.com/alecdray/two-cents/src/internal/accounts/adapters/views"
	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/categorization"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/db"
	"github.com/alecdray/two-cents/src/internal/fakebank"
	"github.com/alecdray/two-cents/src/internal/transactions"
	"github.com/alecdray/two-cents/src/internal/transactions/adapters"
	"github.com/alecdray/two-cents/src/internal/transactions/adapters/views"

	"github.com/pressly/goose/v3"

	_ "github.com/mattn/go-sqlite3"
)

// testKey is a valid 32-byte (AES-256) hex key for the encryption seam.
const testKey = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"

func newTestDB(t *testing.T) *db.DB {
	t.Helper()

	migrationsDir, err := filepath.Abs("../../../../db/migrations")
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

// syncFailProvider wraps the deterministic stand-in but fails the transaction
// pull with an unexpected (non-reauth) error, driving the sync handler down its
// recoverable inline-error path. Account listing/balances still succeed so a
// connection can be registered and accounts-first sync gets past the accounts
// step.
type syncFailProvider struct {
	*fakebank.Service
}

func (p *syncFailProvider) SyncTransactions(_ contextx.ContextX, _, _ string) (banking.TransactionChanges, error) {
	return banking.TransactionChanges{}, errProviderDown
}

var errProviderDown = sql.ErrConnDone // any non-reauth error

// newServices wires the accounts, transactions, and categorization services over
// a shared database and bank provider, the way the composition root does. The
// categorization service has no re-categorization seam — these tests drive the
// transactions side only.
func newServices(t *testing.T, database *db.DB, provider banking.BankProvider) (*accounts.Service, *transactions.Service, *categorization.Service) {
	t.Helper()
	accountsSvc := accounts.NewService(database, provider, testKey)
	categorizationSvc := categorization.NewService(database, nil)
	txnSvc := transactions.NewService(database, provider, accountsSvc, categorizationSvc)
	return accountsSvc, txnSvc, categorizationSvc
}

func registerConnection(t *testing.T, accountsSvc *accounts.Service) {
	t.Helper()
	if _, err := accountsSvc.RegisterConnection(testCtx(), "access-token", "item-id"); err != nil {
		t.Fatalf("RegisterConnection: %v", err)
	}
}

// getPage drives a GET /transactions through the handler and returns the body.
func getPage(t *testing.T, txnSvc *transactions.Service, accountsSvc *accounts.Service, categorizationSvc *categorization.Service) (int, string) {
	t.Helper()
	handler := adapters.NewHttpHandler(txnSvc, accountsSvc, categorizationSvc)
	req := httptest.NewRequest(http.MethodGet, "/transactions", nil)
	rec := httptest.NewRecorder()
	handler.GetTransactionsPage(rec, req)
	return rec.Code, rec.Body.String()
}

// --- Item 2.1: render the transactions list ---

// TestTransactionsPageRendersList connects a bank, backfills the deterministic
// fake transactions, and asserts the page shows the rows newest-first with the
// display-sign convention (outflow negative, inflow positive) and a pending
// marker on the pending row.
func TestTransactionsPageRendersList(t *testing.T) {
	database := newTestDB(t)
	accountsSvc, txnSvc, categorizationSvc := newServices(t, database, fakebank.NewService())
	registerConnection(t, accountsSvc)
	if err := txnSvc.SyncTransactions(testCtx()); err != nil {
		t.Fatalf("SyncTransactions: %v", err)
	}

	status, body := getPage(t, txnSvc, accountsSvc, categorizationSvc)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	mustContain := map[string]string{
		"page testid":     `data-testid="transactions-page"`,
		"list testid":     `data-testid="transactions-list"`,
		"row testid":      `data-testid="transactions-row"`,
		"merchant testid": `data-testid="transactions-row-merchant"`,
		"account testid":  `data-testid="transactions-row-account"`,
		"amount testid":   `data-testid="transactions-row-amount"`,
		"pending testid":  `data-testid="transactions-row-pending"`,
		"sync control":    `data-testid="transactions-sync"`,
		"groceries":       "Whole Foods",
		"paycheck":        "Acme Payroll",
		"coffee":          "Blue Bottle Coffee",
		"account name":    "Everyday Checking",
		// Display sign: stored +84.32 outflow renders negative.
		"outflow negative": "-$84.32",
		// Display sign: stored -2400 inflow renders positive, grouped.
		"inflow positive": "+$2,400.00",
		// Pending outflow stored +5.75 renders negative.
		"pending amount": "-$5.75",
		// Auto-categorization chips: a spending Category on the groceries row, the
		// Transfer signal on the transfer row, and the needs-review flag on the
		// unusable-category inflow.
		"classification chip": `data-testid="txn-classification"`,
		"category chip":       `data-testid="txn-category-chip"`,
		"general merchandise": "General Merchandise",
		"transfer signal row": "Rainy Day Savings",
		"needs review flag":   `data-testid="txn-needs-review"`,
	}
	for label, want := range mustContain {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %s (%q)", label, want)
		}
	}

	// Five rows backfilled.
	if got := strings.Count(body, `data-testid="transactions-row"`); got != 5 {
		t.Errorf("row count = %d, want 5", got)
	}
	// Exactly one pending marker (the coffee charge).
	if got := strings.Count(body, `data-testid="transactions-row-pending"`); got != 1 {
		t.Errorf("pending marker count = %d, want 1", got)
	}

	// Newest-first order: coffee (Jun 3) before paycheck (Jun 2) before
	// groceries (Jun 1).
	coffee := strings.Index(body, "Blue Bottle Coffee")
	paycheck := strings.Index(body, "Acme Payroll")
	groceries := strings.Index(body, "Whole Foods")
	if !(coffee < paycheck && paycheck < groceries) {
		t.Errorf("rows not newest-first: coffee=%d paycheck=%d groceries=%d", coffee, paycheck, groceries)
	}

	// The list never renders an empty state.
	if strings.Contains(body, `data-testid="transactions-empty-no-transactions"`) {
		t.Errorf("populated page rendered the nothing-synced empty state")
	}
	if strings.Contains(body, `data-testid="transactions-empty-no-connections"`) {
		t.Errorf("populated page rendered the no-connections empty state")
	}
}

// --- Item 2.2: empty states ---

// TestNoConnectionsEmptyState asserts that with no connected banks the page
// shows the connect-prompt empty state linking to the overview and renders
// neither the list nor the sync control.
func TestNoConnectionsEmptyState(t *testing.T) {
	database := newTestDB(t)
	accountsSvc, txnSvc, categorizationSvc := newServices(t, database, fakebank.NewService())

	status, body := getPage(t, txnSvc, accountsSvc, categorizationSvc)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	if !strings.Contains(body, `data-testid="transactions-empty-no-connections"`) {
		t.Errorf("body missing the no-connections empty state")
	}
	// Links to the overview to connect a bank.
	if !strings.Contains(body, `href="/"`) {
		t.Errorf("no-connections empty state missing a link to the overview")
	}
	// No list and no sync control until a bank is connected.
	if strings.Contains(body, `data-testid="transactions-list"`) {
		t.Errorf("no-connections state should not render the list")
	}
	if strings.Contains(body, `data-testid="transactions-sync"`) {
		t.Errorf("no-connections state should not render the sync control")
	}
}

// TestNoTransactionsEmptyState asserts that with a connection but nothing synced
// the page shows the nothing-synced empty state WITH the sync control and no
// list.
func TestNoTransactionsEmptyState(t *testing.T) {
	database := newTestDB(t)
	accountsSvc, txnSvc, categorizationSvc := newServices(t, database, fakebank.NewService())
	registerConnection(t, accountsSvc) // accounts present, but no transactions synced

	status, body := getPage(t, txnSvc, accountsSvc, categorizationSvc)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	if !strings.Contains(body, `data-testid="transactions-empty-no-transactions"`) {
		t.Errorf("body missing the nothing-synced empty state")
	}
	// The sync control is shown so the user can pull activity in place.
	if !strings.Contains(body, `data-testid="transactions-sync"`) {
		t.Errorf("nothing-synced state should render the sync control")
	}
	// No list and not the no-connections state.
	if strings.Contains(body, `data-testid="transactions-list"`) {
		t.Errorf("nothing-synced state should not render the list")
	}
	if strings.Contains(body, `data-testid="transactions-empty-no-connections"`) {
		t.Errorf("nothing-synced state should not render the no-connections empty state")
	}
}

// --- Item 2.3: sync now ---

// TestSyncNowRefreshesList drives POST /transactions/sync against a connection
// with nothing synced and asserts the response is an in-place render of the
// refreshed region carrying the backfilled rows — not a redirect, not a full
// page.
func TestSyncNowRefreshesList(t *testing.T) {
	database := newTestDB(t)
	accountsSvc, txnSvc, categorizationSvc := newServices(t, database, fakebank.NewService())
	registerConnection(t, accountsSvc)

	handler := adapters.NewHttpHandler(txnSvc, accountsSvc, categorizationSvc)
	req := httptest.NewRequest(http.MethodPost, "/transactions/sync", nil)
	rec := httptest.NewRecorder()
	handler.PostSync(rec, req)

	if rec.Code >= 300 && rec.Code < 400 {
		t.Fatalf("sync returned a redirect status %d; want an in-place render", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "" {
		t.Fatalf("sync set a redirect Location %q; want an in-place render", loc)
	}

	body := rec.Body.String()
	// The swap response is a fragment, not a full page.
	if strings.Contains(body, "<html") {
		t.Errorf("sync response rendered a full page; want the swap fragment only")
	}
	if !strings.Contains(body, `data-testid="transactions-list"`) {
		t.Errorf("sync response missing the list after a successful sync")
	}
	if got := strings.Count(body, `data-testid="transactions-row"`); got != 5 {
		t.Errorf("synced row count = %d, want 5", got)
	}
}

// TestSyncFailureRendersInlineError drives a failed sync and asserts the response
// is an in-place render carrying the recoverable inline sync error — not a
// redirect — leaving the user on the page (sync control still present).
func TestSyncFailureRendersInlineError(t *testing.T) {
	database := newTestDB(t)
	provider := &syncFailProvider{Service: fakebank.NewService()}
	accountsSvc, txnSvc, categorizationSvc := newServices(t, database, provider)
	registerConnection(t, accountsSvc)

	handler := adapters.NewHttpHandler(txnSvc, accountsSvc, categorizationSvc)
	req := httptest.NewRequest(http.MethodPost, "/transactions/sync", nil)
	rec := httptest.NewRecorder()
	handler.PostSync(rec, req)

	if rec.Code >= 300 && rec.Code < 400 {
		t.Fatalf("sync failure returned a redirect status %d; want an in-place render", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "" {
		t.Fatalf("sync failure set a redirect Location %q; want an in-place render", loc)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `data-testid="transactions-sync-error"`) {
		t.Errorf("body missing the inline sync-error region")
	}
	// The sync control stays so the user can retry in place.
	if !strings.Contains(body, `data-testid="transactions-sync"`) {
		t.Errorf("body missing the sync control after a failed sync")
	}
}

// --- Item 2.4: navbar on both pages ---

// TestNavbarOnTransactionsPage asserts the transactions page renders the navbar
// with both links.
func TestNavbarOnTransactionsPage(t *testing.T) {
	var sb strings.Builder
	if err := views.TransactionsPage(false, nil, nil).Render(testCtx(), &sb); err != nil {
		t.Fatalf("render transactions page: %v", err)
	}
	assertNavbar(t, "transactions page", sb.String())
}

// TestNavbarOnOverviewPage asserts the accounts overview page renders the same
// navbar with both links, so the navbar appears on both surfaces.
func TestNavbarOnOverviewPage(t *testing.T) {
	var sb strings.Builder
	if err := accountsViews.AccountsOverviewPage(accounts.Dashboard{}, accountsViews.BankModeFake).Render(testCtx(), &sb); err != nil {
		t.Fatalf("render overview page: %v", err)
	}
	assertNavbar(t, "overview page", sb.String())
}

func assertNavbar(t *testing.T, page, body string) {
	t.Helper()
	checks := map[string]string{
		"overview link testid":     `data-testid="nav-overview"`,
		"transactions link testid": `data-testid="nav-transactions"`,
		"overview href":            `href="/"`,
		"transactions href":        `href="/transactions"`,
	}
	for label, want := range checks {
		if !strings.Contains(body, want) {
			t.Errorf("%s missing %s (%q)", page, label, want)
		}
	}
}
