package sweep

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/alecdray/two-cents/src/internal/core/db"

	"github.com/pressly/goose/v3"

	_ "github.com/mattn/go-sqlite3"
)

// newTestDB opens a temporary SQLite file, runs all migrations, and returns a
// *db.DB backed by it. The file and connection are cleaned up when t ends.
func newTestDB(t *testing.T) *db.DB {
	t.Helper()

	migrationsDir, err := filepath.Abs("../../../db/migrations")
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

func newTestRepo(t *testing.T) *Repo {
	t.Helper()
	return NewRepo(newTestDB(t).Queries())
}

func ptr(f float64) *float64 { return &f }

// numericSnapshot is a saved-shaped recommendation with a two-event timeline —
// enough that ordering, the running totals and the peak flag all have something
// to lose in a round trip.
func numericSnapshot(id string, computedAt time.Time) Recommendation {
	return Recommendation{
		ID:                id,
		Kind:              KindNumeric,
		ComputedAt:        computedAt,
		CurrentChecking:   5200,
		CurrentSavings:    18000,
		RequiredChecking:  2400,
		FixedSafetyMargin: 500,
		SuggestedSweep:    2300,
		Direction:         DirectionCheckingToSavings,
		Timeline: []TimelineEvent{
			{
				Date:         computedAt.AddDate(0, 0, 3),
				Label:        "Rent",
				Direction:    EventOut,
				Amount:       2400,
				RunningTotal: 2400,
				Peak:         true,
			},
			{
				Date:         computedAt.AddDate(0, 0, 10),
				Label:        "Paycheck",
				Direction:    EventIn,
				Amount:       3100,
				RunningTotal: -700,
			},
		},
	}
}

func TestSaveAndLoad(t *testing.T) {
	loc := appZone(t)
	computedAt := runInstant(loc)
	ctx := context.Background()

	t.Run("a numeric snapshot round-trips every figure", func(t *testing.T) {
		repo := newTestRepo(t)
		want := numericSnapshot("rec-1", computedAt)

		if err := repo.Save(ctx, want); err != nil {
			t.Fatalf("Save: %v", err)
		}
		got, found, err := repo.LoadLatest(ctx)
		if err != nil || !found {
			t.Fatalf("LoadLatest: found=%v err=%v", found, err)
		}

		if got.Kind != KindNumeric {
			t.Errorf("kind = %s, want numeric", got.Kind)
		}
		if got.CurrentChecking != want.CurrentChecking || got.CurrentSavings != want.CurrentSavings {
			t.Errorf("balances = %v / %v, want %v / %v", got.CurrentChecking, got.CurrentSavings, want.CurrentChecking, want.CurrentSavings)
		}
		if got.RequiredChecking != want.RequiredChecking {
			t.Errorf("required = %v, want %v", got.RequiredChecking, want.RequiredChecking)
		}
		if got.SuggestedSweep != want.SuggestedSweep || got.Direction != want.Direction {
			t.Errorf("sweep = %v %s, want %v %s", got.SuggestedSweep, got.Direction, want.SuggestedSweep, want.Direction)
		}
		if got.FixedSafetyMargin != want.FixedSafetyMargin {
			t.Errorf("margin = %v, want %v", got.FixedSafetyMargin, want.FixedSafetyMargin)
		}
		if !got.ComputedAt.Equal(computedAt) {
			t.Errorf("computed at = %s, want %s", got.ComputedAt, computedAt)
		}
	})

	t.Run("the stored timeline comes back whole and in order", func(t *testing.T) {
		// A snapshot has to explain its own arithmetic without recomputing it, so
		// the derivation the page shows is only as good as this round trip.
		repo := newTestRepo(t)
		want := numericSnapshot("rec-1", computedAt)

		if err := repo.Save(ctx, want); err != nil {
			t.Fatalf("Save: %v", err)
		}
		got, _, err := repo.LoadLatest(ctx)
		if err != nil {
			t.Fatalf("LoadLatest: %v", err)
		}

		if len(got.Timeline) != len(want.Timeline) {
			t.Fatalf("timeline = %v, want %v", summaries(got.Timeline), summaries(want.Timeline))
		}
		for i := range want.Timeline {
			w, g := want.Timeline[i], got.Timeline[i]
			if !g.Date.Equal(w.Date) || g.Label != w.Label || g.Direction != w.Direction {
				t.Errorf("event %d = %s, want %s", i, eventSummary(g), eventSummary(w))
			}
			if g.Amount != w.Amount || g.RunningTotal != w.RunningTotal || g.Peak != w.Peak {
				t.Errorf("event %d figures = %v/%v peak=%v, want %v/%v peak=%v",
					i, g.Amount, g.RunningTotal, g.Peak, w.Amount, w.RunningTotal, w.Peak)
			}
		}
	})

	t.Run("an unknown savings balance survives as unknown, not as zero", func(t *testing.T) {
		repo := newTestRepo(t)
		rec := numericSnapshot("rec-1", computedAt)
		rec.SavingsUnknown = true
		rec.CurrentSavings = 0

		if err := repo.Save(ctx, rec); err != nil {
			t.Fatalf("Save: %v", err)
		}
		got, _, _ := repo.LoadLatest(ctx)

		if !got.SavingsUnknown {
			t.Error("SavingsUnknown = false, want true")
		}
	})

	t.Run("a needs-attention snapshot round-trips every reason", func(t *testing.T) {
		repo := newTestRepo(t)
		rec := Recommendation{
			ID:         "rec-1",
			Kind:       KindNeedsAttention,
			ComputedAt: computedAt,
			Reasons:    []NeedsAttentionReason{ReasonCheckingStale, ReasonCardBalanceUnknown},
		}

		if err := repo.Save(ctx, rec); err != nil {
			t.Fatalf("Save: %v", err)
		}
		got, _, _ := repo.LoadLatest(ctx)

		if got.Kind != KindNeedsAttention {
			t.Fatalf("kind = %s, want needs_attention", got.Kind)
		}
		if len(got.Reasons) != 2 || got.Reasons[0] != ReasonCheckingStale || got.Reasons[1] != ReasonCardBalanceUnknown {
			t.Errorf("reasons = %v, want both, in order", got.Reasons)
		}
	})

	t.Run("nothing saved yet is not found, which is not an error", func(t *testing.T) {
		repo := newTestRepo(t)

		_, found, err := repo.LoadLatest(ctx)
		if err != nil {
			t.Fatalf("LoadLatest: %v", err)
		}
		if found {
			t.Error("found = true on an empty history")
		}
	})
}

func TestSaveAppends(t *testing.T) {
	loc := appZone(t)
	computedAt := runInstant(loc)
	ctx := context.Background()

	t.Run("a second save adds to the history rather than replacing", func(t *testing.T) {
		repo := newTestRepo(t)

		if err := repo.Save(ctx, numericSnapshot("older", computedAt)); err != nil {
			t.Fatalf("Save: %v", err)
		}
		newer := numericSnapshot("newer", computedAt.Add(time.Hour))
		newer.CurrentChecking = 9000
		if err := repo.Save(ctx, newer); err != nil {
			t.Fatalf("Save: %v", err)
		}

		all, err := repo.List(ctx)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(all) != 2 {
			t.Fatalf("history holds %d snapshots, want 2", len(all))
		}
		if all[0].ID != "newer" {
			t.Errorf("newest = %s, want the newer snapshot first", all[0].ID)
		}
	})

	t.Run("a run repeating the previous figures still appends", func(t *testing.T) {
		// The record being kept is that the question was asked at that instant, so
		// an identical answer is still a snapshot.
		repo := newTestRepo(t)

		if err := repo.Save(ctx, numericSnapshot("first", computedAt)); err != nil {
			t.Fatalf("Save: %v", err)
		}
		if err := repo.Save(ctx, numericSnapshot("second", computedAt)); err != nil {
			t.Fatalf("Save: %v", err)
		}

		all, _ := repo.List(ctx)
		if len(all) != 2 {
			t.Errorf("history holds %d snapshots, want 2", len(all))
		}
	})

	t.Run("a snapshot keeps the id its run assigned", func(t *testing.T) {
		repo := newTestRepo(t)

		if err := repo.Save(ctx, numericSnapshot("assigned-by-the-run", computedAt)); err != nil {
			t.Fatalf("Save: %v", err)
		}

		got, found, err := repo.LoadByID(ctx, "assigned-by-the-run")
		if err != nil || !found {
			t.Fatalf("LoadByID: found=%v err=%v", found, err)
		}
		if got.ID != "assigned-by-the-run" {
			t.Errorf("id = %s, want the assigned one", got.ID)
		}
	})

	t.Run("a deep link to a snapshot that is not there is not found", func(t *testing.T) {
		repo := newTestRepo(t)

		_, found, err := repo.LoadByID(ctx, "never-saved")
		if err != nil {
			t.Fatalf("LoadByID: %v", err)
		}
		if found {
			t.Error("found = true for an id that was never saved")
		}
	})
}
