package adapters

import (
	"testing"
	"time"

	"github.com/alecdray/two-cents/src/internal/schedule"
)

// These drive overdueOccurrences directly against constructed fixtures, so
// they stay deterministic regardless of which calendar day they run on —
// MatchingWindow and NextOccurrence are the schedule module's own, already
// covered by that package's own tests, so these fixtures are hand-built
// ItemOccurrences and next-maps standing in for whatever those two calls
// would have returned. The end-to-end counterpart, schedule_overdue_row_test.go,
// drives the same question through the real page and asserts the literal
// rendered label.

// occState builds one occurrence state, settled when a transaction id is
// given and outstanding otherwise.
func occState(date time.Time, settledBy string) schedule.OccurrenceState {
	state := schedule.OccurrenceState{Occurrence: date}
	if settledBy != "" {
		state.SettledBy = settledBy
		state.Decision = schedule.MatchManual
	}
	return state
}

func TestOverdueOccurrences(t *testing.T) {
	item := schedule.Item{ID: "rent", Name: "Rent", Cadence: schedule.CadenceMonthly, DayOfMonth: 1}
	dateless := schedule.Item{ID: "repair", Name: "Car repair", Cadence: schedule.CadenceMonthly}

	oct1 := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	nov1 := time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC)
	dec1 := time.Date(2026, time.December, 1, 0, 0, 0, 0, time.UTC)

	t.Run("an outstanding occurrence before the next projection is overdue", func(t *testing.T) {
		windows := []schedule.ItemOccurrences{
			{Item: item, Occurrences: []schedule.OccurrenceState{occState(oct1, "")}},
		}
		next := map[string]time.Time{"rent": nov1}

		got := overdueOccurrences([]schedule.Item{item}, windows, next)
		date, ok := got["rent"]
		if !ok || !date.Equal(oct1) {
			t.Fatalf("overdueOccurrences()[rent] = %v, %v; want %v, true", date, ok, oct1)
		}
	})

	t.Run("a matched occurrence is not overdue", func(t *testing.T) {
		windows := []schedule.ItemOccurrences{
			{Item: item, Occurrences: []schedule.OccurrenceState{occState(oct1, "txn-1")}},
		}
		next := map[string]time.Time{"rent": nov1}

		got := overdueOccurrences([]schedule.Item{item}, windows, next)
		if date, ok := got["rent"]; ok {
			t.Fatalf("got overdue %v, want none: a settled occurrence must not read as overdue", date)
		}
	})

	t.Run("once the stale occurrence ages out of the window, an unresolved new cycle due today reads as ordinary, not overdue", func(t *testing.T) {
		// October's occurrence no longer appears in the window at all —
		// exactly what MatchingWindow itself produces once a cadence
		// interval has passed it by — leaving only November's own
		// occurrence, due today and not yet passed.
		windows := []schedule.ItemOccurrences{
			{Item: item, Occurrences: []schedule.OccurrenceState{occState(nov1, "")}},
		}
		next := map[string]time.Time{"rent": nov1}

		got := overdueOccurrences([]schedule.Item{item}, windows, next)
		if date, ok := got["rent"]; ok {
			t.Fatalf("got overdue %v, want none: the new cycle is due today, not overdue", date)
		}
	})

	t.Run("a new cycle itself overdue is what reports, never the one that aged out", func(t *testing.T) {
		windows := []schedule.ItemOccurrences{
			{Item: item, Occurrences: []schedule.OccurrenceState{occState(nov1, "")}},
		}
		next := map[string]time.Time{"rent": dec1}

		got := overdueOccurrences([]schedule.Item{item}, windows, next)
		date, ok := got["rent"]
		if !ok || !date.Equal(nov1) {
			t.Fatalf("overdueOccurrences()[rent] = %v, %v; want %v, true", date, ok, nov1)
		}
	})

	t.Run("a dateless item never reads as overdue, regardless of its window", func(t *testing.T) {
		windows := []schedule.ItemOccurrences{
			{Item: dateless, Occurrences: []schedule.OccurrenceState{occState(oct1, "")}},
		}
		// NextOccurrence reports false for a dateless item, so it never
		// gains an entry in next — the mechanism that exempts it here.
		next := map[string]time.Time{}

		got := overdueOccurrences([]schedule.Item{dateless}, windows, next)
		if date, ok := got["repair"]; ok {
			t.Fatalf("got overdue %v, want none: a dateless item must never read as overdue", date)
		}
	})
}
