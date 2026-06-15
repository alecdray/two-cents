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

// newHandler builds the adapter over a real Service whose re-categorization seam
// reports recategorized transactions, so the rule feedback is observable.
func newHandler(t *testing.T, recategorized int) *adapters.HttpHandler {
	t.Helper()
	svc := categorization.NewService(newTestDB(t), func(_ contextx.ContextX, _ []string) (int, error) {
		return recategorized, nil
	})
	return adapters.NewHttpHandler(svc)
}

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

func TestRulesPageRenders(t *testing.T) {
	h := newHandler(t, 0)
	rec := httptest.NewRecorder()
	h.GetRulesPage(rec, ctxReq("GET", "/rules", nil))

	body := rec.Body.String()
	for _, want := range []string{`data-testid="rules-page"`, `data-testid="rule-create"`, `data-testid="rules-empty"`} {
		if !strings.Contains(body, want) {
			t.Errorf("rules page missing %q", want)
		}
	}
}

func TestCreateRuleShowsRecategorizedCount(t *testing.T) {
	h := newHandler(t, 4)
	rec := httptest.NewRecorder()
	h.PostRule(rec, ctxReq("POST", "/rules", url.Values{
		"merchant_substring": {"STARBUCKS"},
		"classification":     {"spending"},
		"category_id":        {categorization.CategoryFoodAndDrink},
	}))

	body := rec.Body.String()
	if !strings.Contains(body, `data-testid="rules-feedback"`) || !strings.Contains(body, "4 transactions re-categorized.") {
		t.Errorf("rule create did not surface the re-categorized count:\n%s", body)
	}
	if !strings.Contains(body, `data-testid="rule-row"`) {
		t.Errorf("created rule not listed in the swapped fragment")
	}
}

func TestCreateSpendingRuleWithoutCategoryShowsInlineError(t *testing.T) {
	h := newHandler(t, 0)
	rec := httptest.NewRecorder()
	h.PostRule(rec, ctxReq("POST", "/rules", url.Values{
		"merchant_substring": {"WALMART"},
		"classification":     {"spending"},
		"category_id":        {""},
	}))

	if !strings.Contains(rec.Body.String(), `data-testid="rule-create-error"`) {
		t.Errorf("spending rule without a category did not render the inline error")
	}
}
