package adapters_test

import (
	"strings"
	"testing"
	"time"

	"github.com/alecdray/two-cents/src/internal/schedule"
)

// TestScheduleRowShowsOverdueOccurrence drives the real page the way a
// browser does: a dated item (monthly-with-day, or biweekly) whose current
// lookback still holds an outstanding occurrence must show that occurrence's
// own date, labeled past due, in the compact row — never the ordinary
// next-projected date, which would otherwise silently jump past it to the
// following cycle ([ADR-0031]).
func TestScheduleRowShowsOverdueOccurrence(t *testing.T) {
	t.Run("a monthly item overdue and unmatched shows its own past-due date instead of its next projection", func(t *testing.T) {
		h, _, scheduleSvc, ctx := newHandler(t)

		due := time.Now().UTC().AddDate(0, 0, -3)
		if _, err := scheduleSvc.Create(ctx, schedule.Item{
			Name:       "Rent",
			Direction:  schedule.DirectionOut,
			Amount:     scheduleAmount(2000),
			Cadence:    schedule.CadenceMonthly,
			DayOfMonth: due.Day(),
			Active:     true,
		}); err != nil {
			t.Fatalf("Create: %v", err)
		}

		// The occurrence the service actually projected for the declared day,
		// not the raw "3 days ago" instant, since a declared day of month is
		// clamped and re-projected onto this month's calendar.
		windows, err := scheduleSvc.MatchingWindow(ctx)
		if err != nil {
			t.Fatalf("MatchingWindow: %v", err)
		}
		if len(windows) != 1 || len(windows[0].Occurrences) == 0 {
			t.Fatalf("expected one item with at least one occurrence in its window, got %+v", windows)
		}
		occurrence := windows[0].Occurrences[len(windows[0].Occurrences)-1].Occurrence

		_, body := getSweepPage(t, h)

		wantLabel := occurrence.Format("Jan 2") + ", past due"
		if !strings.Contains(body, `data-testid="schedule-row-overdue"`) {
			t.Error("the row must mark the overdue occurrence with its own element")
		}
		if !strings.Contains(body, wantLabel) {
			t.Errorf("the row must show %q, the overdue occurrence's own date, got body:\n%s", wantLabel, body)
		}
		if strings.Contains(body, `data-testid="schedule-row-next"`) {
			t.Error("the ordinary next-projected date must not also render while an occurrence is overdue")
		}
	})

	t.Run("a biweekly item overdue and unmatched shows its own past-due date instead of its next projection", func(t *testing.T) {
		h, _, scheduleSvc, ctx := newHandler(t)

		// One full cadence step back: this cycle's occurrence (the anchor
		// itself) is already due, and the next cycle — fourteen days later —
		// lands exactly today, which is what makes the anchor overdue rather
		// than merely the most recent past occurrence.
		anchor := time.Now().UTC().AddDate(0, 0, -14)
		if _, err := scheduleSvc.Create(ctx, schedule.Item{
			Name:       "Paycheck",
			Direction:  schedule.DirectionIn,
			Amount:     scheduleAmount(3000),
			Cadence:    schedule.CadenceBiweekly,
			AnchorDate: anchor,
			Active:     true,
		}); err != nil {
			t.Fatalf("Create: %v", err)
		}

		_, body := getSweepPage(t, h)

		wantLabel := anchor.Format("Jan 2") + ", past due"
		if !strings.Contains(body, `data-testid="schedule-row-overdue"`) {
			t.Error("the row must mark the overdue occurrence with its own element")
		}
		if !strings.Contains(body, wantLabel) {
			t.Errorf("the row must show %q, the overdue occurrence's own date, got body:\n%s", wantLabel, body)
		}
	})

	t.Run("matching the overdue occurrence reverts the row to its ordinary next-projected date", func(t *testing.T) {
		h, _, scheduleSvc, ctx := newHandler(t)

		due := time.Now().UTC().AddDate(0, 0, -3)
		item, err := scheduleSvc.Create(ctx, schedule.Item{
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

		windows, err := scheduleSvc.MatchingWindow(ctx)
		if err != nil {
			t.Fatalf("MatchingWindow: %v", err)
		}
		occurrence := windows[0].Occurrences[len(windows[0].Occurrences)-1].Occurrence

		// Sanity check: before matching, the page does show it overdue —
		// otherwise reverting afterward would prove nothing.
		if _, body := getSweepPage(t, h); !strings.Contains(body, `data-testid="schedule-row-overdue"`) {
			t.Fatalf("expected the occurrence to render overdue before it is matched, got body:\n%s", body)
		}

		if err := scheduleSvc.MatchOccurrence(ctx, item.ID, occurrence, "txn-fixture-1"); err != nil {
			t.Fatalf("MatchOccurrence: %v", err)
		}

		_, body := getSweepPage(t, h)
		if strings.Contains(body, `data-testid="schedule-row-overdue"`) {
			t.Error("a matched occurrence must not still render as overdue")
		}
		if strings.Contains(body, "past due") {
			t.Error("a matched occurrence must not still carry the past-due label")
		}
		if !strings.Contains(body, `data-testid="schedule-row-next"`) {
			t.Error("once nothing is overdue, the row must revert to its ordinary next-projected date")
		}
	})

	t.Run("an item not yet due shows only its ordinary next-projected date", func(t *testing.T) {
		h, _, scheduleSvc, ctx := newHandler(t)

		due := time.Now().UTC().AddDate(0, 0, 3)
		item, err := scheduleSvc.Create(ctx, schedule.Item{
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
		// A brand-new item's lookback still reaches back across the one
		// cycle it has no history for at all, which otherwise reads as its
		// own (correctly) outstanding occurrence — settle it so this test
		// isolates the ordinary case this task must leave alone.
		settleAllOutstanding(t, ctx, scheduleSvc, item.ID)

		_, body := getSweepPage(t, h)

		if strings.Contains(body, `data-testid="schedule-row-overdue"`) {
			t.Error("an item not yet due must not render as overdue")
		}
		if strings.Contains(body, "past due") {
			t.Error("an item not yet due must not carry the past-due label")
		}
		if !strings.Contains(body, `data-testid="schedule-row-next"`) {
			t.Error("an item not yet due must still show its ordinary next-projected date")
		}
	})

	t.Run("a dateless monthly item never shows a past-due label", func(t *testing.T) {
		h, _, scheduleSvc, ctx := newHandler(t)

		if _, err := scheduleSvc.Create(ctx, schedule.Item{
			Name:      "Car repair",
			Direction: schedule.DirectionOut,
			Cadence:   schedule.CadenceMonthly,
			// DayOfMonth left zero: dateless.
			Active: true,
		}); err != nil {
			t.Fatalf("Create: %v", err)
		}

		_, body := getSweepPage(t, h)

		if strings.Contains(body, `data-testid="schedule-row-overdue"`) {
			t.Error("a dateless item must never render the overdue element")
		}
		if strings.Contains(body, "past due") {
			t.Error("a dateless item must never carry the past-due label")
		}
		if !strings.Contains(body, "This month, any day") {
			t.Error("a dateless item's own open-month cadence summary must still render, unaffected")
		}
	})
}
