package home

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/alecdray/two-cents/src/internal/accounts"
	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/budget"
	"github.com/alecdray/two-cents/src/internal/categorization"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/db"
	"github.com/alecdray/two-cents/src/internal/transactions"

	"github.com/pressly/goose/v3"

	_ "github.com/mattn/go-sqlite3"
)

// These tests exercise the assembled home composer end-to-end: the real Service
// over a real (migrated, temp-file) SQLite DB, fed by the real accounts +
// transactions + categorization + budget services synced against an in-package
// bank stub (the composer, like every consumer, may never import a provider
// client — the isolation test enforces it even for test code). The stub's canned
// set falls in June 2026 (the same calendar month the dashboard reckons, given
// the pinned clock), so the composed view carries real numbers — including the
// $500 auto-paired savings contribution and the pending coffee charge that makes
// the wrap settling.
//
// Stub set (all June 2026):
//   - Whole Foods         +$84.32  outflow  posted   -> Spending / General Merchandise
//   - Acme Payroll      -$2,400.00 inflow   posted   -> Income
//   - Blue Bottle Coffee   +$5.75  outflow  PENDING  -> Spending / Food & Drink
//   - Rainy Day Savings  +$500.00  outflow  posted   -> Transfer / savings_contribution
//   - Transfer from Checking -$500 inflow   posted   -> Transfer (plain mirror)
//   - Side Hustle Co     -$150.00  inflow   posted   -> needs-review

// fixedNow pins the composer's clock to mid-June 2026 so "the current month" is
// the month the stub set lives in, independent of the wall clock.
var fixedNow = time.Date(2026, time.June, 15, 12, 0, 0, 0, time.UTC)

// stubBank is an in-package banking.BankProvider stand-in: one login exposing a
// checking and a counts-as-savings account, and a fixed transaction set on the
// initial (empty-cursor) pull spanning the categorization ladder and a savings
// transfer pair. It imports no provider client, keeping the home test
// provider-agnostic.
type stubBank struct{}

func (stubBank) accounts() []banking.Account {
	return []banking.Account{
		{ID: "p-check", Name: "Everyday Checking", Kind: banking.KindCash, Type: "depository", Subtype: "checking",
			Balance: banking.Balance{AccountID: "p-check", Known: true, Money: banking.Money{Amount: 1200, Currency: "USD"}}},
		{ID: "p-save", Name: "High-Yield Savings", Kind: banking.KindCash, Type: "depository", Subtype: "savings", CountsAsSavings: true,
			Balance: banking.Balance{AccountID: "p-save", Known: true, Money: banking.Money{Amount: 3400, Currency: "USD"}}},
	}
}

func (s stubBank) ListAccounts(_ contextx.ContextX, _ string) ([]banking.Account, error) {
	return s.accounts(), nil
}

func (s stubBank) GetBalances(_ contextx.ContextX, _ string) ([]banking.Balance, error) {
	accts := s.accounts()
	out := make([]banking.Balance, len(accts))
	for i, a := range accts {
		out[i] = a.Balance
	}
	return out, nil
}

func stubTxn(id, account string, day int, amount float64, pending bool, primary, detailed string) banking.Transaction {
	return banking.Transaction{
		ID: id, AccountID: account, Date: time.Date(2026, time.June, day, 0, 0, 0, 0, time.UTC),
		Amount: banking.Money{Amount: amount, Currency: "USD"}, Merchant: "Merchant " + id, Counterparty: "RAW " + id,
		Category: banking.Category{Primary: primary, Detailed: detailed}, Pending: pending,
	}
}

func (stubBank) SyncTransactions(_ contextx.ContextX, _, cursor string) (banking.TransactionChanges, error) {
	if cursor != "" {
		return banking.TransactionChanges{Cursor: cursor}, nil
	}
	return banking.TransactionChanges{
		Cursor: "c1",
		Added: []banking.Transaction{
			stubTxn("t-groceries", "p-check", 1, 84.32, false, "GENERAL_MERCHANDISE", "GENERAL_MERCHANDISE_SUPERSTORES"),
			stubTxn("t-paycheck", "p-check", 2, -2400.00, false, "INCOME", "INCOME_WAGES"),
			stubTxn("t-coffee", "p-check", 3, 5.75, true, "FOOD_AND_DRINK", "FOOD_AND_DRINK_COFFEE"),
			stubTxn("t-transfer", "p-check", 4, 500.00, false, "TRANSFER_OUT", "TRANSFER_OUT_SAVINGS"),
			stubTxn("t-transfer-in", "p-save", 4, -500.00, false, "TRANSFER_IN", "TRANSFER_IN_ACCOUNT_TRANSFER"),
			stubTxn("t-sidegig", "p-check", 5, -150.00, false, "", ""),
		},
	}, nil
}

func (stubBank) CreateLinkToken(_ contextx.ContextX, _ banking.LinkOptions) (banking.LinkToken, error) {
	return banking.LinkToken{}, nil
}

func (stubBank) ExchangePublicToken(_ contextx.ContextX, _ string) (banking.Item, error) {
	return banking.Item{}, nil
}

func (stubBank) RemoveItem(_ contextx.ContextX, _ string) error { return nil }

var _ banking.BankProvider = stubBank{}

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

// testKey is a valid 32-byte (AES-256) hex key for cryptox in tests.
const testKey = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"

// newSyncedServices builds the full service stack over one migrated temp DB,
// links the fake bank, syncs its transactions (which resolves the savings pair),
// and returns the home composer (clock pinned to mid-June 2026) plus the budget
// service the test sets a plan through.
func newSyncedServices(t *testing.T) (*Service, *budget.Service, contextx.ContextX) {
	t.Helper()
	d := newTestDB(t)
	ctx := testCtx()

	provider := stubBank{}
	accountsSvc := accounts.NewService(d, provider, testKey)
	categorizationSvc := categorization.NewService(d, nil)
	transactionsSvc := transactions.NewService(d, provider, accountsSvc, categorizationSvc)
	budgetSvc := budget.NewService(d, categorizationSvc)

	if _, err := accountsSvc.RegisterConnection(ctx, "fake-token", "fake-item"); err != nil {
		t.Fatalf("RegisterConnection: %v", err)
	}
	if err := transactionsSvc.SyncTransactions(ctx); err != nil {
		t.Fatalf("SyncTransactions: %v", err)
	}

	svc := NewService(budgetSvc, transactionsSvc, categorizationSvc, accountsSvc, time.UTC)
	svc.now = func() time.Time { return fixedNow }
	return svc, budgetSvc, ctx
}

// findCategory returns the tracker row for a Category display name.
func findCategory(rows []CategoryRow, name string) (CategoryRow, bool) {
	for _, r := range rows {
		if r.Name == name {
			return r, true
		}
	}
	return CategoryRow{}, false
}

func findWrapCategory(rows []WrapCategoryRow, name string) (WrapCategoryRow, bool) {
	for _, r := range rows {
		if r.Name == name {
			return r, true
		}
	}
	return WrapCategoryRow{}, false
}

// TestCurrentMonthTrackerComposesBudgetedMonth sets a budget, then asserts the
// composed current-month Tracker: per-Category remaining (incl. an over-budget
// Category), the income and savings progress (savings reflects the auto-paired
// $500 contribution), and the no-budget flag stays off.
func TestCurrentMonthTrackerComposesBudgetedMonth(t *testing.T) {
	svc, budgetSvc, ctx := newSyncedServices(t)

	// General Merchandise limit ($50) is below the $84.32 grocery spend -> over
	// budget; Food & Drink limit ($200) comfortably covers the $5.75 coffee.
	if _, err := budgetSvc.SetBudget(ctx, 3000, 1000, []budget.CategoryLimit{
		{CategoryID: categorization.CategoryGeneralMerchandise, Limit: 50},
		{CategoryID: categorization.CategoryFoodAndDrink, Limit: 200},
	}); err != nil {
		t.Fatalf("SetBudget: %v", err)
	}

	view, err := svc.CurrentMonthTracker(ctx)
	if err != nil {
		t.Fatalf("CurrentMonthTracker: %v", err)
	}
	if view.NeedsBudget {
		t.Fatalf("NeedsBudget = true with a budget set")
	}

	t.Run("over-budget category", func(t *testing.T) {
		gm, ok := findCategory(view.Categories, "General Merchandise")
		if !ok {
			t.Fatalf("General Merchandise row missing: %+v", view.Categories)
		}
		if !gm.OverBudget {
			t.Errorf("General Merchandise should be over budget: net %v limit %v", gm.NetSpend, gm.Limit)
		}
		if gm.NetSpend != 84.32 {
			t.Errorf("General Merchandise net spend = %v, want 84.32", gm.NetSpend)
		}
		// remaining = 50 - 84.32 = -34.32
		if gm.Remaining != -34.32 {
			t.Errorf("General Merchandise remaining = %v, want -34.32", gm.Remaining)
		}
	})

	t.Run("within-budget category with pace", func(t *testing.T) {
		fd, ok := findCategory(view.Categories, "Food & Drink")
		if !ok {
			t.Fatalf("Food & Drink row missing: %+v", view.Categories)
		}
		if fd.OverBudget {
			t.Errorf("Food & Drink should be within budget")
		}
		// remaining = 200 - 5.75 = 194.25
		if fd.Remaining != 194.25 {
			t.Errorf("Food & Drink remaining = %v, want 194.25", fd.Remaining)
		}
	})

	t.Run("income progress", func(t *testing.T) {
		if view.IncomeProgress.SoFar != 2400 {
			t.Errorf("income so far = %v, want 2400", view.IncomeProgress.SoFar)
		}
		if view.IncomeProgress.Target != 3000 {
			t.Errorf("income target = %v, want 3000", view.IncomeProgress.Target)
		}
	})

	t.Run("savings progress reflects the auto-paired contribution", func(t *testing.T) {
		if view.SavingsProgress.SoFar != 500 {
			t.Errorf("savings so far = %v, want 500 (the auto-paired contribution)", view.SavingsProgress.SoFar)
		}
		if view.SavingsProgress.Target != 1000 {
			t.Errorf("savings target = %v, want 1000", view.SavingsProgress.Target)
		}
	})
}

// TestCurrentMonthTrackerNoBudget asserts that with no budget set the composer
// reports actuals only and flags NeedsBudget.
func TestCurrentMonthTrackerNoBudget(t *testing.T) {
	svc, _, ctx := newSyncedServices(t)

	view, err := svc.CurrentMonthTracker(ctx)
	if err != nil {
		t.Fatalf("CurrentMonthTracker: %v", err)
	}
	if !view.NeedsBudget {
		t.Fatalf("NeedsBudget = false with no budget set")
	}
	if view.Income != 2400 {
		t.Errorf("actual income = %v, want 2400", view.Income)
	}
	if view.Savings != 500 {
		t.Errorf("actual savings = %v, want 500", view.Savings)
	}
	// total spend = 84.32 + 5.75 (signed sum of spending legs)
	if view.TotalSpend != 90.07 {
		t.Errorf("actual total spend = %v, want 90.07", view.TotalSpend)
	}
}

// TestCurrentMonthTrackerCarriesMonthList asserts the Tracker composes the current
// month's whole transaction set (every classification, newest-first) as its inline
// Transactions list — the same set the wrap carries, now on the current month.
func TestCurrentMonthTrackerCarriesMonthList(t *testing.T) {
	svc, _, ctx := newSyncedServices(t)

	view, err := svc.CurrentMonthTracker(ctx)
	if err != nil {
		t.Fatalf("CurrentMonthTracker: %v", err)
	}

	// All six canned June rows appear — not just Spending; the list spans income,
	// transfers, and the needs-review leg too.
	if len(view.MonthList) != 6 {
		t.Fatalf("MonthList has %d rows, want 6 (the whole June set): %+v", len(view.MonthList), view.MonthList)
	}

	t.Run("newest-first", func(t *testing.T) {
		for i := 1; i < len(view.MonthList); i++ {
			if view.MonthList[i-1].Date.Before(view.MonthList[i].Date) {
				t.Errorf("MonthList not newest-first at %d: %v before %v", i, view.MonthList[i-1].Date, view.MonthList[i].Date)
			}
		}
		if got := view.MonthList[0].Date.Day(); got != 5 {
			t.Errorf("first row day = %d, want 5 (the side-gig leg)", got)
		}
		if got := view.MonthList[len(view.MonthList)-1].Date.Day(); got != 1 {
			t.Errorf("last row day = %d, want 1 (the grocery leg)", got)
		}
	})

	t.Run("spans classifications", func(t *testing.T) {
		var hasIncome, hasTransfer bool
		for _, r := range view.MonthList {
			switch r.Classification {
			case categorization.Income:
				hasIncome = true
			case categorization.Transfer:
				hasTransfer = true
			}
		}
		if !hasIncome || !hasTransfer {
			t.Errorf("MonthList should span classifications: income=%v transfer=%v", hasIncome, hasTransfer)
		}
	})
}

// TestMonthWrapComposesActuals asserts the composed June wrap: net income,
// savings contributed ($500), spend-by-Category, the settling state (the pending
// coffee charge), and the backfill-edge partial badge (June is the earliest
// transaction's month).
func TestMonthWrapComposesActuals(t *testing.T) {
	svc, _, ctx := newSyncedServices(t)

	view, err := svc.MonthWrap(ctx, 2026, time.June)
	if err != nil {
		t.Fatalf("MonthWrap: %v", err)
	}

	t.Run("net income (transfers excluded both sides)", func(t *testing.T) {
		// income 2400 - spending (84.32 + 5.75) = 2309.93
		if view.NetIncome != 2309.93 {
			t.Errorf("net income = %v, want 2309.93", view.NetIncome)
		}
	})

	t.Run("savings contributed is the source leg only", func(t *testing.T) {
		if view.SavingsContributed != 500 {
			t.Errorf("savings contributed = %v, want 500", view.SavingsContributed)
		}
	})

	t.Run("spend by category", func(t *testing.T) {
		gm, ok := findWrapCategory(view.Categories, "General Merchandise")
		if !ok || gm.NetSpend != 84.32 {
			t.Errorf("General Merchandise spend = %+v ok=%v, want 84.32", gm, ok)
		}
		fd, ok := findWrapCategory(view.Categories, "Food & Drink")
		if !ok || fd.NetSpend != 5.75 {
			t.Errorf("Food & Drink spend = %+v ok=%v, want 5.75", fd, ok)
		}
	})

	t.Run("settling because a row is pending", func(t *testing.T) {
		if !view.Settling {
			t.Errorf("wrap should be settling (the coffee charge is pending)")
		}
	})

	t.Run("partial because June is the backfill edge", func(t *testing.T) {
		if !view.Partial {
			t.Errorf("June should be partial (the earliest transaction's month)")
		}
	})
}

// TestWrapSurplusSurfacedInDollars asserts the composer maps the June wrap's
// Surplus (net income − savings contributed) to dollars: 1809.93 for the canned
// set (net income 2309.93 − savings 500), and that it equals net income − savings
// contributed. Surplus is a closed-month figure and is not carried on the
// current-month Tracker.
func TestWrapSurplusSurfacedInDollars(t *testing.T) {
	svc, _, ctx := newSyncedServices(t)

	view, err := svc.MonthWrap(ctx, 2026, time.June)
	if err != nil {
		t.Fatalf("MonthWrap: %v", err)
	}
	if want := 1809.93; view.Surplus != want {
		t.Errorf("wrap surplus = %v, want %v (net income 2309.93 - savings 500)", view.Surplus, want)
	}
	// Net income − savings contributed, within a cent's rounding (float subtraction
	// of two dollar figures is not bit-exact).
	if diff := view.Surplus - (view.NetIncome - view.SavingsContributed); diff > 0.005 || diff < -0.005 {
		t.Errorf("wrap surplus (%v) must equal net income (%v) - savings contributed (%v)", view.Surplus, view.NetIncome, view.SavingsContributed)
	}
}

// TestMonthRailAtCurrentMonth asserts the Tracker's month rail carries the
// current month as its active, right-most chip linking to the root, and never a
// month after the current. The fake set is all June 2026 with the clock in June
// 2026, so the rail is exactly the current month.
func TestMonthRailAtCurrentMonth(t *testing.T) {
	svc, _, ctx := newSyncedServices(t)

	view, err := svc.CurrentMonthTracker(ctx)
	if err != nil {
		t.Fatalf("CurrentMonthTracker: %v", err)
	}
	if len(view.Rail) == 0 {
		t.Fatalf("month rail is empty")
	}
	last := view.Rail[len(view.Rail)-1]
	if last.YM != "2026-06" {
		t.Errorf("right-most chip = %q, want 2026-06 (current month)", last.YM)
	}
	if last.Label != "June 2026" {
		t.Errorf("label = %q, want \"June 2026\"", last.Label)
	}
	if !last.Active {
		t.Errorf("current month chip should be active on the Tracker")
	}
	if last.Href != "/" {
		t.Errorf("current month chip href = %q, want \"/\"", last.Href)
	}
	for _, c := range view.Rail {
		if c.YM > "2026-06" {
			t.Errorf("rail has a chip after the current month: %q", c.YM)
		}
	}
}
