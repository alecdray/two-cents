package adapters_test

import (
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/alecdray/two-cents/src/internal/schedule"
)

// These tests round-trip the per-item editor the way a browser does: open the
// modal, read back exactly what its form would submit, change one unrelated
// field, and post that through the save route. The amount input is posted back
// on every save, so what it is prefilled with decides whether a computed amount
// survives an edit or gets frozen into a declared one ([ADR-0031]).

var (
	tagPattern    = regexp.MustCompile(`(?s)<(input|select|option|/select)\b([^>]*)>`)
	attrPattern   = regexp.MustCompile(`([a-zA-Z_:][-a-zA-Z0-9_:.]*)(?:="([^"]*)")?`)
	editorFormEnd = "</form>"
)

// submittedEditorForm returns the fields the rendered editor form would post:
// every enabled named input (an unchecked checkbox contributes nothing) and
// each enabled select's selected option.
func submittedEditorForm(t *testing.T, body string) url.Values {
	t.Helper()
	start := strings.Index(body, `data-testid="schedule-item-editor-form"`)
	if start < 0 {
		t.Fatalf("rendered editor has no form:\n%s", body)
	}
	end := strings.Index(body[start:], editorFormEnd)
	if end < 0 {
		t.Fatalf("rendered editor form never closes:\n%s", body)
	}
	form := body[start : start+end]

	out := url.Values{}
	var selectName string
	for _, m := range tagPattern.FindAllStringSubmatch(form, -1) {
		attrs := map[string]string{}
		present := map[string]bool{}
		for _, a := range attrPattern.FindAllStringSubmatch(m[2], -1) {
			attrs[a[1]] = html.UnescapeString(a[2])
			present[a[1]] = true
		}
		switch m[1] {
		case "input":
			if present["disabled"] || attrs["name"] == "" {
				continue
			}
			if attrs["type"] == "checkbox" && !present["checked"] {
				continue
			}
			out.Add(attrs["name"], attrs["value"])
		case "select":
			selectName = ""
			if !present["disabled"] {
				selectName = attrs["name"]
			}
		case "option":
			if selectName != "" && present["selected"] {
				out.Set(selectName, attrs["value"])
			}
		case "/select":
			selectName = ""
		}
	}
	return out
}

// renameThroughEditor opens the item's modal, renames it in the form it
// rendered, and saves that form as-is otherwise.
func renameThroughEditor(t *testing.T, h interface {
	GetEditScheduleItemModal(http.ResponseWriter, *http.Request)
	PutScheduleItem(http.ResponseWriter, *http.Request)
}, itemID, newName string) (modal string, form url.Values) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.GetEditScheduleItemModal(rec, getReq(t, "/sweep/schedule/"+itemID+"/edit", itemID))
	if rec.Code != http.StatusOK {
		t.Fatalf("open: status = %d, body:\n%s", rec.Code, rec.Body.String())
	}
	modal = rec.Body.String()

	form = submittedEditorForm(t, modal)
	form.Set("name", newName)

	rec = httptest.NewRecorder()
	h.PutScheduleItem(rec, postForm(t, "/sweep/schedule/"+itemID, itemID, form))
	if rec.Code != http.StatusOK {
		t.Fatalf("save: status = %d, body:\n%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `data-testid="schedule-item-editor-error"`) {
		t.Fatalf("save was rejected:\n%s", rec.Body.String())
	}
	return modal, form
}

func TestScheduleItemEditorSaveKeepsAmountAsStored(t *testing.T) {
	today := time.Now().UTC()
	due := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	ledger := fakeLedger{candidates: []schedule.Candidate{
		{TransactionID: "txn-power", Date: due, Amount: 85.25, Merchant: "CITY POWER"},
	}}

	t.Run("renaming an item that computes its amount leaves it computing", func(t *testing.T) {
		h, _, sched, ctx := newHandlerWithLedger(t, ledger)
		item, err := sched.Create(ctx, schedule.Item{
			Name:       "Power",
			Direction:  schedule.DirectionOut,
			Cadence:    schedule.CadenceMonthly,
			DayOfMonth: due.Day(),
			Active:     true,
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := sched.MatchOccurrence(ctx, item.ID, due, "txn-power"); err != nil {
			t.Fatalf("MatchOccurrence: %v", err)
		}

		modal, form := renameThroughEditor(t, h, item.ID, "Electric")

		if got := form.Get("amount"); got != "" {
			t.Errorf("the form submitted amount=%q, want blank — the item declares no amount", got)
		}
		if !strings.Contains(modal, `data-testid="schedule-item-editor-computed-amount"`) || !strings.Contains(modal, "$85.25") {
			t.Errorf("the modal does not show the figure the item currently computes:\n%s", modal)
		}

		states, err := sched.AmountStates(ctx)
		if err != nil {
			t.Fatalf("AmountStates: %v", err)
		}
		if st := states[item.ID]; st.Override || !st.Computed {
			t.Errorf("after a rename, AmountState = %+v, want {Override: false, Computed: true} — still a forecast, not frozen", st)
		}
		items, err := sched.List(ctx)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(items) != 1 || items[0].Name != "Electric" {
			t.Fatalf("rename did not save: %+v", items)
		}
		if items[0].Amount == nil || *items[0].Amount != 85.25 {
			t.Errorf("resolved Amount = %v, want 85.25 computed from the match", items[0].Amount)
		}
	})

	t.Run("renaming an item with a declared amount keeps that amount declared", func(t *testing.T) {
		h, _, sched, ctx := newHandlerWithLedger(t, ledger)
		item, err := sched.Create(ctx, schedule.Item{
			Name:       "Power",
			Direction:  schedule.DirectionOut,
			Amount:     scheduleAmount(120),
			Cadence:    schedule.CadenceMonthly,
			DayOfMonth: due.Day(),
			Active:     true,
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := sched.MatchOccurrence(ctx, item.ID, due, "txn-power"); err != nil {
			t.Fatalf("MatchOccurrence: %v", err)
		}

		modal, form := renameThroughEditor(t, h, item.ID, "Electric")

		if got := form.Get("amount"); got != "120" {
			t.Errorf("the form submitted amount=%q, want 120 — the declared amount", got)
		}
		if strings.Contains(modal, `data-testid="schedule-item-editor-computed-amount"`) {
			t.Errorf("the modal showed a forecast for an item whose amount is declared:\n%s", modal)
		}

		states, err := sched.AmountStates(ctx)
		if err != nil {
			t.Fatalf("AmountStates: %v", err)
		}
		if st := states[item.ID]; !st.Override {
			t.Errorf("after a rename, AmountState = %+v, want the declared amount still an override", st)
		}
		items, err := sched.List(ctx)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(items) != 1 || items[0].Amount == nil || *items[0].Amount != 120 {
			t.Errorf("declared amount did not survive the save: %+v", items)
		}
	})
}
