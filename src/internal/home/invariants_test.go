package home

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alecdray/two-cents/src/internal/accounts"
	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/budget"
	"github.com/alecdray/two-cents/src/internal/categorization"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/transactions"
)

// These tests assert the emergent, whole-product invariants of the assembled
// Budget + Tracker + month-wrap dashboard — properties that span the budget
// store, the transactions ledger (with its sign convention and savings pairing),
// the categorization facet, and the app-timezone month math, and so cannot be
// proven by any single module. Each drives the REAL home composer over a
// migrated temp SQLite database fed by the real accounts + transactions +
// categorization + budget services, synced against an in-package bank stub (the
// composer may never import a provider client — the isolation test enforces it
// even for test code), with the clock pinned to mid-June 2026 so "the current
// month" is deterministic. Where an edge needs rows the canned set lacks, the
// test constructs exactly those rows.

// configurableBank is a banking.BankProvider whose accounts and initial
// (empty-cursor) transaction set are supplied per test, so an invariant can
// construct precisely the ledger edge it needs (a refund leg, a prior-month row,
// a month-boundary pair). It imports no provider client.
type configurableBank struct {
	accts []banking.Account
	added []banking.Transaction
}

func (b configurableBank) ListAccounts(_ contextx.ContextX, _ string) ([]banking.Account, error) {
	return b.accts, nil
}

func (b configurableBank) GetBalances(_ contextx.ContextX, _ string) ([]banking.Balance, error) {
	out := make([]banking.Balance, len(b.accts))
	for i, a := range b.accts {
		out[i] = a.Balance
	}
	return out, nil
}

func (b configurableBank) SyncTransactions(_ contextx.ContextX, _, cursor string) (banking.TransactionChanges, error) {
	if cursor != "" {
		return banking.TransactionChanges{Cursor: cursor}, nil
	}
	return banking.TransactionChanges{Cursor: "c1", Added: b.added}, nil
}

func (configurableBank) CreateLinkToken(_ contextx.ContextX, _ banking.LinkOptions) (banking.LinkToken, error) {
	return banking.LinkToken{}, nil
}

func (configurableBank) ExchangePublicToken(_ contextx.ContextX, _ string) (banking.Item, error) {
	return banking.Item{}, nil
}

func (configurableBank) RemoveItem(_ contextx.ContextX, _ string) error { return nil }

var _ banking.BankProvider = configurableBank{}

// checkingAccount is a plain cash checking account.
func checkingAccount() banking.Account {
	return banking.Account{
		ID: "p-check", Name: "Everyday Checking", Kind: banking.KindCash, Type: "depository", Subtype: "checking",
		Balance: banking.Balance{AccountID: "p-check", Known: true, Money: banking.Money{Amount: 1200, Currency: "USD"}},
	}
}

// savingsAccount is a counts-as-savings cash account, the destination an outflow
// transfer must reach to auto-resolve as a savings contribution.
func savingsAccount() banking.Account {
	return banking.Account{
		ID: "p-save", Name: "High-Yield Savings", Kind: banking.KindCash, Type: "depository", Subtype: "savings", CountsAsSavings: true,
		Balance: banking.Balance{AccountID: "p-save", Known: true, Money: banking.Money{Amount: 3400, Currency: "USD"}},
	}
}

// txnOn builds one provider transaction on an explicit calendar date (stored as a
// UTC-midnight date) with the given signed amount (outflow positive, inflow
// negative) and bank category strings.
func txnOn(id, account string, date time.Time, amount float64, pending bool, primary, detailed string) banking.Transaction {
	return banking.Transaction{
		ID: id, AccountID: account, Date: date,
		Amount:   banking.Money{Amount: amount, Currency: "USD"},
		Merchant: "Merchant " + id, Counterparty: "RAW " + id,
		Category: banking.Category{Primary: primary, Detailed: detailed}, Pending: pending,
	}
}

// utcDate is a stored transaction date: a calendar date at UTC midnight.
func utcDate(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// syncedComposer stands up the full service stack over one migrated temp DB,
// links the configurable bank, syncs (resolving any savings pair and
// categorizing every row), and returns the home composer with its clock pinned
// to mid-June 2026, plus the budget service and a context.
func syncedComposer(t *testing.T, accts []banking.Account, added []banking.Transaction) (*Service, *budget.Service, contextx.ContextX) {
	t.Helper()
	d := newTestDB(t)
	ctx := testCtx()

	provider := configurableBank{accts: accts, added: added}
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
	return svc, budgetSvc, ctx
}

// savingsPair is the two legs of a $500 self-transfer into the savings account:
// the outflow source leg auto-resolves to a savings contribution and the inflow
// mirror is left a plain transfer.
func savingsPair(day int) []banking.Transaction {
	return []banking.Transaction{
		txnOn("t-transfer", "p-check", utcDate(2026, time.June, day), 500.00, false, "TRANSFER_OUT", "TRANSFER_OUT_SAVINGS"),
		txnOn("t-transfer-in", "p-save", utcDate(2026, time.June, day), -500.00, false, "TRANSFER_IN", "TRANSFER_IN_ACCOUNT_TRANSFER"),
	}
}

// --- A refund is net spend, never income ---

// TestRefundReducesNetSpend proves a refund (a negative-amount Spending row in a
// Category) nets against that Category's purchases: the Tracker's remaining for
// the Category increases by exactly the refund, the Category's net spend is the
// purchase minus the refund, the wrap's spend-by-Category for it is the same
// netted figure, and the refund is never counted toward income. The refund is
// run against a no-refund baseline so the increase is demonstrated as a delta.
func TestRefundReducesNetSpend(t *testing.T) {
	const limit = 200.0
	setGroceriesBudget := func(t *testing.T, b *budget.Service, ctx contextx.ContextX) {
		t.Helper()
		if _, err := b.SetBudget(ctx, 3000, 0, []budget.CategoryLimit{
			{CategoryID: categorization.CategoryGeneralMerchandise, Limit: limit},
		}); err != nil {
			t.Fatalf("SetBudget: %v", err)
		}
	}

	baseAdded := []banking.Transaction{
		txnOn("t-buy", "p-check", utcDate(2026, time.June, 1), 100.00, false, "GENERAL_MERCHANDISE", "GENERAL_MERCHANDISE_SUPERSTORES"),
		txnOn("t-pay", "p-check", utcDate(2026, time.June, 2), -2400.00, false, "INCOME", "INCOME_WAGES"),
	}
	refundAdded := append(append([]banking.Transaction{}, baseAdded...),
		txnOn("t-refund", "p-check", utcDate(2026, time.June, 3), -30.00, false, "GENERAL_MERCHANDISE", "GENERAL_MERCHANDISE_SUPERSTORES"),
	)

	// Baseline: purchase only, no refund.
	baseSvc, baseBudget, baseCtx := syncedComposer(t, []banking.Account{checkingAccount()}, baseAdded)
	setGroceriesBudget(t, baseBudget, baseCtx)
	baseView, err := baseSvc.CurrentMonthTracker(baseCtx)
	if err != nil {
		t.Fatalf("baseline CurrentMonthTracker: %v", err)
	}
	baseRow, ok := findCategory(baseView.Categories, "General Merchandise")
	if !ok {
		t.Fatalf("baseline General Merchandise row missing: %+v", baseView.Categories)
	}

	// With the refund leg added.
	svc, budgetSvc, ctx := syncedComposer(t, []banking.Account{checkingAccount()}, refundAdded)
	setGroceriesBudget(t, budgetSvc, ctx)
	view, err := svc.CurrentMonthTracker(ctx)
	if err != nil {
		t.Fatalf("CurrentMonthTracker: %v", err)
	}
	row, ok := findCategory(view.Categories, "General Merchandise")
	if !ok {
		t.Fatalf("General Merchandise row missing: %+v", view.Categories)
	}

	t.Run("net spend is the purchase minus the refund", func(t *testing.T) {
		if row.NetSpend != 70.00 {
			t.Errorf("net spend = %v, want 70 (100 purchase - 30 refund)", row.NetSpend)
		}
	})

	t.Run("remaining increases by exactly the refund", func(t *testing.T) {
		if delta := row.Remaining - baseRow.Remaining; delta != 30.00 {
			t.Errorf("remaining moved by %v with the refund, want +30 (the refund amount)", delta)
		}
		if row.Remaining != limit-70.00 {
			t.Errorf("remaining = %v, want %v (limit - net spend)", row.Remaining, limit-70.00)
		}
	})

	t.Run("the refund is not counted as income", func(t *testing.T) {
		if view.Income != 2400.00 {
			t.Errorf("income = %v, want 2400 (the refund must not add to income)", view.Income)
		}
		if view.Income != baseView.Income {
			t.Errorf("income moved with the refund: %v -> %v", baseView.Income, view.Income)
		}
	})

	t.Run("the wrap's spend-by-category shows the netted figure", func(t *testing.T) {
		wrap, err := svc.MonthWrap(ctx, 2026, time.June)
		if err != nil {
			t.Fatalf("MonthWrap: %v", err)
		}
		gm, ok := findWrapCategory(wrap.Categories, "General Merchandise")
		if !ok || gm.NetSpend != 70.00 {
			t.Errorf("wrap General Merchandise spend = %+v ok=%v, want 70 (netted)", gm, ok)
		}
	})
}

// --- A savings move is counted once, consistently across tracker and wrap ---

// TestSavingsCountedOnceAcrossTrackerAndWrap proves the $500 savings contribution
// is counted exactly once and identically by both projections: the current-month
// Tracker's savings-so-far equals the month wrap's savings-contributed equals the
// single $500 source-leg amount. The paired mirror inflow leg is never added, so
// neither view double-counts the move.
func TestSavingsCountedOnceAcrossTrackerAndWrap(t *testing.T) {
	svc, _, ctx := newSyncedServices(t)

	tracker, err := svc.CurrentMonthTracker(ctx)
	if err != nil {
		t.Fatalf("CurrentMonthTracker: %v", err)
	}
	wrap, err := svc.MonthWrap(ctx, 2026, time.June)
	if err != nil {
		t.Fatalf("MonthWrap: %v", err)
	}

	if tracker.Savings != 500.00 {
		t.Errorf("tracker savings-so-far = %v, want 500 (the single source leg, counted once)", tracker.Savings)
	}
	if wrap.SavingsContributed != 500.00 {
		t.Errorf("wrap savings-contributed = %v, want 500 (the single source leg, counted once)", wrap.SavingsContributed)
	}
	if tracker.Savings != wrap.SavingsContributed {
		t.Errorf("tracker (%v) and wrap (%v) disagree on savings; the same move must read the same in both", tracker.Savings, wrap.SavingsContributed)
	}
}

// --- The budget is optional and non-destructive ---

// TestBudgetIsOptionalAndNonDestructive proves the plan is optional and that an
// over-allocated plan persists rather than being rejected. With no budget the
// Tracker reports needs-budget plus actuals (no remaining/pace). After SetBudget
// with Σlimits + savings exceeding income, the verdict is over-allocated yet the
// plan still saves and the Tracker becomes budget-relative. And the Tracker
// reckons only the current month's spend — a prior-month row never bleeds in (no
// rollover / no cumulative carry).
func TestBudgetIsOptionalAndNonDestructive(t *testing.T) {
	added := []banking.Transaction{
		txnOn("t-buy", "p-check", utcDate(2026, time.June, 1), 100.00, false, "GENERAL_MERCHANDISE", "GENERAL_MERCHANDISE_SUPERSTORES"),
		txnOn("t-pay", "p-check", utcDate(2026, time.June, 2), -2400.00, false, "INCOME", "INCOME_WAGES"),
		// A prior-month (May) spending row that must NOT count toward June.
		txnOn("t-may", "p-check", utcDate(2026, time.May, 20), 999.00, false, "GENERAL_MERCHANDISE", "GENERAL_MERCHANDISE_SUPERSTORES"),
	}
	added = append(added, savingsPair(4)...)

	svc, budgetSvc, ctx := syncedComposer(t, []banking.Account{checkingAccount(), savingsAccount()}, added)

	t.Run("no budget: needs-budget plus current-month actuals", func(t *testing.T) {
		view, err := svc.CurrentMonthTracker(ctx)
		if err != nil {
			t.Fatalf("CurrentMonthTracker: %v", err)
		}
		if !view.NeedsBudget {
			t.Fatalf("NeedsBudget = false with no budget set")
		}
		if len(view.Categories) != 0 || view.TotalRemaining != 0 {
			t.Errorf("budget-relative fields populated with no budget: categories=%d totalRemaining=%v", len(view.Categories), view.TotalRemaining)
		}
		// Actuals are the current month only: June spend (100), not June+May (1099).
		if view.TotalSpend != 100.00 {
			t.Errorf("total spend = %v, want 100 (June only — the May row must not carry in)", view.TotalSpend)
		}
		if view.Income != 2400.00 {
			t.Errorf("income = %v, want 2400", view.Income)
		}
		if view.Savings != 500.00 {
			t.Errorf("savings = %v, want 500", view.Savings)
		}
	})

	t.Run("an over-allocated plan still persists and is non-blocking", func(t *testing.T) {
		// Σlimits (500) + savings (800) = 1300 > income (1000) -> over-allocated.
		status, err := budgetSvc.SetBudget(ctx, 1000, 800, []budget.CategoryLimit{
			{CategoryID: categorization.CategoryGeneralMerchandise, Limit: 500},
		})
		if err != nil {
			t.Fatalf("SetBudget returned an error; over-allocation must not block: %v", err)
		}
		if status != budget.OverAllocated {
			t.Errorf("verdict = %q, want over_allocated", status)
		}

		// The plan was saved (not rejected).
		b, limits, err := budgetSvc.GetBudget(ctx)
		if err != nil {
			t.Fatalf("GetBudget: %v", err)
		}
		if budget.IsNoBudget(b, limits) {
			t.Fatalf("the over-allocated plan did not persist (reads as no-budget)")
		}
		if b.IncomeTarget != 1000 || b.SavingsTarget != 800 {
			t.Errorf("persisted targets = (%v, %v), want (1000, 800)", b.IncomeTarget, b.SavingsTarget)
		}
	})

	t.Run("with a budget the tracker is budget-relative over the current month", func(t *testing.T) {
		view, err := svc.CurrentMonthTracker(ctx)
		if err != nil {
			t.Fatalf("CurrentMonthTracker: %v", err)
		}
		if view.NeedsBudget {
			t.Fatalf("NeedsBudget = true after a budget was set")
		}
		gm, ok := findCategory(view.Categories, "General Merchandise")
		if !ok {
			t.Fatalf("General Merchandise row missing: %+v", view.Categories)
		}
		// Current-month spend only: 100 (June), not 1099 (June + the May row).
		if gm.NetSpend != 100.00 {
			t.Errorf("General Merchandise net spend = %v, want 100 (current month only)", gm.NetSpend)
		}
		if gm.Remaining != 400.00 {
			t.Errorf("remaining = %v, want 400 (500 limit - 100 spend)", gm.Remaining)
		}
	})
}

// --- A transaction lands in exactly one month, on its stored calendar date ---

// TestMonthBoundaryAssignment proves the calendar-date bucketing is exact at a
// month boundary: a row stored on the last day of the previous month and a row
// stored on the first day of the current month each land in exactly one month —
// no double-count, no drop. The current-month Tracker counts the June-1 row and
// not the May-31 row, and the wraps agree: June's wrap holds the June-1 row,
// May's wrap holds the May-31 row, and neither holds the other.
func TestMonthBoundaryAssignment(t *testing.T) {
	added := []banking.Transaction{
		// Last calendar day of the previous month (stored UTC-midnight).
		txnOn("t-may-edge", "p-check", utcDate(2026, time.May, 31), 11.11, false, "GENERAL_MERCHANDISE", "GENERAL_MERCHANDISE_SUPERSTORES"),
		// First calendar day of the current month (stored UTC-midnight).
		txnOn("t-jun-edge", "p-check", utcDate(2026, time.June, 1), 22.22, false, "GENERAL_MERCHANDISE", "GENERAL_MERCHANDISE_SUPERSTORES"),
	}
	svc, _, ctx := syncedComposer(t, []banking.Account{checkingAccount()}, added)

	t.Run("the current-month tracker counts the June-1 row, not the May-31 row", func(t *testing.T) {
		view, err := svc.CurrentMonthTracker(ctx)
		if err != nil {
			t.Fatalf("CurrentMonthTracker: %v", err)
		}
		if view.TotalSpend != 22.22 {
			t.Errorf("June total spend = %v, want 22.22 (the June-1 row only; the May-31 row belongs to May)", view.TotalSpend)
		}
	})

	t.Run("the June wrap holds only the June-1 row", func(t *testing.T) {
		wrap, err := svc.MonthWrap(ctx, 2026, time.June)
		if err != nil {
			t.Fatalf("MonthWrap(June): %v", err)
		}
		gm, ok := findWrapCategory(wrap.Categories, "General Merchandise")
		if !ok || gm.NetSpend != 22.22 {
			t.Errorf("June wrap spend = %+v ok=%v, want 22.22", gm, ok)
		}
	})

	t.Run("the May wrap holds only the May-31 row", func(t *testing.T) {
		wrap, err := svc.MonthWrap(ctx, 2026, time.May)
		if err != nil {
			t.Fatalf("MonthWrap(May): %v", err)
		}
		gm, ok := findWrapCategory(wrap.Categories, "General Merchandise")
		if !ok || gm.NetSpend != 11.11 {
			t.Errorf("May wrap spend = %+v ok=%v, want 11.11", gm, ok)
		}
	})
}

// --- A wrap is actuals only, independent of any budget ---

// TestWrapIsActualsOnly proves a month wrap never compares against a budget: its
// output carries only actuals (net income, savings, spend-by-Category, state,
// partial) and the same numbers whether or not a budget is set, and the WrapView
// type exposes no budget/remaining/limit/over-budget field at all.
func TestWrapIsActualsOnly(t *testing.T) {
	svc, budgetSvc, ctx := newSyncedServices(t)

	before, err := svc.MonthWrap(ctx, 2026, time.June)
	if err != nil {
		t.Fatalf("MonthWrap (no budget): %v", err)
	}

	if _, err := budgetSvc.SetBudget(ctx, 3000, 1000, []budget.CategoryLimit{
		{CategoryID: categorization.CategoryGeneralMerchandise, Limit: 50},
		{CategoryID: categorization.CategoryFoodAndDrink, Limit: 200},
	}); err != nil {
		t.Fatalf("SetBudget: %v", err)
	}

	after, err := svc.MonthWrap(ctx, 2026, time.June)
	if err != nil {
		t.Fatalf("MonthWrap (with budget): %v", err)
	}

	t.Run("the wrap is identical with and without a budget", func(t *testing.T) {
		if !reflect.DeepEqual(before, after) {
			t.Errorf("the wrap changed when a budget was set:\n before = %+v\n after  = %+v", before, after)
		}
	})

	t.Run("the WrapView type exposes no budget-comparison field", func(t *testing.T) {
		forbidden := []string{"budget", "remaining", "limit", "over"}
		ty := reflect.TypeOf(WrapView{})
		for i := 0; i < ty.NumField(); i++ {
			name := strings.ToLower(ty.Field(i).Name)
			for _, f := range forbidden {
				if strings.Contains(name, f) {
					t.Errorf("WrapView field %q reads as a budget comparison; a wrap is actuals only", ty.Field(i).Name)
				}
			}
		}
	})
}
