package sweep

import (
	"strconv"
	"testing"
	"time"

	"github.com/alecdray/two-cents/src/internal/schedule"
)

// The timeline is reckoned in the configured app timezone, so the tests are
// too — a zone with DST, where a bug that reasons in 24h blocks rather than
// calendar days shows up.
func appZone(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	return loc
}

// runInstant is a deliberately mid-afternoon run: a snapshot can be produced at
// any moment, so nothing may assume the run lands at midnight.
func runInstant(loc *time.Location) time.Time {
	return time.Date(2026, time.September, 7, 14, 30, 0, 0, loc)
}

func monthly(name string, dir schedule.Direction, amount float64, day int) schedule.Item {
	return schedule.Item{
		Name:       name,
		Direction:  dir,
		Amount:     amount,
		Cadence:    schedule.CadenceMonthly,
		DayOfMonth: day,
		Active:     true,
	}
}

func biweekly(name string, dir schedule.Direction, amount float64, anchor time.Time) schedule.Item {
	return schedule.Item{
		Name:       name,
		Direction:  dir,
		Amount:     amount,
		Cadence:    schedule.CadenceBiweekly,
		AnchorDate: anchor,
		Active:     true,
	}
}

// eventSummary renders one event as "date label direction amount" so a whole
// timeline can be asserted in one readable line.
func eventSummary(e TimelineEvent) string {
	return e.Date.Format("2006-01-02") + " " + e.Label + " " + string(e.Direction) + " " +
		strconv.FormatFloat(e.Amount, 'f', -1, 64)
}

func summaries(events []TimelineEvent) []string {
	out := make([]string, len(events))
	for i, e := range events {
		out[i] = eventSummary(e)
	}
	return out
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestHorizonFrom(t *testing.T) {
	loc := appZone(t)

	t.Run("the horizon is exactly one month from the run instant", func(t *testing.T) {
		got := horizonFrom(runInstant(loc))
		if want := "2026-10-07 14:30:00"; got.Format("2006-01-02 15:04:05") != want {
			t.Errorf("horizon = %s, want %s", got.Format("2006-01-02 15:04:05"), want)
		}
	})

	t.Run("a run on the 31st clamps to the last day of a short month", func(t *testing.T) {
		got := horizonFrom(time.Date(2026, time.January, 31, 9, 0, 0, 0, loc))
		if want := "2026-02-28"; got.Format("2006-01-02") != want {
			t.Errorf("horizon = %s, want %s", got.Format("2006-01-02"), want)
		}
	})
}

func TestBuildTimeline(t *testing.T) {
	loc := appZone(t)
	now := runInstant(loc)
	horizon := horizonFrom(now)

	t.Run("a card with no statement detail lands its whole balance at the run instant", func(t *testing.T) {
		got := buildTimeline(timelineInput{
			now:     now,
			horizon: horizon,
			cards:   []cardObligation{{label: "Sapphire", balance: 840}},
		})

		if len(got) != 1 {
			t.Fatalf("timeline = %v, want one card event", summaries(got))
		}
		if !got[0].Date.Equal(now) {
			t.Errorf("card event dated %s, want the run instant %s", got[0].Date, now)
		}
		if got[0].Amount != 840 || got[0].Direction != EventOut {
			t.Errorf("card event = %+v, want an outflow of 840", got[0])
		}
	})

	t.Run("a card paid on its due date bills the statement on that date", func(t *testing.T) {
		due := now.AddDate(0, 0, 10)
		got := buildTimeline(timelineInput{
			now:     now,
			horizon: horizon,
			cards: []cardObligation{{
				label:     "Sapphire",
				balance:   840,
				statement: &cardStatement{balance: 500, due: due},
			}},
		})

		if len(got) != 1 {
			t.Fatalf("timeline = %v, want one card event", summaries(got))
		}
		if !got[0].Date.Equal(due) {
			t.Errorf("card event dated %s, want the due date %s", got[0].Date, due)
		}
		if got[0].Amount != 500 {
			t.Errorf("card amount = %v, want the statement balance 500 (unbilled spend stays off)", got[0].Amount)
		}
	})

	t.Run("a statement already paid down bills only what the balance still owes", func(t *testing.T) {
		got := buildTimeline(timelineInput{
			now:     now,
			horizon: horizon,
			cards: []cardObligation{{
				label:     "Sapphire",
				balance:   120,
				statement: &cardStatement{balance: 500, due: now.AddDate(0, 0, 10)},
			}},
		})

		if len(got) != 1 {
			t.Fatalf("timeline = %v, want one card event", summaries(got))
		}
		if got[0].Amount != 120 {
			t.Errorf("card amount = %v, want the balance 120 — the statement was paid down and the model never saw the payment", got[0].Amount)
		}
	})

	t.Run("a due date already past is imminent, so it lands at the run instant", func(t *testing.T) {
		got := buildTimeline(timelineInput{
			now:     now,
			horizon: horizon,
			cards: []cardObligation{{
				label:     "Sapphire",
				balance:   840,
				statement: &cardStatement{balance: 500, due: now.AddDate(0, 0, -3)},
			}},
		})

		if len(got) != 1 {
			t.Fatalf("timeline = %v, want one card event", summaries(got))
		}
		if !got[0].Date.Equal(now) {
			t.Errorf("card event dated %s, want the run instant %s — an overdue payment is imminent, not historical", got[0].Date, now)
		}
	})

	t.Run("a due date beyond the horizon contributes nothing", func(t *testing.T) {
		got := buildTimeline(timelineInput{
			now:     now,
			horizon: horizon,
			cards: []cardObligation{{
				label:     "Sapphire",
				balance:   840,
				statement: &cardStatement{balance: 500, due: horizon.AddDate(0, 0, 5)},
			}},
		})
		if len(got) != 0 {
			t.Errorf("timeline = %v, want no events — the payment falls outside the window", summaries(got))
		}
	})

	t.Run("a card owing nothing contributes nothing", func(t *testing.T) {
		got := buildTimeline(timelineInput{
			now:     now,
			horizon: horizon,
			cards:   []cardObligation{{label: "Cleared", balance: 0}},
		})
		if len(got) != 0 {
			t.Errorf("timeline = %v, want no events", summaries(got))
		}
	})

	t.Run("a future outflow lands on its own date", func(t *testing.T) {
		got := buildTimeline(timelineInput{
			now:     now,
			horizon: horizon,
			items:   []schedule.Item{monthly("Rent", schedule.DirectionOut, 2400, 20)},
		})

		want := []string{"2026-09-20 Rent out 2400"}
		if !equalStrings(summaries(got), want) {
			t.Errorf("timeline = %v, want %v", summaries(got), want)
		}
	})

	t.Run("a monthly item falls exactly once, even when the run lands on its day", func(t *testing.T) {
		// Day 7, run on the 7th: the horizon's own 7 October must not be counted as
		// a second occurrence of the same monthly item.
		got := buildTimeline(timelineInput{
			now:     now,
			horizon: horizon,
			items:   []schedule.Item{monthly("Rent", schedule.DirectionOut, 2400, 7)},
		})

		want := []string{"2026-09-07 Rent out 2400"}
		if !equalStrings(summaries(got), want) {
			t.Errorf("timeline = %v, want %v", summaries(got), want)
		}
	})

	t.Run("an outflow due today lands at the run instant, still owed", func(t *testing.T) {
		// Midnight on the 7th has passed by the time the run is made, but the bill
		// has not: it is the most imminent obligation there is.
		got := buildTimeline(timelineInput{
			now:     now,
			horizon: horizon,
			items:   []schedule.Item{monthly("Rent", schedule.DirectionOut, 2400, 7)},
		})

		if len(got) != 1 {
			t.Fatalf("timeline = %v, want one event", summaries(got))
		}
		if !got[0].Date.Equal(now) {
			t.Errorf("event dated %s, want the run instant %s", got[0].Date, now)
		}
	})

	t.Run("an inflow due today is omitted — it may never arrive", func(t *testing.T) {
		got := buildTimeline(timelineInput{
			now:     now,
			horizon: horizon,
			items:   []schedule.Item{monthly("Paycheck", schedule.DirectionIn, 3000, 7)},
		})

		if len(got) != 0 {
			t.Errorf("timeline = %v, want none — an inflow is never assumed to have landed", summaries(got))
		}
	})

	t.Run("an occurrence that has already fallen due is not reached backwards for", func(t *testing.T) {
		// Rent fell due on the 1st, six days before the run. Whether it was paid is
		// a question for matching, not for the window — reserving it again here
		// would double every monthly bill.
		got := buildTimeline(timelineInput{
			now:     now,
			horizon: horizon,
			items:   []schedule.Item{monthly("Rent", schedule.DirectionOut, 2400, 1)},
		})

		want := []string{"2026-10-01 Rent out 2400"}
		if !equalStrings(summaries(got), want) {
			t.Errorf("timeline = %v, want %v", summaries(got), want)
		}
	})

	t.Run("a biweekly occurrence beyond the horizon contributes nothing", func(t *testing.T) {
		// Anchored on the 11th, the third occurrence falls on 9 October — past a
		// horizon of 7 October, so the window simply ends before it.
		got := buildTimeline(timelineInput{
			now:     now,
			horizon: horizon,
			items: []schedule.Item{
				biweekly("Paycheck", schedule.DirectionIn, 1500, time.Date(2026, time.September, 11, 0, 0, 0, 0, loc)),
			},
		})

		want := []string{
			"2026-09-11 Paycheck in 1500",
			"2026-09-25 Paycheck in 1500",
		}
		if !equalStrings(summaries(got), want) {
			t.Errorf("timeline = %v, want %v", summaries(got), want)
		}
	})

	t.Run("a biweekly inflow places every occurrence in the window", func(t *testing.T) {
		got := buildTimeline(timelineInput{
			now:     now,
			horizon: horizon,
			items: []schedule.Item{
				biweekly("Paycheck", schedule.DirectionIn, 1500, time.Date(2026, time.September, 11, 0, 0, 0, 0, loc)),
			},
		})

		want := []string{
			"2026-09-11 Paycheck in 1500",
			"2026-09-25 Paycheck in 1500",
		}
		if !equalStrings(summaries(got), want) {
			t.Errorf("timeline = %v, want %v", summaries(got), want)
		}
	})
}

func TestEvaluate(t *testing.T) {
	loc := appZone(t)
	on := func(d int) time.Time { return time.Date(2026, time.September, d, 0, 0, 0, 0, loc) }

	t.Run("the peak is taken, not the final total", func(t *testing.T) {
		// The month nets out to a $500 surplus, but $3,000 has to be present on the
		// 20th for the bill to clear. A paycheck on the 30th cannot pay it.
		required, _ := evaluate([]TimelineEvent{
			{Date: on(20), Label: "Rent", Direction: EventOut, Amount: 3000},
			{Date: on(30), Label: "Paycheck", Direction: EventIn, Amount: 3500},
		})

		if required != 3000 {
			t.Errorf("required = %v, want the 3000 peak rather than the -500 total", required)
		}
	})

	t.Run("an inflow offsets only what follows it", func(t *testing.T) {
		required, _ := evaluate([]TimelineEvent{
			{Date: on(10), Label: "Paycheck", Direction: EventIn, Amount: 3500},
			{Date: on(20), Label: "Rent", Direction: EventOut, Amount: 3000},
		})

		if required != 0 {
			t.Errorf("required = %v, want 0 — the paycheck lands first", required)
		}
	})

	t.Run("on the same date the outflow is taken first", func(t *testing.T) {
		// Never assume a deposit clears before a debit posted the same day.
		required, ordered := evaluate([]TimelineEvent{
			{Date: on(15), Label: "Paycheck", Direction: EventIn, Amount: 1000},
			{Date: on(15), Label: "Rent", Direction: EventOut, Amount: 1000},
		})

		if ordered[0].Label != "Rent" {
			t.Errorf("first event = %s, want the outflow first", ordered[0].Label)
		}
		if required != 1000 {
			t.Errorf("required = %v, want 1000 — the debit is not covered by the same-day deposit", required)
		}
	})

	t.Run("an all-inflow timeline requires nothing", func(t *testing.T) {
		required, _ := evaluate([]TimelineEvent{
			{Date: on(10), Label: "Paycheck", Direction: EventIn, Amount: 3500},
		})
		if required != 0 {
			t.Errorf("required = %v, want 0", required)
		}
	})

	t.Run("an empty timeline requires nothing", func(t *testing.T) {
		required, ordered := evaluate(nil)
		if required != 0 {
			t.Errorf("required = %v, want 0", required)
		}
		if len(ordered) != 0 {
			t.Errorf("ordered = %v, want empty", summaries(ordered))
		}
	})

	t.Run("every event carries the running total it produced", func(t *testing.T) {
		_, ordered := evaluate([]TimelineEvent{
			{Date: on(5), Label: "Rent", Direction: EventOut, Amount: 2000},
			{Date: on(10), Label: "Paycheck", Direction: EventIn, Amount: 1500},
			{Date: on(15), Label: "Utilities", Direction: EventOut, Amount: 200},
		})

		want := []float64{2000, 500, 700}
		for i, w := range want {
			if ordered[i].RunningTotal != w {
				t.Errorf("event %d running total = %v, want %v", i, ordered[i].RunningTotal, w)
			}
		}
	})

	t.Run("the event that set the figure is marked as the peak", func(t *testing.T) {
		_, ordered := evaluate([]TimelineEvent{
			{Date: on(5), Label: "Rent", Direction: EventOut, Amount: 2000},
			{Date: on(10), Label: "Utilities", Direction: EventOut, Amount: 200},
			{Date: on(15), Label: "Paycheck", Direction: EventIn, Amount: 5000},
		})

		if !ordered[1].Peak {
			t.Error("the Utilities event set the peak and should be marked")
		}
		if ordered[0].Peak || ordered[2].Peak {
			t.Error("only the event that set the figure is the peak")
		}
	})

	t.Run("a timeline that never goes negative marks no peak", func(t *testing.T) {
		_, ordered := evaluate([]TimelineEvent{
			{Date: on(5), Label: "Paycheck", Direction: EventIn, Amount: 1000},
		})
		if ordered[0].Peak {
			t.Error("required stays at its zero floor, so no event set it")
		}
	})

}
