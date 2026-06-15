package categorization

import (
	"testing"
	"time"

	"github.com/alecdray/two-cents/src/internal/banking"
)

// strptr is a small helper for the *string Category ids the Decision carries.
func strptr(s string) *string { return &s }

// bankPrimary builds a banking.Category carrying only a primary value.
func bankPrimary(primary string) banking.Category { return banking.Category{Primary: primary} }

// spendingRule builds a spending Rule matching substring with the given target
// Category and edit time.
func spendingRule(id, substring, categoryID string, updatedAt time.Time) Rule {
	return Rule{
		ID:                id,
		MerchantSubstring: substring,
		Classification:    Spending,
		CategoryID:        strptr(categoryID),
		UpdatedAt:         updatedAt,
	}
}

// TestResolveOverrideWins asserts the defensive manual-override branch: when an
// override Decision is present the engine returns it verbatim and consults
// nothing else, even when a Rule and a bank category would otherwise fire.
func TestResolveOverrideWins(t *testing.T) {
	override := Decision{Classification: Transfer}
	got := ResolveCategorization(ResolveInput{
		Override:      &override,
		CleanMerchant: "WHOLE FOODS",
		Rules:         []Rule{spendingRule("r1", "WHOLE FOODS", CategoryFoodAndDrink, time.Now())},
		BankCategory:  banking.Category{Primary: pfcFoodAndDrink},
		Amount:        50,
	})
	if got.Classification != Transfer || got.CategoryID != nil {
		t.Fatalf("override not honored: got %+v, want Transfer/no-category", got)
	}
}

// TestResolveRuleBeatsBankCategory asserts a matching Rule wins over the bank
// category (precedence 2 over 3).
func TestResolveRuleBeatsBankCategory(t *testing.T) {
	got := ResolveCategorization(ResolveInput{
		CleanMerchant: "SHELL OIL",
		Rules:         []Rule{spendingRule("r1", "SHELL", CategoryTransportation, time.Now())},
		// Bank says general merchandise; the Rule must override to transportation.
		BankCategory: banking.Category{Primary: pfcGeneralMerch},
		Amount:       40,
	})
	if got.Classification != Spending || got.CategoryID == nil || *got.CategoryID != CategoryTransportation {
		t.Fatalf("rule did not beat bank category: got %+v", got)
	}
}

// TestResolveLongestSubstringWins asserts the longest matching substring wins
// when several Rules match the same merchant.
func TestResolveLongestSubstringWins(t *testing.T) {
	now := time.Now()
	got := ResolveCategorization(ResolveInput{
		CleanMerchant: "STARBUCKS RESERVE",
		Rules: []Rule{
			spendingRule("short", "STAR", CategoryGeneralMerchandise, now),
			spendingRule("long", "STARBUCKS", CategoryFoodAndDrink, now),
		},
		Amount: 8,
	})
	if got.CategoryID == nil || *got.CategoryID != CategoryFoodAndDrink {
		t.Fatalf("longest substring did not win: got %+v, want food_and_drink", got)
	}
}

// TestResolveRecencyTiebreak asserts that when two matching Rules have
// equal-length substrings the most-recently-edited one wins.
func TestResolveRecencyTiebreak(t *testing.T) {
	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	got := ResolveCategorization(ResolveInput{
		CleanMerchant: "ACME WIDGETS",
		Rules: []Rule{
			spendingRule("old", "ACME", CategoryGeneralMerchandise, older),
			spendingRule("new", "ACME", CategoryEntertainment, newer),
		},
		Amount: 25,
	})
	if got.CategoryID == nil || *got.CategoryID != CategoryEntertainment {
		t.Fatalf("recency tiebreak failed: got %+v, want entertainment (most recently edited)", got)
	}
}

// TestResolveSkipsArchivedRuleTarget asserts a spending Rule whose Category is
// archived is skipped, letting a still-active shorter Rule win instead.
func TestResolveSkipsArchivedRuleTarget(t *testing.T) {
	now := time.Now()
	got := ResolveCategorization(ResolveInput{
		CleanMerchant: "STARBUCKS RESERVE",
		Rules: []Rule{
			spendingRule("archived-long", "STARBUCKS", "custom_coffee", now),
			spendingRule("active-short", "STAR", CategoryFoodAndDrink, now),
		},
		Categories: []Category{{ID: "custom_coffee", Name: "Coffee", Archived: true}},
		Amount:     8,
	})
	if got.CategoryID == nil || *got.CategoryID != CategoryFoodAndDrink {
		t.Fatalf("archived rule target not skipped: got %+v, want food_and_drink", got)
	}
}

// TestResolveBankCategoryTransfer asserts each transfer-signal primary resolves
// to Transfer with no Category.
func TestResolveBankCategoryTransfer(t *testing.T) {
	for _, primary := range []string{pfcTransferIn, pfcTransferOut, pfcLoanPayments} {
		got := ResolveCategorization(ResolveInput{
			CleanMerchant: "INTERNAL MOVE",
			BankCategory:  banking.Category{Primary: primary},
			Amount:        100,
		})
		if got.Classification != Transfer || got.CategoryID != nil {
			t.Errorf("primary %s: got %+v, want Transfer/no-category", primary, got)
		}
	}
}

// TestResolveBankCategoryIncome asserts the INCOME primary resolves to Income.
func TestResolveBankCategoryIncome(t *testing.T) {
	got := ResolveCategorization(ResolveInput{
		CleanMerchant: "ACME PAYROLL",
		BankCategory:  banking.Category{Primary: pfcIncome},
		Amount:        -2400, // an inflow
	})
	if got.Classification != Income || got.CategoryID != nil {
		t.Fatalf("income primary: got %+v, want Income/no-category", got)
	}
}

// TestResolveBankCategorySpending asserts a spending primary maps to its built-in
// Category and is case-insensitive on the primary value.
func TestResolveBankCategorySpending(t *testing.T) {
	got := ResolveCategorization(ResolveInput{
		CleanMerchant: "WHOLE FOODS",
		BankCategory:  banking.Category{Primary: "food_and_drink"}, // lower-case primary
		Amount:        62,
	})
	if got.Classification != Spending || got.CategoryID == nil || *got.CategoryID != CategoryFoodAndDrink {
		t.Fatalf("spending primary mapping: got %+v, want food_and_drink", got)
	}
}

// TestResolveRefundStaysSpending asserts a spending-mapped INFLOW (refund) stays
// Spending with its Category — the negative amount carries the refund sign; it is
// never re-routed to Income.
func TestResolveRefundStaysSpending(t *testing.T) {
	got := ResolveCategorization(ResolveInput{
		CleanMerchant: "WHOLE FOODS",
		BankCategory:  banking.Category{Primary: pfcFoodAndDrink},
		Amount:        -19.99, // an inflow: a refund
	})
	if got.Classification != Spending || got.CategoryID == nil || *got.CategoryID != CategoryFoodAndDrink {
		t.Fatalf("refund did not stay Spending: got %+v", got)
	}
}

// TestResolveSkipsArchivedBankCategoryTarget asserts that when the PFC-mapped
// Category is archived the bank-category step is skipped and the engine falls
// through to the direction fallback.
func TestResolveSkipsArchivedBankCategoryTarget(t *testing.T) {
	got := ResolveCategorization(ResolveInput{
		CleanMerchant: "WHOLE FOODS",
		BankCategory:  banking.Category{Primary: pfcFoodAndDrink},
		Categories:    []Category{{ID: CategoryFoodAndDrink, Name: "Food & Drink", Builtin: true, Archived: true}},
		Amount:        62, // outflow → falls through to uncategorized Spending
	})
	if got.Classification != Spending || got.CategoryID != nil {
		t.Fatalf("archived bank-category target not skipped: got %+v, want uncategorized Spending", got)
	}
}

// TestResolveDirectionFallback asserts the final fallback: an outflow with no
// other signal is uncategorized Spending; an inflow is needs-review (never
// auto-Income).
func TestResolveDirectionFallback(t *testing.T) {
	outflow := ResolveCategorization(ResolveInput{CleanMerchant: "UNKNOWN", Amount: 30})
	if outflow.Classification != Spending || outflow.CategoryID != nil {
		t.Errorf("outflow fallback: got %+v, want uncategorized Spending", outflow)
	}

	inflow := ResolveCategorization(ResolveInput{CleanMerchant: "UNKNOWN", Amount: -30})
	if inflow.Classification != NeedsReview || inflow.CategoryID != nil {
		t.Errorf("inflow fallback: got %+v, want needs-review", inflow)
	}
}

// TestCleanMerchantName table-drives the cleaned-merchant derivation: the
// provider merchant is preferred; otherwise the counterparty is normalized.
func TestCleanMerchantName(t *testing.T) {
	cases := []struct {
		name         string
		merchant     string
		counterparty string
		want         string
	}{
		{"prefers provider merchant", "Whole Foods", "WHOLEFDS #123 AUSTIN", "Whole Foods"},
		{"trims provider merchant", "  Starbucks  ", "ignored", "Starbucks"},
		{"falls back to counterparty when merchant blank", "", "amazon.com", "AMAZON.COM"},
		{"strips trailing store number", "", "WALMART #1234", "WALMART"},
		{"strips bare trailing numeric id", "", "STARBUCKS 00456", "STARBUCKS"},
		{"strips multiple trailing numeric ids", "", "SHELL OIL 8842 0099", "SHELL OIL"},
		{"collapses internal whitespace", "", "  costco    wholesale  ", "COSTCO WHOLESALE"},
		{"keeps embedded number that is part of a name", "", "7 ELEVEN", "7 ELEVEN"},
		{"empty both", "", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := CleanMerchantName(c.merchant, c.counterparty); got != c.want {
				t.Errorf("CleanMerchantName(%q, %q) = %q, want %q", c.merchant, c.counterparty, got, c.want)
			}
		})
	}
}

// TestBankCategoryMapCoversSixteenPrimaries asserts the bank-category mapping
// covers all sixteen Plaid primaries: four non-spending (INCOME + three transfer
// signals) and exactly twelve spending primaries mapping to the twelve built-in
// Category ids.
func TestBankCategoryMapCoversSixteenPrimaries(t *testing.T) {
	nonSpending := []struct {
		primary string
		want    Classification
	}{
		{pfcIncome, Income},
		{pfcTransferIn, Transfer},
		{pfcTransferOut, Transfer},
		{pfcLoanPayments, Transfer},
	}
	for _, c := range nonSpending {
		got := ResolveCategorization(ResolveInput{BankCategory: banking.Category{Primary: c.primary}, Amount: 10})
		if got.Classification != c.want {
			t.Errorf("primary %s: got %s, want %s", c.primary, got.Classification, c.want)
		}
	}

	spendingPrimaries := []string{
		pfcBankFees, pfcEntertainment, pfcFoodAndDrink, pfcGeneralMerch,
		pfcHomeImprove, pfcMedical, pfcPersonalCare, pfcGeneralServ,
		pfcGovNonProfit, pfcTransport, pfcTravel, pfcRentUtilities,
	}
	if len(spendingPrimaries) != 12 {
		t.Fatalf("expected 12 spending primaries, listed %d", len(spendingPrimaries))
	}
	if len(pfcSpendingCategory) != 12 {
		t.Fatalf("pfcSpendingCategory has %d entries, want 12", len(pfcSpendingCategory))
	}
	seenCategories := map[string]bool{}
	for _, primary := range spendingPrimaries {
		categoryID, ok := pfcSpendingCategory[primary]
		if !ok {
			t.Errorf("spending primary %s missing from the map", primary)
			continue
		}
		if seenCategories[categoryID] {
			t.Errorf("category %s mapped from more than one primary", categoryID)
		}
		seenCategories[categoryID] = true

		got := ResolveCategorization(ResolveInput{BankCategory: banking.Category{Primary: primary}, Amount: 10})
		if got.Classification != Spending || got.CategoryID == nil || *got.CategoryID != categoryID {
			t.Errorf("primary %s: got %+v, want Spending/%s", primary, got, categoryID)
		}
	}

	// All twelve built-in spending Category ids are covered exactly once.
	builtins := []string{
		CategoryFoodAndDrink, CategoryGeneralMerchandise, CategoryTransportation,
		CategoryTravel, CategoryRentAndUtilities, CategoryMedical, CategoryPersonalCare,
		CategoryGeneralServices, CategoryEntertainment, CategoryHomeImprovement,
		CategoryBankFees, CategoryGovernmentAndNonProfit,
	}
	for _, id := range builtins {
		if !seenCategories[id] {
			t.Errorf("built-in category %s is not the target of any spending primary", id)
		}
	}
}

// TestResolveIsPure asserts the engine mutates none of its inputs: the Rules and
// Categories slices (and their pointer fields) are unchanged after a resolve.
func TestResolveIsPure(t *testing.T) {
	now := time.Now()
	rules := []Rule{spendingRule("r1", "ACME", CategoryGeneralMerchandise, now)}
	categories := []Category{{ID: CategoryGeneralMerchandise, Name: "General Merchandise", Builtin: true}}
	originalCategoryID := *rules[0].CategoryID

	got := ResolveCategorization(ResolveInput{
		CleanMerchant: "ACME WIDGETS",
		Rules:         rules,
		Categories:    categories,
		Amount:        25,
	})

	// Mutating the returned Decision's CategoryID must not reach back into the Rule.
	if got.CategoryID != nil {
		*got.CategoryID = "tampered"
	}
	if *rules[0].CategoryID != originalCategoryID {
		t.Errorf("engine returned an aliased CategoryID: rule mutated to %q", *rules[0].CategoryID)
	}
	if len(rules) != 1 || rules[0].MerchantSubstring != "ACME" {
		t.Errorf("rules slice was mutated: %+v", rules)
	}
	if categories[0].Archived {
		t.Errorf("categories slice was mutated: %+v", categories)
	}
}
