package adapters_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alecdray/two-cents/src/internal/fakebank"
	"github.com/alecdray/two-cents/src/internal/schedule"
	"github.com/alecdray/two-cents/src/internal/transactions/adapters"
)

// rowSegment isolates one row's own HTML out of a rendered /transactions list, so
// an assertion about its badge cannot be satisfied by a different row's markup.
// Rows are delimited by their own DOM id (views.TransactionRowID), and this takes
// everything up to the next row's id (or the end of the body when the row is
// last).
func rowSegment(t *testing.T, body, txnID string) string {
	t.Helper()
	marker := `id="txn-row-` + txnID + `"`
	start := strings.Index(body, marker)
	if start < 0 {
		t.Fatalf("row %q not found in body:\n%s", txnID, body)
	}
	rest := body[start+len(marker):]
	if end := strings.Index(rest, `id="txn-row-`); end >= 0 {
		return rest[:end]
	}
	return rest
}

// TestTransactionsListScheduleMatchBadge drives GET /transactions over the real
// accounts, transactions, and schedule Services sharing one migrated DB, and
// asserts the list's per-row match badge ([ADR-0031]) shows exactly on a
// checking-account row currently matched to a scheduled item, visible on the list
// itself rather than only inside that row's own editor: never on an unmatched
// checking row, and never on a non-checking row regardless of its own match state.
func TestTransactionsListScheduleMatchBadge(t *testing.T) {
	database := newTestDB(t)
	accountsSvc, txnSvc, categorizationSvc := newServices(t, database, fakebank.NewService())
	scheduleSvc := schedule.NewService(database, nil, time.UTC)
	registerConnection(t, accountsSvc)
	if err := txnSvc.SyncTransactions(testCtx()); err != nil {
		t.Fatalf("SyncTransactions: %v", err)
	}

	groceries, err := txnSvc.RecentTransaction(testCtx(), "fake-txn-groceries")
	if err != nil {
		t.Fatalf("RecentTransaction(groceries): %v", err)
	}
	coffee, err := txnSvc.RecentTransaction(testCtx(), "fake-txn-coffee")
	if err != nil {
		t.Fatalf("RecentTransaction(coffee): %v", err)
	}

	// Match the checking groceries row to a scheduled item.
	groceriesItem, err := scheduleSvc.Create(testCtx(), schedule.Item{
		Name:       "Groceries budget",
		Direction:  schedule.DirectionOut,
		Amount:     scheduleAmount(90),
		Cadence:    schedule.CadenceMonthly,
		DayOfMonth: groceries.Date.Day(),
		Active:     true,
	})
	if err != nil {
		t.Fatalf("Create(groceries item): %v", err)
	}
	if err := scheduleSvc.MatchOccurrence(testCtx(), groceriesItem.ID, groceries.Date, "fake-txn-groceries"); err != nil {
		t.Fatalf("MatchOccurrence(groceries): %v", err)
	}

	// Also force a match onto the non-checking coffee row (a credit-card charge)
	// so criterion 3 proves the checking gate itself, not merely the absence of
	// any match on that row.
	coffeeItem, err := scheduleSvc.Create(testCtx(), schedule.Item{
		Name:       "Coffee habit",
		Direction:  schedule.DirectionOut,
		Amount:     scheduleAmount(6),
		Cadence:    schedule.CadenceMonthly,
		DayOfMonth: coffee.Date.Day(),
		Active:     true,
	})
	if err != nil {
		t.Fatalf("Create(coffee item): %v", err)
	}
	if err := scheduleSvc.MatchOccurrence(testCtx(), coffeeItem.ID, coffee.Date, "fake-txn-coffee"); err != nil {
		t.Fatalf("MatchOccurrence(coffee): %v", err)
	}

	// fake-txn-paycheck (also checking) is left unmatched by this test on purpose,
	// so it stands in for criterion 2.

	handler := adapters.NewHttpHandler(txnSvc, accountsSvc, categorizationSvc, scheduleSvc)
	req := httptest.NewRequest(http.MethodGet, "/transactions", nil)
	rec := httptest.NewRecorder()
	handler.GetTransactionsPage(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()

	// Criterion 1: the matched checking row carries the badge, naming the item —
	// on the list itself, not merely inside the row's own opened editor.
	groceriesRow := rowSegment(t, body, "fake-txn-groceries")
	if !strings.Contains(groceriesRow, `data-testid="txn-schedule-match"`) {
		t.Errorf("matched checking row missing the schedule-match badge:\n%s", groceriesRow)
	}
	if !strings.Contains(groceriesRow, "Groceries budget") {
		t.Errorf("badge did not name the matched item:\n%s", groceriesRow)
	}

	// Criterion 2: an unmatched checking row (the paycheck deposit) carries no
	// badge.
	paycheckRow := rowSegment(t, body, "fake-txn-paycheck")
	if strings.Contains(paycheckRow, `data-testid="txn-schedule-match"`) {
		t.Errorf("unmatched checking row unexpectedly rendered the schedule-match badge:\n%s", paycheckRow)
	}

	// Criterion 3: the non-checking coffee row carries no badge despite being
	// matched itself — the checking gate withholds it regardless of match state.
	coffeeRow := rowSegment(t, body, "fake-txn-coffee")
	if strings.Contains(coffeeRow, `data-testid="txn-schedule-match"`) {
		t.Errorf("non-checking row unexpectedly rendered the schedule-match badge despite being matched:\n%s", coffeeRow)
	}
}
