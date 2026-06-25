package categorization

import (
	"testing"
	"time"

	"github.com/alecdray/two-cents/src/internal/banking"
)

// TestRulesMatchingWinnerAndOrdering asserts that a merchant matching several
// Rules returns them in precedence order with exactly one winner first, that the
// winner is the longest-substring match, and that the winner agrees with what
// ResolveCategorization would apply for the same merchant (the no-divergence
// property).
func TestRulesMatchingWinnerAndOrdering(t *testing.T) {
	svc := NewService(newTestDB(t), (&reapplyStub{}).fn())
	ctx := testCtx()

	// Three overlapping spending Rules; "STARBUCKS" is the longest substring of
	// the merchant, so it governs.
	if _, _, err := svc.CreateRule(ctx, "STAR", Spending, strptr(CategoryGeneralMerchandise)); err != nil {
		t.Fatalf("CreateRule STAR: %v", err)
	}
	if _, _, err := svc.CreateRule(ctx, "STARB", Spending, strptr(CategoryEntertainment)); err != nil {
		t.Fatalf("CreateRule STARB: %v", err)
	}
	if _, _, err := svc.CreateRule(ctx, "STARBUCKS", Spending, strptr(CategoryFoodAndDrink)); err != nil {
		t.Fatalf("CreateRule STARBUCKS: %v", err)
	}

	got, err := svc.RulesMatching(ctx, "STARBUCKS RESERVE", "")
	if err != nil {
		t.Fatalf("RulesMatching: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d matches, want 3: %+v", len(got), got)
	}

	// Exactly one winner, and it is first.
	winners := 0
	for i, m := range got {
		if m.IsWinner {
			winners++
			if i != 0 {
				t.Errorf("winner is not first: index %d", i)
			}
		}
	}
	if winners != 1 {
		t.Fatalf("got %d winners, want exactly 1", winners)
	}

	// Ordering is longest-substring first.
	wantOrder := []string{"STARBUCKS", "STARB", "STAR"}
	for i, want := range wantOrder {
		if got[i].Rule.MerchantSubstring != want {
			t.Errorf("position %d: got %q, want %q", i, got[i].Rule.MerchantSubstring, want)
		}
	}

	// The winner carries its target Category's display name.
	if got[0].CategoryName != "Food & Drink" {
		t.Errorf("winner category name = %q, want %q", got[0].CategoryName, "Food & Drink")
	}

	// No-divergence: the winner equals what the engine applies for the same merchant.
	decision, err := svc.Resolve(ctx, banking.Category{}, "STARBUCKS RESERVE", "", banking.Money{Amount: 8})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	winnerDecision := decisionFromRule(got[0].Rule)
	if decision.Classification != winnerDecision.Classification {
		t.Errorf("winner classification %q != engine %q", winnerDecision.Classification, decision.Classification)
	}
	if (decision.CategoryID == nil) != (winnerDecision.CategoryID == nil) ||
		(decision.CategoryID != nil && *decision.CategoryID != *winnerDecision.CategoryID) {
		t.Errorf("winner category %v != engine category %v", winnerDecision.CategoryID, decision.CategoryID)
	}
}

// TestMatchingRulesRecencyTiebreak asserts that when two matching Rules have
// equal-length substrings the most-recently-edited one is first (the winner),
// matching the engine's precedence. It drives the shared precedence helper that
// RulesMatching uses with explicit edit times — the DB stores updated_at at
// second granularity, too coarse to order edits made within the same test.
func TestMatchingRulesRecencyTiebreak(t *testing.T) {
	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	rules := []Rule{
		spendingRule("old", "ACME", CategoryGeneralMerchandise, older),
		spendingRule("new", "ACME", CategoryEntertainment, newer),
	}

	got := matchingRulesBestFirst("ACME WIDGETS", rules, nil)
	if len(got) != 2 {
		t.Fatalf("got %d matches, want 2: %+v", len(got), got)
	}
	if got[0].ID != "new" {
		t.Errorf("recency tiebreak: first = %q, want the most-recently-edited %q", got[0].ID, "new")
	}

	// The helper's winner agrees with what the engine applies for the same merchant.
	decision := ResolveCategorization(ResolveInput{CleanMerchant: "ACME WIDGETS", Rules: rules, Amount: 25})
	if got[0].CategoryID == nil || decision.CategoryID == nil || *got[0].CategoryID != *decision.CategoryID {
		t.Errorf("helper winner %+v disagrees with engine %+v", got[0].CategoryID, decision.CategoryID)
	}
}

// TestRulesMatchingSkipsArchivedTarget asserts a spending Rule whose target
// Category is archived is excluded from the result, exactly as the engine skips
// it, letting a still-active shorter Rule govern.
func TestRulesMatchingSkipsArchivedTarget(t *testing.T) {
	svc := NewService(newTestDB(t), (&reapplyStub{}).fn())
	ctx := testCtx()

	// A custom Category that we will archive after pointing a Rule at it.
	custom, err := svc.CreateCategory(ctx, "Coffee")
	if err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}
	if _, _, err := svc.CreateRule(ctx, "STARBUCKS", Spending, strptr(custom.ID)); err != nil {
		t.Fatalf("CreateRule long: %v", err)
	}
	if _, _, err := svc.CreateRule(ctx, "STAR", Spending, strptr(CategoryFoodAndDrink)); err != nil {
		t.Fatalf("CreateRule short: %v", err)
	}
	if _, err := svc.ArchiveCategory(ctx, custom.ID); err != nil {
		t.Fatalf("ArchiveCategory: %v", err)
	}

	got, err := svc.RulesMatching(ctx, "STARBUCKS RESERVE", "")
	if err != nil {
		t.Fatalf("RulesMatching: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d matches, want 1 (archived-target Rule excluded): %+v", len(got), got)
	}
	if !got[0].IsWinner || got[0].Rule.MerchantSubstring != "STAR" {
		t.Errorf("winner = %+v, want active STAR Rule", got[0])
	}

	// And the engine agrees: the active shorter Rule governs the resolve.
	decision, err := svc.Resolve(ctx, banking.Category{}, "STARBUCKS RESERVE", "", banking.Money{Amount: 8})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if decision.CategoryID == nil || *decision.CategoryID != CategoryFoodAndDrink {
		t.Errorf("engine resolve = %+v, want food_and_drink", decision)
	}
}

// TestRulesMatchingNoMatch asserts a merchant no Rule matches returns an empty
// slice and no error.
func TestRulesMatchingNoMatch(t *testing.T) {
	svc := NewService(newTestDB(t), (&reapplyStub{}).fn())
	ctx := testCtx()

	if _, _, err := svc.CreateRule(ctx, "STARBUCKS", Spending, strptr(CategoryFoodAndDrink)); err != nil {
		t.Fatalf("CreateRule: %v", err)
	}

	got, err := svc.RulesMatching(ctx, "WHOLE FOODS", "")
	if err != nil {
		t.Fatalf("RulesMatching: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d matches, want 0: %+v", len(got), got)
	}
}

// TestRulesMatchingIncomeRuleHasNoCategoryName asserts an income/transfer Rule
// match carries an empty CategoryName (it targets no Category).
func TestRulesMatchingIncomeRuleHasNoCategoryName(t *testing.T) {
	svc := NewService(newTestDB(t), (&reapplyStub{}).fn())
	ctx := testCtx()

	if _, _, err := svc.CreateRule(ctx, "ACME PAYROLL", Income, nil); err != nil {
		t.Fatalf("CreateRule: %v", err)
	}

	got, err := svc.RulesMatching(ctx, "ACME PAYROLL", "")
	if err != nil {
		t.Fatalf("RulesMatching: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d matches, want 1", len(got))
	}
	if got[0].CategoryName != "" {
		t.Errorf("income Rule CategoryName = %q, want empty", got[0].CategoryName)
	}
}
