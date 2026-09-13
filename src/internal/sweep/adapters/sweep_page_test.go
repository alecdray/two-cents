package adapters_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/db"
	"github.com/alecdray/two-cents/src/internal/schedule"
	"github.com/alecdray/two-cents/src/internal/sweep"
	"github.com/alecdray/two-cents/src/internal/sweep/adapters"

	"github.com/pressly/goose/v3"

	_ "github.com/mattn/go-sqlite3"
)

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

// newID hands each stored fixture its own snapshot id.
var idSeq int

func newID() string {
	idSeq++
	return fmt.Sprintf("snapshot-%d", idSeq)
}

// newHandler builds the page handler over real sweep and schedule Services on
// one migrated temp DB — the page composes both, so a stand-in for either would
// stop the test exercising what the page actually renders.
func newHandler(t *testing.T) (*adapters.HttpHandler, *sweep.Service, *schedule.Service, contextx.ContextX) {
	t.Helper()
	database := newTestDB(t)
	scheduleSvc := schedule.NewService(database)
	sweepSvc := sweep.NewService(nil, scheduleSvc, database, time.UTC, 500)
	return adapters.NewHttpHandler(sweepSvc, scheduleSvc),
		sweepSvc,
		scheduleSvc,
		contextx.NewContextX(context.Background())
}

// getSweepPage drives a GET /sweep through the handler and returns status + body.
func getSweepPage(t *testing.T, h *adapters.HttpHandler) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/sweep", nil)
	rec := httptest.NewRecorder()
	h.GetPage(rec, req)
	return rec.Code, rec.Body.String()
}

func mustContainAll(t *testing.T, body string, wants map[string]string) {
	t.Helper()
	for what, fragment := range wants {
		if !strings.Contains(body, fragment) {
			t.Errorf("page is missing %s (%q)", what, fragment)
		}
	}
}

// computedAt is a fixed instant so the rendered horizon label is deterministic.
var computedAt = time.Date(2026, time.September, 7, 14, 30, 0, 0, time.UTC)

// numericSnapshot is a saved-shaped recommendation whose timeline has an outflow,
// an inflow, and a marked peak — everything the derivation table must render.
func numericSnapshot() sweep.Recommendation {
	return sweep.Recommendation{
		ID:                newID(),
		Kind:              sweep.KindNumeric,
		ComputedAt:        computedAt,
		CurrentChecking:   3500,
		CurrentSavings:    1200.50,
		RequiredChecking:  2400,
		FixedSafetyMargin: 500,
		SuggestedSweep:    600,
		Direction:         sweep.DirectionCheckingToSavings,
		Timeline: []sweep.TimelineEvent{
			{
				Date:         computedAt.AddDate(0, 0, 13),
				Label:        "Rent",
				Direction:    sweep.EventOut,
				Amount:       2400,
				RunningTotal: 2400,
				Peak:         true,
			},
			{
				Date:         computedAt.AddDate(0, 0, 20),
				Label:        "Paycheck",
				Direction:    sweep.EventIn,
				Amount:       3100,
				RunningTotal: -700,
			},
		},
	}
}

func TestNumericSnapshotPage(t *testing.T) {
	t.Run("leads with the action line and the figures behind it", func(t *testing.T) {
		h, svc, _, ctx := newHandler(t)
		if err := svc.Save(ctx, numericSnapshot()); err != nil {
			t.Fatalf("Save: %v", err)
		}

		status, body := getSweepPage(t, h)
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200", status)
		}

		mustContainAll(t, body, map[string]string{
			"the numeric section": `data-testid="sweep-numeric"`,
			"the action wording":  "from checking to savings",
			"the headline amount": "$600",
			"current checking":    "$3,500.00",
			"current savings":     "$1,200.50",
			"required checking":   "$2,400.00",
			"the safety margin":   "$500.00",
		})
	})

	t.Run("shows the timeline that produced the figure", func(t *testing.T) {
		h, svc, _, ctx := newHandler(t)
		if err := svc.Save(ctx, numericSnapshot()); err != nil {
			t.Fatalf("Save: %v", err)
		}

		_, body := getSweepPage(t, h)

		mustContainAll(t, body, map[string]string{
			"the timeline region": `data-testid="sweep-timeline"`,
			"the outflow's label": "Rent",
			"the inflow's label":  "Paycheck",
			"the outflow's date":  "Sep 20",
			"the inflow's date":   "Sep 27",
			"the horizon":         "through October 7",
		})
	})

	t.Run("renders an inflow as a negative draw on checking", func(t *testing.T) {
		// The running-total column has to add up on the page exactly as it does in
		// the arithmetic, so the sign convention has to survive rendering.
		h, svc, _, ctx := newHandler(t)
		if err := svc.Save(ctx, numericSnapshot()); err != nil {
			t.Fatalf("Save: %v", err)
		}

		_, body := getSweepPage(t, h)

		mustContainAll(t, body, map[string]string{
			"the inflow's negative amount": "-$3,100.00",
			"the negative running total":   "-$700.00",
		})
	})

	t.Run("marks the event that set the figure", func(t *testing.T) {
		h, svc, _, ctx := newHandler(t)
		if err := svc.Save(ctx, numericSnapshot()); err != nil {
			t.Fatalf("Save: %v", err)
		}

		_, body := getSweepPage(t, h)

		if !strings.Contains(body, `data-testid="sweep-timeline-peak"`) {
			t.Error("the peak row is not marked, so nothing on the page says which moment set the number")
		}
		if strings.Count(body, `data-testid="sweep-timeline-peak"`) != 1 {
			t.Error("exactly one row sets the figure")
		}
	})

	t.Run("an unknown savings balance reads as unknown, not as zero", func(t *testing.T) {
		h, svc, _, ctx := newHandler(t)
		rec := numericSnapshot()
		rec.SavingsUnknown = true
		rec.CurrentSavings = 0
		if err := svc.Save(ctx, rec); err != nil {
			t.Fatalf("Save: %v", err)
		}

		_, body := getSweepPage(t, h)

		if !strings.Contains(body, "unknown") {
			t.Error("an unreported savings balance must not render as $0.00")
		}
	})

	t.Run("a timeline with nothing on it says so", func(t *testing.T) {
		h, svc, _, ctx := newHandler(t)
		rec := numericSnapshot()
		rec.Timeline = nil
		rec.RequiredChecking = 0
		if err := svc.Save(ctx, rec); err != nil {
			t.Fatalf("Save: %v", err)
		}

		_, body := getSweepPage(t, h)

		if !strings.Contains(body, `data-testid="sweep-timeline-empty"`) {
			t.Error("an empty timeline needs an explanation, not a blank table")
		}
	})
}

func TestNeedsAttentionPage(t *testing.T) {
	t.Run("lists every reason the run could not produce a number", func(t *testing.T) {
		h, svc, _, ctx := newHandler(t)
		rec := sweep.Recommendation{
			ID:         newID(),
			Kind:       sweep.KindNeedsAttention,
			ComputedAt: computedAt,
			Reasons: []sweep.NeedsAttentionReason{
				sweep.ReasonCheckingStale,
				sweep.ReasonCardBalanceUnknown,
			},
		}
		if err := svc.Save(ctx, rec); err != nil {
			t.Fatalf("Save: %v", err)
		}

		_, body := getSweepPage(t, h)

		if !strings.Contains(body, `data-testid="sweep-needs-attention"`) {
			t.Fatal("body missing the needs-attention section")
		}
		if got := strings.Count(body, `data-testid="sweep-reason"`); got != 2 {
			t.Errorf("rendered %d reasons, want both — naming one at a time turns a single fix into several rounds", got)
		}
	})
}

func TestEmptyHistoryPage(t *testing.T) {
	t.Run("offers the run action before any snapshot exists", func(t *testing.T) {
		h, _, _, _ := newHandler(t)

		status, body := getSweepPage(t, h)
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200", status)
		}

		mustContainAll(t, body, map[string]string{
			"the first-run empty state": `data-testid="sweep-empty"`,
			// Without the control a new user could not produce a first snapshot
			// before the monthly tick.
			"the run control": `data-testid="sweep-run"`,
		})
	})
}

func TestSchedulePage(t *testing.T) {
	t.Run("the schedule is managed on the sweep page", func(t *testing.T) {
		h, _, sched, ctx := newHandler(t)
		if _, err := sched.Create(ctx, schedule.Item{
			Name:       "Rent",
			Direction:  schedule.DirectionOut,
			Amount:     2400,
			Cadence:    schedule.CadenceMonthly,
			DayOfMonth: 1,
			Active:     true,
		}); err != nil {
			t.Fatalf("Create: %v", err)
		}

		_, body := getSweepPage(t, h)

		mustContainAll(t, body, map[string]string{
			"the schedule region": `data-testid="sweep-schedule"`,
			"the add form":        `data-testid="schedule-add-form"`,
			"the declared item":   "Rent",
		})
	})

	t.Run("a freshly declared item is on the timeline", func(t *testing.T) {
		// The add form carries no active toggle, so nothing in the submitted body
		// says the item is on. Declaring something in order to leave it switched
		// off is not a thing anyone means to do.
		h, _, sched, ctx := newHandler(t)

		form := url.Values{
			"name":         {"Rent"},
			"direction":    {"out"},
			"amount":       {"2400"},
			"cadence":      {"monthly"},
			"day_of_month": {"1"},
		}
		req := httptest.NewRequest(http.MethodPost, "/sweep/schedule", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		h.PostScheduleItem(httptest.NewRecorder(), req)

		active, err := sched.ActiveItems(ctx)
		if err != nil {
			t.Fatalf("ActiveItems: %v", err)
		}
		if len(active) != 1 {
			t.Fatalf("%d active items, want the one just declared", len(active))
		}
	})

	t.Run("an empty schedule says the sweep still works without one", func(t *testing.T) {
		h, _, _, _ := newHandler(t)

		_, body := getSweepPage(t, h)

		if !strings.Contains(body, `data-testid="schedule-empty"`) {
			t.Error("an undeclared schedule needs an explanation, not a bare form")
		}
	})
}
