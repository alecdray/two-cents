package sweep

import (
	"context"
	"testing"
	"time"

	"github.com/alecdray/two-cents/src/internal/core/contextx"
	cron "github.com/robfig/cron/v3"
)

// testCtx returns a minimal ContextX for unit tests (no app / request / user
// values needed — only the database context is used by the repo underneath).
func testCtx() contextx.ContextX {
	return contextx.NewContextX(context.Background())
}

// --- Schedule wiring ---

// TestMonthlyTaskScheduleFiresOnSeventhAtMidnightInAppTimezone verifies that
// the task fires at 00:00 on the 7th of each month in the configured app
// timezone, not in server-local or UTC time.
func TestMonthlyTaskScheduleFiresOnSeventhAtMidnightInAppTimezone(t *testing.T) {
	// Use America/Chicago (CDT/CST, UTC-5/UTC-6) — clearly differs from UTC so
	// the timezone anchoring is observable.
	loc, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Fatalf("load America/Chicago: %v", err)
	}

	mt := NewMonthlyTask(nil, loc)

	if mt.Name() != "sweep-monthly" {
		t.Errorf("Name: want sweep-monthly, got %q", mt.Name())
	}

	sched := mt.Schedule()
	if sched == nil {
		t.Fatal("Schedule is nil; a recurring task must return a cadence")
	}

	// Parse with the standard parser. robfig/cron v3 strips the CRON_TZ= prefix
	// before parsing the remaining 5-field spec and embeds the location in the
	// returned Schedule — Next() then produces times in that zone.
	p := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	s, err := p.Parse(sched.String())
	if err != nil {
		t.Fatalf("invalid cron expression %q: %v", sched.String(), err)
	}

	// Next after 2026-07-06 23:59:59 Chicago must be 2026-07-07 00:00:00 Chicago.
	ref := time.Date(2026, 7, 6, 23, 59, 59, 0, loc)
	next := s.Next(ref)
	nextInChicago := next.In(loc)
	if nextInChicago.Year() != 2026 || nextInChicago.Month() != 7 || nextInChicago.Day() != 7 {
		t.Errorf("next fire date: want 2026-07-07, got %s", nextInChicago.Format("2006-01-02"))
	}
	if nextInChicago.Hour() != 0 || nextInChicago.Minute() != 0 || nextInChicago.Second() != 0 {
		t.Errorf("next fire time: want 00:00:00, got %02d:%02d:%02d",
			nextInChicago.Hour(), nextInChicago.Minute(), nextInChicago.Second())
	}

	// Verify the tick is NOT 00:00 UTC when Chicago and UTC differ. On 2026-07-07
	// CDT (UTC-5), midnight Chicago is 05:00 UTC — the task must fire at 05:00
	// UTC, not 00:00 UTC. A UTC-anchored scheduler would fire five hours early.
	nextUTCHour := next.UTC().Hour()
	if nextUTCHour == 0 {
		// Only a coincidence if UTC offset is 0; Chicago is never at offset 0,
		// so this would be wrong.
		t.Errorf("next fire UTC hour is 00:00 — schedule appears to be UTC-anchored rather than Chicago-anchored")
	}

	// Next after 2026-08-08 (past the 7th of August) must land on 2026-09-07 in
	// Chicago — skips to the next month's 7th.
	ref2 := time.Date(2026, 8, 8, 1, 0, 0, 0, loc)
	next2 := s.Next(ref2).In(loc)
	if next2.Year() != 2026 || next2.Month() != 9 || next2.Day() != 7 {
		t.Errorf("next after Aug 8: want 2026-09-07, got %s", next2.Format("2006-01-02"))
	}
	if next2.Hour() != 0 || next2.Minute() != 0 {
		t.Errorf("next after Aug 8 time: want 00:00, got %02d:%02d", next2.Hour(), next2.Minute())
	}
}

// --- Run behaviour ---

// fakeSweeperSvc is a minimal stand-in that lets the task_test drive Compute
// and SaveLatest without wiring the full *Service dependency graph.
type fakeSweeperSvc struct {
	computeF func(ctx contextx.ContextX) (Recommendation, error)
	saveF    func(ctx contextx.ContextX, rec Recommendation) error
}

func (f *fakeSweeperSvc) Compute(ctx contextx.ContextX) (Recommendation, error) {
	return f.computeF(ctx)
}

func (f *fakeSweeperSvc) SaveLatest(ctx contextx.ContextX, rec Recommendation) error {
	return f.saveF(ctx, rec)
}

// TestMonthlyTaskRunStoresLatest verifies that Run computes a recommendation
// and persists it so that LoadLatest afterwards returns it.
func TestMonthlyTaskRunStoresLatest(t *testing.T) {
	database := newTestDB(t)
	repo := NewRepo(database.Queries())
	ctx := testCtx()

	want := Recommendation{
		Kind:              KindNumeric,
		CurrentChecking:   4200.00,
		FixedSafetyMargin: 500,
		SuggestedSweep:    1234,
		Direction:         DirectionCheckingToSavings,
	}

	fake := &fakeSweeperSvc{
		computeF: func(_ contextx.ContextX) (Recommendation, error) {
			return want, nil
		},
		saveF: func(c contextx.ContextX, rec Recommendation) error {
			return repo.SaveLatest(c, rec)
		},
	}

	mt := &monthlyTask{svc: fake, schedule: buildMonthlySchedule(time.UTC)}
	if err := mt.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	got, found, err := repo.LoadLatest(ctx)
	if err != nil {
		t.Fatalf("LoadLatest: %v", err)
	}
	if !found {
		t.Fatal("LoadLatest: found=false after Run")
	}
	if got.CurrentChecking != want.CurrentChecking {
		t.Errorf("CurrentChecking: want %v, got %v", want.CurrentChecking, got.CurrentChecking)
	}
	if got.SuggestedSweep != want.SuggestedSweep {
		t.Errorf("SuggestedSweep: want %v, got %v", want.SuggestedSweep, got.SuggestedSweep)
	}
}

// TestMonthlyTaskRunReplacesOnRefire verifies that firing a second time replaces
// the stored result — one row always, never a duplicate.
func TestMonthlyTaskRunReplacesOnRefire(t *testing.T) {
	database := newTestDB(t)
	repo := NewRepo(database.Queries())
	ctx := testCtx()

	call := 0
	results := []Recommendation{
		{Kind: KindNumeric, CurrentChecking: 1000, FixedSafetyMargin: 500, Direction: DirectionCheckingToSavings},
		{Kind: KindNumeric, CurrentChecking: 9999, FixedSafetyMargin: 500, Direction: DirectionCheckingToSavings},
	}

	fake := &fakeSweeperSvc{
		computeF: func(_ contextx.ContextX) (Recommendation, error) {
			r := results[call]
			call++
			return r, nil
		},
		saveF: func(c contextx.ContextX, rec Recommendation) error {
			return repo.SaveLatest(c, rec)
		},
	}

	mt := &monthlyTask{svc: fake, schedule: buildMonthlySchedule(time.UTC)}

	if err := mt.Run(ctx); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if err := mt.Run(ctx); err != nil {
		t.Fatalf("second Run: %v", err)
	}

	got, found, err := repo.LoadLatest(ctx)
	if err != nil {
		t.Fatalf("LoadLatest: %v", err)
	}
	if !found {
		t.Fatal("found=false after two runs")
	}
	// Must return the second result, not the first.
	if got.CurrentChecking != 9999 {
		t.Errorf("CurrentChecking: want 9999 (second run replaces first), got %v", got.CurrentChecking)
	}
}
