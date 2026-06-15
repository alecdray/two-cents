package adapters_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/alecdray/two-cents/src/internal/accounts"
	"github.com/alecdray/two-cents/src/internal/accounts/adapters"
	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/categorization"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/fakebank"
	"github.com/alecdray/two-cents/src/internal/transactions"
)

// recentLimit caps the recent-activity read the backfill assertions make.
const recentLimit = 100

// TestConnectBacksfillsTransactions drives a successful connect through the POST
// handler with the real server-wired seam (a backfill hook that calls
// transactions.SyncTransactions) and asserts the freshly linked bank's
// transactions are available via RecentTransactions immediately — without the
// test ever calling SyncTransactions itself. This is the connect-time backfill:
// connecting a bank makes its transactions readable with no manual "Sync now".
func TestConnectBacksfillsTransactions(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	// One bank provider behind the seam for both services — the deterministic
	// stand-in, whose first (empty-cursor) sync backfills a fixed transaction set.
	provider := fakebank.NewService()
	accountsSvc := accounts.NewService(database, provider, testKey)
	categorizationSvc := categorization.NewService(database, nil)
	transactionsSvc := transactions.NewService(database, provider, accountsSvc, categorizationSvc)

	var backfillCalls int
	backfill := func(c contextx.ContextX) error {
		backfillCalls++
		return transactionsSvc.SyncTransactions(c)
	}
	handler := adapters.NewHttpHandler(accountsSvc, adapters.BankModeFake, backfill)

	// Before connecting there is nothing to read.
	if before, err := transactionsSvc.RecentTransactions(ctx, recentLimit); err != nil {
		t.Fatalf("RecentTransactions (before connect): %v", err)
	} else if len(before) != 0 {
		t.Fatalf("expected no transactions before connect, got %d", len(before))
	}

	form := url.Values{"public_token": {"any-public-token"}}
	req := httptest.NewRequest(http.MethodPost, "/accounts/connections", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.PostConnection(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("connect status = %d, want 200", rec.Code)
	}
	if backfillCalls != 1 {
		t.Fatalf("backfill hook invoked %d times, want exactly 1 after a successful connect", backfillCalls)
	}

	// The bank's transactions are now readable WITHOUT the test ever calling
	// SyncTransactions — the connect handler ran the backfill through the seam.
	recent, err := transactionsSvc.RecentTransactions(ctx, recentLimit)
	if err != nil {
		t.Fatalf("RecentTransactions (after connect): %v", err)
	}
	if len(recent) != 5 {
		t.Fatalf("got %d transactions after connect, want the stand-in's 5", len(recent))
	}

	gotMerchants := make(map[string]bool, len(recent))
	for _, r := range recent {
		gotMerchants[r.Merchant] = true
	}
	for _, want := range []string{"Whole Foods", "Acme Payroll", "Blue Bottle Coffee"} {
		if !gotMerchants[want] {
			t.Errorf("recent transactions missing %q from the connect-time backfill", want)
		}
	}
}

// TestReconnectBackfillsTransactions drives a needs-reconnect connection through
// the reconnect POST handler with the server-wired seam and asserts the now-
// restored bank's transactions are pulled — proving the reconnect handler runs
// the backfill. The connection's transactions are empty before reconnect (no
// backfill runs on the initial register, only through the handler) and present
// after, so a populated read is unambiguous evidence the reconnect triggered it.
func TestReconnectBackfillsTransactions(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	provider := &backfillProvider{
		accounts: []banking.Account{
			providerAccount("p-check", "Everyday Checking", banking.KindCash, "checking", knownBalance("p-check", 1200)),
			providerAccount("p-card", "Travel Card", banking.KindCredit, "credit card", knownBalance("p-card", 300)),
		},
		transactions: []banking.Transaction{
			{ID: "t-grocery", AccountID: "p-check", Amount: banking.Money{Amount: 42.10, Currency: "USD"}, Merchant: "Corner Market"},
			{ID: "t-refund", AccountID: "p-card", Amount: banking.Money{Amount: -15.00, Currency: "USD"}, Merchant: "Travel Card Refund"},
		},
	}
	accountsSvc := accounts.NewService(database, provider, testKey)
	categorizationSvc := categorization.NewService(database, nil)
	transactionsSvc := transactions.NewService(database, provider, accountsSvc, categorizationSvc)

	var backfillCalls int
	backfill := func(c contextx.ContextX) error {
		backfillCalls++
		return transactionsSvc.SyncTransactions(c)
	}
	handler := adapters.NewHttpHandler(accountsSvc, adapters.BankModeFake, backfill)

	conn, err := accountsSvc.RegisterConnection(ctx, "stale-token", "item-stale")
	if err != nil {
		t.Fatalf("RegisterConnection: %v", err)
	}

	// Drive the connection into needs-reconnect — registering alone runs no
	// backfill, so nothing is stored yet.
	provider.armReauth = true
	if err := accountsSvc.SyncAccounts(ctx); err != nil {
		t.Fatalf("SyncAccounts (reauth): %v", err)
	}
	if before, err := transactionsSvc.RecentTransactions(ctx, recentLimit); err != nil {
		t.Fatalf("RecentTransactions (before reconnect): %v", err)
	} else if len(before) != 0 {
		t.Fatalf("expected no transactions before reconnect, got %d", len(before))
	}

	// The login is healthy again; reconnect succeeds and the handler backfills.
	req := httptest.NewRequest(http.MethodPost, "/accounts/connections/"+conn.ID+"/reconnect", nil)
	req.SetPathValue("id", conn.ID)
	rec := httptest.NewRecorder()
	handler.PostReconnect(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("reconnect status = %d, want 200", rec.Code)
	}
	if backfillCalls != 1 {
		t.Fatalf("backfill hook invoked %d times, want exactly 1 after a successful reconnect", backfillCalls)
	}

	recent, err := transactionsSvc.RecentTransactions(ctx, recentLimit)
	if err != nil {
		t.Fatalf("RecentTransactions (after reconnect): %v", err)
	}
	if len(recent) != 2 {
		t.Fatalf("got %d transactions after reconnect, want 2 from the backfill", len(recent))
	}
}

// backfillProvider is a banking.BankProvider stand-in that serves a fixed set of
// accounts and transactions and can be armed to surface ErrReauthRequired once
// (to drive a connection into needs-reconnect). Its empty-cursor sync returns
// every transaction as added; a non-empty cursor reports no further changes.
type backfillProvider struct {
	accounts     []banking.Account
	transactions []banking.Transaction
	armReauth    bool
}

func (p *backfillProvider) ListAccounts(_ contextx.ContextX, _ string) ([]banking.Account, error) {
	if p.armReauth {
		p.armReauth = false
		return nil, banking.ErrReauthRequired
	}
	out := make([]banking.Account, len(p.accounts))
	copy(out, p.accounts)
	return out, nil
}

func (p *backfillProvider) GetBalances(_ contextx.ContextX, _ string) ([]banking.Balance, error) {
	out := make([]banking.Balance, len(p.accounts))
	for i, a := range p.accounts {
		out[i] = a.Balance
	}
	return out, nil
}

func (p *backfillProvider) SyncTransactions(_ contextx.ContextX, _, cursor string) (banking.TransactionChanges, error) {
	if cursor == "" {
		added := make([]banking.Transaction, len(p.transactions))
		copy(added, p.transactions)
		return banking.TransactionChanges{Added: added, Cursor: "backfill-cursor-v1"}, nil
	}
	return banking.TransactionChanges{Cursor: cursor}, nil
}

func (p *backfillProvider) CreateLinkToken(_ contextx.ContextX, _ banking.LinkOptions) (banking.LinkToken, error) {
	return banking.LinkToken{}, nil
}

func (p *backfillProvider) ExchangePublicToken(_ contextx.ContextX, _ string) (banking.Item, error) {
	return banking.Item{AccessToken: "access-token", ProviderItemID: "item-id"}, nil
}

func (p *backfillProvider) RemoveItem(_ contextx.ContextX, _ string) error {
	return nil
}
