package adapters_test

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecdray/two-cents/src/internal/accounts"
	"github.com/alecdray/two-cents/src/internal/accounts/adapters"
	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/db"

	"github.com/pressly/goose/v3"

	_ "github.com/mattn/go-sqlite3"
)

// testKey is a valid 32-byte (AES-256) hex key for the encryption seam.
const testKey = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"

// fakeProvider is a banking.BankProvider stand-in returning a fixed account set.
// It lets the test seed real accounts through RegisterConnection without a
// provider client, keeping the seam isolation intact.
type fakeProvider struct {
	accounts []banking.Account
}

func (f *fakeProvider) ListAccounts(_ contextx.ContextX, _ string) ([]banking.Account, error) {
	return f.accounts, nil
}

func (f *fakeProvider) GetBalances(_ contextx.ContextX, _ string) ([]banking.Balance, error) {
	out := make([]banking.Balance, len(f.accounts))
	for i, a := range f.accounts {
		out[i] = a.Balance
	}
	return out, nil
}

func (f *fakeProvider) SyncTransactions(_ contextx.ContextX, _, _ string) (banking.TransactionChanges, error) {
	return banking.TransactionChanges{}, nil
}

// The connection-lifecycle methods aren't exercised by the overview tests;
// trivial stubs keep the fake satisfying the seam.
func (f *fakeProvider) CreateLinkToken(_ contextx.ContextX, _ banking.LinkOptions) (banking.LinkToken, error) {
	return banking.LinkToken{}, nil
}

func (f *fakeProvider) ExchangePublicToken(_ contextx.ContextX, _ string) (banking.Item, error) {
	return banking.Item{}, nil
}

func (f *fakeProvider) RemoveItem(_ contextx.ContextX, _ string) error {
	return nil
}

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

func knownBalance(id string, amount float64) banking.Balance {
	return banking.Balance{
		AccountID: id,
		Known:     true,
		Money:     banking.Money{Amount: amount, Currency: "USD"},
	}
}

func unknownBalance(id string) banking.Balance {
	return banking.Balance{AccountID: id, Known: false}
}

func providerAccount(id, name string, kind banking.AccountKind, subtype string, balance banking.Balance) banking.Account {
	return banking.Account{
		ID:      id,
		Name:    name,
		Kind:    kind,
		Subtype: subtype,
		Balance: balance,
	}
}

// getOverview drives a GET / through the constructed handler and returns the
// rendered body.
func getOverview(t *testing.T, svc *accounts.Service) (int, string) {
	t.Helper()
	handler := adapters.NewHttpHandler(svc, adapters.BankModeFake)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.GetOverviewPage(rec, req)

	return rec.Code, rec.Body.String()
}

func TestOverviewPagePopulated(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	// A healthy connection: checking + savings (cash), a credit card, an unknown
	// balance account, and a mortgage in the other bucket.
	healthy := &fakeProvider{accounts: []banking.Account{
		providerAccount("p-check", "Everyday Checking", banking.KindCash, "checking", knownBalance("p-check", 2500)),
		providerAccount("p-save", "Rainy Day", banking.KindCash, "savings", knownBalance("p-save", 8000)),
		providerAccount("p-card", "Travel Card", banking.KindCredit, "credit card", knownBalance("p-card", 1200)),
		providerAccount("p-unknown", "Pending Account", banking.KindCash, "checking", unknownBalance("p-unknown")),
		providerAccount("p-loan", "Home Loan", banking.KindOther, "mortgage", knownBalance("p-loan", 250000)),
	}}
	svcHealthy := accounts.NewService(database, healthy, testKey)
	if _, err := svcHealthy.RegisterConnection(ctx, "token-healthy", "item-healthy"); err != nil {
		t.Fatalf("register healthy connection: %v", err)
	}

	// A second connection that has gone stale: its card should carry the
	// needs-reconnect indicator. RegisterConnection seeds it active; flip it via
	// a re-auth on the next sync.
	stale := &reauthProvider{inner: &fakeProvider{accounts: []banking.Account{
		providerAccount("p-stale-card", "Old Card", banking.KindCredit, "credit card", knownBalance("p-stale-card", 300)),
	}}}
	svcStale := accounts.NewService(database, stale, testKey)
	if _, err := svcStale.RegisterConnection(ctx, "token-stale", "item-stale"); err != nil {
		t.Fatalf("register stale connection: %v", err)
	}
	stale.armReauth = true
	if err := svcStale.SyncAccounts(ctx); err != nil {
		t.Fatalf("sync to trigger reauth: %v", err)
	}

	svc := accounts.NewService(database, healthy, testKey)
	status, body := getOverview(t, svc)

	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	mustContain := map[string]string{
		"root testid":          `data-testid="accounts-overview-page"`,
		"net cash value":       `data-testid="accounts-overview-net-cash"`,
		"total cash value":     `data-testid="accounts-overview-total-cash"`,
		"total debt value":     `data-testid="accounts-overview-total-debt"`,
		"cash group":           `data-testid="accounts-overview-cash"`,
		"credit group":         `data-testid="accounts-overview-credit"`,
		"other section":        `data-testid="accounts-overview-other"`,
		"needs-reconnect":      `data-testid="accounts-overview-needs-reconnect"`,
		"account row":          `data-testid="accounts-overview-account-row"`,
		"excluded copy":        "Excluded from net cash",
		"checking name":        "Everyday Checking",
		"savings name":         "Rainy Day",
		"card name":            "Travel Card",
		"mortgage name":        "Home Loan",
		"unknown account":      "Pending Account",
		"mortgage subtype":     "mortgage",
		"unknown balance dash": "—",
	}
	for label, want := range mustContain {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %s (%q)", label, want)
		}
	}

	// Across both connections: total cash = 2500 + 8000 = 10,500 (savings
	// included); total credit debt = 1200 (Travel Card) + 300 (Old Card) =
	// 1,500; net cash = 10,500 − 1,500 = 9,000. The unknown-balance account and
	// the mortgage in the other bucket are excluded from every total.
	if !strings.Contains(body, "$9,000.00") {
		t.Errorf("body missing net cash $9,000.00")
	}
	if !strings.Contains(body, "$10,500.00") {
		t.Errorf("body missing total cash $10,500.00")
	}
	if !strings.Contains(body, "$1,500.00") {
		t.Errorf("body missing total credit debt $1,500.00")
	}

	// The unknown-balance account must never render as $0.
	if strings.Contains(body, "$0.00") {
		t.Errorf("an unknown balance rendered as $0.00; it must render as —")
	}

	if !strings.Contains(body, "Old Card") {
		t.Errorf("body missing the stale connection's account")
	}
}

func TestOverviewPageEmpty(t *testing.T) {
	database := newTestDB(t)

	svc := accounts.NewService(database, &fakeProvider{}, testKey)
	status, body := getOverview(t, svc)

	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if !strings.Contains(body, `data-testid="accounts-overview-empty"`) {
		t.Errorf("body missing the empty-state testid")
	}
	// No zeroed chrome on the empty state.
	if strings.Contains(body, `data-testid="accounts-overview-net-cash"`) {
		t.Errorf("empty state should not render the net-cash headline")
	}
	if strings.Contains(body, "$0.00") {
		t.Errorf("empty state should not render zeroed totals")
	}
}

// exchangeFailProvider is a provider whose public-token exchange always fails,
// driving the connect handler down its recoverable-error path.
type exchangeFailProvider struct {
	*fakeProvider
}

func (p *exchangeFailProvider) ExchangePublicToken(_ contextx.ContextX, _ string) (banking.Item, error) {
	return banking.Item{}, errors.New("provider exchange failed")
}

// TestConnectFailureRendersInlineError drives a failed connect through the POST
// handler and asserts the response is a normal in-place render carrying the
// recoverable inline error — not a redirect — with the already-linked accounts
// still present.
func TestConnectFailureRendersInlineError(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	// Seed one already-linked account so we can prove it survives a failed
	// connect attempt rather than being wiped by a full-page replacement.
	seed := &fakeProvider{accounts: []banking.Account{
		providerAccount("p-check", "Everyday Checking", banking.KindCash, "checking", knownBalance("p-check", 1200)),
	}}
	if _, err := accounts.NewService(database, seed, testKey).RegisterConnection(ctx, "seed-token", "seed-item"); err != nil {
		t.Fatalf("seed existing connection: %v", err)
	}

	failing := &exchangeFailProvider{fakeProvider: &fakeProvider{}}
	handler := adapters.NewHttpHandler(accounts.NewService(database, failing, testKey), adapters.BankModeFake)

	form := url.Values{"public_token": {"any-public-token"}}
	req := httptest.NewRequest(http.MethodPost, "/accounts/connections", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.PostConnection(rec, req)

	// A recoverable failure renders in place — never a redirect.
	if rec.Code >= 300 && rec.Code < 400 {
		t.Fatalf("connect failure returned a redirect status %d; want an in-place render", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "" {
		t.Fatalf("connect failure set a redirect Location %q; want an in-place render", loc)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `data-testid="accounts-overview-connect-error"`) {
		t.Errorf("body missing the inline connect-error region")
	}
	// The connect control stays so the user can retry in place.
	if !strings.Contains(body, `data-testid="accounts-overview-connect"`) {
		t.Errorf("body missing the connect control after a failed connect")
	}
	// The previously linked account remains visible — the failure didn't blow
	// away the existing overview.
	if !strings.Contains(body, "Everyday Checking") {
		t.Errorf("existing account missing after a failed connect; it must remain visible")
	}
}

// reauthProvider wraps a provider and, when armed, surfaces ErrReauthRequired on
// the next ListAccounts call to drive a connection into needs-reconnect.
type reauthProvider struct {
	inner     *fakeProvider
	armReauth bool
}

func (r *reauthProvider) ListAccounts(ctx contextx.ContextX, token string) ([]banking.Account, error) {
	if r.armReauth {
		r.armReauth = false
		return nil, banking.ErrReauthRequired
	}
	return r.inner.ListAccounts(ctx, token)
}

func (r *reauthProvider) GetBalances(ctx contextx.ContextX, token string) ([]banking.Balance, error) {
	return r.inner.GetBalances(ctx, token)
}

func (r *reauthProvider) SyncTransactions(ctx contextx.ContextX, token, cursor string) (banking.TransactionChanges, error) {
	return r.inner.SyncTransactions(ctx, token, cursor)
}

func (r *reauthProvider) CreateLinkToken(ctx contextx.ContextX, opts banking.LinkOptions) (banking.LinkToken, error) {
	return r.inner.CreateLinkToken(ctx, opts)
}

func (r *reauthProvider) ExchangePublicToken(ctx contextx.ContextX, publicToken string) (banking.Item, error) {
	return r.inner.ExchangePublicToken(ctx, publicToken)
}

func (r *reauthProvider) RemoveItem(ctx contextx.ContextX, accessToken string) error {
	return r.inner.RemoveItem(ctx, accessToken)
}
