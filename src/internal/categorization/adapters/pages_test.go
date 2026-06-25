package adapters_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecdray/two-cents/src/internal/categorization"
	"github.com/alecdray/two-cents/src/internal/categorization/adapters"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/db"

	"github.com/pressly/goose/v3"

	_ "github.com/mattn/go-sqlite3"
)

// These tests drive the categorization adapter end-to-end over the real Service
// and a migrated temp SQLite DB, asserting the pages render their testids and
// that the htmx mutation fragments reflect the new state and surface inline
// errors / re-categorized counts. The re-categorization seam is a stub returning
// a fixed count so the rule-mutation feedback is observable.

func newTestDB(t *testing.T) *db.DB {
	t.Helper()
	migrationsDir, err := filepath.Abs("../../../../db/migrations")
	if err != nil {
		t.Fatalf("resolve migrations dir: %v", err)
	}
	dbPath := filepath.Join(t.TempDir(), "test.db")
	sqlDB, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("set dialect: %v", err)
	}
	if err := goose.Up(sqlDB, migrationsDir); err != nil {
		t.Fatalf("goose up: %v", err)
	}
	return db.WrapSqlDB(sqlDB)
}

// newServiceHandler builds the adapter over a real Service whose re-categorization
// seam reports a fixed count (so the rule feedback is observable) and returns the
// Service too, so a test can seed Rules and read them back through the public API.
func newServiceHandler(t *testing.T, recategorized int) (*categorization.Service, *adapters.HttpHandler) {
	t.Helper()
	svc := categorization.NewService(newTestDB(t), func(_ contextx.ContextX, _ []string) (int, error) {
		return recategorized, nil
	})
	return svc, adapters.NewHttpHandler(svc)
}

// newHandler builds just the adapter for tests that don't need to seed through the
// Service.
func newHandler(t *testing.T, recategorized int) *adapters.HttpHandler {
	t.Helper()
	_, h := newServiceHandler(t, recategorized)
	return h
}

// bgCtx is a background ContextX for driving the Service directly in a test.
func bgCtx() contextx.ContextX { return contextx.NewContextX(context.Background()) }

// strptr returns a pointer to s, for the optional Category id a spending Rule carries.
func strptr(s string) *string { return &s }

func ctxReq(method, target string, form url.Values) *http.Request {
	var body *strings.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	} else {
		body = strings.NewReader("")
	}
	r := httptest.NewRequest(method, target, body)
	r = r.WithContext(contextx.NewContextX(context.Background()))
	if form != nil {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	return r
}

func TestCategoriesPageRendersBuiltIns(t *testing.T) {
	h := newHandler(t, 0)
	rec := httptest.NewRecorder()
	h.GetCategoriesPage(rec, ctxReq("GET", "/categories", nil))

	body := rec.Body.String()
	for _, want := range []string{`data-testid="categories-page"`, `data-testid="category-create"`, "Food &amp; Drink", "General Merchandise"} {
		if !strings.Contains(body, want) {
			t.Errorf("categories page missing %q", want)
		}
	}
}

func TestCreateCategoryReflectedInFragment(t *testing.T) {
	h := newHandler(t, 0)
	rec := httptest.NewRecorder()
	h.PostCategory(rec, ctxReq("POST", "/categories", url.Values{"name": {"Coffee Runs"}}))

	if !strings.Contains(rec.Body.String(), "Coffee Runs") {
		t.Errorf("created category not in the swapped fragment:\n%s", rec.Body.String())
	}
}

func TestCreateCategoryBlankShowsInlineError(t *testing.T) {
	h := newHandler(t, 0)
	rec := httptest.NewRecorder()
	h.PostCategory(rec, ctxReq("POST", "/categories", url.Values{"name": {"   "}}))

	if rec.Code != http.StatusOK {
		t.Fatalf("blank create should render inline (200), got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `data-testid="category-create-error"`) {
		t.Errorf("blank create did not render the inline error fragment")
	}
}

func TestRulesPageRendersReadOnlyRowsAndOpeners(t *testing.T) {
	svc, h := newServiceHandler(t, 0)
	if _, _, err := svc.CreateRule(bgCtx(), "STARBUCKS", categorization.Spending, strptr(categorization.CategoryFoodAndDrink)); err != nil {
		t.Fatalf("seed rule: %v", err)
	}

	rec := httptest.NewRecorder()
	h.GetRulesPage(rec, ctxReq("GET", "/rules", nil))

	body := rec.Body.String()
	for _, want := range []string{
		`data-testid="rules-page"`,
		`data-testid="rule-new"`, // the New rule opener
		`hx-get="/rules/new"`,    // it opens the create modal
		`data-testid="rules-list"`,
		`data-testid="rule-row"`,
		`data-testid="rule-row-substring"`, // read-only substring display
		"STARBUCKS",
		`data-testid="rule-edit"`, // the per-row Edit opener
		`data-testid="rule-delete"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("rules page missing %q", want)
		}
	}
	// The in-page create / inline-edit forms are gone — only the modal openers remain.
	for _, gone := range []string{`data-testid="rule-create"`, `data-testid="rule-edit-submit"`, `data-testid="rule-create-substring"`} {
		if strings.Contains(body, gone) {
			t.Errorf("rules page still renders the removed in-page form element %q", gone)
		}
	}
}

func TestNewRuleModalRendersEmptyEditor(t *testing.T) {
	h := newHandler(t, 0)
	rec := httptest.NewRecorder()
	h.GetNewRuleModal(rec, ctxReq("GET", "/rules/new", nil))

	body := rec.Body.String()
	for _, want := range []string{
		`data-testid="modal"`,
		`data-testid="rule-editor"`,
		`data-testid="rule-editor-form"`,
		`hx-post="/rules"`, // create target
		`data-testid="rule-editor-substring"`,
		`data-testid="rule-editor-classification"`,
		`data-testid="rule-editor-category"`,
		`data-testid="rule-editor-submit"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("new-rule modal missing %q:\n%s", want, body)
		}
	}
	// No handle ⇒ no return field and no dismiss listener.
	for _, gone := range []string{`data-testid="rule-editor-return-to"`, `close from:closest dialog`} {
		if strings.Contains(body, gone) {
			t.Errorf("new-rule modal with no handle still rendered %q", gone)
		}
	}
}

func TestEditRuleModalRendersPrefilled(t *testing.T) {
	svc, h := newServiceHandler(t, 0)
	rule, _, err := svc.CreateRule(bgCtx(), "WHOLE FOODS", categorization.Spending, strptr(categorization.CategoryFoodAndDrink))
	if err != nil {
		t.Fatalf("seed rule: %v", err)
	}

	req := ctxReq("GET", "/rules/"+rule.ID+"/edit", nil)
	req.SetPathValue("id", rule.ID)
	rec := httptest.NewRecorder()
	h.GetEditRuleModal(rec, req)

	body := rec.Body.String()
	for _, want := range []string{
		`data-testid="rule-editor"`,
		`hx-post="/rules/` + rule.ID + `/edit"`,                        // edit target
		`value="WHOLE FOODS"`,                                          // prefilled substring
		`value="` + categorization.CategoryFoodAndDrink + `" selected`, // prefilled category
	} {
		if !strings.Contains(body, want) {
			t.Errorf("edit-rule modal missing %q:\n%s", want, body)
		}
	}
}

func TestCreateRuleBlankSubstringKeepsModalOpen(t *testing.T) {
	svc, h := newServiceHandler(t, 0)
	rec := httptest.NewRecorder()
	h.PostRule(rec, ctxReq("POST", "/rules", url.Values{
		"merchant_substring": {"   "},
		"classification":     {"spending"},
		"category_id":        {categorization.CategoryFoodAndDrink},
	}))

	body := rec.Body.String()
	if !strings.Contains(body, `data-testid="rule-editor"`) || !strings.Contains(body, `data-testid="rule-editor-error"`) {
		t.Errorf("blank substring did not re-render the editor region with an inline error:\n%s", body)
	}
	rules, err := svc.ListRules(bgCtx())
	if err != nil {
		t.Fatalf("list rules: %v", err)
	}
	if len(rules) != 0 {
		t.Errorf("a validation failure created a rule (%d exist)", len(rules))
	}
}

func TestCreateSpendingRuleWithoutCategoryKeepsModalOpen(t *testing.T) {
	svc, h := newServiceHandler(t, 0)
	rec := httptest.NewRecorder()
	h.PostRule(rec, ctxReq("POST", "/rules", url.Values{
		"merchant_substring": {"WALMART"},
		"classification":     {"spending"},
		"category_id":        {""},
	}))

	body := rec.Body.String()
	if !strings.Contains(body, `data-testid="rule-editor"`) || !strings.Contains(body, `data-testid="rule-editor-error"`) {
		t.Errorf("spending rule without a category did not re-render the editor with an inline error:\n%s", body)
	}
	rules, err := svc.ListRules(bgCtx())
	if err != nil {
		t.Fatalf("list rules: %v", err)
	}
	if len(rules) != 0 {
		t.Errorf("a validation failure created a rule (%d exist)", len(rules))
	}
}

func TestEditRuleBlankSubstringKeepsModalOpenAndUnchanged(t *testing.T) {
	svc, h := newServiceHandler(t, 0)
	rule, _, err := svc.CreateRule(bgCtx(), "TARGET", categorization.Spending, strptr(categorization.CategoryFoodAndDrink))
	if err != nil {
		t.Fatalf("seed rule: %v", err)
	}

	req := ctxReq("POST", "/rules/"+rule.ID+"/edit", url.Values{
		"merchant_substring": {""},
		"classification":     {"spending"},
		"category_id":        {categorization.CategoryFoodAndDrink},
	})
	req.SetPathValue("id", rule.ID)
	rec := httptest.NewRecorder()
	h.PostEditRule(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, `data-testid="rule-editor"`) || !strings.Contains(body, `data-testid="rule-editor-error"`) {
		t.Errorf("blank edit did not re-render the editor with an inline error:\n%s", body)
	}
	// The form still targets the edit endpoint so a corrected resubmit hits the same Rule.
	if !strings.Contains(body, `hx-post="/rules/`+rule.ID+`/edit"`) {
		t.Errorf("edit validation re-render lost the edit target")
	}
	got, err := svc.Rule(bgCtx(), rule.ID)
	if err != nil {
		t.Fatalf("read rule: %v", err)
	}
	if got.MerchantSubstring != "TARGET" {
		t.Errorf("a validation failure mutated the rule (substring now %q)", got.MerchantSubstring)
	}
}

func TestReturnHandleHonoredWhenSameOriginRelative(t *testing.T) {
	h := newHandler(t, 0)
	rec := httptest.NewRecorder()
	h.GetNewRuleModal(rec, ctxReq("GET", "/rules/new?return_to=/transactions/txn-1/edit", nil))

	body := rec.Body.String()
	for _, want := range []string{
		`data-testid="rule-editor-return-to"`,
		`value="/transactions/txn-1/edit"`,
		`close from:closest dialog`, // the dismiss-to-origin listener
	} {
		if !strings.Contains(body, want) {
			t.Errorf("same-origin return handle not honored, missing %q:\n%s", want, body)
		}
	}
}

func TestReturnHandleRejectedWhenNonSameOrigin(t *testing.T) {
	// Three rejection classes: a scheme+host, a protocol-relative host, and a
	// handle that does not start with a slash at all (a bare host or a relative
	// path) — none is a same-origin relative path, so none may be followed.
	for _, bad := range []string{"https://evil.com/x", "//evil.com", "evil.com/x"} {
		h := newHandler(t, 0)
		rec := httptest.NewRecorder()
		h.GetNewRuleModal(rec, ctxReq("GET", "/rules/new?return_to="+url.QueryEscape(bad), nil))

		body := rec.Body.String()
		for _, gone := range []string{`data-testid="rule-editor-return-to"`, `close from:closest dialog`} {
			if strings.Contains(body, gone) {
				t.Errorf("non-same-origin handle %q was followed (rendered %q)", bad, gone)
			}
		}
	}
}

func TestCreateRuleNoHandleClosesModalAndRefreshesList(t *testing.T) {
	h := newHandler(t, 4)
	rec := httptest.NewRecorder()
	h.PostRule(rec, ctxReq("POST", "/rules", url.Values{
		"merchant_substring": {"STARBUCKS"},
		"classification":     {"spending"},
		"category_id":        {categorization.CategoryFoodAndDrink},
	}))

	if got := rec.Header().Get("HX-Retarget"); got != "#rules" {
		t.Errorf("no-handle save did not retarget the rules region, HX-Retarget=%q", got)
	}
	if got := rec.Header().Get("HX-Reswap"); got != "innerHTML" {
		t.Errorf("no-handle save did not reswap innerHTML, HX-Reswap=%q", got)
	}
	if !strings.Contains(rec.Header().Get("HX-Trigger"), "transaction-changed") {
		t.Errorf("no-handle save did not announce transaction-changed, HX-Trigger=%q", rec.Header().Get("HX-Trigger"))
	}

	body := rec.Body.String()
	for _, want := range []string{
		`data-testid="modal-container"`, // the OOB-empty container that closes the modal
		`hx-swap-oob="true"`,
		`data-testid="rules-feedback"`,
		"4 transactions re-categorized.",
		`data-testid="rule-row"`, // the new rule now listed
	} {
		if !strings.Contains(body, want) {
			t.Errorf("no-handle save response missing %q:\n%s", want, body)
		}
	}
}

func TestSaveWithReturnHandleRendersReturnLoader(t *testing.T) {
	svc, h := newServiceHandler(t, 2)
	rule, _, err := svc.CreateRule(bgCtx(), "COSTCO", categorization.Spending, strptr(categorization.CategoryFoodAndDrink))
	if err != nil {
		t.Fatalf("seed rule: %v", err)
	}

	req := ctxReq("POST", "/rules/"+rule.ID+"/edit", url.Values{
		"merchant_substring": {"COSTCO WHOLESALE"},
		"classification":     {"spending"},
		"category_id":        {categorization.CategoryFoodAndDrink},
		"return_to":          {"/transactions/txn-9/edit"},
	})
	req.SetPathValue("id", rule.ID)
	rec := httptest.NewRecorder()
	h.PostEditRule(rec, req)

	if !strings.Contains(rec.Header().Get("HX-Trigger"), "transaction-changed") {
		t.Errorf("handle save did not announce transaction-changed, HX-Trigger=%q", rec.Header().Get("HX-Trigger"))
	}
	if got := rec.Header().Get("HX-Retarget"); got != "" {
		t.Errorf("handle save unexpectedly retargeted the rules region, HX-Retarget=%q", got)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`data-testid="rule-editor-return-loader"`,
		`hx-get="/transactions/txn-9/edit"`,
		`hx-trigger="load"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("handle save did not render the return loader, missing %q:\n%s", want, body)
		}
	}
}

func TestDeleteRuleShowsCountAndAnnouncesChange(t *testing.T) {
	svc, h := newServiceHandler(t, 3)
	rule, _, err := svc.CreateRule(bgCtx(), "BLOCKBUSTER", categorization.Spending, strptr(categorization.CategoryFoodAndDrink))
	if err != nil {
		t.Fatalf("seed rule: %v", err)
	}

	req := ctxReq("POST", "/rules/"+rule.ID+"/delete", nil)
	req.SetPathValue("id", rule.ID)
	rec := httptest.NewRecorder()
	h.PostDeleteRule(rec, req)

	if !strings.Contains(rec.Header().Get("HX-Trigger"), "transaction-changed") {
		t.Errorf("delete did not announce transaction-changed, HX-Trigger=%q", rec.Header().Get("HX-Trigger"))
	}
	body := rec.Body.String()
	if !strings.Contains(body, `data-testid="rules-feedback"`) || !strings.Contains(body, "3 transactions re-categorized.") {
		t.Errorf("delete did not surface the re-categorized count:\n%s", body)
	}
	if !strings.Contains(body, `data-testid="rules-empty"`) {
		t.Errorf("deleting the only rule did not fall back to the empty state:\n%s", body)
	}
}
