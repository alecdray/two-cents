package adapters_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/schedule"
	"github.com/alecdray/two-cents/src/internal/sweep"
	"github.com/alecdray/two-cents/src/internal/sweep/adapters"
)

// These tests drive the two ways the transaction editor reaches the schedule
// ([ADR-0031]): creating a new item from a transaction (GET /sweep/schedule/new,
// POST /sweep/schedule carrying a transaction_id) and attaching a transaction to
// an existing item (GET and POST /sweep/schedule/attach) — over the real sweep
// and schedule Services on a migrated temp DB.

// fakeLedger stands in for the composition root's adapter over accounts +
// transactions ([ADR-0027]) — the port CreateItemFromTransaction reads a named
// transaction's direction, amount, and date through.
type fakeLedger struct {
	candidates []schedule.Candidate
}

func (f fakeLedger) CheckingCandidates(_ contextx.ContextX, from, to time.Time) ([]schedule.Candidate, error) {
	var out []schedule.Candidate
	for _, c := range f.candidates {
		if !c.Date.Before(from) && !c.Date.After(to) {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f fakeLedger) TransactionsByID(_ contextx.ContextX, ids []string) ([]schedule.Candidate, error) {
	byID := make(map[string]schedule.Candidate, len(f.candidates))
	for _, c := range f.candidates {
		byID[c.TransactionID] = c
	}
	var out []schedule.Candidate
	for _, id := range ids {
		if c, ok := byID[id]; ok {
			out = append(out, c)
		}
	}
	return out, nil
}

// newHandlerWithLedger builds the page handler over real sweep and schedule
// Services backed by the given ledger — needed only by the create-from-
// transaction path, which reads the named transaction through it.
func newHandlerWithLedger(t *testing.T, ledger schedule.Ledger) (*adapters.HttpHandler, *sweep.Service, *schedule.Service, contextx.ContextX) {
	t.Helper()
	database := newTestDB(t)
	scheduleSvc := schedule.NewService(database, ledger, time.UTC)
	sweepSvc := sweep.NewService(nil, scheduleSvc, database, time.UTC, 500)
	return adapters.NewHttpHandler(sweepSvc, scheduleSvc), sweepSvc, scheduleSvc, contextx.NewContextX(context.Background())
}

func TestGetNewScheduleItemModal(t *testing.T) {
	h, _, _, _ := newHandler(t)

	target := "/sweep/schedule/new?" + url.Values{
		"transaction_id": {"txn-1"},
		"merchant":       {"Whole Foods"},
		"direction":      {"out"},
		"day":            {"5"},
		"return_to":      {"/transactions/txn-1/edit"},
	}.Encode()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	h.GetNewScheduleItemModal(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body:\n%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for what, want := range map[string]string{
		"the modal shell":                 `data-testid="modal"`,
		"the editor region":               `data-testid="schedule-item-editor"`,
		"the transaction id hidden field": `data-testid="schedule-item-editor-transaction-id"`,
		"the transaction id value":        `value="txn-1"`,
		"the name prefilled from merchant": `value="Whole Foods"`,
		"the day prefilled":               `value="5"`,
		"the locked-fields note":          `data-testid="schedule-item-editor-locked-note"`,
		"the return handle":               `data-testid="schedule-item-editor-return-to"`,
		"the save button":                 `data-testid="schedule-item-editor-submit"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("create modal missing %s (%q):\n%s", what, want, body)
		}
	}
	// Nothing exists yet to delete, toggle, or show history for.
	for _, gone := range []string{
		`data-testid="schedule-item-editor-delete-submit"`,
		`data-testid="schedule-item-editor-active"`,
		`data-testid="schedule-occurrences"`,
	} {
		if strings.Contains(body, gone) {
			t.Errorf("create modal unexpectedly rendered %q:\n%s", gone, body)
		}
	}
}

func TestCreateScheduleItemFromTransaction(t *testing.T) {
	t.Run("saving derives direction from the transaction (ignoring the submitted one), leaves amount to compute, and follows the return handle", func(t *testing.T) {
		now := time.Now().UTC()
		ledger := fakeLedger{candidates: []schedule.Candidate{
			{TransactionID: "txn-9", Date: now, Amount: 42.50, Merchant: "WHOLEFOODS"},
		}}
		h, _, sched, ctx := newHandlerWithLedger(t, ledger)

		form := url.Values{
			"transaction_id": {"txn-9"},
			"name":           {"Groceries"},
			"cadence":        {"monthly"},
			"day_of_month":   {strconv.Itoa(now.Day())},
			// Deliberately wrong/present — CreateItemFromTransaction must ignore both.
			"direction": {"in"},
			"amount":    {"999"},
			"return_to": {"/transactions/txn-9/edit"},
		}
		req := httptest.NewRequest(http.MethodPost, "/sweep/schedule", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		h.PostScheduleItem(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200, body:\n%s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if !strings.Contains(body, `data-testid="schedule-item-editor-return-loader"`) || !strings.Contains(body, `hx-get="/transactions/txn-9/edit"`) {
			t.Fatalf("create-from-transaction save did not follow the return handle:\n%s", body)
		}

		items, err := sched.List(ctx)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(items) != 1 {
			t.Fatalf("%d items stored, want 1", len(items))
		}
		item := items[0]
		if item.Name != "Groceries" {
			t.Errorf("Name = %q, want Groceries (user-declared, never seeded)", item.Name)
		}
		if item.Direction != schedule.DirectionOut {
			t.Errorf("Direction = %q, want out — derived from the transaction's own positive amount, the submitted 'in' must be ignored", item.Direction)
		}
		if item.DayOfMonth != now.Day() {
			t.Errorf("DayOfMonth = %d, want %d (user-declared cadence date)", item.DayOfMonth, now.Day())
		}
		if !item.Active {
			t.Error("a created item must be active")
		}

		states, err := sched.AmountStates(ctx)
		if err != nil {
			t.Fatalf("AmountStates: %v", err)
		}
		if st := states[item.ID]; st.Override || !st.Computed {
			t.Errorf("AmountState = %+v, want {Override: false, Computed: true} — the submitted 999 must be ignored, and the seeding match makes a figure computable", st)
		}

		matched, ok, err := sched.MatchedItem(ctx, "txn-9")
		if err != nil {
			t.Fatalf("MatchedItem: %v", err)
		}
		if !ok || matched.ID != item.ID {
			t.Errorf("MatchedItem(txn-9) = (%+v, %v), want the created item matched as its seeding transaction", matched, ok)
		}
	})

	t.Run("a rejected declaration re-renders the create editor with the transaction id and return handle preserved, nothing stored", func(t *testing.T) {
		h, _, sched, ctx := newHandlerWithLedger(t, fakeLedger{})

		form := url.Values{
			"transaction_id": {"txn-missing"}, // no ledger row for this id
			"name":           {"Groceries"},
			"cadence":        {"monthly"},
			"day_of_month":   {"1"},
			"return_to":      {"/transactions/txn-missing/edit"},
		}
		req := httptest.NewRequest(http.MethodPost, "/sweep/schedule", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		h.PostScheduleItem(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200, body:\n%s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if !strings.Contains(body, `data-testid="schedule-item-editor"`) || !strings.Contains(body, `data-testid="schedule-item-editor-error"`) {
			t.Errorf("rejected create-from-transaction did not re-render the editor with an inline error:\n%s", body)
		}
		if !strings.Contains(body, `data-testid="schedule-item-editor-transaction-id"`) {
			t.Errorf("rejected save dropped the transaction id, breaking a corrected resubmit:\n%s", body)
		}
		if !strings.Contains(body, `data-testid="schedule-item-editor-return-to"`) {
			t.Errorf("rejected save dropped the return handle:\n%s", body)
		}
		if rec.Header().Get("HX-Retarget") != "" {
			t.Errorf("a validation failure unexpectedly retargeted the schedule region")
		}

		items, err := sched.List(ctx)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(items) != 0 {
			t.Errorf("rejected declaration stored %d item(s), want 0", len(items))
		}
	})

	t.Run("the ordinary add form (no transaction id) is unaffected", func(t *testing.T) {
		h, _, sched, ctx := newHandler(t)

		form := url.Values{
			"name":         {"Rent"},
			"direction":    {"out"},
			"amount":       {"2000"},
			"cadence":      {"monthly"},
			"day_of_month": {"1"},
		}
		req := httptest.NewRequest(http.MethodPost, "/sweep/schedule", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		h.PostScheduleItem(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200, body:\n%s", rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), `data-testid="schedule-item-editor-return-loader"`) {
			t.Errorf("the plain add form unexpectedly rendered a return loader")
		}
		items, err := sched.List(ctx)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(items) != 1 || items[0].Name != "Rent" {
			t.Errorf("plain add did not store the declared item: %+v", items)
		}
	})
}

func TestGetAttachScheduleItemModal(t *testing.T) {
	h, _, sched, ctx := newHandler(t)

	// Outstanding, same direction: offered.
	due := time.Now().UTC().AddDate(0, 0, -2)
	rent, err := sched.Create(ctx, schedule.Item{
		Name:       "Rent",
		Direction:  schedule.DirectionOut,
		Amount:     scheduleAmount(2000),
		Cadence:    schedule.CadenceMonthly,
		DayOfMonth: due.Day(),
		Active:     true,
	})
	if err != nil {
		t.Fatalf("Create rent: %v", err)
	}

	// Same direction, but already settled: not offered.
	settled, err := sched.Create(ctx, schedule.Item{
		Name:       "Internet",
		Direction:  schedule.DirectionOut,
		Amount:     scheduleAmount(80),
		Cadence:    schedule.CadenceMonthly,
		DayOfMonth: due.Day(),
		Active:     true,
	})
	if err != nil {
		t.Fatalf("Create internet: %v", err)
	}
	if err := sched.MatchOccurrence(ctx, settled.ID, due, "txn-already-settled"); err != nil {
		t.Fatalf("MatchOccurrence: %v", err)
	}

	// Outstanding, but the wrong direction: not offered.
	if _, err := sched.Create(ctx, schedule.Item{
		Name:       "Paycheck",
		Direction:  schedule.DirectionIn,
		Amount:     scheduleAmount(3000),
		Cadence:    schedule.CadenceMonthly,
		DayOfMonth: due.Day(),
		Active:     true,
	}); err != nil {
		t.Fatalf("Create paycheck: %v", err)
	}

	target := "/sweep/schedule/attach?" + url.Values{
		"transaction_id": {"txn-new"},
		"direction":      {"out"},
		"date":           {due.Format("2006-01-02")},
		"return_to":      {"/transactions/txn-new/edit"},
	}.Encode()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	h.GetAttachScheduleItemModal(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body:\n%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Rent") {
		t.Errorf("attach picker missing the eligible item (Rent):\n%s", body)
	}
	if got := strings.Count(body, `data-testid="schedule-attach-option"`); got != 1 {
		t.Errorf("attach picker offered %d option(s), want 1 (only Rent qualifies)", got)
	}
	for _, gone := range []string{"Internet", "Paycheck"} {
		if strings.Contains(body, gone) {
			t.Errorf("attach picker unexpectedly offered %q (settled, or wrong direction):\n%s", gone, body)
		}
	}
	// The option's form carries the nearest outstanding occurrence and the
	// transaction/return context, so choosing it needs nothing further.
	wantOccurrence := `value="` + due.Format("2006-01-02") + `"`
	wantItemID := `value="` + rent.ID + `"`
	for _, want := range []string{wantOccurrence, wantItemID, `value="txn-new"`} {
		if !strings.Contains(body, want) {
			t.Errorf("attach option missing %q:\n%s", want, body)
		}
	}
}

// TestGetAttachScheduleItemModalDatelessItemOffersItsOpenMonth demonstrates the
// other half of "nearest occurrence, or its containing calendar month for a
// dateless item" ([ADR-0031]): a dateless item's only occurrence is the current
// open month's stand-in, so a transaction dated anywhere in that month attaches
// to it with no special-casing needed in the picker itself.
func TestGetAttachScheduleItemModalDatelessItemOffersItsOpenMonth(t *testing.T) {
	h, _, sched, ctx := newHandler(t)
	today := time.Now().UTC()

	if _, err := sched.Create(ctx, schedule.Item{
		Name:      "Car repair",
		Direction: schedule.DirectionOut,
		Amount:    scheduleAmount(300),
		Cadence:   schedule.CadenceMonthly,
		// DayOfMonth left zero: dateless.
		Active: true,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	target := "/sweep/schedule/attach?" + url.Values{
		"transaction_id": {"txn-repair"},
		"direction":      {"out"},
		"date":           {today.Format("2006-01-02")},
	}.Encode()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	h.GetAttachScheduleItemModal(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "Car repair") {
		t.Fatalf("the dateless item was not offered:\n%s", body)
	}
	wantMonth := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	if !strings.Contains(body, `value="`+wantMonth+`"`) {
		t.Errorf("the dateless item's occurrence was not this month's stand-in (%s):\n%s", wantMonth, body)
	}
}

func TestGetAttachScheduleItemModalEmptyWhenNothingQualifies(t *testing.T) {
	h, _, _, _ := newHandler(t)

	target := "/sweep/schedule/attach?" + url.Values{
		"transaction_id": {"txn-new"},
		"direction":      {"out"},
		"date":           {time.Now().UTC().Format("2006-01-02")},
	}.Encode()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	h.GetAttachScheduleItemModal(rec, req)

	if !strings.Contains(rec.Body.String(), `data-testid="schedule-attach-empty"`) {
		t.Errorf("an empty schedule must still render the attach picker's empty state:\n%s", rec.Body.String())
	}
}

func TestPostAttachScheduleItem(t *testing.T) {
	t.Run("choosing an option matches the occurrence and follows the return handle", func(t *testing.T) {
		h, _, sched, ctx := newHandler(t)
		due := time.Now().UTC().AddDate(0, 0, -2)
		item, err := sched.Create(ctx, schedule.Item{
			Name:       "Rent",
			Direction:  schedule.DirectionOut,
			Amount:     scheduleAmount(2000),
			Cadence:    schedule.CadenceMonthly,
			DayOfMonth: due.Day(),
			Active:     true,
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		form := url.Values{
			"item_id":        {item.ID},
			"occurrence":     {due.Format("2006-01-02")},
			"transaction_id": {"txn-new"},
			"return_to":      {"/transactions/txn-new/edit"},
		}
		req := httptest.NewRequest(http.MethodPost, "/sweep/schedule/attach", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		h.PostAttachScheduleItem(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200, body:\n%s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if !strings.Contains(body, `data-testid="schedule-item-editor-return-loader"`) || !strings.Contains(body, `hx-get="/transactions/txn-new/edit"`) {
			t.Fatalf("attach did not follow the return handle:\n%s", body)
		}

		matched, ok, err := sched.MatchedItem(ctx, "txn-new")
		if err != nil {
			t.Fatalf("MatchedItem: %v", err)
		}
		if !ok || matched.ID != item.ID {
			t.Errorf("MatchedItem(txn-new) = (%+v, %v), want %q matched", matched, ok, item.ID)
		}
	})

	t.Run("no return handle closes the modal and refreshes the schedule list", func(t *testing.T) {
		h, _, sched, ctx := newHandler(t)
		due := time.Now().UTC().AddDate(0, 0, -2)
		item, err := sched.Create(ctx, schedule.Item{
			Name:       "Rent",
			Direction:  schedule.DirectionOut,
			Amount:     scheduleAmount(2000),
			Cadence:    schedule.CadenceMonthly,
			DayOfMonth: due.Day(),
			Active:     true,
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		form := url.Values{
			"item_id":        {item.ID},
			"occurrence":     {due.Format("2006-01-02")},
			"transaction_id": {"txn-new"},
		}
		req := httptest.NewRequest(http.MethodPost, "/sweep/schedule/attach", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		h.PostAttachScheduleItem(rec, req)

		if got := rec.Header().Get("HX-Retarget"); got != "#sweep-schedule" {
			t.Errorf("HX-Retarget = %q, want #sweep-schedule", got)
		}
		body := rec.Body.String()
		if !strings.Contains(body, `data-testid="modal-container"`) || !strings.Contains(body, `hx-swap-oob="true"`) {
			t.Errorf("no-handle attach response did not close the modal:\n%s", body)
		}
		if !strings.Contains(body, `data-testid="sweep-schedule"`) {
			t.Errorf("no-handle attach response did not refresh the schedule list:\n%s", body)
		}

		_, ok, err := sched.MatchedItem(ctx, "txn-new")
		if err != nil {
			t.Fatalf("MatchedItem: %v", err)
		}
		if !ok {
			t.Error("the match was not recorded")
		}
	})
}
