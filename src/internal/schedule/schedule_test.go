package schedule

import (
	"testing"
	"time"
)

// nyc is the kind of location the app actually reckons in — a zone with DST, so
// a biweekly step that crossed a transition would show up here rather than
// passing silently under UTC.
func nyc(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	return loc
}

func day(loc *time.Location, y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
}

func dates(times []time.Time) []string {
	out := make([]string, len(times))
	for i, tm := range times {
		out[i] = tm.Format("2006-01-02")
	}
	return out
}

func equalDates(got []time.Time, want []string) bool {
	g := dates(got)
	if len(g) != len(want) {
		return false
	}
	for i := range g {
		if g[i] != want[i] {
			return false
		}
	}
	return true
}

func TestOccurrences(t *testing.T) {
	loc := nyc(t)

	tests := []struct {
		name string
		item Item
		from time.Time
		to   time.Time
		want []string
	}{
		{
			name: "monthly falls once in a one-month window",
			item: Item{Cadence: CadenceMonthly, DayOfMonth: 15},
			from: day(loc, 2026, time.September, 7),
			to:   day(loc, 2026, time.October, 7),
			want: []string{"2026-09-15"},
		},
		{
			name: "monthly on the window's first day is included",
			item: Item{Cadence: CadenceMonthly, DayOfMonth: 7},
			from: day(loc, 2026, time.September, 7),
			to:   day(loc, 2026, time.October, 7),
			want: []string{"2026-09-07", "2026-10-07"},
		},
		{
			name: "monthly day 31 clamps to the last day of a short month",
			item: Item{Cadence: CadenceMonthly, DayOfMonth: 31},
			from: day(loc, 2026, time.April, 1),
			to:   day(loc, 2026, time.April, 30),
			want: []string{"2026-04-30"},
		},
		{
			name: "monthly day 31 clamps to February in a non-leap year",
			item: Item{Cadence: CadenceMonthly, DayOfMonth: 31},
			from: day(loc, 2026, time.February, 1),
			to:   day(loc, 2026, time.February, 28),
			want: []string{"2026-02-28"},
		},
		{
			name: "monthly outside the window contributes nothing",
			item: Item{Cadence: CadenceMonthly, DayOfMonth: 20},
			from: day(loc, 2026, time.September, 1),
			to:   day(loc, 2026, time.September, 10),
			want: nil,
		},
		{
			name: "biweekly steps forward from an anchor in the past",
			item: Item{Cadence: CadenceBiweekly, AnchorDate: day(loc, 2026, time.January, 9)},
			from: day(loc, 2026, time.September, 7),
			to:   day(loc, 2026, time.October, 7),
			want: []string{"2026-09-18", "2026-10-02"},
		},
		{
			name: "biweekly steps backward from an anchor in the future",
			item: Item{Cadence: CadenceBiweekly, AnchorDate: day(loc, 2027, time.January, 1)},
			from: day(loc, 2026, time.September, 7),
			to:   day(loc, 2026, time.October, 7),
			want: []string{"2026-09-11", "2026-09-25"},
		},
		{
			name: "biweekly crossing a DST transition keeps landing on calendar days",
			item: Item{Cadence: CadenceBiweekly, AnchorDate: day(loc, 2026, time.October, 25)},
			from: day(loc, 2026, time.October, 25),
			to:   day(loc, 2026, time.December, 6),
			want: []string{"2026-10-25", "2026-11-08", "2026-11-22", "2026-12-06"},
		},
		{
			name: "an anchor inside the window is itself an occurrence",
			item: Item{Cadence: CadenceBiweekly, AnchorDate: day(loc, 2026, time.September, 10)},
			from: day(loc, 2026, time.September, 7),
			to:   day(loc, 2026, time.September, 30),
			want: []string{"2026-09-10", "2026-09-24"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.item.Occurrences(tc.from, tc.to)
			if !equalDates(got, tc.want) {
				t.Errorf("Occurrences = %v, want %v", dates(got), tc.want)
			}
		})
	}
}
