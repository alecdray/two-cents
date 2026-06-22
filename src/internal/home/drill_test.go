package home

import (
	"testing"
	"time"

	"github.com/alecdray/two-cents/src/internal/budget"
	"github.com/alecdray/two-cents/src/internal/categorization"
)

// findDrillRow returns the drilled row for a merchant display name.
func findDrillRow(view DrillView, merchant string) bool {
	for _, r := range view.Rows {
		if r.Merchant == merchant {
			return true
		}
	}
	return false
}

// TestSpendDrillReconcilesToWrapFigure is the core invariant: drilling each wrap
// Category bucket yields a list whose net total equals the spend-by-Category
// figure it was reached from. The drill and the wrap aggregate the same month's
// Spending, so the two must agree to the cent.
func TestSpendDrillReconcilesToWrapFigure(t *testing.T) {
	svc, _, ctx := newSyncedServices(t)

	wrap, err := svc.MonthWrap(ctx, 2026, time.June)
	if err != nil {
		t.Fatalf("MonthWrap: %v", err)
	}
	if len(wrap.Categories) == 0 {
		t.Fatalf("wrap has no category figures to drill")
	}

	for _, row := range wrap.Categories {
		drill, err := svc.SpendDrill(ctx, 2026, time.June, row.Bucket)
		if err != nil {
			t.Fatalf("SpendDrill(%q): %v", row.Bucket, err)
		}
		if drill.NetTotal != row.NetSpend {
			t.Errorf("bucket %q: drill total %v != wrap figure %v", row.Bucket, drill.NetTotal, row.NetSpend)
		}
		if len(drill.Rows) == 0 {
			t.Errorf("bucket %q: drill has no rows but the figure is %v", row.Bucket, row.NetSpend)
		}
	}
}

// TestSpendDrillCategoryBucket asserts a single-Category drill: the Spending rows
// assigned that Category, its label, and the reconciling total (the pending
// coffee charge is included, as the wrap counts it).
func TestSpendDrillCategoryBucket(t *testing.T) {
	svc, _, ctx := newSyncedServices(t)

	drill, err := svc.SpendDrill(ctx, 2026, time.June, categorization.CategoryFoodAndDrink)
	if err != nil {
		t.Fatalf("SpendDrill: %v", err)
	}
	if drill.Label != "Food & Drink" {
		t.Errorf("label = %q, want \"Food & Drink\"", drill.Label)
	}
	if len(drill.Rows) != 1 {
		t.Fatalf("rows = %d, want 1 (the coffee charge)", len(drill.Rows))
	}
	if drill.NetTotal != 5.75 {
		t.Errorf("net total = %v, want 5.75", drill.NetTotal)
	}
	if !drill.Rows[0].Pending {
		t.Errorf("the coffee charge should be pending")
	}
}

// TestSpendDrillEverythingElseResidual asserts the residual bucket reconciles to
// the Tracker's everything-else spend: with only General Merchandise budgeted, the
// residual is the unbudgeted Food & Drink spend, and the drill lists exactly it.
func TestSpendDrillEverythingElseResidual(t *testing.T) {
	svc, budgetSvc, ctx := newSyncedServices(t)

	if _, err := budgetSvc.SetBudget(ctx, 3000, 1000, []budget.CategoryLimit{
		{CategoryID: categorization.CategoryGeneralMerchandise, Limit: 50},
	}); err != nil {
		t.Fatalf("SetBudget: %v", err)
	}

	tracker, err := svc.CurrentMonthTracker(ctx)
	if err != nil {
		t.Fatalf("CurrentMonthTracker: %v", err)
	}

	drill, err := svc.SpendDrill(ctx, 2026, time.June, "everything-else")
	if err != nil {
		t.Fatalf("SpendDrill(everything-else): %v", err)
	}
	if drill.Label != "Everything else" {
		t.Errorf("label = %q, want \"Everything else\"", drill.Label)
	}
	if drill.NetTotal != tracker.EverythingElseSpent {
		t.Errorf("residual drill total %v != tracker everything-else spent %v", drill.NetTotal, tracker.EverythingElseSpent)
	}
	if !findDrillRow(drill, "Merchant t-coffee") {
		t.Errorf("residual drill should include the unbudgeted Food & Drink row: %+v", drill.Rows)
	}
	if findDrillRow(drill, "Merchant t-groceries") {
		t.Errorf("residual drill must exclude the budgeted General Merchandise row")
	}
}

// TestSpendDrillEverythingElsePastMonthRejected asserts the residual bucket is
// unavailable for any month but the current one — there is no budget residual to
// drill outside the current month.
func TestSpendDrillEverythingElsePastMonthRejected(t *testing.T) {
	svc, _, ctx := newSyncedServices(t)

	if _, err := svc.SpendDrill(ctx, 2026, time.May, "everything-else"); err != ErrResidualBucketUnavailable {
		t.Errorf("SpendDrill(May, everything-else) err = %v, want ErrResidualBucketUnavailable", err)
	}
}

// TestIncomeDrillReconcilesToGrossIncome asserts the income bucket lists the month's
// Income legs and its total equals the wrap's gross-income figure, with the rows
// oriented positive so they visibly sum to it.
func TestIncomeDrillReconcilesToGrossIncome(t *testing.T) {
	svc, _, ctx := newSyncedServices(t)

	wrap, err := svc.MonthWrap(ctx, 2026, time.June)
	if err != nil {
		t.Fatalf("MonthWrap: %v", err)
	}
	drill, err := svc.SpendDrill(ctx, 2026, time.June, "income")
	if err != nil {
		t.Fatalf("SpendDrill(income): %v", err)
	}
	if drill.Label != "Income" {
		t.Errorf("label = %q, want \"Income\"", drill.Label)
	}
	if drill.NetTotal != wrap.GrossIncome {
		t.Errorf("income drill total %v != wrap gross income %v", drill.NetTotal, wrap.GrossIncome)
	}
	if len(drill.Rows) == 0 {
		t.Fatalf("income drill has no rows but gross income is %v", wrap.GrossIncome)
	}
	for _, r := range drill.Rows {
		if r.Amount.Amount < 0 {
			t.Errorf("income drill row not oriented positive: %v", r.Amount.Amount)
		}
	}
}

// TestSavingsDrillReconcilesToSavingsContributed asserts the savings bucket lists the
// savings-contribution source legs and its total equals the wrap's savings figure.
func TestSavingsDrillReconcilesToSavingsContributed(t *testing.T) {
	svc, _, ctx := newSyncedServices(t)

	wrap, err := svc.MonthWrap(ctx, 2026, time.June)
	if err != nil {
		t.Fatalf("MonthWrap: %v", err)
	}
	drill, err := svc.SpendDrill(ctx, 2026, time.June, "savings")
	if err != nil {
		t.Fatalf("SpendDrill(savings): %v", err)
	}
	if drill.Label != "Savings contributed" {
		t.Errorf("label = %q, want \"Savings contributed\"", drill.Label)
	}
	if drill.NetTotal != wrap.SavingsContributed {
		t.Errorf("savings drill total %v != wrap savings contributed %v", drill.NetTotal, wrap.SavingsContributed)
	}
	if len(drill.Rows) == 0 {
		t.Fatalf("savings drill has no rows but savings contributed is %v", wrap.SavingsContributed)
	}
}

// TestIncomeSavingsDrillAnyMonth asserts the income/savings buckets carry no
// month restriction (unlike the budget residual): a past month with no data
// returns an empty, zero-total drill, not an error.
func TestIncomeSavingsDrillAnyMonth(t *testing.T) {
	svc, _, ctx := newSyncedServices(t)

	for _, bucket := range []string{"income", "savings"} {
		drill, err := svc.SpendDrill(ctx, 2026, time.May, bucket)
		if err != nil {
			t.Fatalf("SpendDrill(May, %s): %v", bucket, err)
		}
		if len(drill.Rows) != 0 || drill.NetTotal != 0 {
			t.Errorf("%s drill for an empty month = %d rows / %v, want 0 / 0", bucket, len(drill.Rows), drill.NetTotal)
		}
	}
}

// TestSpendDrillUncategorizedBucket asserts the uncategorized bucket lists only
// Spending with no Category — empty here, since the synced set is fully
// categorized. The empty list reconciles to a zero total.
func TestSpendDrillUncategorizedBucket(t *testing.T) {
	svc, _, ctx := newSyncedServices(t)

	drill, err := svc.SpendDrill(ctx, 2026, time.June, "uncategorized")
	if err != nil {
		t.Fatalf("SpendDrill(uncategorized): %v", err)
	}
	if drill.Label != "Uncategorized" {
		t.Errorf("label = %q, want \"Uncategorized\"", drill.Label)
	}
	if len(drill.Rows) != 0 || drill.NetTotal != 0 {
		t.Errorf("uncategorized drill = %d rows / %v total, want 0 / 0", len(drill.Rows), drill.NetTotal)
	}
}

// TestSpendDrillReComposesAfterAnEditMovesARow asserts the region's self-refresh
// keeps it honest: after the shared editor's write moves the grocery row out of
// General Merchandise, re-querying that bucket's drill (what the transaction-changed
// listener does) empties it and zeroes its total, and the row reappears under Food &
// Drink with the combined total. The editor issues the write; the drill region owns
// the re-query and re-sum.
func TestSpendDrillReComposesAfterAnEditMovesARow(t *testing.T) {
	svc, _, ctx := newSyncedServices(t)

	foodID := categorization.CategoryFoodAndDrink
	// The write the shared modal issues (Categorization decides, Transactions writes).
	if err := svc.transactions.ReCategorize(ctx, "t-groceries", categorization.Spending, &foodID); err != nil {
		t.Fatalf("ReCategorize: %v", err)
	}

	gm, err := svc.SpendDrill(ctx, 2026, time.June, categorization.CategoryGeneralMerchandise)
	if err != nil {
		t.Fatalf("SpendDrill(General Merchandise): %v", err)
	}
	if len(gm.Rows) != 0 || gm.NetTotal != 0 {
		t.Errorf("General Merchandise drill after move = %d rows / %v, want 0 / 0", len(gm.Rows), gm.NetTotal)
	}

	food, err := svc.SpendDrill(ctx, 2026, time.June, foodID)
	if err != nil {
		t.Fatalf("SpendDrill(Food & Drink): %v", err)
	}
	if len(food.Rows) != 2 {
		t.Errorf("Food & Drink drill rows = %d, want 2 (coffee + moved groceries)", len(food.Rows))
	}
	if food.NetTotal != 90.07 {
		t.Errorf("Food & Drink drill total = %v, want 90.07 (84.32 + 5.75)", food.NetTotal)
	}
}
