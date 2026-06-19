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

	"github.com/alecdray/two-cents/src/internal/budget"
	"github.com/alecdray/two-cents/src/internal/budget/adapters"
	"github.com/alecdray/two-cents/src/internal/categorization"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/db"

	"github.com/pressly/goose/v3"

	_ "github.com/mattn/go-sqlite3"
)

// These tests drive the budget adapter end-to-end over the real budget +
// categorization Services and a migrated temp SQLite DB, asserting the page
// renders its income/savings fields, a limit row per active Category, the
// residual, and the balance banner; that a save persists and re-renders the
// saved values; and that bad numeric input surfaces inline without saving.

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

// newHandler builds the adapter over real budget + categorization Services. The
// categorization re-categorize seam is unused here, so it reports nothing.
func newHandler(t *testing.T) *adapters.HttpHandler {
	t.Helper()
	d := newTestDB(t)
	catSvc := categorization.NewService(d, func(_ contextx.ContextX, _ []string) (int, error) {
		return 0, nil
	})
	budgetSvc := budget.NewService(d, catSvc)
	return adapters.NewHttpHandler(budgetSvc, catSvc)
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

func TestBudgetPageRendersFieldsLimitsResidualBanner(t *testing.T) {
	h := newHandler(t)
	rec := httptest.NewRecorder()
	h.GetBudgetPage(rec, ctxReq("GET", "/budget", nil))

	body := rec.Body.String()
	for _, want := range []string{
		`data-testid="budget-page"`,
		`data-testid="budget-income"`,
		`data-testid="budget-savings"`,
		`data-testid="budget-limit-row"`,
		`data-testid="budget-residual"`,
		`data-testid="budget-balance-banner"`,
		`data-testid="budget-save"`,
		// With no budget set every Category is unbudgeted, so no limit row is
		// shown and every active Category is offered in the add-category control;
		// the active list is carried to the client in the form's data-seed.
		`data-testid="budget-add-category"`,
		"food_and_drink",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("budget page missing %q", want)
		}
	}
}

func TestSaveBudgetPersistsAndReflectsValues(t *testing.T) {
	h := newHandler(t)

	rec := httptest.NewRecorder()
	h.PostBudget(rec, ctxReq("POST", "/budget", url.Values{
		"income":               {"5000"},
		"savings":              {"1000"},
		"limit_food_and_drink": {"600"},
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("save should render the swapped fragment (200), got %d", rec.Code)
	}
	body := rec.Body.String()
	// Income and savings render their saved values directly; the food limit
	// (600.00) comes back in the form's client seed, which drives the live
	// residual (5000 - 600 - 1000 = 3400) the client computes from these values.
	for _, want := range []string{`value="5000.00"`, `value="1000.00"`, "600.00"} {
		if !strings.Contains(body, want) {
			t.Errorf("saved budget fragment missing %q:\n%s", want, body)
		}
	}

	// A fresh GET reads the persisted budget back.
	rec = httptest.NewRecorder()
	h.GetBudgetPage(rec, ctxReq("GET", "/budget", nil))
	reloaded := rec.Body.String()
	for _, want := range []string{`value="5000.00"`, `value="1000.00"`, "600.00"} {
		if !strings.Contains(reloaded, want) {
			t.Errorf("reloaded budget page did not persist %q", want)
		}
	}
}

func TestSaveOverAllocatedBudgetShowsBannerAndSaves(t *testing.T) {
	h := newHandler(t)
	rec := httptest.NewRecorder()
	h.PostBudget(rec, ctxReq("POST", "/budget", url.Values{
		"income":               {"1000"},
		"savings":              {"800"},
		"limit_food_and_drink": {"500"},
	}))

	body := rec.Body.String()
	// The banner is rendered; its balanced/over-allocated verdict is computed
	// live on the client from the saved figures (the arithmetic is covered by the
	// budget package's BalanceCheck tests).
	if !strings.Contains(body, `data-testid="budget-balance-banner"`) {
		t.Errorf("over-allocated save did not render the balance banner:\n%s", body)
	}
	// Over-allocated still saves: the values come back on the swap.
	if !strings.Contains(body, `value="1000.00"`) {
		t.Errorf("over-allocated budget was not persisted")
	}
}

func TestSaveBudgetBadAmountShowsInlineError(t *testing.T) {
	h := newHandler(t)
	rec := httptest.NewRecorder()
	h.PostBudget(rec, ctxReq("POST", "/budget", url.Values{
		"income":  {"not-a-number"},
		"savings": {"0"},
	}))

	if rec.Code != http.StatusOK {
		t.Fatalf("bad input should render inline (200), got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `data-testid="budget-error"`) {
		t.Errorf("bad amount did not render the inline error fragment")
	}
}
