package adapters_test

import (
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alecdray/two-cents/src/internal/core/app"
	"github.com/alecdray/two-cents/src/internal/core/httpx"
	"github.com/alecdray/two-cents/src/internal/schedule"
	"github.com/alecdray/two-cents/src/internal/sweep/adapters"
)

// TestCreateScheduleItemFromTransactionThroughRoutes opens the create-from-
// transaction modal and submits its own rendered save form exactly as the
// browser would — to the URL the form declares, with only the fields a browser
// serializes (disabled controls omitted) — all dispatched through the real
// registered routes behind the session middleware, mirroring the composition
// root. Calling PostScheduleItem directly would bypass the dispatch that once
// sent this form to the per-item save at /sweep/schedule/{id} with id "new".
func TestCreateScheduleItemFromTransactionThroughRoutes(t *testing.T) {
	// A negative amount is an inflow: the item's direction must come from this
	// transaction, not from the disabled direction control a browser never submits.
	txnDate := time.Now().UTC().Truncate(24 * time.Hour)
	ledger := fakeLedger{candidates: []schedule.Candidate{
		{TransactionID: "txn-pay", Date: txnDate, Amount: -1500, Merchant: "Employer Inc"},
	}}
	h, _, sched, ctx := newHandlerWithLedger(t, ledger)

	application := app.NewApp(app.Config{Env: app.EnvLocal, JwtSecret: "test-secret"})
	appMux := httpx.NewMux(application, httpx.JwtMiddleware)
	adapters.RegisterRoutes(appMux, h)

	issueRec := httptest.NewRecorder()
	if err := application.IssueSession(issueRec, "user-1"); err != nil {
		t.Fatalf("IssueSession: %v", err)
	}
	serve := func(req *http.Request) *httptest.ResponseRecorder {
		for _, c := range issueRec.Result().Cookies() {
			req.AddCookie(c)
		}
		req.Header.Set("HX-Request", "true")
		rec := httptest.NewRecorder()
		appMux.ServeHTTP(rec, req)
		return rec
	}

	// Open the modal the way the transaction editor's create link does.
	openURL := "/sweep/schedule/new?" + url.Values{
		"transaction_id": {"txn-pay"},
		"merchant":       {"Employer Inc"},
		"direction":      {"in"},
		"day":            {strconv.Itoa(txnDate.Day())},
		"return_to":      {"/transactions/txn-pay/edit"},
	}.Encode()
	openRec := serve(httptest.NewRequest(http.MethodGet, openURL, nil))
	if openRec.Code != http.StatusOK {
		t.Fatalf("open: status = %d, want 200, body:\n%s", openRec.Code, openRec.Body.String())
	}

	action, fields := renderedEditorForm(t, openRec.Body.String())
	if _, ok := fields["direction"]; ok {
		t.Fatalf("the locked direction control was serialized; a browser omits disabled controls: %v", fields)
	}

	saveReq := httptest.NewRequest(http.MethodPost, action, strings.NewReader(fields.Encode()))
	saveReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	saveRec := serve(saveReq)
	if saveRec.Code != http.StatusOK {
		t.Fatalf("save to %s: status = %d, want 200, body:\n%s", action, saveRec.Code, saveRec.Body.String())
	}
	if body := saveRec.Body.String(); !strings.Contains(body, `hx-get="/transactions/txn-pay/edit"`) {
		t.Fatalf("save to %s did not create and return to the transaction:\n%s", action, body)
	}

	items, err := sched.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("%d items stored, want 1", len(items))
	}
	item := items[0]
	if item.Name != "Employer Inc" {
		t.Errorf("Name = %q, want the prefilled merchant", item.Name)
	}
	if item.Direction != schedule.DirectionIn {
		t.Errorf("Direction = %q, want in — read from the seeding transaction", item.Direction)
	}
	if item.Cadence != schedule.CadenceMonthly || item.DayOfMonth != txnDate.Day() {
		t.Errorf("cadence = %q day %d, want monthly on day %d (the transaction's date)", item.Cadence, item.DayOfMonth, txnDate.Day())
	}

	matched, ok, err := sched.MatchedItem(ctx, "txn-pay")
	if err != nil {
		t.Fatalf("MatchedItem: %v", err)
	}
	if !ok || matched.ID != item.ID {
		t.Fatalf("MatchedItem(txn-pay) = (%+v, %v), want the created item matched to its seeding transaction", matched, ok)
	}
	matches, err := sched.Matches(ctx, txnDate.AddDate(0, -1, 0), txnDate.AddDate(0, 1, 0))
	if err != nil {
		t.Fatalf("Matches: %v", err)
	}
	if len(matches) != 1 || !matches[0].Occurrence.Equal(txnDate) || matches[0].TransactionID != "txn-pay" {
		t.Errorf("matches = %+v, want one match of txn-pay on the occurrence dated %s", matches, txnDate.Format("2006-01-02"))
	}
}

var (
	editorFormRe = regexp.MustCompile(`(?s)<form\b([^>]*data-testid="schedule-item-editor-form"[^>]*)>(.*?)</form>`)
	inputTagRe   = regexp.MustCompile(`<input\b[^>]*>`)
	selectRe     = regexp.MustCompile(`(?s)<select\b([^>]*)>(.*?)</select>`)
	optionRe     = regexp.MustCompile(`<option\b[^>]*>`)
	attrRe       = regexp.MustCompile(`([a-zA-Z_:@][\w:.@-]*)(?:="([^"]*)")?`)
)

// renderedEditorForm reads the editor form out of a rendered body the way a
// browser submits it: its hx-post target, and every named control that is not
// disabled (a checkbox only when checked, a select by its selected option).
func renderedEditorForm(t *testing.T, body string) (string, url.Values) {
	t.Helper()
	m := editorFormRe.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no schedule item editor form rendered:\n%s", body)
	}
	action := tagAttrs(m[1])["hx-post"]
	if action == "" {
		t.Fatalf("editor form declares no hx-post target: %s", m[1])
	}

	fields := url.Values{}
	for _, tag := range inputTagRe.FindAllString(m[2], -1) {
		a := tagAttrs(tag)
		if _, disabled := a["disabled"]; disabled || a["name"] == "" {
			continue
		}
		if a["type"] == "checkbox" || a["type"] == "radio" {
			if _, checked := a["checked"]; !checked {
				continue
			}
		}
		fields.Add(a["name"], a["value"])
	}
	for _, sel := range selectRe.FindAllStringSubmatch(m[2], -1) {
		a := tagAttrs(sel[1])
		if _, disabled := a["disabled"]; disabled || a["name"] == "" {
			continue
		}
		options := optionRe.FindAllString(sel[2], -1)
		if len(options) == 0 {
			continue
		}
		chosen := tagAttrs(options[0])["value"]
		for _, opt := range options {
			if oa := tagAttrs(opt); hasAttr(oa, "selected") {
				chosen = oa["value"]
				break
			}
		}
		fields.Add(a["name"], chosen)
	}
	return html.UnescapeString(action), fields
}

func tagAttrs(tag string) map[string]string {
	out := map[string]string{}
	for _, m := range attrRe.FindAllStringSubmatch(tag, -1) {
		out[m[1]] = html.UnescapeString(m[2])
	}
	return out
}

func hasAttr(attrs map[string]string, name string) bool {
	_, ok := attrs[name]
	return ok
}
