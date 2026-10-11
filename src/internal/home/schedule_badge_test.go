package home

import (
	"testing"
	"time"

	"github.com/alecdray/two-cents/src/internal/accounts"
	"github.com/alecdray/two-cents/src/internal/budget"
	"github.com/alecdray/two-cents/src/internal/categorization"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/schedule"
	"github.com/alecdray/two-cents/src/internal/transactions"
)

// newSyncedServicesWithMatch builds the same assembled stack as
// newSyncedServices, plus the schedule Service, and declares+matches one item
// against the stub's "t-groceries" checking transaction (June 1) — the fixture
// every test in this file reads the badge for.
func newSyncedServicesWithMatch(t *testing.T) (*Service, *schedule.Service, contextx.ContextX) {
	t.Helper()
	d := newTestDB(t)
	ctx := testCtx()

	provider := stubBank{}
	accountsSvc := accounts.NewService(d, provider, testKey)
	categorizationSvc := categorization.NewService(d, nil)
	transactionsSvc := transactions.NewService(d, provider, accountsSvc, categorizationSvc, nil, nil)
	budgetSvc := budget.NewService(d, categorizationSvc)

	if _, err := accountsSvc.RegisterConnection(ctx, "fake-token", "fake-item"); err != nil {
		t.Fatalf("RegisterConnection: %v", err)
	}
	if err := transactionsSvc.SyncTransactions(ctx); err != nil {
		t.Fatalf("SyncTransactions: %v", err)
	}

	scheduleSvc := schedule.NewService(d, nil, time.UTC)
	item, err := scheduleSvc.Create(ctx, schedule.Item{
		Name:       "Groceries Co",
		Direction:  schedule.DirectionOut,
		Cadence:    schedule.CadenceMonthly,
		DayOfMonth: 1,
		Active:     true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	occurrence := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	if err := scheduleSvc.MatchOccurrence(ctx, item.ID, occurrence, "t-groceries"); err != nil {
		t.Fatalf("MatchOccurrence: %v", err)
	}

	svc := NewService(budgetSvc, transactionsSvc, categorizationSvc, accountsSvc, scheduleSvc, time.UTC)
	svc.now = func() time.Time { return fixedNow }
	return svc, scheduleSvc, ctx
}

// TestCurrentMonthTrackerCarriesMatchedNames confirms the Tracker's inline
// transaction list carries the same schedule-match badge data the /transactions
// list reads ([ADR-0031]) — the gap this test guards: home's shared row
// component silently rendered no badge for every surface but /transactions
// until MatchedNames was wired through.
func TestCurrentMonthTrackerCarriesMatchedNames(t *testing.T) {
	svc, _, ctx := newSyncedServicesWithMatch(t)

	view, err := svc.CurrentMonthTracker(ctx)
	if err != nil {
		t.Fatalf("CurrentMonthTracker: %v", err)
	}
	if got := view.MatchedNames["t-groceries"]; got != "Groceries Co" {
		t.Errorf("MatchedNames[t-groceries] = %q, want \"Groceries Co\"", got)
	}
	if _, ok := view.MatchedNames["t-paycheck"]; ok {
		t.Errorf("MatchedNames carries t-paycheck, want only the matched row present")
	}
}

// TestMonthWrapCarriesMatchedNames is the wrap's half of the same guard.
func TestMonthWrapCarriesMatchedNames(t *testing.T) {
	svc, _, ctx := newSyncedServicesWithMatch(t)

	view, err := svc.MonthWrap(ctx, 2026, time.June)
	if err != nil {
		t.Fatalf("MonthWrap: %v", err)
	}
	if got := view.MatchedNames["t-groceries"]; got != "Groceries Co" {
		t.Errorf("MatchedNames[t-groceries] = %q, want \"Groceries Co\"", got)
	}
}

// TestSpendDrillCarriesMatchedNames is the drill-down's half, via the income
// figure's own figureDrill path — distinct code from the spending-bucket's
// kept-rows slice, so it needs its own guard against the same gap. It matches
// the stub's paycheck (an inflow) rather than reusing newSyncedServicesWithMatch's
// outflow fixture, since an income declaration must itself be an inflow.
func TestSpendDrillCarriesMatchedNames(t *testing.T) {
	svc, scheduleSvc, ctx := newSyncedServicesWithMatch(t)

	item, err := scheduleSvc.Create(ctx, schedule.Item{
		Name:       "Acme Payroll",
		Direction:  schedule.DirectionIn,
		Cadence:    schedule.CadenceMonthly,
		DayOfMonth: 2,
		Active:     true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	occurrence := time.Date(2026, time.June, 2, 0, 0, 0, 0, time.UTC)
	if err := scheduleSvc.MatchOccurrence(ctx, item.ID, occurrence, "t-paycheck"); err != nil {
		t.Fatalf("MatchOccurrence: %v", err)
	}

	view, err := svc.SpendDrill(ctx, 2026, time.June, "income")
	if err != nil {
		t.Fatalf("SpendDrill: %v", err)
	}
	if got := view.MatchedNames["t-paycheck"]; got != "Acme Payroll" {
		t.Errorf("MatchedNames[t-paycheck] = %q, want \"Acme Payroll\"", got)
	}
}
