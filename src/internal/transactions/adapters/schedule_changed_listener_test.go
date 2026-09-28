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

// TestTransactionsListListensForScheduleChanged asserts the list's existing
// self-refresh listener ([ADR-0010]) now also re-fetches on schedule-changed —
// the event the schedule-item modal announces on a save, delete, or match
// change ([ADR-0031]) — not only on its original transaction-changed. This is
// what makes every row's match badge come current when the modal that changed
// it was reached from /sweep, or from a different transaction's own editor,
// rather than this row's: the list never learns what a scheduled item is, it
// just re-fetches itself by name whenever either event bubbles to body.
func TestTransactionsListListensForScheduleChanged(t *testing.T) {
	database := newTestDB(t)
	accountsSvc, txnSvc, categorizationSvc := newServices(t, database, fakebank.NewService())
	registerConnection(t, accountsSvc)
	if err := txnSvc.SyncTransactions(testCtx()); err != nil {
		t.Fatalf("SyncTransactions: %v", err)
	}

	_, body := getPage(t, txnSvc, accountsSvc, categorizationSvc)

	if !strings.Contains(body, `data-testid="transactions-refresh-listener"`) {
		t.Fatalf("the refresh listener is not rendered:\n%s", body)
	}
	for what, want := range map[string]string{
		"still listening for transaction-changed": "transaction-changed from:body",
		"also listening for schedule-changed":     "schedule-changed from:body",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("refresh listener missing %s (%q):\n%s", what, want, body)
		}
	}
}

// TestTransactionsListSelfRefreshReflectsAScheduleRename demonstrates what that
// listener's re-fetch actually returns once a schedule item is renamed:
// every checking row matched to it — not only whichever one happened to open
// the modal, if any — shows the new name, because the listener's target is a
// full region re-fetch (schedule.MatchedItemNames re-read for every row on the
// page), not a single row's own re-render. Two separate rows are matched to
// the same item so the "other rows too" half of the claim is provable, not
// just the row a reader might assume was the one edited.
func TestTransactionsListSelfRefreshReflectsAScheduleRename(t *testing.T) {
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
	paycheck, err := txnSvc.RecentTransaction(testCtx(), "fake-txn-paycheck")
	if err != nil {
		t.Fatalf("RecentTransaction(paycheck): %v", err)
	}

	// One item, matched to two different checking rows: proves a rename reaches
	// every row that depends on it, not only the one row an editor might have
	// opened this modal from.
	item, err := scheduleSvc.Create(testCtx(), schedule.Item{
		Name:       "Household",
		Direction:  schedule.DirectionOut,
		Amount:     scheduleAmount(90),
		Cadence:    schedule.CadenceMonthly,
		DayOfMonth: groceries.Date.Day(),
		Active:     true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := scheduleSvc.MatchOccurrence(testCtx(), item.ID, groceries.Date, "fake-txn-groceries"); err != nil {
		t.Fatalf("MatchOccurrence(groceries): %v", err)
	}
	// A second, independent settled occurrence of the same item, satisfied by
	// the unrelated paycheck row purely to give it a match to the same item —
	// what settles which occurrence is not this test's concern, only that both
	// rows now name the one item.
	if err := scheduleSvc.MatchOccurrence(testCtx(), item.ID, paycheck.Date, "fake-txn-paycheck"); err != nil {
		t.Fatalf("MatchOccurrence(paycheck): %v", err)
	}

	handler := adapters.NewHttpHandler(txnSvc, accountsSvc, categorizationSvc, scheduleSvc)
	before := renderTransactionsPage(t, handler)
	for _, txnID := range []string{"fake-txn-groceries", "fake-txn-paycheck"} {
		if !strings.Contains(rowSegment(t, before, txnID), "Household") {
			t.Fatalf("row %s did not carry the badge before the rename:\n%s", txnID, before)
		}
	}

	// The rename is made through schedule.Service directly here, standing in for
	// the sweep module's modal save — the seam this task actually wires — since
	// this test package must not import sweep/adapters (transactions and sweep
	// never import each other, an enforced boundary). What matters here is only
	// what the transactions region shows once it re-fetches itself, which is
	// exactly what the schedule-changed listener drives regardless of which
	// module's endpoint performed the write; the sweep-side tests separately
	// prove that endpoint announces the event.
	item.Name = "Household (renamed)"
	if err := scheduleSvc.Update(testCtx(), item); err != nil {
		t.Fatalf("Update: %v", err)
	}

	after := renderTransactionsPage(t, handler)
	for _, txnID := range []string{"fake-txn-groceries", "fake-txn-paycheck"} {
		if !strings.Contains(rowSegment(t, after, txnID), "Household (renamed)") {
			t.Errorf("row %s did not pick up the rename on self-refresh:\n%s", txnID, after)
		}
	}
}

// renderTransactionsPage drives GET /transactions through an already-built
// handler, the shape a self-refresh listener's hx-get repeats verbatim.
func renderTransactionsPage(t *testing.T, handler *adapters.HttpHandler) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/transactions", nil)
	w := httptest.NewRecorder()
	handler.GetTransactionsPage(w, req)
	return w.Body.String()
}
