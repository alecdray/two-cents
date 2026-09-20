package views

import (
	"time"

	"github.com/alecdray/two-cents/src/internal/schedule"
)

// occurrenceDateFormat is how an occurrence travels in a URL — the same
// unambiguous calendar-date spelling storage uses, so the round trip through the
// address never shifts the day.
const occurrenceDateFormat = "2006-01-02"

// occurrencePath builds the endpoint one decision about an occurrence posts to.
func occurrencePath(itemID string, occurrence time.Time, action string) string {
	return "/sweep/schedule/" + itemID + "/occurrence/" + occurrence.Format(occurrenceDateFormat) + "/" + action
}

// candidateLabel renders a candidate as the three facts that identify it to
// someone who was there: when it left, who it went to, and how much.
func candidateLabel(candidate schedule.Candidate) string {
	return candidate.Date.Format("2 Jan") + " · " + candidate.Merchant + " · " + formatUSD(candidate.Amount)
}

// settledLabel says who decided, because an automatic match is a guess the user
// may not have looked at yet and a confirmed one is not.
func settledLabel(occurrence schedule.OccurrenceState) string {
	if occurrence.Decision == schedule.MatchAuto {
		return "Matched automatically — off the timeline"
	}
	return "You matched this — off the timeline"
}
