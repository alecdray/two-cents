package adapters_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alecdray/two-cents/src/internal/accounts"
	"github.com/alecdray/two-cents/src/internal/categorization"
	"github.com/alecdray/two-cents/src/internal/core/timex"
	"github.com/alecdray/two-cents/src/internal/fakebank"
	"github.com/alecdray/two-cents/src/internal/transactions"
	"github.com/alecdray/two-cents/src/internal/transactions/adapters"
)

// currentMonthLabel is the "January 2006" month divider the populated view renders
// for the fake set. The fake dates anchor to the current month (fakebank), and a
// bare testCtx carries no app timezone so the anchor falls back to UTC — reckon the
// expected label the same way rather than hard-coding a month that rots each roll.
func currentMonthLabel() string {
	year, month := timex.CurrentMonth(time.UTC, time.Now())
	return fmt.Sprintf("%s %d", month, year)
}

// syncedServices wires the services over a fresh DB, registers a connection, and
// backfills the deterministic fake transactions — the populated starting point the
// view-filter tests read against. The fake fixture is six current-month rows; the
// only needs-attention row is "Side Hustle Co" (an empty-category inflow that
// resolves to needs-review).
func syncedServices(t *testing.T) (*transactions.Service, *accounts.Service, *categorization.Service) {
	t.Helper()
	database := newTestDB(t)
	accountsSvc, txnSvc, categorizationSvc := newServices(t, database, fakebank.NewService())
	registerConnection(t, accountsSvc)
	if err := txnSvc.SyncTransactions(testCtx()); err != nil {
		t.Fatalf("SyncTransactions: %v", err)
	}
	return txnSvc, accountsSvc, categorizationSvc
}

// getView drives a GET against the given target (path + query) through the handler
// and returns the status and body. htmx sets the header so the search/toggle paths
// render the bare content fragment; without it the full page renders.
func getView(t *testing.T, txnSvc *transactions.Service, accountsSvc *accounts.Service, categorizationSvc *categorization.Service, target string, htmx bool) (int, string) {
	t.Helper()
	handler := adapters.NewHttpHandler(txnSvc, accountsSvc, categorizationSvc)
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if htmx {
		req.Header.Set("HX-Request", "true")
	}
	rec := httptest.NewRecorder()
	handler.GetTransactionsPage(rec, req)
	return rec.Code, rec.Body.String()
}

// TestPopulatedViewShowsControlsAndMonthHeader asserts the populated default view
// renders the controls bar (search + both toggle tabs) and a month divider labeled
// by the rows' transaction-date month.
func TestPopulatedViewShowsControlsAndMonthHeader(t *testing.T) {
	txnSvc, accountsSvc, categorizationSvc := syncedServices(t)

	status, body := getView(t, txnSvc, accountsSvc, categorizationSvc, "/transactions", false)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	for label, want := range map[string]string{
		"controls bar":        `data-testid="transactions-controls"`,
		"search box":          `data-testid="transactions-search"`,
		"all tab":             `data-testid="transactions-view-all"`,
		"needs-attention tab": `data-testid="transactions-view-needs-attention"`,
		"month header testid": `data-testid="transactions-month-header"`,
		"month label":         currentMonthLabel(),
	} {
		if !strings.Contains(body, want) {
			t.Errorf("populated view missing %s (%q)", label, want)
		}
	}
}

// TestMerchantSearchFiltersList asserts a merchant search narrows the list to the
// matching rows (case-insensitively) and drops the rest.
func TestMerchantSearchFiltersList(t *testing.T) {
	txnSvc, accountsSvc, categorizationSvc := syncedServices(t)

	status, body := getView(t, txnSvc, accountsSvc, categorizationSvc, "/transactions?q=coffee", true)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	if !strings.Contains(body, "Blue Bottle Coffee") {
		t.Errorf("search for 'coffee' dropped the matching row")
	}
	for _, gone := range []string{"Whole Foods", "Acme Payroll", "Side Hustle Co"} {
		if strings.Contains(body, gone) {
			t.Errorf("search for 'coffee' should not show %q", gone)
		}
	}
	if got := strings.Count(body, `data-testid="transactions-row"`); got != 1 {
		t.Errorf("search row count = %d, want 1", got)
	}
	// An htmx swap renders the bare content fragment, never the full page.
	if strings.Contains(body, "<html") {
		t.Errorf("htmx search rendered a full page; want the content fragment only")
	}
}

// TestSearchNoMatchShowsFilteredEmpty asserts a search matching nothing shows the
// filtered empty state (not the nothing-synced state) with the controls still up.
func TestSearchNoMatchShowsFilteredEmpty(t *testing.T) {
	txnSvc, accountsSvc, categorizationSvc := syncedServices(t)

	_, body := getView(t, txnSvc, accountsSvc, categorizationSvc, "/transactions?q=zzznomatch", true)

	if !strings.Contains(body, `data-testid="transactions-empty-filtered"`) {
		t.Errorf("no-match search missing the filtered empty state")
	}
	if strings.Contains(body, `data-testid="transactions-empty-no-transactions"`) {
		t.Errorf("no-match search wrongly showed the nothing-synced empty state")
	}
	if strings.Contains(body, `data-testid="transactions-list"`) {
		t.Errorf("no-match search should not render the list")
	}
	// Controls stay so the user can clear the search.
	if !strings.Contains(body, `data-testid="transactions-controls"`) {
		t.Errorf("no-match search dropped the controls bar")
	}
}

// TestNeedsAttentionFilter asserts the needs-attention view shows only the
// unresolved rows — here the single needs-review inflow — and excludes income,
// categorized spending, and a paired transfer.
func TestNeedsAttentionFilter(t *testing.T) {
	txnSvc, accountsSvc, categorizationSvc := syncedServices(t)

	status, body := getView(t, txnSvc, accountsSvc, categorizationSvc, "/transactions?view=needs-attention", true)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	if !strings.Contains(body, "Side Hustle Co") {
		t.Errorf("needs-attention view dropped the needs-review row")
	}
	for _, gone := range []string{"Acme Payroll", "Whole Foods", "Blue Bottle Coffee", "Rainy Day Savings"} {
		if strings.Contains(body, gone) {
			t.Errorf("needs-attention view should not show %q", gone)
		}
	}
	if got := strings.Count(body, `data-testid="transactions-row"`); got != 1 {
		t.Errorf("needs-attention row count = %d, want 1", got)
	}
	// The needs-attention tab carries the active-tab styling.
	if !strings.Contains(body, `data-testid="transactions-view-needs-attention"`) {
		t.Errorf("needs-attention view missing its toggle tab")
	}
}

// TestSearchAndNeedsAttentionCompose asserts the two filters intersect: a search
// that matches a row which is NOT needs-attention yields the filtered empty state
// in the needs-attention view.
func TestSearchAndNeedsAttentionCompose(t *testing.T) {
	txnSvc, accountsSvc, categorizationSvc := syncedServices(t)

	// "whole" matches Whole Foods, which is categorized spending — not needs-attention.
	_, body := getView(t, txnSvc, accountsSvc, categorizationSvc, "/transactions?view=needs-attention&q=whole", true)
	if strings.Contains(body, "Whole Foods") {
		t.Errorf("compose: Whole Foods is not needs-attention and should be excluded")
	}
	if !strings.Contains(body, `data-testid="transactions-empty-filtered"`) {
		t.Errorf("compose: expected the filtered empty state when the search has no needs-attention match")
	}

	// "side" matches Side Hustle Co, which IS needs-attention — it survives the intersection.
	_, body = getView(t, txnSvc, accountsSvc, categorizationSvc, "/transactions?view=needs-attention&q=side", true)
	if !strings.Contains(body, "Side Hustle Co") {
		t.Errorf("compose: a needs-attention row matching the search should survive")
	}
	if got := strings.Count(body, `data-testid="transactions-row"`); got != 1 {
		t.Errorf("compose row count = %d, want 1", got)
	}
}
