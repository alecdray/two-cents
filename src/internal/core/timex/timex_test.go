package timex

import (
	"testing"
	"time"
)

// mustLoad loads an IANA zone for the tests, failing loudly if the host has no
// zoneinfo (the math under test is zone-driven, so a missing zone is a setup
// failure, not a silent pass).
func mustLoad(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	return loc
}

func TestMonthRange(t *testing.T) {
	tests := []struct {
		name      string
		year      int
		month     time.Month
		wantStart time.Time
		wantEnd   time.Time
	}{
		{
			name:      "mid-year June",
			year:      2026,
			month:     time.June,
			wantStart: time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "December wraps to next January",
			year:      2026,
			month:     time.December,
			wantStart: time.Date(2026, time.December, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "February in a leap year ends on March 1",
			year:      2028,
			month:     time.February,
			wantStart: time.Date(2028, time.February, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2028, time.March, 1, 0, 0, 0, 0, time.UTC),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			start, end := MonthRange(tc.year, tc.month)
			if !start.Equal(tc.wantStart) {
				t.Errorf("start = %s, want %s", start, tc.wantStart)
			}
			if !end.Equal(tc.wantEnd) {
				t.Errorf("end = %s, want %s", end, tc.wantEnd)
			}
			// The boundaries must be UTC midnight — a loc-zoned boundary would carry
			// a non-zero offset and mis-bucket calendar-date rows.
			if start.Location() != time.UTC || end.Location() != time.UTC {
				t.Errorf("boundaries must be UTC, got start=%s end=%s", start.Location(), end.Location())
			}
		})
	}
}

// TestMonthRangeBucketsUTCMidnightRows pins the M1 invariant: a transaction
// stored at 2026-06-01T00:00:00Z belongs to June (>= June start, < July start)
// and is NOT in May. A loc-zoned boundary (e.g. America/New_York midnight =
// 04:00Z) would have pushed this row into May.
func TestMonthRangeBucketsUTCMidnightRows(t *testing.T) {
	row := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)

	juneStart, juneEnd := MonthRange(2026, time.June)
	if row.Before(juneStart) || !row.Before(juneEnd) {
		t.Errorf("June-1 UTC-midnight row not in [%s, %s)", juneStart, juneEnd)
	}

	// The same row must NOT fall in May's half-open range: May ends at the June-1
	// boundary, so the row sits exactly on (and excluded by) mayEnd.
	mayStart, mayEnd := MonthRange(2026, time.May)
	inMay := !row.Before(mayStart) && row.Before(mayEnd)
	if inMay {
		t.Errorf("June-1 UTC-midnight row wrongly falls in May [%s, %s)", mayStart, mayEnd)
	}
}

func TestCurrentMonth(t *testing.T) {
	ny := mustLoad(t, "America/New_York")

	tests := []struct {
		name      string
		now       time.Time
		loc       *time.Location
		wantYear  int
		wantMonth time.Month
	}{
		{
			name:      "mid-month in UTC",
			now:       time.Date(2026, time.June, 14, 12, 0, 0, 0, time.UTC),
			loc:       time.UTC,
			wantYear:  2026,
			wantMonth: time.June,
		},
		{
			// 2026-07-01T02:00Z is still June 30 22:00 in New York, so the
			// configured-zone current month is June, not July.
			name:      "instant near month boundary resolves by loc",
			now:       time.Date(2026, time.July, 1, 2, 0, 0, 0, time.UTC),
			loc:       ny,
			wantYear:  2026,
			wantMonth: time.June,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			y, m := CurrentMonth(tc.loc, tc.now)
			if y != tc.wantYear || m != tc.wantMonth {
				t.Errorf("CurrentMonth = %d-%s, want %d-%s", y, m, tc.wantYear, tc.wantMonth)
			}
		})
	}
}

func TestDaysLeftInclusive(t *testing.T) {
	utc := time.UTC
	ny := mustLoad(t, "America/New_York")

	tests := []struct {
		name string
		loc  *time.Location
		now  time.Time
		want int
	}{
		{
			name: "first of a 30-day month counts all 30 days",
			loc:  utc,
			now:  time.Date(2026, time.June, 1, 0, 0, 0, 0, utc),
			want: 30,
		},
		{
			name: "mid-month",
			loc:  utc,
			now:  time.Date(2026, time.June, 14, 9, 30, 0, 0, utc),
			want: 17, // 14..30 inclusive
		},
		{
			name: "last day of the month is 1",
			loc:  utc,
			now:  time.Date(2026, time.June, 30, 23, 59, 0, 0, utc),
			want: 1,
		},
		{
			// March 8, 2026 is the US spring-forward DST transition (a 23h day).
			// The calendar-date difference must still count March 8..31 = 24 days.
			name: "across US spring-forward DST does not miscount",
			loc:  ny,
			now:  time.Date(2026, time.March, 8, 12, 0, 0, 0, ny),
			want: 24,
		},
		{
			// November 1, 2026 is the US fall-back DST transition (a 25h day).
			// November 1..30 inclusive = 30 days.
			name: "across US fall-back DST does not miscount",
			loc:  ny,
			now:  time.Date(2026, time.November, 1, 12, 0, 0, 0, ny),
			want: 30,
		},
		{
			// An instant that is a different calendar date in loc than in UTC: it is
			// still June 30 in New York, so 1 day remains, not 31 (July).
			name: "days-left reckoned in loc, not UTC",
			loc:  ny,
			now:  time.Date(2026, time.July, 1, 2, 0, 0, 0, utc),
			want: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := DaysLeftInclusive(tc.loc, tc.now)
			if got != tc.want {
				t.Errorf("DaysLeftInclusive = %d, want %d", got, tc.want)
			}
		})
	}
}
