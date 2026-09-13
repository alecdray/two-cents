// Package sweep computes the cash-sweep recommendation: the suggested dollar
// amount to move between the user's checking and savings accounts to keep
// checking adequately funded while earning what the rest can in savings.
//
// It places every expected inflow and outflow on a dated **cash-flow timeline**
// covering one month from the run instant, and the amount that must stay in
// checking is the highest point the running total reaches over that window
// ([ADR-0024]). Its inputs are synced balances and the user-declared schedule —
// never a forecast. Each run appends an immutable snapshot, carrying the
// timeline it was computed from, to the sweep_recommendation table; the /sweep
// page reads and navigates that history. Runs come from the scheduled monthly
// job and from the user's on-demand action and are identical — Compute does not
// know its caller, and nothing about the trigger is stored.
//
// It must never import a bank provider. Credit-account balances come from the
// ordinary accounts sync.
package sweep

import (
	"sort"
	"time"

	"github.com/alecdray/two-cents/src/internal/core/timex"
	"github.com/alecdray/two-cents/src/internal/schedule"
)

// RecommendationKind classifies the sweep output: numeric when the computation
// can be completed, needs-attention when a required input is unavailable.
type RecommendationKind string

const (
	// KindNumeric is a fully computed result with its timeline.
	KindNumeric RecommendationKind = "numeric"
	// KindNeedsAttention means the computation cannot proceed — the Reasons field
	// names what is missing.
	KindNeedsAttention RecommendationKind = "needs_attention"
)

// NeedsAttentionReason identifies one cause that prevents a numeric result.
//
// Every blocking reason is a missing *dollar value*. A missing date never
// appears here: it degrades to the worst case instead, so the number still
// forms and is simply more conservative ([ADR-0024]).
type NeedsAttentionReason string

const (
	// ReasonCheckingUndetermined is returned when zero or more than one active
	// cash account with CountsAsSavings=false is found; the checking account
	// cannot be uniquely derived.
	ReasonCheckingUndetermined NeedsAttentionReason = "checking_undetermined"
	// ReasonCheckingBalanceUnknown is returned when the checking account is
	// uniquely identified but the bank has never reported a balance for it.
	ReasonCheckingBalanceUnknown NeedsAttentionReason = "checking_balance_unknown"
	// ReasonCheckingStale is returned when the checking account reports a
	// balance that has gone too long without a confirmed refresh to advise on.
	ReasonCheckingStale NeedsAttentionReason = "checking_stale"
	// ReasonCardBalanceUnknown is returned when an active credit account reports
	// no balance, so what is owed cannot be placed on the timeline.
	ReasonCardBalanceUnknown NeedsAttentionReason = "card_balance_unknown"
	// ReasonCardBalanceStale is returned when an active credit account's balance
	// has gone too long without a confirmed refresh to reserve against.
	ReasonCardBalanceStale NeedsAttentionReason = "card_balance_stale"
)

// SweepDirection is the direction of the suggested transfer.
type SweepDirection string

const (
	// DirectionCheckingToSavings means SuggestedSweep is positive: move money
	// from checking to savings.
	DirectionCheckingToSavings SweepDirection = "checking->savings"
	// DirectionSavingsToChecking means SuggestedSweep is negative: pull money
	// back from savings to checking.
	DirectionSavingsToChecking SweepDirection = "savings->checking"
	// DirectionNone means SuggestedSweep is exactly zero: no transfer needed.
	DirectionNone SweepDirection = "none"
)

// EventDirection is which way one timeline event moves money through checking.
type EventDirection string

const (
	// EventOut is money leaving checking — a bill, a card statement, a standing
	// transfer to savings.
	EventOut EventDirection = "out"
	// EventIn is money arriving in checking.
	EventIn EventDirection = "in"
)

// TimelineEvent is one dated movement through checking on the cash-flow
// timeline. Amount is always a positive magnitude; EventDirection carries the
// sign, so no event can describe two opposite things.
//
// RunningTotal and Peak are filled by evaluate and stored with the snapshot, so
// the page can show the derivation — and name the row that set the figure —
// without recomputing anything ([ADR-0022]).
type TimelineEvent struct {
	Date         time.Time
	Label        string
	Direction    EventDirection
	Amount       float64
	RunningTotal float64
	Peak         bool
}

// Recommendation is the output of the sweep computation. It is either a numeric
// result carrying its timeline (Kind == KindNumeric) or a needs-attention result
// listing the reasons a number cannot be produced (Kind == KindNeedsAttention).
//
// Numeric field semantics (sign convention: outflow positive, inflow negative):
//
//   - CurrentChecking: the checking account's live balance in dollars.
//   - CurrentSavings: the savings account's live balance in dollars. Zero when
//     SavingsUnknown=true; never used in the sweep arithmetic.
//   - SavingsUnknown: true when the savings balance cannot be stood behind —
//     absent, ambiguous, unreported, or stale. The result is still numeric:
//     savings is not a term in the formula, so it never blocks.
//   - Timeline: the dated events the figure was computed from, in the order
//     they were evaluated, each carrying the running total it produced.
//   - RequiredChecking: the highest point that running total reaches over the
//     horizon, floored at zero — what must stay in checking for the balance
//     never to go negative within the window.
//   - FixedSafetyMargin: the flat dollar cushion kept in checking beyond the
//     requirement (default $500, configurable). Headroom, not a term sized to
//     cover an undeclared outflow.
//   - SuggestedSweep: the net dollars to move. Positive → move from checking
//     to savings; negative → pull from savings; zero → no action.
//     SuggestedSweep = CurrentChecking − RequiredChecking − FixedSafetyMargin
//     (not floored — a negative value is a meaningful pull-back signal).
//   - Direction: the transfer direction encoded from the sign of SuggestedSweep.
//   - ComputedAt: the instant the run measured against.
//   - ID: the stored snapshot's identifier; empty until saved.
type Recommendation struct {
	Kind RecommendationKind

	// Numeric fields — meaningful when Kind == KindNumeric.
	CurrentChecking   float64
	CurrentSavings    float64
	SavingsUnknown    bool
	Timeline          []TimelineEvent
	RequiredChecking  float64
	FixedSafetyMargin float64
	SuggestedSweep    float64
	Direction         SweepDirection

	// Needs-attention field — populated when Kind == KindNeedsAttention.
	Reasons []NeedsAttentionReason

	// ComputedAt is the instant the run measured against — the value Compute read
	// from the clock, not the moment the row was written. It is the snapshot's
	// ordering key, its deep-link ordering, its on-screen label, and the start of
	// the horizon its timeline covers, so it has to be the same instant the
	// timeline was built over.
	ComputedAt time.Time

	// ID identifies a stored snapshot, assigned when it is saved. Empty for an
	// in-memory result that has not been persisted.
	ID string
}

// Horizon is the far end of the window this recommendation was computed over:
// exactly one month from the run instant. It is derived rather than stored,
// because it is a pure function of ComputedAt and can never disagree with it.
func (r Recommendation) Horizon() time.Time {
	return horizonFrom(r.ComputedAt)
}

// horizonFrom returns the far end of the sweep window: exactly one month from
// the run instant, rolling rather than to a month boundary, clamped when the day
// does not exist in the target month.
//
// One month is the length at which the window holds exactly one occurrence of
// every monthly item. Lengthening it does not remove truncation — it only moves
// the cut, and moving the cut past an outflow without also passing the income
// that covers it makes the answer worse rather than safer ([ADR-0024]).
func horizonFrom(now time.Time) time.Time {
	return timex.AddMonthsClamped(now, 1)
}

// cardObligation is what one credit Account owes at the run instant, with the
// label to show for it.
type cardObligation struct {
	label   string
	balance float64
}

// timelineInput carries everything the timeline is built from: the window, the
// cards' obligations, and the declared schedule.
type timelineInput struct {
	now     time.Time
	horizon time.Time
	cards   []cardObligation
	items   []schedule.Item
}

// buildTimeline places every expected movement on its date. It is a pure
// function — the caller resolves all live data first.
//
// Both degradations here err toward holding *more* cash, never less, which is
// what makes an incompletely-informed run safe rather than merely tolerable:
// a card with no statement detail has its whole balance fall due immediately,
// and an unmatched past inflow is dropped rather than assumed to have arrived.
func buildTimeline(in timelineInput) []TimelineEvent {
	var events []TimelineEvent

	for _, card := range in.cards {
		// Without statement detail there is a known dollar value with no known
		// date, so the missing-date rule puts the whole balance at the run
		// instant — the most conservative reading, and the reason a bank that
		// reports no statements needs no special case. A card owing nothing is
		// not an obligation and contributes no row.
		if card.balance <= 0 {
			continue
		}
		events = append(events, TimelineEvent{
			Date:      in.now,
			Label:     card.label,
			Direction: EventOut,
			Amount:    card.balance,
		})
	}

	windowStart, windowEnd := occurrenceWindow(in.now, in.horizon)
	for _, item := range in.items {
		// The window looks only forward. Reaching back for an occurrence that has
		// already fallen due is not a wider window — it is a *reconciliation* of
		// what was declared against what actually happened, which is matching's
		// job. Doing it here, with no way to tell a paid occurrence from an unpaid
		// one, would reserve every monthly item twice for the whole month.
		for _, occurrence := range item.Occurrences(windowStart, windowEnd) {
			event := TimelineEvent{
				Date:      occurrence,
				Label:     item.Name,
				Direction: eventDirection(item.Direction),
				Amount:    item.Amount,
			}
			if occurrence.Before(in.now) {
				// An outflow already due is still owed and lands immediately; an
				// inflow already due may never arrive, and omitting it is the same
				// as placing it at the far end of the horizon — the worst case.
				if event.Direction == EventIn {
					continue
				}
				event.Date = in.now
			}
			events = append(events, event)
		}
	}

	return events
}

// occurrenceWindow reduces the run instant and the horizon to the range of
// calendar *dates* an occurrence may fall on: from today through the day before
// the horizon's own date, inclusive.
//
// Both ends matter. Starting at midnight today rather than at the run instant is
// what keeps an item falling due *today* on the timeline — a run is made at some
// hour of the day, and a bill due today is the most urgent one there is. Ending
// the day before the horizon's date is what makes the window hold **exactly one
// occurrence of every monthly item**, which is the property the one-month
// horizon was chosen for: including both the 7th of this month and the 7th of
// next would count a monthly item twice for anyone whose run lands on its day.
func occurrenceWindow(now, horizon time.Time) (start, end time.Time) {
	start = startOfDay(now)
	end = startOfDay(horizon).AddDate(0, 0, -1)
	return start, end
}

// startOfDay reduces an instant to midnight of the same calendar day in its own
// location.
func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// eventDirection maps a declared schedule direction onto a timeline one.
func eventDirection(d schedule.Direction) EventDirection {
	if d == schedule.DirectionIn {
		return EventIn
	}
	return EventOut
}

// evaluate orders the timeline and walks it, returning the required checking
// balance and the events annotated with the running total each produced and
// which one set the figure.
//
// **The maximum, not the final total.** The running total's end value is what
// the month nets out to; its peak is what must be present for the balance never
// to go negative. Taking the peak is also what confines an inflow to offsetting
// only what follows it — summing netted periods would let a paycheck on the 30th
// pay a bill due on the 20th. The property is structural rather than a rule the
// arithmetic has to remember, so do not "simplify" this into a sum.
func evaluate(events []TimelineEvent) (float64, []TimelineEvent) {
	ordered := make([]TimelineEvent, len(events))
	copy(ordered, events)

	// Same-day ordering puts the outflow first: never assume a deposit clears
	// before a debit posted the same day. The sort is stable so a timeline with
	// several same-day, same-direction events evaluates deterministically.
	sort.SliceStable(ordered, func(i, j int) bool {
		if !ordered[i].Date.Equal(ordered[j].Date) {
			return ordered[i].Date.Before(ordered[j].Date)
		}
		return ordered[i].Direction == EventOut && ordered[j].Direction == EventIn
	})

	var running, required float64
	peak := -1
	for i := range ordered {
		if ordered[i].Direction == EventIn {
			running -= ordered[i].Amount
		} else {
			running += ordered[i].Amount
		}
		ordered[i].RunningTotal = running
		ordered[i].Peak = false
		if running > required {
			required = running
			peak = i
		}
	}
	if peak >= 0 {
		ordered[peak].Peak = true
	}

	// required starts at zero and only ever rises, so it is floored there by
	// construction: a timeline that never goes into deficit requires nothing.
	return required, ordered
}

// computeInput carries the pre-fetched figures the sweep arithmetic operates on.
// The Service.Compute method resolves live data and fills this struct; tests
// build it directly to stay off the database.
type computeInput struct {
	// checking is the live checking balance. It is nil whenever any of the
	// blocking flags below is set — there is no figure to compute with.
	checking               *float64
	checkingUndetermined   bool
	checkingBalanceUnknown bool
	checkingStale          bool

	// savingsBalance is shown on the snapshot and never used in the arithmetic.
	// Nil means it cannot be stood behind; savings never blocks a result.
	savingsBalance *float64

	// cards carries one obligation per active credit Account whose balance is
	// known and fresh. A card we cannot stand behind sets a flag instead: the
	// total is load-bearing, so it blocks rather than being silently read as
	// nothing owed.
	cards              []cardObligation
	cardBalanceUnknown bool
	cardBalanceStale   bool

	items             []schedule.Item
	fixedSafetyMargin float64
}

// compute derives the Recommendation from the pre-fetched inputs. It is a pure
// function; all I/O is resolved by the caller before this is invoked.
func compute(in computeInput, now time.Time) Recommendation {
	// Every applicable reason is listed, never just the first: a run that names
	// one problem at a time turns a single fix into several rounds.
	var reasons []NeedsAttentionReason
	if in.checkingUndetermined {
		reasons = append(reasons, ReasonCheckingUndetermined)
	}
	if in.checkingBalanceUnknown {
		reasons = append(reasons, ReasonCheckingBalanceUnknown)
	}
	if in.checkingStale {
		reasons = append(reasons, ReasonCheckingStale)
	}
	if in.cardBalanceUnknown {
		reasons = append(reasons, ReasonCardBalanceUnknown)
	}
	if in.cardBalanceStale {
		reasons = append(reasons, ReasonCardBalanceStale)
	}
	if len(reasons) > 0 {
		return Recommendation{Kind: KindNeedsAttention, Reasons: reasons, ComputedAt: now}
	}

	required, timeline := evaluate(buildTimeline(timelineInput{
		now:     now,
		horizon: horizonFrom(now),
		cards:   in.cards,
		items:   in.items,
	}))

	// Not floored: a negative value is a meaningful pull back from savings.
	suggestedSweep := *in.checking - required - in.fixedSafetyMargin

	direction := DirectionNone
	if suggestedSweep > 0 {
		direction = DirectionCheckingToSavings
	} else if suggestedSweep < 0 {
		direction = DirectionSavingsToChecking
	}

	rec := Recommendation{
		Kind:              KindNumeric,
		ComputedAt:        now,
		CurrentChecking:   *in.checking,
		Timeline:          timeline,
		RequiredChecking:  required,
		FixedSafetyMargin: in.fixedSafetyMargin,
		SuggestedSweep:    suggestedSweep,
		Direction:         direction,
	}
	if in.savingsBalance == nil {
		rec.SavingsUnknown = true
	} else {
		rec.CurrentSavings = *in.savingsBalance
	}
	return rec
}

// Snapshot is one recommendation positioned in the history: the snapshot itself
// plus where the user can step from it. OlderID and NewerID are empty at the
// ends of the timeline, which is how the page knows to omit a step — the control
// is absent, not inert, because there is nowhere to go.
type Snapshot struct {
	Recommendation Recommendation
	OlderID        string
	NewerID        string
}

// neighbors locates id in a newest-first history and reports its neighbours. An
// empty id selects the newest snapshot, which is what a plain page load wants.
// found is false for an empty history and for an id that is not in it — a deep
// link to a snapshot that does not exist must not silently show a different one.
func neighbors(all []Recommendation, id string) (Snapshot, bool) {
	if len(all) == 0 {
		return Snapshot{}, false
	}

	i := 0
	if id != "" {
		i = -1
		for n, rec := range all {
			if rec.ID == id {
				i = n
				break
			}
		}
		if i < 0 {
			return Snapshot{}, false
		}
	}

	// The list runs newest-first, so the older neighbour is the next element and
	// the newer one is the previous.
	snap := Snapshot{Recommendation: all[i]}
	if i+1 < len(all) {
		snap.OlderID = all[i+1].ID
	}
	if i > 0 {
		snap.NewerID = all[i-1].ID
	}
	return snap, true
}
