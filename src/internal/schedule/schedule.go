// Package schedule owns the sweep schedule: the user-declared recurring
// checking activity — bills, income, and standing transfers to savings — that
// the cash-flow timeline is built from ([ADR-0024]).
//
// It is the only source of dated checking activity. Card spending is never
// declared here; it reaches the timeline as a statement on its due date. Only
// genuinely scheduled movements are declared, never intentions: an aspiration on
// a timeline of dated facts would have the sweep hold money back from savings so
// the user could move it to savings.
//
// The module owns the schedule_items table and the pure occurrence projection
// the sweep consumes. It reads no other module.
package schedule

import (
	"time"

	"github.com/alecdray/two-cents/src/internal/core/timex"
)

// Direction is which way a scheduled item moves money through checking, in the
// app-wide outflow-positive sign convention.
type Direction string

const (
	// DirectionOut is a bill, a card payment, or a standing transfer to savings
	// — money leaving checking.
	DirectionOut Direction = "out"
	// DirectionIn is income — money arriving in checking.
	DirectionIn Direction = "in"
)

// Cadence is how often a scheduled item recurs. Only these two exist: not
// semi-monthly, not weekly.
type Cadence string

const (
	// CadenceMonthly recurs on one day of each month, clamped to the last day in
	// a month too short to hold it.
	CadenceMonthly Cadence = "monthly"
	// CadenceBiweekly recurs every 14 days from an anchor date.
	CadenceBiweekly Cadence = "biweekly"
)

// biweeklyStepDays is the fixed interval of a biweekly item. It steps in
// calendar days rather than hours so an occurrence stays on its weekday across a
// DST transition.
const biweeklyStepDays = 14

// Item is one declared recurring movement through checking — collectively, the
// sweep schedule.
//
// Amount is the **conservative** figure: the maximum expected for an outflow,
// the minimum expected for an inflow. It is named for its meaning rather than
// its arithmetic because the safe direction flips with the sign — over-stating a
// bill holds extra cash, while over-stating a paycheck discounts real debt
// against money that may not arrive. A field called "maximum" would invite
// someone to later "fix" income to use an average, which is the one direction
// that makes the sweep unsafe.
//
// DayOfMonth applies to CadenceMonthly and AnchorDate to CadenceBiweekly; the
// other is zero. An inactive Item is kept but leaves the timeline.
type Item struct {
	ID         string
	Name       string
	Direction  Direction
	Amount     float64
	Cadence    Cadence
	DayOfMonth int
	AnchorDate time.Time
	Active     bool
}

// Occurrences projects the dated instances of the Item falling in [from, to],
// inclusive at both ends. Dates come back at midnight in from's location and in
// ascending order.
//
// It is a pure projection of the declaration: it says nothing about whether an
// occurrence has already happened or already been paid. Placing an occurrence on
// the timeline — or dropping it — is the sweep's decision, not this one's.
func (i Item) Occurrences(from, to time.Time) []time.Time {
	if to.Before(from) {
		return nil
	}
	switch i.Cadence {
	case CadenceMonthly:
		return i.monthlyOccurrences(from, to)
	case CadenceBiweekly:
		return i.biweeklyOccurrences(from, to)
	default:
		return nil
	}
}

// monthlyOccurrences walks the calendar months the window touches and places the
// item's day in each, clamped to the last day of a month too short to hold it —
// so a 31st item falls on the 30th in April and the 28th (or 29th) in February.
func (i Item) monthlyOccurrences(from, to time.Time) []time.Time {
	if i.DayOfMonth < 1 {
		return nil
	}
	loc := from.Location()

	var out []time.Time
	// Start at the first of the window's opening month and step month by month
	// until past the window. Stepping the 1st avoids AddDate's day overflow,
	// which would turn 31 January into 3 March.
	cursor := time.Date(from.Year(), from.Month(), 1, 0, 0, 0, 0, loc)
	for !cursor.After(to) {
		day := i.DayOfMonth
		if last := timex.DaysInMonth(cursor.Year(), cursor.Month()); day > last {
			day = last
		}
		occurrence := time.Date(cursor.Year(), cursor.Month(), day, 0, 0, 0, 0, loc)
		if !occurrence.Before(from) && !occurrence.After(to) {
			out = append(out, occurrence)
		}
		cursor = cursor.AddDate(0, 1, 0)
	}
	return out
}

// biweeklyOccurrences steps 14 days at a time from the anchor, in whichever
// direction the window lies — the anchor is any occurrence, past or future, not
// a start date.
func (i Item) biweeklyOccurrences(from, to time.Time) []time.Time {
	if i.AnchorDate.IsZero() {
		return nil
	}
	loc := from.Location()
	// Re-read the anchor as a calendar date in the window's zone: it is a day the
	// money moves, not an instant, so a stored UTC midnight must not shift it a
	// day when the app timezone is behind UTC.
	y, m, d := i.AnchorDate.Date()
	cursor := time.Date(y, m, d, 0, 0, 0, 0, loc)

	for cursor.After(from) {
		cursor = cursor.AddDate(0, 0, -biweeklyStepDays)
	}
	for cursor.Before(from) {
		cursor = cursor.AddDate(0, 0, biweeklyStepDays)
	}

	var out []time.Time
	for !cursor.After(to) {
		out = append(out, cursor)
		cursor = cursor.AddDate(0, 0, biweeklyStepDays)
	}
	return out
}
