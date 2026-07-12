package views

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/categorization"
	"github.com/alecdray/two-cents/src/internal/transactions"
)

// renderRow renders one transaction row fragment and returns its HTML. This is the
// shared row every transaction surface delegates to, so asserting on it covers the
// avatar everywhere it appears.
func renderRow(t *testing.T, row transactions.RecentTransaction) string {
	t.Helper()
	var sb strings.Builder
	if err := TransactionRowFrag(row).Render(context.Background(), &sb); err != nil {
		t.Fatalf("rendering row: %v", err)
	}
	return sb.String()
}

// spendingRow is a minimally populated Spending row with the given Category and no
// merchant logo — the no-logo case that falls back to the category glyph.
func spendingRow(categoryID string) transactions.RecentTransaction {
	id := categoryID
	return transactions.RecentTransaction{
		ID:             "t1",
		Merchant:       "Some Merchant",
		Date:           time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		Amount:         banking.Money{Amount: 12.50, Currency: "USD"},
		Classification: categorization.Spending,
		CategoryID:     &id,
	}
}

// TestNoLogoRowShowsBuiltinCategoryGlyph asserts a no-logo Spending row in a
// built-in Category renders that Category's glyph tinted by its palette hue, and no
// merchant image.
func TestNoLogoRowShowsBuiltinCategoryGlyph(t *testing.T) {
	html := renderRow(t, spendingRow(categorization.CategoryFoodAndDrink))

	if !strings.Contains(html, "bi-cup-straw") {
		t.Errorf("expected food-and-drink glyph bi-cup-straw, got:\n%s", html)
	}
	if !strings.Contains(html, "text-category-1") {
		t.Errorf("expected food-and-drink color text-category-1, got:\n%s", html)
	}
	if strings.Contains(html, "merchant-avatar-image") {
		t.Errorf("no-logo row should not render a merchant image, got:\n%s", html)
	}
	if !strings.Contains(html, `data-testid="merchant-avatar"`) {
		t.Errorf("row must always render an avatar, got:\n%s", html)
	}
}

// TestDifferentBuiltinCategoriesDiffer asserts two different built-in Categories
// render different glyphs and different hues, so the buckets are told apart.
func TestDifferentBuiltinCategoriesDiffer(t *testing.T) {
	food := renderRow(t, spendingRow(categorization.CategoryFoodAndDrink))
	transport := renderRow(t, spendingRow(categorization.CategoryTransportation))

	if !strings.Contains(food, "bi-cup-straw") || !strings.Contains(food, "text-category-1") {
		t.Fatalf("food row missing its glyph/color:\n%s", food)
	}
	if !strings.Contains(transport, "bi-car-front") || !strings.Contains(transport, "text-category-3") {
		t.Fatalf("transportation row missing its glyph/color:\n%s", transport)
	}
	if strings.Contains(transport, "bi-cup-straw") {
		t.Errorf("transportation row should not reuse the food glyph")
	}
}

// TestCustomCategoryGlyphIsGenericAndStable asserts a custom (non-built-in)
// Category renders the generic glyph and a hue that is stable across renders of the
// same id.
func TestCustomCategoryGlyphIsGenericAndStable(t *testing.T) {
	first := renderRow(t, spendingRow("a-custom-category-id"))
	second := renderRow(t, spendingRow("a-custom-category-id"))

	if !strings.Contains(first, "bi-tag") {
		t.Errorf("expected generic custom glyph bi-tag, got:\n%s", first)
	}

	colorOf := func(html string) string {
		for i := 1; i <= 12; i++ {
			cls := "text-category-" + itoa(i)
			// match the exact class token, not a prefix of a longer number
			if strings.Contains(html, cls+`"`) || strings.Contains(html, cls+` `) {
				return cls
			}
		}
		return ""
	}
	c1, c2 := colorOf(first), colorOf(second)
	if c1 == "" {
		t.Fatalf("custom category rendered no palette color:\n%s", first)
	}
	if c1 != c2 {
		t.Errorf("custom category color not stable across renders: %q vs %q", c1, c2)
	}
}

// TestIncomeTransferSavingsRowsShowDistinctGlyphs asserts income, plain transfer,
// and savings-contribution transfer rows each render their own distinct glyph and
// color — not the neutral default and not each other's.
func TestIncomeTransferSavingsRowsShowDistinctGlyphs(t *testing.T) {
	incomeRow := transactions.RecentTransaction{
		ID:             "t1",
		Merchant:       "Some Employer",
		Date:           time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		Amount:         banking.Money{Amount: -2400.00, Currency: "USD"},
		Classification: categorization.Income,
	}
	plainTransferRow := transactions.RecentTransaction{
		ID:             "t2",
		Merchant:       "Transfer",
		Date:           time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		Amount:         banking.Money{Amount: 500.00, Currency: "USD"},
		Classification: categorization.Transfer,
		TransferSubtype: categorization.SubtypePlain,
	}
	savingsRow := transactions.RecentTransaction{
		ID:             "t3",
		Merchant:       "Transfer",
		Date:           time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		Amount:         banking.Money{Amount: 200.00, Currency: "USD"},
		Classification: categorization.Transfer,
		TransferSubtype: categorization.SubtypeSavingsContribution,
	}

	incomeHTML := renderRow(t, incomeRow)
	if !strings.Contains(incomeHTML, "bi-cash-stack") {
		t.Errorf("income row: expected glyph bi-cash-stack, got:\n%s", incomeHTML)
	}
	if !strings.Contains(incomeHTML, "text-category-income") {
		t.Errorf("income row: expected color text-category-income, got:\n%s", incomeHTML)
	}

	plainHTML := renderRow(t, plainTransferRow)
	if !strings.Contains(plainHTML, "bi-arrow-left-right") {
		t.Errorf("plain transfer row: expected glyph bi-arrow-left-right, got:\n%s", plainHTML)
	}
	if !strings.Contains(plainHTML, "text-category-transfer") {
		t.Errorf("plain transfer row: expected color text-category-transfer, got:\n%s", plainHTML)
	}

	savingsHTML := renderRow(t, savingsRow)
	if !strings.Contains(savingsHTML, "bi-piggy-bank") {
		t.Errorf("savings transfer row: expected glyph bi-piggy-bank, got:\n%s", savingsHTML)
	}
	if !strings.Contains(savingsHTML, "text-category-savings") {
		t.Errorf("savings transfer row: expected color text-category-savings, got:\n%s", savingsHTML)
	}

	// All three must be distinct from each other and from the neutral default.
	if strings.Contains(incomeHTML, "bi-receipt") || strings.Contains(incomeHTML, "text-category-neutral") {
		t.Errorf("income row must not use the neutral default")
	}
	if strings.Contains(plainHTML, "bi-receipt") || strings.Contains(plainHTML, "text-category-neutral") {
		t.Errorf("plain transfer row must not use the neutral default")
	}
	if strings.Contains(savingsHTML, "bi-receipt") || strings.Contains(savingsHTML, "text-category-neutral") {
		t.Errorf("savings transfer row must not use the neutral default")
	}
	if strings.Contains(incomeHTML, "bi-arrow-left-right") || strings.Contains(incomeHTML, "bi-piggy-bank") {
		t.Errorf("income row must not share a glyph with transfer rows")
	}
	if strings.Contains(plainHTML, "bi-piggy-bank") {
		t.Errorf("plain transfer row must not use the savings glyph")
	}
}

// TestNeutralRowsShowDefaultGlyph asserts uncategorized spending and needs-review
// rows still render the shared neutral default — never blank, never a distinct glyph.
func TestNeutralRowsShowDefaultGlyph(t *testing.T) {
	cases := []struct {
		name           string
		classification categorization.Classification
	}{
		{"uncategorized-spending", categorization.Spending},
		{"needs-review", categorization.NeedsReview},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			row := transactions.RecentTransaction{
				ID:             "t1",
				Merchant:       "Some Merchant",
				Date:           time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
				Amount:         banking.Money{Amount: 12.50, Currency: "USD"},
				Classification: tc.classification,
			}
			html := renderRow(t, row)
			if !strings.Contains(html, "bi-receipt") {
				t.Errorf("expected default glyph bi-receipt, got:\n%s", html)
			}
			if !strings.Contains(html, "text-category-neutral") {
				t.Errorf("expected default color text-category-neutral, got:\n%s", html)
			}
			if strings.Contains(html, "merchant-avatar-image") {
				t.Errorf("no-logo row should not render a merchant image")
			}
		})
	}
}

// TestCachedLogoRowShowsImageNotGlyph asserts a row with a positively cached logo
// renders the served image (our own origin path, never a third-party host) and no
// category glyph, while the same row with the field empty falls back to the glyph.
func TestCachedLogoRowShowsImageNotGlyph(t *testing.T) {
	served := transactions.MerchantLogoRoutePrefix + "abc123"
	row := spendingRow(categorization.CategoryFoodAndDrink)
	row.MerchantLogoURL = served

	html := renderRow(t, row)

	if !strings.Contains(html, `src="`+served+`"`) {
		t.Errorf("expected image src %q, got:\n%s", served, html)
	}
	if !strings.HasPrefix(served, "/") {
		t.Fatalf("served logo URL should be an origin-relative path, got %q", served)
	}
	if strings.Contains(html, "http://") || strings.Contains(html, "https://") {
		t.Errorf("avatar image must be served from our own origin, never a third-party host:\n%s", html)
	}
	if strings.Contains(html, "bi-cup-straw") {
		t.Errorf("logo row should not also render the category glyph:\n%s", html)
	}

	// The same row with no cached logo falls back to the category glyph.
	row.MerchantLogoURL = ""
	fallback := renderRow(t, row)
	if !strings.Contains(fallback, "bi-cup-straw") {
		t.Errorf("empty-logo row should show the category glyph, got:\n%s", fallback)
	}
	if strings.Contains(fallback, "merchant-avatar-image") {
		t.Errorf("empty-logo row should not render an image, got:\n%s", fallback)
	}
}

// TestAllSurfacesDelegateToSharedRow anchors the claim that the four transaction
// surfaces all reach the one shared row (directly, or via AllTransactionsFrag), so
// editing that single row component covers the avatar everywhere.
func TestAllSurfacesDelegateToSharedRow(t *testing.T) {
	cases := []struct {
		file  string
		token string
	}{
		{"transactions_page.templ", "TransactionRowFrag"},
		{"../../../home/adapters/views/drill_page.templ", "TransactionRowFrag"},
		{"../../../home/adapters/views/all_transactions_frag.templ", "TransactionRowFrag"},
		{"../../../home/adapters/views/tracker_page.templ", "AllTransactionsFrag"},
		{"../../../home/adapters/views/wrap_page.templ", "AllTransactionsFrag"},
	}
	for _, tc := range cases {
		src, err := os.ReadFile(tc.file)
		if err != nil {
			t.Fatalf("reading %s: %v", tc.file, err)
		}
		if !strings.Contains(string(src), tc.token) {
			t.Errorf("%s should delegate via %s", tc.file, tc.token)
		}
	}
}

// itoa is a tiny local int-to-string for building palette class names in the test.
func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}
