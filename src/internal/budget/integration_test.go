package budget

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/alecdray/two-cents/src/internal/categorization"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/db"

	"github.com/pressly/goose/v3"

	_ "github.com/mattn/go-sqlite3"
)

// These tests exercise the assembled budget module end-to-end: the real Service
// over a real (migrated, temp-file) SQLite DB through the real repo, with a real
// categorization Service over the same DB for the Category list it consults,
// mirroring the categorization integration tests.

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

// newServices builds a budget Service plus the categorization Service it reads,
// both over the same migrated temp DB.
func newServices(t *testing.T) (*Service, *categorization.Service, contextx.ContextX) {
	t.Helper()
	d := newTestDB(t)
	cat := categorization.NewService(d, nil)
	return NewService(d, cat), cat, testCtx()
}

func findLimit(limits []CategoryLimit, categoryID string) (CategoryLimit, bool) {
	for _, l := range limits {
		if l.CategoryID == categoryID {
			return l, true
		}
	}
	return CategoryLimit{}, false
}

// TestSetThenGetRoundTrips asserts a saved budget reads back with its income,
// savings, and limit set intact.
func TestSetThenGetRoundTrips(t *testing.T) {
	svc, _, ctx := newServices(t)

	limits := []CategoryLimit{
		{CategoryID: categorization.CategoryFoodAndDrink, Limit: 400},
		{CategoryID: categorization.CategoryTransportation, Limit: 150},
	}
	status, err := svc.SetBudget(ctx, 3000, 500, limits)
	if err != nil {
		t.Fatalf("SetBudget: %v", err)
	}
	if status != Balanced {
		t.Errorf("status = %v, want %v", status, Balanced)
	}

	b, got, err := svc.GetBudget(ctx)
	if err != nil {
		t.Fatalf("GetBudget: %v", err)
	}
	if b.IncomeTarget != 3000 || b.SavingsTarget != 500 {
		t.Errorf("budget = %+v, want income 3000 savings 500", b)
	}
	if len(got) != 2 {
		t.Fatalf("got %d limits, want 2", len(got))
	}
	if l, ok := findLimit(got, categorization.CategoryFoodAndDrink); !ok || l.Limit != 400 {
		t.Errorf("food limit = %+v ok=%v, want 400", l, ok)
	}
	if l, ok := findLimit(got, categorization.CategoryTransportation); !ok || l.Limit != 150 {
		t.Errorf("transportation limit = %+v ok=%v, want 150", l, ok)
	}
}

// TestReSetReplacesLimitSet asserts re-saving replaces the entire limit set —
// limits dropped on the second save are gone.
func TestReSetReplacesLimitSet(t *testing.T) {
	svc, _, ctx := newServices(t)

	if _, err := svc.SetBudget(ctx, 3000, 500, []CategoryLimit{
		{CategoryID: categorization.CategoryFoodAndDrink, Limit: 400},
		{CategoryID: categorization.CategoryTransportation, Limit: 150},
	}); err != nil {
		t.Fatalf("first SetBudget: %v", err)
	}

	if _, err := svc.SetBudget(ctx, 2500, 300, []CategoryLimit{
		{CategoryID: categorization.CategoryFoodAndDrink, Limit: 350},
	}); err != nil {
		t.Fatalf("second SetBudget: %v", err)
	}

	b, got, err := svc.GetBudget(ctx)
	if err != nil {
		t.Fatalf("GetBudget: %v", err)
	}
	if b.IncomeTarget != 2500 || b.SavingsTarget != 300 {
		t.Errorf("budget = %+v, want income 2500 savings 300", b)
	}
	if len(got) != 1 {
		t.Fatalf("got %d limits, want 1 (old set must be replaced)", len(got))
	}
	if l, ok := findLimit(got, categorization.CategoryFoodAndDrink); !ok || l.Limit != 350 {
		t.Errorf("food limit = %+v ok=%v, want 350", l, ok)
	}
	if _, ok := findLimit(got, categorization.CategoryTransportation); ok {
		t.Errorf("transportation limit should have been removed on re-set")
	}
}

// TestArchivedLimitOmittedButPersists asserts a limit on an archived Category is
// dropped from GetBudget but stays in storage, reappearing on un-archive.
func TestArchivedLimitOmittedButPersists(t *testing.T) {
	svc, cat, ctx := newServices(t)

	if _, err := svc.SetBudget(ctx, 3000, 500, []CategoryLimit{
		{CategoryID: categorization.CategoryFoodAndDrink, Limit: 400},
		{CategoryID: categorization.CategoryTravel, Limit: 600},
	}); err != nil {
		t.Fatalf("SetBudget: %v", err)
	}

	if _, err := cat.ArchiveCategory(ctx, categorization.CategoryTravel); err != nil {
		t.Fatalf("ArchiveCategory: %v", err)
	}

	_, got, err := svc.GetBudget(ctx)
	if err != nil {
		t.Fatalf("GetBudget after archive: %v", err)
	}
	if _, ok := findLimit(got, categorization.CategoryTravel); ok {
		t.Errorf("archived category limit should be omitted from GetBudget")
	}
	if _, ok := findLimit(got, categorization.CategoryFoodAndDrink); !ok {
		t.Errorf("active category limit should remain")
	}

	if _, err := cat.UnarchiveCategory(ctx, categorization.CategoryTravel); err != nil {
		t.Fatalf("UnarchiveCategory: %v", err)
	}
	_, revived, err := svc.GetBudget(ctx)
	if err != nil {
		t.Fatalf("GetBudget after unarchive: %v", err)
	}
	if l, ok := findLimit(revived, categorization.CategoryTravel); !ok || l.Limit != 600 {
		t.Errorf("un-archived category limit should reappear from storage: %+v ok=%v", l, ok)
	}
}

// TestUnsetReadsAsNoBudget asserts an all-zero, no-limit config (never set, or
// saved as zero) reads as no-budget.
func TestUnsetReadsAsNoBudget(t *testing.T) {
	svc, _, ctx := newServices(t)

	b, limits, err := svc.GetBudget(ctx)
	if err != nil {
		t.Fatalf("GetBudget: %v", err)
	}
	if !IsNoBudget(b, limits) {
		t.Errorf("unset config should read as no-budget: %+v %+v", b, limits)
	}
}

// TestOverAllocatedStillPersists asserts an over-allocated plan is saved anyway
// and SetBudget returns OverAllocated (the verdict is non-blocking).
func TestOverAllocatedStillPersists(t *testing.T) {
	svc, _, ctx := newServices(t)

	limits := []CategoryLimit{
		{CategoryID: categorization.CategoryFoodAndDrink, Limit: 2000},
		{CategoryID: categorization.CategoryTransportation, Limit: 1500},
	}
	status, err := svc.SetBudget(ctx, 3000, 500, limits)
	if err != nil {
		t.Fatalf("SetBudget: %v", err)
	}
	if status != OverAllocated {
		t.Errorf("status = %v, want %v", status, OverAllocated)
	}

	b, got, err := svc.GetBudget(ctx)
	if err != nil {
		t.Fatalf("GetBudget: %v", err)
	}
	if b.IncomeTarget != 3000 || len(got) != 2 {
		t.Errorf("over-allocated plan must persist: budget=%+v limits=%d", b, len(got))
	}
}

// TestSetBudgetRejectsUnknownCategory asserts a limit on a nonexistent Category
// is a ValidationError and nothing is saved.
func TestSetBudgetRejectsUnknownCategory(t *testing.T) {
	svc, _, ctx := newServices(t)

	_, err := svc.SetBudget(ctx, 3000, 500, []CategoryLimit{
		{CategoryID: "no_such_category", Limit: 400},
	})
	if err == nil {
		t.Fatalf("SetBudget accepted an unknown category")
	}
	if _, ok := IsValidationError(err); !ok {
		t.Errorf("unknown category error is not a ValidationError: %v", err)
	}

	b, limits, err := svc.GetBudget(ctx)
	if err != nil {
		t.Fatalf("GetBudget: %v", err)
	}
	if !IsNoBudget(b, limits) {
		t.Errorf("nothing should have been saved on validation failure: %+v %+v", b, limits)
	}
}
