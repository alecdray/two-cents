package adapters_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/alecdray/two-cents/src/internal/fakebank"
	"github.com/alecdray/two-cents/src/internal/transactions/adapters"
)

// postCategorize drives POST /transactions/{id}/categorize through the handler
// with the given form values and returns the recorder.
func postCategorize(t *testing.T, handler *adapters.HttpHandler, id string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/transactions/"+id+"/categorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	handler.PostCategorize(rec, req)
	return rec
}

// getEditModal drives GET /transactions/{id}/edit through the handler and returns
// the recorder.
func getEditModal(t *testing.T, handler *adapters.HttpHandler, id string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/transactions/"+id+"/edit", nil)
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	handler.GetEditModal(rec, req)
	return rec
}

// TestEditModalOpensTheEditor drives the edit endpoint and asserts it returns the
// shared modal shell loaded with the row's editor body — not a full page.
func TestEditModalOpensTheEditor(t *testing.T) {
	database := newTestDB(t)
	accountsSvc, txnSvc, categorizationSvc := newServices(t, database, fakebank.NewService())
	registerConnection(t, accountsSvc)
	if err := txnSvc.SyncTransactions(testCtx()); err != nil {
		t.Fatalf("SyncTransactions: %v", err)
	}

	handler := adapters.NewHttpHandler(txnSvc, accountsSvc, categorizationSvc)
	rec := getEditModal(t, handler, "fake-txn-groceries")

	body := rec.Body.String()
	if strings.Contains(body, "<html") {
		t.Errorf("edit response rendered a full page; want the modal fragment only")
	}
	if !strings.Contains(body, `data-testid="modal"`) {
		t.Errorf("edit response missing the modal shell")
	}
	if !strings.Contains(body, `data-testid="transaction-editor"`) {
		t.Errorf("edit response missing the editor body")
	}
	if !strings.Contains(body, `data-testid="txn-categorize"`) {
		t.Errorf("edit response missing the re-categorize control")
	}
}

// TestCategorizePersistsAndSwapsTheEditor drives a valid re-categorize and asserts
// the response swaps the editor body back in place (not a redirect, not a full
// page), announces transaction-changed so each list region self-refreshes, and that
// the choice persisted as a sticky override.
func TestCategorizePersistsAndSwapsTheEditor(t *testing.T) {
	database := newTestDB(t)
	accountsSvc, txnSvc, categorizationSvc := newServices(t, database, fakebank.NewService())
	registerConnection(t, accountsSvc)
	if err := txnSvc.SyncTransactions(testCtx()); err != nil {
		t.Fatalf("SyncTransactions: %v", err)
	}

	handler := adapters.NewHttpHandler(txnSvc, accountsSvc, categorizationSvc)
	rec := postCategorize(t, handler, "fake-txn-groceries", url.Values{"classification": {"transfer"}})

	if rec.Code >= 300 && rec.Code < 400 {
		t.Fatalf("categorize returned redirect status %d; want an in-place swap", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "" {
		t.Fatalf("categorize set a redirect Location %q; want an in-place swap", loc)
	}
	if trigger := rec.Header().Get("HX-Trigger"); !strings.Contains(trigger, "transaction-changed") {
		t.Errorf("categorize HX-Trigger = %q, want it to announce transaction-changed", trigger)
	}

	body := rec.Body.String()
	if strings.Contains(body, "<html") {
		t.Errorf("categorize response rendered a full page; want the editor body only")
	}
	if !strings.Contains(body, `data-testid="transaction-editor"`) {
		t.Errorf("categorize response missing the swapped editor body")
	}
	if !strings.Contains(body, `data-testid="txn-classification"`) {
		t.Errorf("categorize response missing the classification chip")
	}

	var class string
	var overridden int
	if err := database.Sql().QueryRow("SELECT classification, categorization_overridden FROM transactions WHERE id = ?", "fake-txn-groceries").Scan(&class, &overridden); err != nil {
		t.Fatalf("read row: %v", err)
	}
	if class != "transfer" || overridden != 1 {
		t.Errorf("persisted (%q, overridden=%d), want transfer + overridden 1", class, overridden)
	}
}

// TestCategorizeInvalidRendersInlineError drives a Spending pick with no Category
// and asserts the response renders the inline picker error in the editor — no
// redirect, no full page — announces NO change (nothing was written), and the row
// is left unchanged.
func TestCategorizeInvalidRendersInlineError(t *testing.T) {
	database := newTestDB(t)
	accountsSvc, txnSvc, categorizationSvc := newServices(t, database, fakebank.NewService())
	registerConnection(t, accountsSvc)
	if err := txnSvc.SyncTransactions(testCtx()); err != nil {
		t.Fatalf("SyncTransactions: %v", err)
	}

	var beforeClass string
	if err := database.Sql().QueryRow("SELECT classification FROM transactions WHERE id = ?", "fake-txn-groceries").Scan(&beforeClass); err != nil {
		t.Fatalf("read row before: %v", err)
	}

	handler := adapters.NewHttpHandler(txnSvc, accountsSvc, categorizationSvc)
	rec := postCategorize(t, handler, "fake-txn-groceries", url.Values{"classification": {"spending"}, "category_id": {""}})

	if rec.Code >= 300 && rec.Code < 400 {
		t.Fatalf("invalid categorize returned redirect status %d; want an in-place render", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "" {
		t.Fatalf("invalid categorize set a redirect Location %q; want an in-place render", loc)
	}
	if trigger := rec.Header().Get("HX-Trigger"); strings.Contains(trigger, "transaction-changed") {
		t.Errorf("invalid categorize announced transaction-changed (%q); nothing changed, so it must not", trigger)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `data-testid="txn-categorize-error"`) {
		t.Errorf("invalid categorize missing the inline picker error")
	}

	var afterClass string
	var overridden int
	if err := database.Sql().QueryRow("SELECT classification, categorization_overridden FROM transactions WHERE id = ?", "fake-txn-groceries").Scan(&afterClass, &overridden); err != nil {
		t.Fatalf("read row after: %v", err)
	}
	if afterClass != beforeClass || overridden != 0 {
		t.Errorf("invalid submit changed the row: before %q, after (%q, overridden=%d)", beforeClass, afterClass, overridden)
	}
}
