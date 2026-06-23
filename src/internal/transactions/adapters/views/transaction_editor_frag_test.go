package views

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/transactions"
)

// renderEditor renders the editor content fragment for a row and returns the HTML.
func renderEditor(t *testing.T, row transactions.RecentTransaction, loc *time.Location) string {
	t.Helper()
	var sb strings.Builder
	if err := TransactionEditContentFrag(row, nil, nil, loc, "", "").Render(context.Background(), &sb); err != nil {
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
