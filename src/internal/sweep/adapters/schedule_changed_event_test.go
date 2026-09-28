package adapters_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/alecdray/two-cents/src/internal/schedule"
)

// These tests drive the schedule-changed event ([ADR-0010]) the per-item modal
// now announces on every save, delete, or match change, so the transactions
// list's per-row match badge and the /sweep compact list itself both
// self-refresh without a manual reload — whether the modal was opened from
// /sweep directly (the response already re-renders the list in place) or, via
// a return handle, from a transaction's editor (the response never touches
// this list at all, so only the event gets it refreshed).
//
// schedule-changed is a new event, not a reuse of transactions' own
// transaction-changed: nothing here writes a Transaction row, so announcing
// that event would misdescribe what changed to a future reader.

// wantHXTrigger asserts a response's HX-Trigger header names the given event.
func wantHXTrigger(t *testing.T, rec *httptest.ResponseRecorder, event string) {
	t.Helper()
	got := rec.Header().Get("HX-Trigger")
	if !strings.Contains(got, event) {
		t.Errorf("HX-Trigger = %q, want it to announce %q", got, event)
	}
}

// wantNoHXTrigger asserts a response did not announce the given event — the
// restraint half of the same claim: a decision that changed nothing must not
// tell every listening region to re-fetch itself for no reason.
func wantNoHXTrigger(t *testing.T, rec *httptest.ResponseRecorder, event string) {
	t.Helper()
	if got := rec.Header().Get("HX-Trigger"); strings.Contains(got, event) {
		t.Errorf("HX-Trigger = %q, unexpectedly announced %q", got, event)
	}
}

func TestScheduleItemMutatedAnnouncesScheduleChanged(t *testing.T) {
	t.Run("save with no return handle", func(t *testing.T) {
		h, _, sched, ctx := newHandler(t)
		item, err := sched.Create(ctx, schedule.Item{
			Name: "Rent", Direction: schedule.DirectionOut, Amount: scheduleAmount(2000),
			Cadence: schedule.CadenceMonthly, DayOfMonth: 1, Active: true,
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		rec := httptest.NewRecorder()
		h.PutScheduleItem(rec, postForm(t, "/sweep/schedule/"+item.ID, item.ID, url.Values{
			"name": {"Rent (updated)"}, "direction": {"out"}, "amount": {"2100"},
			"cadence": {"monthly"}, "day_of_month": {"1"}, "active": {"1"},
		}))
		wantHXTrigger(t, rec, "schedule-changed")
	})

	t.Run("save with a return handle", func(t *testing.T) {
		h, _, sched, ctx := newHandler(t)
		item, err := sched.Create(ctx, schedule.Item{
			Name: "Rent", Direction: schedule.DirectionOut, Amount: scheduleAmount(2000),
			Cadence: schedule.CadenceMonthly, DayOfMonth: 1, Active: true,
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		rec := httptest.NewRecorder()
		h.PutScheduleItem(rec, postForm(t, "/sweep/schedule/"+item.ID, item.ID, url.Values{
			"name": {"Rent (updated)"}, "direction": {"out"}, "amount": {"2100"},
			"cadence": {"monthly"}, "day_of_month": {"1"}, "active": {"1"},
			"return_to": {"/transactions/txn-9/edit"},
		}))
		wantHXTrigger(t, rec, "schedule-changed")
	})

	t.Run("a validation failure announces nothing — the store is untouched", func(t *testing.T) {
		h, _, sched, ctx := newHandler(t)
		item, err := sched.Create(ctx, schedule.Item{
			Name: "Rent", Direction: schedule.DirectionOut, Amount: scheduleAmount(2000),
			Cadence: schedule.CadenceMonthly, DayOfMonth: 1, Active: true,
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		rec := httptest.NewRecorder()
		h.PutScheduleItem(rec, postForm(t, "/sweep/schedule/"+item.ID, item.ID, url.Values{
			"name": {""}, "direction": {"out"}, "amount": {"2100"},
			"cadence": {"monthly"}, "day_of_month": {"1"}, "active": {"1"},
		}))
		wantNoHXTrigger(t, rec, "schedule-changed")
	})

	t.Run("delete with no return handle", func(t *testing.T) {
		h, _, sched, ctx := newHandler(t)
		item, err := sched.Create(ctx, schedule.Item{
			Name: "Old subscription", Direction: schedule.DirectionOut, Amount: scheduleAmount(10),
			Cadence: schedule.CadenceMonthly, DayOfMonth: 1, Active: true,
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		rec := httptest.NewRecorder()
		h.DeleteScheduleItem(rec, postForm(t, "/sweep/schedule/"+item.ID+"/delete", item.ID, nil))
		wantHXTrigger(t, rec, "schedule-changed")
	})

	t.Run("delete with a return handle", func(t *testing.T) {
		h, _, sched, ctx := newHandler(t)
		item, err := sched.Create(ctx, schedule.Item{
			Name: "Old subscription", Direction: schedule.DirectionOut, Amount: scheduleAmount(10),
			Cadence: schedule.CadenceMonthly, DayOfMonth: 1, Active: true,
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		rec := httptest.NewRecorder()
		h.DeleteScheduleItem(rec, postForm(t, "/sweep/schedule/"+item.ID+"/delete", item.ID, url.Values{
			"return_to": {"/transactions/txn-7/edit"},
		}))
		wantHXTrigger(t, rec, "schedule-changed")
	})

	t.Run("create from a transaction, following the return handle", func(t *testing.T) {
		now := time.Now().UTC()
		ledger := fakeLedger{candidates: []schedule.Candidate{
			{TransactionID: "txn-9", Date: now, Amount: 42.50, Merchant: "WHOLEFOODS"},
		}}
		h, _, _, _ := newHandlerWithLedger(t, ledger)

		form := url.Values{
			"transaction_id": {"txn-9"}, "name": {"Groceries"}, "cadence": {"monthly"},
			"day_of_month": {"5"}, "return_to": {"/transactions/txn-9/edit"},
		}
		req := httptest.NewRequest(http.MethodPost, "/sweep/schedule", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		h.PostScheduleItem(rec, req)
		wantHXTrigger(t, rec, "schedule-changed")
	})

	t.Run("attach an existing item to a transaction", func(t *testing.T) {
		h, _, sched, ctx := newHandler(t)
		due := time.Now().UTC().AddDate(0, 0, -2)
		item, err := sched.Create(ctx, schedule.Item{
			Name: "Rent", Direction: schedule.DirectionOut, Amount: scheduleAmount(2000),
			Cadence: schedule.CadenceMonthly, DayOfMonth: due.Day(), Active: true,
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		form := url.Values{
			"item_id": {item.ID}, "occurrence": {due.Format("2006-01-02")},
			"transaction_id": {"txn-new"}, "return_to": {"/transactions/txn-new/edit"},
		}
		req := httptest.NewRequest(http.MethodPost, "/sweep/schedule/attach", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		h.PostAttachScheduleItem(rec, req)
		wantHXTrigger(t, rec, "schedule-changed")
	})
}

// TestOccurrenceDecisionsAnnounceScheduleChanged covers the three decisions
// made against one occurrence from inside the item's modal (match, confirm,
// clear). None of them go through scheduleItemMutated — they keep the modal
// open by re-rendering the editor body in place — so each announces the event
// itself; otherwise a decision made while the modal was reached from a
// transaction's editor would never tell the list behind that editor (or the
// /sweep list, if the modal was reached from there instead) that a match
// changed.
func TestOccurrenceDecisionsAnnounceScheduleChanged(t *testing.T) {
	t.Run("matching an occurrence to a transaction", func(t *testing.T) {
		h, _, sched, ctx := newHandler(t)
		due := time.Now().UTC().AddDate(0, 0, -3)
		item, err := sched.Create(ctx, schedule.Item{
			Name: "Rent", Direction: schedule.DirectionOut, Amount: scheduleAmount(2000),
			Cadence: schedule.CadenceMonthly, DayOfMonth: due.Day(), Active: true,
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		rec := httptest.NewRecorder()
		h.PostOccurrenceMatch(rec, occurrenceReq(t, item.ID, due, "match", url.Values{
			"transaction_id": {"txn-payer"},
		}))
		wantHXTrigger(t, rec, "schedule-changed")
	})

	t.Run("submitting the match form with no transaction chosen announces nothing", func(t *testing.T) {
		h, _, sched, ctx := newHandler(t)
		due := time.Now().UTC().AddDate(0, 0, -3)
		item, err := sched.Create(ctx, schedule.Item{
			Name: "Rent", Direction: schedule.DirectionOut, Amount: scheduleAmount(2000),
			Cadence: schedule.CadenceMonthly, DayOfMonth: due.Day(), Active: true,
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		rec := httptest.NewRecorder()
		h.PostOccurrenceMatch(rec, occurrenceReq(t, item.ID, due, "match", url.Values{}))
		wantNoHXTrigger(t, rec, "schedule-changed")
	})

	t.Run("a race with another tab announces nothing — the picker's attempt did not take", func(t *testing.T) {
		h, _, sched, ctx := newHandler(t)
		due := time.Now().UTC().AddDate(0, 0, -3)
		// Two separate items so the same transaction id can already settle one
		// occurrence (elsewhere) when the picker below tries to settle a second —
		// the unique-transaction-id violation PostOccurrenceMatch's own comment
		// describes as a race with another tab.
		already, err := sched.Create(ctx, schedule.Item{
			Name: "Internet", Direction: schedule.DirectionOut, Amount: scheduleAmount(80),
			Cadence: schedule.CadenceMonthly, DayOfMonth: due.Day(), Active: true,
		})
		if err != nil {
			t.Fatalf("Create(already): %v", err)
		}
		if err := sched.MatchOccurrence(ctx, already.ID, due, "txn-already-used"); err != nil {
			t.Fatalf("seed match: %v", err)
		}

		item, err := sched.Create(ctx, schedule.Item{
			Name: "Rent", Direction: schedule.DirectionOut, Amount: scheduleAmount(2000),
			Cadence: schedule.CadenceMonthly, DayOfMonth: due.Day(), Active: true,
		})
		if err != nil {
			t.Fatalf("Create(rent): %v", err)
		}

		rec := httptest.NewRecorder()
		h.PostOccurrenceMatch(rec, occurrenceReq(t, item.ID, due, "match", url.Values{
			"transaction_id": {"txn-already-used"},
		}))
		wantNoHXTrigger(t, rec, "schedule-changed")
	})

	t.Run("confirming an automatic match", func(t *testing.T) {
		h, _, sched, ctx := newHandler(t)
		due := time.Now().UTC().AddDate(0, 0, -3)
		item, err := sched.Create(ctx, schedule.Item{
			Name: "Rent", Direction: schedule.DirectionOut, Amount: scheduleAmount(2000),
			Cadence: schedule.CadenceMonthly, DayOfMonth: due.Day(), Active: true,
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := sched.RecordAutoMatches(ctx, []schedule.Match{
			{ItemID: item.ID, Occurrence: due, TransactionID: "txn-auto-1"},
		}); err != nil {
			t.Fatalf("RecordAutoMatches: %v", err)
		}

		rec := httptest.NewRecorder()
		h.PostOccurrenceConfirm(rec, occurrenceReq(t, item.ID, due, "confirm", url.Values{}))
		wantHXTrigger(t, rec, "schedule-changed")
	})

	t.Run("clearing an occurrence", func(t *testing.T) {
		h, _, sched, ctx := newHandler(t)
		due := time.Now().UTC().AddDate(0, 0, -3)
		item, err := sched.Create(ctx, schedule.Item{
			Name: "Rent", Direction: schedule.DirectionOut, Amount: scheduleAmount(2000),
			Cadence: schedule.CadenceMonthly, DayOfMonth: due.Day(), Active: true,
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		rec := httptest.NewRecorder()
		h.PostOccurrenceClear(rec, occurrenceReq(t, item.ID, due, "clear", url.Values{}))
		wantHXTrigger(t, rec, "schedule-changed")
	})
}

// TestScheduleRegionRefreshListensForScheduleChanged asserts the /sweep page's
// own schedule region carries a hidden listener wired to re-fetch it on
// schedule-changed, exactly the shape transactions' own transactionsRefreshListener
// already uses for transaction-changed ([ADR-0010]) — the half of this task
// that makes criterion 2 hold even when the mutating response went to a
// transaction's editor instead of back to this page.
func TestScheduleRegionRefreshListensForScheduleChanged(t *testing.T) {
	h, _, _, _ := newHandler(t)

	_, body := getSweepPage(t, h)
	for what, want := range map[string]string{
		"the listener element":                 `data-testid="schedule-refresh-listener"`,
		"listening for the event on body":      `hx-trigger="schedule-changed from:body"`,
		"re-fetching the region-only endpoint": `hx-get="/sweep/schedule"`,
		"targeting the schedule region":        `hx-target="#sweep-schedule"`,
		"replacing the whole region":           `hx-swap="outerHTML"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("schedule region missing %s (%q):\n%s", what, want, body)
		}
	}
}

// TestGetScheduleRegionRefetchesCurrentState drives the endpoint the schedule
// region's own listener hx-gets: a plain, region-only read (no page shell, no
// snapshot) that reflects whatever is currently stored — what a currently
// loaded /sweep page's self-refresh actually receives when schedule-changed
// fires from a mutation whose own response went elsewhere (a return handle
// back to a transaction's editor).
func TestGetScheduleRegionRefetchesCurrentState(t *testing.T) {
	h, _, sched, ctx := newHandler(t)
	item, err := sched.Create(ctx, schedule.Item{
		Name: "Rent", Direction: schedule.DirectionOut, Amount: scheduleAmount(2000),
		Cadence: schedule.CadenceMonthly, DayOfMonth: 1, Active: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Mutate the way a modal reached via return_to would — the store changes,
	// but nothing in this test's process renders this page again except the
	// self-refresh endpoint itself.
	item.Name = "Rent (renamed elsewhere)"
	if err := sched.Update(ctx, item); err != nil {
		t.Fatalf("Update: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/sweep/schedule", nil)
	rec := httptest.NewRecorder()
	h.GetScheduleRegion(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body:\n%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `data-testid="sweep-schedule"`) {
		t.Errorf("response is not the schedule region:\n%s", body)
	}
	if strings.Contains(body, `data-testid="sweep-page"`) {
		t.Errorf("response unexpectedly carried the full page shell:\n%s", body)
	}
	if !strings.Contains(body, "Rent (renamed elsewhere)") {
		t.Errorf("response did not reflect the current stored name:\n%s", body)
	}
}
