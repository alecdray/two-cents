package views

import (
	"github.com/alecdray/two-cents/src/internal/sweep"
)

// sweepActionLine produces the plain-language action sentence for a numeric
// recommendation. It matches the stored direction and headline amount.
func sweepActionLine(rec sweep.Recommendation) string {
	switch rec.Direction {
	case sweep.DirectionCheckingToSavings:
		return "Move " + headlineAmount(rec.SuggestedSweep) + " from checking to savings"
	case sweep.DirectionSavingsToChecking:
		return "Move " + headlineAmount(rec.SuggestedSweep) + " from savings to checking"
	default:
		return "No transfer needed"
	}
}

// sweepSavingsLabel renders the current savings figure, or "unknown" when the
// balance could not be stood behind. Savings is not a term in the arithmetic, so
// not knowing it is a display detail rather than a failure.
func sweepSavingsLabel(rec sweep.Recommendation) string {
	if rec.SavingsUnknown {
		return "unknown"
	}
	return formatUSD(rec.CurrentSavings)
}

// sweepReasonLabel maps a NeedsAttentionReason to a human-readable description.
func sweepReasonLabel(r sweep.NeedsAttentionReason) string {
	switch r {
	case sweep.ReasonCheckingUndetermined:
		return "Checking account cannot be uniquely identified"
	case sweep.ReasonCheckingBalanceUnknown:
		return "Your bank is not reporting a checking balance"
	case sweep.ReasonCheckingStale:
		return "Checking balance has not refreshed recently enough to advise on"
	case sweep.ReasonCardBalanceUnknown:
		return "A card is not reporting its balance, so what you owe cannot be placed"
	case sweep.ReasonCardBalanceStale:
		return "A card balance has not refreshed recently enough to reserve against"
	default:
		return string(r)
	}
}

// sweepSnapshotLabel names the instant a snapshot was computed against. It
// carries the time of day, not just the month: the sweep can be run whenever the
// user wants one, so several snapshots a day is routine and a date alone would
// not tell two of them apart.
func sweepSnapshotLabel(rec sweep.Recommendation) string {
	return rec.ComputedAt.Format("January 2, 2006 · 3:04 PM")
}

// sweepHorizonLabel names the window the timeline covers. The far end is derived
// from the snapshot's own instant, so an old snapshot describes the window it
// actually measured rather than one reckoned from today.
func sweepHorizonLabel(rec sweep.Recommendation) string {
	return "What's coming · through " + rec.Horizon().Format("January 2")
}

// sweepEventDate renders a timeline event's date.
//
// An event dated at the run instant is one that must be covered immediately: a
// card with no statement detail, a bill due today, or — since the occurrence
// window reaches back a cadence — one that fell due earlier and was never
// settled. The last of those is the reason DueOn exists: "at the run instant"
// alone cannot tell a bill due today from one due eleven days ago, and those
// read very differently to someone asking why the peak is where it is.
func sweepEventDate(event sweep.TimelineEvent) string {
	return event.Date.Format("Jan 2")
}

// sweepEventDueQualifier names the date an event was originally due, and only
// when it was moved to the run instant from some other day. It is empty for
// every event sitting on its own date, where it would say nothing.
func sweepEventDueQualifier(event sweep.TimelineEvent) string {
	if event.DueOn.IsZero() {
		return ""
	}
	return "due " + event.DueOn.Format("Jan 2")
}

// sweepEventAmount renders an event's amount in the app-wide outflow-positive
// convention: an outflow reads as a positive draw on checking, an inflow as a
// negative one, so the running-total column adds up on the page exactly as it
// does in the arithmetic.
func sweepEventAmount(event sweep.TimelineEvent) string {
	if event.Direction == sweep.EventIn {
		return formatUSD(-event.Amount)
	}
	return formatUSD(event.Amount)
}

// sweepRowClass highlights the row that set the figure.
func sweepRowClass(event sweep.TimelineEvent) string {
	if event.Peak {
		return "bg-base-200"
	}
	return ""
}
