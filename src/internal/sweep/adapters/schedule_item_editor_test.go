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

// These tests drive the schedule item editor modal end-to-end over the real
// sweep and schedule Services on a migrated temp DB, exercising the seam the
// prior change left on the compact row's Edit affordance: the modal renders
// the item's complete declaration form, its active toggle, its delete
// control, and its occurrence-matching history and controls — all reached
// without leaving /sweep — and saving, deleting, or dismissing it returns to
// the schedule list showing the current, correct state.

// getReq builds a GET request carrying the item's path value, the shape every
// handler here reads its target from.
func getReq(t *testing.T, target, id string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.SetPathValue("id", id)
	return req
}

// postForm builds a POST request carrying the item's path value and a
// url-encoded body, the shape every mutation here is submitted as.
func postForm(t *testing.T, target, id string, form url.Values) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", id)
	return req
}

func TestScheduleItemEditorModal(t *testing.T) {
	t.Run("opens with the item's complete form, active toggle, delete control, and occurrence history", func(t *testing.T) {
		h, _, sched, ctx := newHandler(t)

		due := time.Now().UTC().AddDate(0, 0, -3)
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

		rec := httptest.NewRecorder()
		h.GetEditScheduleItemModal(rec, getReq(t, "/sweep/schedule/"+item.ID+"/edit", item.ID))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200, body:\n%s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		for what, want := range map[string]string{
			"the shared modal shell":        `data-testid="modal"`,
			"the editor region":             `data-testid="schedule-item-editor"`,
			"the complete declaration form": `data-testid="schedule-item-editor-form"`,
			"the name field":                `data-testid="schedule-item-editor-name"`,
			"the direction field":           `data-testid="schedule-item-editor-direction"`,
			"the amount field":              `data-testid="schedule-item-editor-amount"`,
			"the cadence field":             `data-testid="schedule-item-editor-cadence"`,
			"the name prefilled":            `value="Rent"`,
			"the active toggle":             `data-testid="schedule-item-editor-active"`,
			"the delete control":            `data-testid="schedule-item-editor-delete-submit"`,
			"the occurrence history block":  `data-testid="schedule-occurrences"`,
			"an outstanding occurrence":     `data-testid="schedule-occurrence-outstanding"`,
			"the clear control":             `data-testid="schedule-occurrence-clear"`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("modal missing %s (%q):\n%s", what, want, body)
			}
		}
		// No return_to was passed, so no return machinery renders — a bare dismiss
		// simply closes, leaving the untouched schedule list behind it.
		for _, gone := range []string{`data-testid="schedule-item-editor-return-to"`, `close from:closest dialog`} {
			if strings.Contains(body, gone) {
				t.Errorf("modal opened with no handle unexpectedly rendered %q", gone)
			}
		}
	})

	t.Run("an unknown item id is a 404", func(t *testing.T) {
		h, _, _, _ := newHandler(t)

		rec := httptest.NewRecorder()
		h.GetEditScheduleItemModal(rec, getReq(t, "/sweep/schedule/nope/edit", "nope"))

		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", rec.Code)
		}
	})
}

func TestScheduleItemEditorSaveNoHandleClosesAndRefreshesList(t *testing.T) {
	h, _, sched, ctx := newHandler(t)

	item, err := sched.Create(ctx, schedule.Item{
		Name:       "Rent",
		Direction:  schedule.DirectionOut,
		Amount:     scheduleAmount(2000),
		Cadence:    schedule.CadenceMonthly,
		DayOfMonth: 1,
		Active:     true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	rec := httptest.NewRecorder()
	h.PutScheduleItem(rec, postForm(t, "/sweep/schedule/"+item.ID, item.ID, url.Values{
		"name":         {"Rent (updated)"},
		"direction":    {"out"},
		"amount":       {"2100"},
		"cadence":      {"monthly"},
		"day_of_month": {"1"},
		"active":       {"1"},
	}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body:\n%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("HX-Retarget"); got != "#sweep-schedule" {
		t.Errorf("HX-Retarget = %q, want #sweep-schedule", got)
	}
	if got := rec.Header().Get("HX-Reswap"); got != "outerHTML" {
		t.Errorf("HX-Reswap = %q, want outerHTML", got)
	}
	body := rec.Body.String()
	for what, want := range map[string]string{
		"the OOB-empty modal container that closes the modal": `data-testid="modal-container"`,
		"the OOB swap attribute":                              `hx-swap-oob="true"`,
		"the refreshed schedule region":                       `data-testid="sweep-schedule"`,
		"the saved change reflected":                          "Rent (updated)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("no-handle save response missing %s (%q):\n%s", what, want, body)
		}
	}

	saved, err := sched.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(saved) != 1 || saved[0].Name != "Rent (updated)" || saved[0].Amount == nil || *saved[0].Amount != 2100 {
		t.Errorf("stored item not updated: %+v", saved)
	}
}

func TestScheduleItemEditorSaveValidationFailureKeepsModalOpenAndUnchanged(t *testing.T) {
	h, _, sched, ctx := newHandler(t)

	item, err := sched.Create(ctx, schedule.Item{
		Name:       "Rent",
		Direction:  schedule.DirectionOut,
		Amount:     scheduleAmount(2000),
		Cadence:    schedule.CadenceMonthly,
		DayOfMonth: 1,
		Active:     true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	rec := httptest.NewRecorder()
	h.PutScheduleItem(rec, postForm(t, "/sweep/schedule/"+item.ID, item.ID, url.Values{
		"name":         {""}, // blank name is rejected
		"direction":    {"out"},
		"amount":       {"2100"},
		"cadence":      {"monthly"},
		"day_of_month": {"1"},
		"active":       {"1"},
	}))

	body := rec.Body.String()
	if !strings.Contains(body, `data-testid="schedule-item-editor"`) || !strings.Contains(body, `data-testid="schedule-item-editor-error"`) {
		t.Errorf("blank name did not re-render the editor with an inline error:\n%s", body)
	}
	if rec.Header().Get("HX-Retarget") != "" {
		t.Errorf("a validation failure unexpectedly retargeted the schedule region")
	}

	saved, err := sched.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(saved) != 1 || saved[0].Name != "Rent" {
		t.Errorf("a validation failure mutated the stored item: %+v", saved)
	}
}

func TestScheduleItemEditorDeleteNoHandleClosesAndRefreshesList(t *testing.T) {
	h, _, sched, ctx := newHandler(t)

	item, err := sched.Create(ctx, schedule.Item{
		Name:       "Old subscription",
		Direction:  schedule.DirectionOut,
		Amount:     scheduleAmount(10),
		Cadence:    schedule.CadenceMonthly,
		DayOfMonth: 1,
		Active:     true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	rec := httptest.NewRecorder()
	h.DeleteScheduleItem(rec, postForm(t, "/sweep/schedule/"+item.ID+"/delete", item.ID, nil))

	if got := rec.Header().Get("HX-Retarget"); got != "#sweep-schedule" {
		t.Errorf("HX-Retarget = %q, want #sweep-schedule", got)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `data-testid="modal-container"`) || !strings.Contains(body, `hx-swap-oob="true"`) {
		t.Errorf("delete response did not close the modal:\n%s", body)
	}
	if !strings.Contains(body, `data-testid="schedule-empty"`) {
		t.Errorf("deleting the only item did not fall back to the empty state:\n%s", body)
	}

	remaining, err := sched.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("delete left %d items", len(remaining))
	}
}

func TestScheduleItemEditorReturnHandle(t *testing.T) {
	t.Run("a same-origin relative handle is echoed back by the open route", func(t *testing.T) {
		h, _, sched, ctx := newHandler(t)
		item, err := sched.Create(ctx, schedule.Item{
			Name:       "Rent",
			Direction:  schedule.DirectionOut,
			Amount:     scheduleAmount(2000),
			Cadence:    schedule.CadenceMonthly,
			DayOfMonth: 1,
			Active:     true,
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		rec := httptest.NewRecorder()
		h.GetEditScheduleItemModal(rec, getReq(t, "/sweep/schedule/"+item.ID+"/edit?return_to=/transactions/txn-1/edit", item.ID))

		body := rec.Body.String()
		for _, want := range []string{
			`data-testid="schedule-item-editor-return-to"`,
			`value="/transactions/txn-1/edit"`,
			`close from:closest dialog`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("same-origin return handle not honored, missing %q:\n%s", want, body)
			}
		}
	})

	t.Run("a non-same-origin handle is rejected by the open route", func(t *testing.T) {
		h, _, sched, ctx := newHandler(t)
		item, err := sched.Create(ctx, schedule.Item{
			Name:       "Rent",
			Direction:  schedule.DirectionOut,
			Amount:     scheduleAmount(2000),
			Cadence:    schedule.CadenceMonthly,
			DayOfMonth: 1,
			Active:     true,
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		for _, bad := range []string{"https://evil.com/x", "//evil.com", "evil.com/x"} {
			rec := httptest.NewRecorder()
			h.GetEditScheduleItemModal(rec, getReq(t, "/sweep/schedule/"+item.ID+"/edit?return_to="+url.QueryEscape(bad), item.ID))

			body := rec.Body.String()
			for _, gone := range []string{`data-testid="schedule-item-editor-return-to"`, `close from:closest dialog`} {
				if strings.Contains(body, gone) {
					t.Errorf("non-same-origin handle %q was followed (rendered %q)", bad, gone)
				}
			}
		}
	})

	t.Run("saving with a return handle renders the return loader instead of closing the modal", func(t *testing.T) {
		h, _, sched, ctx := newHandler(t)
		item, err := sched.Create(ctx, schedule.Item{
			Name:       "Rent",
			Direction:  schedule.DirectionOut,
			Amount:     scheduleAmount(2000),
			Cadence:    schedule.CadenceMonthly,
			DayOfMonth: 1,
			Active:     true,
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		rec := httptest.NewRecorder()
		h.PutScheduleItem(rec, postForm(t, "/sweep/schedule/"+item.ID, item.ID, url.Values{
			"name":         {"Rent (updated)"},
			"direction":    {"out"},
			"amount":       {"2100"},
			"cadence":      {"monthly"},
			"day_of_month": {"1"},
			"active":       {"1"},
			"return_to":    {"/transactions/txn-9/edit"},
		}))

		if got := rec.Header().Get("HX-Retarget"); got != "" {
			t.Errorf("handle save unexpectedly retargeted the schedule region, HX-Retarget=%q", got)
		}
		body := rec.Body.String()
		for _, want := range []string{
			`data-testid="schedule-item-editor-return-loader"`,
			`hx-get="/transactions/txn-9/edit"`,
			`hx-trigger="load"`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("handle save did not render the return loader, missing %q:\n%s", want, body)
			}
		}

		saved, err := sched.List(ctx)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(saved) != 1 || saved[0].Name != "Rent (updated)" {
			t.Errorf("the save itself did not go through: %+v", saved)
		}
	})

	t.Run("deleting with a return handle renders the return loader instead of closing the modal", func(t *testing.T) {
		h, _, sched, ctx := newHandler(t)
		item, err := sched.Create(ctx, schedule.Item{
			Name:       "Rent",
			Direction:  schedule.DirectionOut,
			Amount:     scheduleAmount(2000),
			Cadence:    schedule.CadenceMonthly,
			DayOfMonth: 1,
			Active:     true,
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		rec := httptest.NewRecorder()
		h.DeleteScheduleItem(rec, postForm(t, "/sweep/schedule/"+item.ID+"/delete", item.ID, url.Values{
			"return_to": {"/transactions/txn-7/edit"},
		}))

		if got := rec.Header().Get("HX-Retarget"); got != "" {
			t.Errorf("handle delete unexpectedly retargeted the schedule region, HX-Retarget=%q", got)
		}
		body := rec.Body.String()
		for _, want := range []string{
			`data-testid="schedule-item-editor-return-loader"`,
			`hx-get="/transactions/txn-7/edit"`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("handle delete did not render the return loader, missing %q:\n%s", want, body)
			}
		}

		remaining, err := sched.List(ctx)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(remaining) != 0 {
			t.Errorf("delete left %d items", len(remaining))
		}
	})
}

func TestScheduleItemEditorModalRendersConfirmForAnAutoMatch(t *testing.T) {
	h, _, sched, ctx := newHandler(t)

	due := time.Now().UTC().AddDate(0, 0, -3)
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
	if err := sched.RecordAutoMatches(ctx, []schedule.Match{
		{ItemID: item.ID, Occurrence: due, TransactionID: "txn-auto-1"},
	}); err != nil {
		t.Fatalf("RecordAutoMatches: %v", err)
	}

	rec := httptest.NewRecorder()
	h.GetEditScheduleItemModal(rec, getReq(t, "/sweep/schedule/"+item.ID+"/edit", item.ID))

	body := rec.Body.String()
	for what, want := range map[string]string{
		"the settled occurrence":   `data-testid="schedule-occurrence-settled"`,
		"the confirm control":      `data-testid="schedule-occurrence-confirm"`,
		"the not-this-one control": `data-testid="schedule-occurrence-clear"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("modal missing %s (%q):\n%s", what, want, body)
		}
	}
}

func TestScheduleItemEditorOccurrenceControlsStayInModal(t *testing.T) {
	h, _, sched, ctx := newHandler(t)

	due := time.Now().UTC().AddDate(0, 0, -3)
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

	req := httptest.NewRequest(http.MethodPost, "/sweep/schedule/"+item.ID+"/occurrence/"+due.Format("2006-01-02")+"/clear", strings.NewReader(""))
	req.SetPathValue("id", item.ID)
	req.SetPathValue("date", due.Format("2006-01-02"))
	rec := httptest.NewRecorder()
	h.PostOccurrenceClear(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body:\n%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `data-testid="schedule-item-editor"`) {
		t.Errorf("an occurrence decision did not re-render the editor region, stayed in modal:\n%s", body)
	}
	if strings.Contains(body, `data-testid="sweep-schedule"`) {
		t.Errorf("an occurrence decision unexpectedly rendered the outer schedule region")
	}
	if !strings.Contains(body, `data-testid="schedule-occurrence-cleared"`) {
		t.Errorf("the cleared decision was not reflected in the re-rendered history:\n%s", body)
	}
}
