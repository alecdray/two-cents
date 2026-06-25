package views

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/categorization"
	"github.com/alecdray/two-cents/src/internal/transactions"
)

// renderEditor renders the editor content fragment for a row and returns the HTML.
func renderEditor(t *testing.T, row transactions.RecentTransaction, loc *time.Location) string {
	t.Helper()
	var sb strings.Builder
	if err := TransactionEditContentFrag(row, nil, nil, nil, loc, "", "").Render(context.Background(), &sb); err != nil {
		t.Fatalf("rendering editor: %v", err)
	}
	return sb.String()
}

// renderEditorWithRules renders the editor content fragment for a row with the given
// matching Rules and returns the HTML.
func renderEditorWithRules(t *testing.T, row transactions.RecentTransaction, matches []categorization.MatchingRule) string {
	t.Helper()
	var sb strings.Builder
	if err := TransactionEditContentFrag(row, nil, nil, matches, time.UTC, "", "").Render(context.Background(), &sb); err != nil {
		t.Fatalf("rendering editor: %v", err)
	}
	return sb.String()
}

// TestEditorRendersBankDisplayDetail asserts the editor surfaces the read-only
// bank display detail (ADR-0013) when the bank populated it: the merchant logo,
// the "via {intermediary}" note from the counterparties list, the website, the
// raw descriptor, the payment channel, the authorized/posted timing in the app
// timezone, and a low-confidence flag.
func TestEditorRendersBankDisplayDetail(t *testing.T) {
	row := transactions.RecentTransaction{
		ID:                 "t1",
		AccountName:        "Checking",
		Date:               time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC),
		Amount:             banking.Money{Amount: 18.40, Currency: "USD"},
		Merchant:           "Two Boots",
		Counterparty:       "Twobootsp",
		Description:        "DD *DOORDASH TWOBOOTSP",
		LogoURL:            "https://logos.example/doordash.png",
		Website:            "doordash.com",
		PaymentChannel:     "online",
		CategoryConfidence: "LOW",
		CategoryPrimary:    "FOOD_AND_DRINK",
		CategoryDetailed:   "FOOD_AND_DRINK_FAST_FOOD",
		AuthorizedDatetime: ptrTime(time.Date(2026, 6, 21, 19, 45, 0, 0, time.UTC)),
		Datetime:           ptrTime(time.Date(2026, 6, 23, 2, 10, 0, 0, time.UTC)),
		Counterparties: []banking.Counterparty{
			{Name: "Two Boots", Type: "merchant"},
			{Name: "DoorDash", Type: "marketplace"},
		},
	}

	body := renderEditor(t, row, time.UTC)

	wants := []string{
		`data-testid="transaction-editor-logo"`,
		"https://logos.example/doordash.png",
		`data-testid="transaction-editor-via"`,
		"via DoorDash",
		`data-testid="transaction-editor-website"`,
		"doordash.com",
		`data-testid="transaction-editor-description"`,
		"DD *DOORDASH TWOBOOTSP",
		`data-testid="transaction-editor-channel"`,
		"Online",
		`data-testid="transaction-editor-timing"`,
		"Authorized Jun 21, 7:45 PM · Posted Jun 23, 2:10 AM",
		`data-testid="transaction-editor-confidence"`,
	}
	for _, want := range wants {
		if !strings.Contains(body, want) {
			t.Errorf("editor missing %q", want)
		}
	}
}

// TestEditorOmitsAbsentDisplayDetail asserts every display-detail element is
// conditional: a row carrying none of it renders none of the optional markers.
func TestEditorOmitsAbsentDisplayDetail(t *testing.T) {
	row := transactions.RecentTransaction{
		ID:          "t2",
		AccountName: "Checking",
		Date:        time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC),
		Amount:      banking.Money{Amount: 5, Currency: "USD"},
		Merchant:    "Corner Store",
		// No logo, website, counterparties, descriptor, channel, confidence, timestamps.
	}

	body := renderEditor(t, row, time.UTC)

	absent := []string{
		`data-testid="transaction-editor-logo"`,
		`data-testid="transaction-editor-via"`,
		`data-testid="transaction-editor-website"`,
		`data-testid="transaction-editor-description"`,
		`data-testid="transaction-editor-channel"`,
		`data-testid="transaction-editor-timing"`,
		`data-testid="transaction-editor-confidence"`,
	}
	for _, a := range absent {
		if strings.Contains(body, a) {
			t.Errorf("editor rendered %q for a row without that detail", a)
		}
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

func strptr(s string) *string { return &s }

// TestEditorListsMatchingRulesWinnerFirst asserts a transaction whose merchant
// matches one or more Rules renders the Rules section listing them in the order
// RulesMatching supplies (governing Rule first, marked Applied), each opener
// pointing at the rule editor's edit endpoint with this transaction's own edit URL
// as the URL-encoded return handle — and no Create-rule control.
func TestEditorListsMatchingRulesWinnerFirst(t *testing.T) {
	row := transactions.RecentTransaction{
		ID:           "txn-9",
		AccountName:  "Checking",
		Date:         time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC),
		Amount:       banking.Money{Amount: 42, Currency: "USD"},
		Merchant:     "WHOLEFOODS",
		Counterparty: "WHOLEFOODS",
	}
	matches := []categorization.MatchingRule{
		{
			Rule:         categorization.Rule{ID: "rule-winner", MerchantSubstring: "WHOLEFOODS", Classification: categorization.Spending, CategoryID: strptr("food_and_drink")},
			CategoryName: "Food & Drink",
			IsWinner:     true,
		},
		{
			Rule:     categorization.Rule{ID: "rule-other", MerchantSubstring: "FOODS", Classification: categorization.Spending, CategoryID: strptr("general_merchandise")},
			IsWinner: false,
		},
	}

	body := renderEditorWithRules(t, row, matches)

	returnTo := url.Values{"return_to": {"/transactions/txn-9/edit"}}.Encode()
	wants := []string{
		`data-testid="transaction-editor-rules"`,
		`data-testid="transaction-editor-rule"`,
		`data-testid="transaction-editor-rule-applied"`, // the governing Rule is marked
		`hx-get="/rules/rule-winner/edit?` + returnTo + `"`,
		`hx-get="/rules/rule-other/edit?` + returnTo + `"`,
		"WHOLEFOODS → Spending · Food &amp; Drink", // readable substring -> outcome · Category
	}
	for _, want := range wants {
		if !strings.Contains(body, want) {
			t.Errorf("editor rules section missing %q:\n%s", want, body)
		}
	}

	// The governing Rule (winner) is listed first — order is preserved, never re-sorted.
	if winner, other := strings.Index(body, "rule-winner"), strings.Index(body, "rule-other"); winner == -1 || other == -1 || winner > other {
		t.Errorf("governing rule not listed first (winner at %d, other at %d)", winner, other)
	}

	// A matching transaction never offers the Create-rule control.
	if strings.Contains(body, `data-testid="transaction-editor-rule-create"`) {
		t.Errorf("editor rendered the Create-rule control despite matching rules:\n%s", body)
	}
}

// TestEditorOffersCreateRuleWhenNoMatch asserts a transaction no Rule matches
// renders the Create-rule opener prefilled from the transaction — the merchant
// substring, this transaction's own edit URL as the return handle, and the row's
// current classification + Category — and lists no Rules.
func TestEditorOffersCreateRuleWhenNoMatch(t *testing.T) {
	row := transactions.RecentTransaction{
		ID:             "txn-2",
		AccountName:    "Checking",
		Date:           time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC),
		Amount:         banking.Money{Amount: 6, Currency: "USD"},
		Merchant:       "Blue Bottle Coffee",
		Classification: categorization.Spending,
		CategoryID:     strptr("food_and_drink"),
	}

	body := renderEditorWithRules(t, row, nil)

	wants := []string{
		`data-testid="transaction-editor-rules"`,
		`data-testid="transaction-editor-rule-create"`,
		`hx-get="/rules/new?`,
		"merchant=" + url.QueryEscape("Blue Bottle Coffee"), // human-readable merchant prefill
		"return_to=" + url.QueryEscape("/transactions/txn-2/edit"),
		"classification=spending", // the row's current outcome
		"category_id=food_and_drink",
	}
	for _, want := range wants {
		if !strings.Contains(body, want) {
			t.Errorf("create-rule opener missing %q:\n%s", want, body)
		}
	}

	// No matches ⇒ no rule list.
	if strings.Contains(body, `data-testid="transaction-editor-rule"`) {
		t.Errorf("editor listed a rule despite no matches:\n%s", body)
	}
}

// TestEditorRuleReturnToIsSameOriginTransactionPath asserts the return handle the
// openers carry is this transaction's own edit path — a same-origin leading-slash
// relative path, the only shape the rule editor's validReturnHandle accepts.
func TestEditorRuleReturnToIsSameOriginTransactionPath(t *testing.T) {
	row := transactions.RecentTransaction{
		ID:          "txn-7",
		AccountName: "Checking",
		Date:        time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC),
		Amount:      banking.Money{Amount: 6, Currency: "USD"},
		Merchant:    "Corner Store",
	}

	want := "/transactions/" + row.ID + "/edit"
	if !strings.HasPrefix(want, "/") || strings.HasPrefix(want, "//") || strings.Contains(want, "://") {
		t.Fatalf("test's expected handle %q is not same-origin relative", want)
	}

	body := renderEditorWithRules(t, row, nil)
	if !strings.Contains(body, "return_to="+url.QueryEscape(want)) {
		t.Errorf("opener did not carry the same-origin transaction edit path as return_to (%q):\n%s", want, body)
	}
}
