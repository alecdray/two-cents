package categorization

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/db"

	"github.com/pressly/goose/v3"

	_ "github.com/mattn/go-sqlite3"
)

// These tests exercise the assembled categorization module end-to-end: the real
// Service over a real (migrated, temp-file) SQLite DB through the real repo,
// mirroring the accounts/transactions integration tests. The cross-domain
// re-categorization seam is driven by a recording stub so the tests can assert
// exactly which substrings each Rule mutation passes and that Category mutations
// never call it.

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

// reapplyStub records the substrings each Rule mutation passes through the seam
// and returns a fixed count, so tests can assert the seam contract.
type reapplyStub struct {
	calls [][]string
	count int
}

func (r *reapplyStub) fn() ReapplyCategorization {
	return func(_ contextx.ContextX, substrings []string) (int, error) {
		captured := make([]string, len(substrings))
		copy(captured, substrings)
		r.calls = append(r.calls, captured)
		return r.count, nil
	}
}

// builtInIDs are the twelve stable ids the categories migration seeds.
var builtInIDs = []string{
	CategoryFoodAndDrink, CategoryGeneralMerchandise, CategoryTransportation,
	CategoryTravel, CategoryRentAndUtilities, CategoryMedical, CategoryPersonalCare,
	CategoryGeneralServices, CategoryEntertainment, CategoryHomeImprovement,
	CategoryBankFees, CategoryGovernmentAndNonProfit,
}

// TestBuiltInsSeeded asserts the migration seeds the twelve built-in spending
// Categories with their stable ids, all built-in and active.
func TestBuiltInsSeeded(t *testing.T) {
	svc := NewService(newTestDB(t), nil)
	ctx := testCtx()

	all, err := svc.ListCategories(ctx, true)
	if err != nil {
		t.Fatalf("ListCategories: %v", err)
	}

	byID := map[string]Category{}
	for _, c := range all {
		byID[c.ID] = c
	}
	for _, id := range builtInIDs {
		c, ok := byID[id]
		if !ok {
			t.Errorf("built-in category %q not seeded", id)
			continue
		}
		if !c.Builtin {
			t.Errorf("category %q should be built-in", id)
		}
		if c.Archived {
			t.Errorf("category %q should not be archived on seed", id)
		}
		if c.Name == "" {
			t.Errorf("category %q has no name", id)
		}
	}
	if len(byID) != len(builtInIDs) {
		t.Errorf("expected exactly %d seeded categories, got %d", len(builtInIDs), len(byID))
	}
}

// TestCreateAndRenameCategory asserts a created custom category appears in the
// active list and that renaming keeps its id stable.
func TestCreateAndRenameCategory(t *testing.T) {
	svc := NewService(newTestDB(t), nil)
	ctx := testCtx()

	created, err := svc.CreateCategory(ctx, "Coffee Runs")
	if err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}
	if created.Builtin {
		t.Errorf("a created category must be custom, not built-in")
	}

	active, err := svc.ListCategories(ctx, false)
	if err != nil {
		t.Fatalf("ListCategories: %v", err)
	}
	if !containsID(active, created.ID) {
		t.Fatalf("created category %q not in active list", created.ID)
	}

	renamed, err := svc.RenameCategory(ctx, created.ID, "Coffee")
	if err != nil {
		t.Fatalf("RenameCategory: %v", err)
	}
	if renamed.ID != created.ID {
		t.Errorf("rename changed the id: %q -> %q", created.ID, renamed.ID)
	}
	if renamed.Name != "Coffee" {
		t.Errorf("rename did not apply: name = %q", renamed.Name)
	}
}

// TestArchiveExcludesFromActiveAndAutoAssign asserts archiving removes a Category
// from the active list and from ResolveCategorization auto-assignment, while
// unarchive restores it.
func TestArchiveExcludesFromActiveAndAutoAssign(t *testing.T) {
	svc := NewService(newTestDB(t), nil)
	ctx := testCtx()

	if _, err := svc.ArchiveCategory(ctx, CategoryFoodAndDrink); err != nil {
		t.Fatalf("ArchiveCategory: %v", err)
	}

	active, err := svc.ListCategories(ctx, false)
	if err != nil {
		t.Fatalf("ListCategories(active): %v", err)
	}
	if containsID(active, CategoryFoodAndDrink) {
		t.Errorf("archived category still in active list")
	}

	all, err := svc.ListCategories(ctx, true)
	if err != nil {
		t.Fatalf("ListCategories(all): %v", err)
	}
	if !containsID(all, CategoryFoodAndDrink) {
		t.Errorf("archived category dropped from the full list — archive must not hard-delete")
	}

	// Auto-assignment skips the archived target: a food-and-drink outflow now
	// falls through to uncategorized Spending instead of the archived category.
	decision := ResolveCategorization(ResolveInput{
		CleanMerchant: "WHOLE FOODS",
		Categories:    all,
		BankCategory:  bankPrimary(pfcFoodAndDrink),
		Amount:        40,
	})
	if decision.Classification != Spending || decision.CategoryID != nil {
		t.Errorf("archived category was still auto-assigned: %+v", decision)
	}

	if _, err := svc.UnarchiveCategory(ctx, CategoryFoodAndDrink); err != nil {
		t.Fatalf("UnarchiveCategory: %v", err)
	}
	activeAgain, err := svc.ListCategories(ctx, false)
	if err != nil {
		t.Fatalf("ListCategories after unarchive: %v", err)
	}
	if !containsID(activeAgain, CategoryFoodAndDrink) {
		t.Errorf("unarchive did not restore the category to the active list")
	}
}

// TestCategoryNameValidation asserts blank and duplicate (case-insensitive) names
// are rejected as ValidationErrors.
func TestCategoryNameValidation(t *testing.T) {
	svc := NewService(newTestDB(t), nil)
	ctx := testCtx()

	if _, err := svc.CreateCategory(ctx, "   "); err == nil {
		t.Errorf("blank name was accepted")
	} else if _, ok := IsValidationError(err); !ok {
		t.Errorf("blank name error is not a ValidationError: %v", err)
	}

	// Duplicate of a seeded built-in, different case.
	if _, err := svc.CreateCategory(ctx, "food & drink"); err == nil {
		t.Errorf("duplicate name was accepted")
	} else if _, ok := IsValidationError(err); !ok {
		t.Errorf("duplicate name error is not a ValidationError: %v", err)
	}
}

// TestCategoryOpsNeverCallSeam asserts none of the Category lifecycle operations
// invoke the re-categorization seam.
func TestCategoryOpsNeverCallSeam(t *testing.T) {
	stub := &reapplyStub{count: 5}
	svc := NewService(newTestDB(t), stub.fn())
	ctx := testCtx()

	custom, err := svc.CreateCategory(ctx, "Coffee")
	if err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}
	if _, err := svc.RenameCategory(ctx, custom.ID, "Coffee Shops"); err != nil {
		t.Fatalf("RenameCategory: %v", err)
	}
	if _, err := svc.ArchiveCategory(ctx, custom.ID); err != nil {
		t.Fatalf("ArchiveCategory: %v", err)
	}
	if _, err := svc.UnarchiveCategory(ctx, custom.ID); err != nil {
		t.Fatalf("UnarchiveCategory: %v", err)
	}

	if len(stub.calls) != 0 {
		t.Errorf("category operations called the re-categorization seam %d times; they must never call it", len(stub.calls))
	}
}

func containsID(categories []Category, id string) bool {
	for _, c := range categories {
		if c.ID == id {
			return true
		}
	}
	return false
}
