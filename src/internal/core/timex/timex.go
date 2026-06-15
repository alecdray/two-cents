// Package timex holds the pure month/day time math the budget, tracker, and
// wrap derivations reckon over. It is a stdlib-only leaf (only time) with no
// domain concepts, so it is unit-testable with an injected now.
//
// The central subtlety is that the configured app timezone decides only WHICH
// calendar month "now" falls in (CurrentMonth) and how many days remain in it
// (DaysLeftInclusive). The month-range boundaries themselves (MonthRange) are
// UTC midnight, because transaction dates are stored as zoneless calendar dates
// at midnight UTC; a loc-zoned boundary would mis-bucket a 1st-of-month
// UTC-midnight row into the previous month. So both sides of the range filter
// are compared as calendar dates in one reckoning, and the off-by-one cannot
// happen.
package timex

import "time"

// CurrentMonth returns the calendar year and month of now as observed in loc.
// The location is what makes "the current month" a single configured-zone
// reckoning rather than a server-local or UTC one.
func CurrentMonth(loc *time.Location, now time.Time) (year int, month time.Month) {
	y, m, _ := now.In(loc).Date()
	return y, m
}

// MonthRange returns the half-open [start, end) bounding a calendar month, where
// both bounds are UTC midnight of the 1st of this month and the 1st of next
// month. It deliberately takes no location: transaction dates are stored at
// midnight UTC and must be filtered as calendar dates, so the boundaries are
// UTC-anchored. December rolls over to January of the next year.
func MonthRange(year int, month time.Month) (start, end time.Time) {
	start = time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	end = start.AddDate(0, 1, 0)
	return start, end
}

// DaysLeftInclusive returns the number of calendar days from today through the
// last day of the month, inclusive of today, with now observed in loc. It is a
// calendar-date difference (both dates reduced to UTC midnight before
// differencing, so a daylight-saving transition cannot miscount), never a 24h
// duration division. The result is always at least 1 — on the last day of the
// month, only today remains.
func DaysLeftInclusive(loc *time.Location, now time.Time) int {
	y, m, d := now.In(loc).Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	// The last day of the month is the day before the 1st of next month.
	firstOfNext := time.Date(y, m, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)
	lastDay := firstOfNext.AddDate(0, 0, -1)

	days := int(lastDay.Sub(today).Hours()/24) + 1
	if days < 1 {
		days = 1
	}
	return days
}
