package home

import (
	"testing"
	"time"

	"github.com/alecdray/two-cents/src/internal/accounts"
	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/budget"
	"github.com/alecdray/two-cents/src/internal/categorization"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/transactions"
)

// multiMonthBank serves transactions spanning April–June 2026 on one checking
// account: a backfill-edge month (April), a complete interior month (May), and
// the current/connect month (June). The connection itself is created "now" (real
// CURRENT_TIMESTAMP) — i.e. in June — which is exactly the real-Plaid shape: you
// connect this month and Plaid backfills months of prior history.
type multiMonthBank struct{}

func (multiMonthBank) account() banking.Account {
	return banking.Account{ID: "p-check", Name: "Everyday Checking", Kind: banking.KindCash,
		Type: "depository", Subtype: "checking",
		Balance: banking.Balance{AccountID: "p-check", Known: true, Money: banking.Money{Amount: 1200, Currency: "USD"}}}
}

func (b multiMonthBank) ListAccounts(_ contextx.ContextX, _ string) ([]banking.Account, error) {
	return []banking.Account{b.account()}, nil
}

func (b multiMonthBank) GetBalances(_ contextx.ContextX, _ string) ([]banking.Balance, error) {
	return []banking.Balance{b.account().Balance}, nil
}

func monthTxn(id string, month time.Month, amount float64) banking.Transaction {
	return banking.Transaction{
		ID: id, AccountID: "p-check", Date: time.Date(2026, month, 10, 0, 0, 0, 0, time.UTC),
		Amount: banking.Money{Amount: amount, Currency: "USD"}, Merchant: "Merchant " + id, Counterparty: "RAW " + id,
		Category: banking.Category{Primary: "GENERAL_MERCHANDISE", Detailed: "GENERAL_MERCHANDISE_SUPERSTORES"},
	}
}

func (multiMonthBank) SyncTransactions(_ contextx.ContextX, _, cursor string) (banking.TransactionChanges, error) {
	if cursor != "" {
		return banking.TransactionChanges{Cursor: cursor}, nil
	}
	return banking.TransactionChanges{
		Cursor: "c1",
		Added: []banking.Transaction{
			monthTxn("t-apr", time.April, 40.00),
			monthTxn("t-may", time.May, 50.00),
			monthTxn("t-jun", time.June, 60.00),
		},
	}, nil
}

func (multiMonthBank) CreateLinkToken(_ contextx.ContextX, _ banking.LinkOptions) (banking.LinkToken, error) {
	return banking.LinkToken{}, nil
}

func (multiMonthBank) ExchangePublicToken(_ contextx.ContextX, _ string) (banking.Item, error) {
	return banking.Item{}, nil
}

func (multiMonthBank) RemoveItem(_ contextx.ContextX, _ string) error { return nil }

var _ banking.BankProvider = multiMonthBank{}

func newMultiMonthServices(t *testing.T) (*Service, contextx.ContextX) {
	t.Helper()
	d := newTestDB(t)
	ctx := testCtx()

	provider := multiMonthBank{}
	accountsSvc := accounts.NewService(d, provider, testKey)
	categorizationSvc := categorization.NewService(d, nil)
	transactionsSvc := transactions.NewService(d, provider, accountsSvc, categorizationSvc, nil)
	budgetSvc := budget.NewService(d, categorizationSvc)

	if _, err := accountsSvc.RegisterConnection(ctx, "fake-token", "fake-item"); err != nil {
		t.Fatalf("RegisterConnection: %v", err)
	}
	if err := transactionsSvc.SyncTransactions(ctx); err != nil {
		t.Fatalf("SyncTransactions: %v", err)
	}

	svc := NewService(budgetSvc, transactionsSvc, categorizationSvc, accountsSvc, time.UTC)
	svc.now = func() time.Time { return fixedNow }
	return svc, ctx
}

// TestPartialFlagTracksBackfillEdgeNotConnectDate pins the partial flag to the
// earliest *transaction* month (the real backfill edge), not the connection's
// created_at. With history backfilled before the connect month, an interior
// complete month must NOT be flagged partial.
func TestPartialFlagTracksBackfillEdgeNotConnectDate(t *testing.T) {
	svc, ctx := newMultiMonthServices(t)

	apr, err := svc.MonthWrap(ctx, 2026, time.April)
	if err != nil {
		t.Fatalf("MonthWrap April: %v", err)
	}
	if !apr.Partial {
		t.Errorf("April is the earliest-transaction (backfill-edge) month — it should be partial")
	}

	may, err := svc.MonthWrap(ctx, 2026, time.May)
	if err != nil {
		t.Fatalf("MonthWrap May: %v", err)
	}
	if may.Partial {
		t.Errorf("May is a complete interior month (after the backfill edge) — it must NOT be partial. " +
			"Anchoring partial to the connection created_at (this month) wrongly flags all backfilled history.")
	}
}
