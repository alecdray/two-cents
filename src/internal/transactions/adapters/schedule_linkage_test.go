package adapters_test

import (
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alecdray/two-cents/src/internal/fakebank"
	"github.com/alecdray/two-cents/src/internal/schedule"
	"github.com/alecdray/two-cents/src/internal/transactions/adapters"
)

// scheduleAmount is a declared amount as the schedule package's Item.Amount
// pointer, so a test can seed a stored item without leaving it undeclared.
func scheduleAmount(v float64) *float64 { return &v }

// htmlEscapeQuery rewrites an encoded query string's "&" separators the way
// templ escapes an attribute value, so an expected URL built from url.Values
// matches what the rendered HTML actually carries.
func htmlEscapeQuery(encoded string) string {
	return strings.ReplaceAll(encoded, "&", "&amp;")
}

// TestScheduleLinkage drives the transaction editor's schedule section end to
// end over the real accounts, transactions, and schedule Services sharing one
// migrated DB, exercising every state a row's editor can be in ([ADR-0031]):
// an unmatched checking row offers the attach/create controls, a matched one
// shows only the link into its item, and a non-checking row shows neither —
// regardless of match state, since the checking gate is checked first.
func TestScheduleLinkage(t *testing.T) {
	t.Run("an unmatched checking transaction offers the attach and create controls, never the matched link", func(t *testing.T) {
		database := newTestDB(t)
		accountsSvc, txnSvc, categorizationSvc := newServices(t, database, fakebank.NewService())
		scheduleSvc := schedule.NewService(database, nil, time.UTC)
		registerConnection(t, accountsSvc)
		if err := txnSvc.SyncTransactions(testCtx()); err != nil {
			t.Fatalf("SyncTransactions: %v", err)
		}

		row, err := txnSvc.RecentTransaction(testCtx(), "fake-txn-groceries")
		if err != nil {
			t.Fatalf("RecentTransaction: %v", err)
		}

		handler := adapters.NewHttpHandler(txnSvc, accountsSvc, categorizationSvc, scheduleSvc)
		rec := getEditModal(t, handler, "fake-txn-groceries")

		body := rec.Body.String()
		if !strings.Contains(body, `data-testid="transaction-editor-schedule"`) {
			t.Fatalf("editor missing the schedule section for a checking row:\n%s", body)
		}
		if !strings.Contains(body, `data-testid="transaction-editor-schedule-attach"`) {
			t.Errorf("unmatched checking row missing the attach control:\n%s", body)
		}
		if !strings.Contains(body, `data-testid="transaction-editor-schedule-create"`) {
			t.Errorf("unmatched checking row missing the create control:\n%s", body)
		}
		if strings.Contains(body, `data-testid="transaction-editor-schedule-item"`) {
			t.Errorf("unmatched row unexpectedly rendered the matched-item link:\n%s", body)
		}

		returnTo := "/transactions/fake-txn-groceries/edit"
		wantAttach := htmlEscapeQuery(url.Values{
			"transaction_id": {"fake-txn-groceries"},
			"direction":      {"out"},
			"date":           {row.Date.Format("2006-01-02")},
			"return_to":      {returnTo},
		}.Encode())
		if !strings.Contains(body, `hx-get="/sweep/schedule/attach?`+wantAttach+`"`) {
			t.Errorf("attach control URL missing expected params (want query %q):\n%s", wantAttach, body)
		}
		wantCreate := htmlEscapeQuery(url.Values{
			"transaction_id": {"fake-txn-groceries"},
			"merchant":       {"Whole Foods"},
			"direction":      {"out"},
			"day":            {strconv.Itoa(row.Date.Day())},
			"return_to":      {returnTo},
		}.Encode())
		if !strings.Contains(body, `hx-get="/sweep/schedule/new?`+wantCreate+`"`) {
			t.Errorf("create control URL missing expected params (want query %q):\n%s", wantCreate, body)
		}
	})

	t.Run("a matched checking transaction shows only the link into its item", func(t *testing.T) {
		database := newTestDB(t)
		accountsSvc, txnSvc, categorizationSvc := newServices(t, database, fakebank.NewService())
		scheduleSvc := schedule.NewService(database, nil, time.UTC)
		registerConnection(t, accountsSvc)
		if err := txnSvc.SyncTransactions(testCtx()); err != nil {
			t.Fatalf("SyncTransactions: %v", err)
		}

		row, err := txnSvc.RecentTransaction(testCtx(), "fake-txn-groceries")
		if err != nil {
			t.Fatalf("RecentTransaction: %v", err)
		}

		item, err := scheduleSvc.Create(testCtx(), schedule.Item{
			Name:       "Groceries budget",
			Direction:  schedule.DirectionOut,
			Amount:     scheduleAmount(90),
			Cadence:    schedule.CadenceMonthly,
			DayOfMonth: row.Date.Day(),
			Active:     true,
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := scheduleSvc.MatchOccurrence(testCtx(), item.ID, row.Date, "fake-txn-groceries"); err != nil {
			t.Fatalf("MatchOccurrence: %v", err)
		}

		handler := adapters.NewHttpHandler(txnSvc, accountsSvc, categorizationSvc, scheduleSvc)
		rec := getEditModal(t, handler, "fake-txn-groceries")

		body := rec.Body.String()
		if !strings.Contains(body, `data-testid="transaction-editor-schedule-item"`) {
			t.Fatalf("matched checking row missing the item link:\n%s", body)
		}
		if !strings.Contains(body, "Groceries budget") {
			t.Errorf("item link did not name the matched item:\n%s", body)
		}
		returnTo := "/transactions/fake-txn-groceries/edit"
		wantURL := `hx-get="/sweep/schedule/` + item.ID + `/edit?` + url.Values{"return_to": {returnTo}}.Encode() + `"`
		if !strings.Contains(body, wantURL) {
			t.Errorf("item link URL = missing %q:\n%s", wantURL, body)
		}

		for _, gone := range []string{
			`data-testid="transaction-editor-schedule-attach"`,
			`data-testid="transaction-editor-schedule-create"`,
		} {
			if strings.Contains(body, gone) {
				t.Errorf("matched row unexpectedly rendered %q (attach/create and the link must never both show):\n%s", gone, body)
			}
		}
	})

	t.Run("a non-checking transaction shows neither control, regardless of match state", func(t *testing.T) {
		database := newTestDB(t)
		accountsSvc, txnSvc, categorizationSvc := newServices(t, database, fakebank.NewService())
		scheduleSvc := schedule.NewService(database, nil, time.UTC)
		registerConnection(t, accountsSvc)
		if err := txnSvc.SyncTransactions(testCtx()); err != nil {
			t.Fatalf("SyncTransactions: %v", err)
		}

		// fake-txn-coffee is on the credit card, not checking. Force a match onto
		// it anyway, so the assertion proves the checking gate — not merely the
		// absence of a match — is what withholds the section.
		row, err := txnSvc.RecentTransaction(testCtx(), "fake-txn-coffee")
		if err != nil {
			t.Fatalf("RecentTransaction: %v", err)
		}
		item, err := scheduleSvc.Create(testCtx(), schedule.Item{
			Name:       "Coffee habit",
			Direction:  schedule.DirectionOut,
			Amount:     scheduleAmount(6),
			Cadence:    schedule.CadenceMonthly,
			DayOfMonth: row.Date.Day(),
			Active:     true,
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := scheduleSvc.MatchOccurrence(testCtx(), item.ID, row.Date, "fake-txn-coffee"); err != nil {
			t.Fatalf("MatchOccurrence: %v", err)
		}

		handler := adapters.NewHttpHandler(txnSvc, accountsSvc, categorizationSvc, scheduleSvc)
		rec := getEditModal(t, handler, "fake-txn-coffee")

		body := rec.Body.String()
		for _, gone := range []string{
			`data-testid="transaction-editor-schedule"`,
			`data-testid="transaction-editor-schedule-attach"`,
			`data-testid="transaction-editor-schedule-create"`,
			`data-testid="transaction-editor-schedule-item"`,
		} {
			if strings.Contains(body, gone) {
				t.Errorf("non-checking row unexpectedly rendered %q:\n%s", gone, body)
			}
		}
	})
}
