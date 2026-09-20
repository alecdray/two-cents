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
// The module owns the schedule_items and schedule_occurrence_matches tables, the
// pure occurrence projection the sweep consumes, and the reconciliation of a
// declaration against what actually happened. It imports no other module: the
// ledger it reconciles against is reached through the Ledger port it declares
// here, whose adapter lives at the composition root ([ADR-0027]).
package schedule

import (
	"math"
	"time"

	"github.com/alecdray/two-cents/src/internal/core/contextx"
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

// OneCadenceBefore reports where a single cadence interval before t lands: one
// calendar month for a monthly item, fourteen calendar days for a biweekly one.
//
// The sweep decides *how far* to reach back; this answers *where that is*,
// because the length of a cadence is the cadence's own fact and a second copy of
// the arithmetic elsewhere is what would drift. It stays a pure statement about
// the cadence, so it says nothing about whether reaching back is a good idea -
// that remains the consumer's policy.
//
// A monthly step clamps to a month too short to hold the day, so 31 March steps
// back to 28 February rather than overflowing forward into March.
func (i Item) OneCadenceBefore(t time.Time) time.Time {
	if i.Cadence == CadenceBiweekly {
		return t.AddDate(0, 0, -biweeklyStepDays)
	}
	return timex.AddMonthsClamped(t, -1)
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

// MatchSource records who decided an occurrence's match. It is the whole basis
// of precedence: a manual decision is never overwritten by an automatic one,
// the same grain the app already uses for a categorization override.
type MatchSource string

const (

	// MatchManual is the user's own decision — an association they made, or their

	// assertion that nothing satisfied the occurrence.

	MatchManual MatchSource = "manual"

	// MatchAuto is best-effort resolution's decision, re-resolved from scratch on

	// every pass and freely superseded by a later automatic one.

	MatchAuto MatchSource = "auto"
)

// Match is the stored decision about one occurrence, identified by its item and
// the calendar date the occurrence falls on.
//
// TransactionID is empty when the decision is that *nothing* satisfied the
// occurrence — a manual clear. That is a different fact from no record at all:
// the occurrence is still placed on the timeline either way, but a stored clear
// also tells automatic resolution to leave it alone, without which the next pass
// would simply re-make the match the user just rejected.
type Match struct {
	ItemID        string
	Occurrence    time.Time
	TransactionID string
	Source        MatchSource
}

// Settled reports whether this decision takes the occurrence off the timeline.
// A clear does not: the user said nothing satisfied it, so it is still owed.
func (m Match) Settled() bool { return m.TransactionID != "" }

// occurrenceKey renders an occurrence as the calendar date it falls on. An
// occurrence is a day, not an instant, so the key must not carry a zone that
// could shift it across a boundary on the way to or from storage.
func occurrenceKey(occurrence time.Time) string {
	return occurrence.Format("2006-01-02")
}

// SettledSet reports which occurrences carry a match that settles them, keyed by
// item. It deliberately carries no transaction ids: the sweep is told *which*
// occurrences are settled and never *what* settled them, because taking the id
// would hand it a ledger reference it must then be trusted not to follow
// ([ADR-0027]).
type SettledSet struct {
	byItem map[string]map[string]struct{}
}

// Has reports whether the given occurrence of the given item is settled.
func (s SettledSet) Has(itemID string, occurrence time.Time) bool {
	dates, ok := s.byItem[itemID]
	if !ok {
		return false
	}
	_, settled := dates[occurrenceKey(occurrence)]
	return settled
}

// How far from an occurrence a candidate may fall, and still be admissible. Five
// days is comfortably inside the fourteen a biweekly cadence allows, so the
// windows of adjacent occurrences never overlap and a candidate can only be
// near-enough to one of them.
const matchDateToleranceDays = 5

// The narrow amount test: a candidate stands on its amount alone only when it is
// this close to the declared figure. The dollar floor keeps a small declaration
// from being matched by a percentage band too tight to be useful.
const (
	matchAmountTolerancePercent = 0.05

	matchAmountToleranceFloor = 5.00
)

// The wide test, reached only with a distinctive learned merchant: a sanity
// bound rather than a measurement. `Amount` is the *maximum* expected for an
// outflow, so the real figure is routinely well under it, and anything inside
// half-to-double is plausibly the same obligation.
const (
	matchLearnedLowerFactor = 0.5

	matchLearnedUpperFactor = 2.0
)

// Candidate is one checking transaction that could satisfy an occurrence, in
// this module's own vocabulary.
//
// Deliberately not a transactions.Transaction: the adapter at the composition
// root translates into this shape, which is what lets resolution reconcile
// against the ledger without either module importing the other ([ADR-0027]).
// Amount follows the app-wide convention, outflow positive.
type Candidate struct {
	TransactionID string
	Date          time.Time
	Amount        float64
	Merchant      string
}

// Ledger is the port resolution reads the ledger through. Its adapter is the
// only code holding both `accounts` (to identify checking) and `transactions`
// (to query the range), and it lives at the composition root.
type Ledger interface {
	// CheckingCandidates returns the checking transactions dated in [from, to].
	// Scoping to checking is the adapter's job, and it is not an optimisation:
	// the schedule is checking activity by definition, so a match anywhere else
	// would drop an occurrence against money that never left the account the
	// sweep reasons about.
	CheckingCandidates(ctx contextx.ContextX, from, to time.Time) ([]Candidate, error)
	// TransactionsByID returns the given transactions, skipping any that no
	// longer exist. Absence is itself an answer: a provider `removed` deletes
	// rows, so a stored match can be orphaned by an ordinary sync and an id that
	// comes back missing is one whose match must be dropped. The merchant it
	// carries is the other half — the learned signal is read from the
	// transaction behind a match the user already made, which may be older than
	// any resolution window.
	TransactionsByID(ctx contextx.ContextX, ids []string) ([]Candidate, error)
}

// bestCandidate picks the one candidate that satisfies an occurrence, or reports
// false.
//
// Ambiguity declines. Two admissible candidates the tie-breaks cannot separate
// produce no match rather than a guess: a miss leaves the occurrence on the
// timeline and over-reserves, which is visible and correctable, while a false
// match drops an obligation silently and under-reserves - the one direction the
// model forbids.
func bestCandidate(item Item, occurrence time.Time, candidates []Candidate, learnedMerchant, holder string, claimedBy map[string]string) (Candidate, bool) {
	var best Candidate
	var bestFound, tied bool
	for _, c := range candidates {
		if takenElsewhere(claimedBy, c.TransactionID, holder) || !admissible(item, occurrence, c, learnedMerchant) {
			continue
		}
		if !bestFound {
			best, bestFound = c, true
			continue
		}
		switch compareCandidates(item, occurrence, c, best) {
		case -1:
			best, tied = c, false
		case 0:
			tied = true
		}
	}
	if !bestFound || tied {
		return Candidate{}, false
	}
	return best, true
}

// occurrenceHolder names the occurrence a transaction is bound to.
func occurrenceHolder(itemID string, occurrence time.Time) string {
	return itemID + "|" + occurrenceKey(occurrence)
}

// takenElsewhere reports whether a transaction is already settling a *different*
// occurrence than the one being scored.
func takenElsewhere(claimedBy map[string]string, transactionID, holder string) bool {
	owner, taken := claimedBy[transactionID]
	return taken && owner != holder
}

// admissible applies every criterion a candidate must clear.
func admissible(item Item, occurrence time.Time, c Candidate, learnedMerchant string) bool {
	if !directionAgrees(item, c) {
		return false
	}
	if daysApart(c.Date, occurrence) > matchDateToleranceDays {
		return false
	}
	return amountAdmissible(item, c, learnedMerchant)
}

// directionAgrees tests the sign, in the app-wide outflow-positive convention. A
// zero-amount candidate agrees with nothing.
func directionAgrees(item Item, c Candidate) bool {
	if item.Direction == DirectionOut {
		return c.Amount > 0
	}
	return c.Amount < 0
}

// amountAdmissible runs the two tests an amount may clear.
//
// The learned test exists because `Amount` is the *maximum* expected for an
// outflow: a $200 declaration against a $63 bill is the declaration working
// exactly as intended, and no amount-only rule accepts it without accepting far
// too much. A merchant the user has already confirmed for this item is evidence
// of a different kind - but only while it is distinctive, which is the caller's
// to establish (see learnedMerchants).
func amountAdmissible(item Item, c Candidate, learnedMerchant string) bool {
	actual := math.Abs(c.Amount)
	if learnedMerchant != "" && c.Merchant == learnedMerchant {
		return actual >= item.Amount*matchLearnedLowerFactor && actual <= item.Amount*matchLearnedUpperFactor
	}
	tolerance := math.Max(item.Amount*matchAmountTolerancePercent, matchAmountToleranceFloor)
	return math.Abs(actual-item.Amount) <= tolerance
}

// compareCandidates orders two admissible candidates: nearer the occurrence
// wins, then nearer the declared amount. It reports 0 when nothing separates
// them, which is what makes the caller decline.
func compareCandidates(item Item, occurrence time.Time, a, b Candidate) int {
	switch da, db := daysApart(a.Date, occurrence), daysApart(b.Date, occurrence); {
	case da < db:
		return -1
	case da > db:
		return 1
	}
	switch aa, ab := math.Abs(math.Abs(a.Amount)-item.Amount), math.Abs(math.Abs(b.Amount)-item.Amount); {
	case aa < ab:
		return -1
	case aa > ab:
		return 1
	}
	return 0
}

// daysApart counts whole calendar days between two dates, either direction.
func daysApart(a, b time.Time) int {
	diff := startOfDay(a).Sub(startOfDay(b)).Hours() / 24
	return int(math.Abs(math.Round(diff)))
}

// startOfDay reduces an instant to the calendar day it falls on. An occurrence
// is a day, not a moment, and every comparison here is between days.
func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// earliestLookback is the start of the widest window any item looks back over.
// Each item's lookback is its own cadence interval, so the set's earliest is
// what a single range query has to reach to cover all of them.
func earliestLookback(items []Item, today time.Time) time.Time {
	from := today
	for _, item := range items {
		if start := item.OneCadenceBefore(today); start.Before(from) {
			from = start
		}
	}
	return from
}

// OccurrenceState is one dated instance of a declared item together with
// whatever has been decided about it — the read model the matching surface
// renders.
//
// The three states are the three the record can hold: nothing decided, settled
// by a transaction, or manually cleared. Only the middle one leaves the
// timeline.
type OccurrenceState struct {
	Occurrence time.Time
	// Decision is the source of the stored decision, empty when there is none.
	Decision MatchSource
	// SettledBy is the transaction the decision points at, empty for a clear.
	// What the decision *says* is separate from what can be shown of it: between
	// a provider `removed` and the sync that drops the match, this still names a
	// row the ledger no longer holds.
	SettledBy string
	// Transaction is that row, carried for display and zero when the ledger no
	// longer holds it — the user confirms a match against what they can see of
	// it, not against an id.
	Transaction Candidate
	// Candidates are the rows this occurrence could be settled by, filled only
	// while it is unsettled. A settled occurrence offers none: the way to change
	// one is to clear it first, which is a decision the user states rather than
	// one a stray click makes.
	Candidates OccurrenceCandidates
}

// Settled reports whether a transaction satisfied this occurrence.
func (o OccurrenceState) Settled() bool { return o.SettledBy != "" }

// Cleared reports the user's assertion that nothing satisfied this occurrence.
// It is a decision, not the absence of one, which is why it is distinguishable
// from an outstanding occurrence.
func (o OccurrenceState) Cleared() bool {
	return o.Decision == MatchManual && o.SettledBy == ""
}

// Outstanding reports an occurrence nothing has been decided about — the one
// state that offers candidates.
func (o OccurrenceState) Outstanding() bool { return o.Decision == "" }

// ItemOccurrences is one declared item beside the occurrences of it that fall in
// the matching window.
type ItemOccurrences struct {
	Item        Item
	Occurrences []OccurrenceState
}

// offerContext is everything the candidate split needs, read once: the checking
// rows across the widest window any item looks back over, which of them are
// already spoken for, and each item's learned merchant.
type offerContext struct {
	rows    []Candidate
	claimed map[string]bool
	learned map[string]string
}

// offer splits the read rows for one occurrence.
func (c offerContext) offer(item Item, occurrence time.Time) OccurrenceCandidates {
	var offered OccurrenceCandidates
	for _, row := range c.rows {
		if c.claimed[row.TransactionID] {
			continue
		}
		if admissible(item, occurrence, row, c.learned[item.ID]) {
			offered.Admissible = append(offered.Admissible, row)
			continue
		}
		offered.Other = append(offered.Other, row)
	}
	return offered
}

// OccurrenceCandidates is what an outstanding occurrence offers the user: the
// rows resolution judged admissible, and every other checking row it could have
// been.
//
// The split is presentational, not a rule. Manual association is the guaranteed
// path and may point at any checking row — but the admissible set is usually
// right, so it leads.
type OccurrenceCandidates struct {
	Admissible []Candidate
	Other      []Candidate
}
