package accounts

import "time"

// PaymentScheduleMode is how a card's payment date is decided. Autopay pulls
// when it is configured to, and no bank reports that date, so this is a user
// statement about the card rather than a fact from it ([ADR-0024]).
type PaymentScheduleMode string

const (
	// PaidOnDueDate pays on the reported next payment due date. The default,
	// and the model's one deliberately optimistic assumption: a due date is an
	// upper bound, since autopay can only pull earlier.
	PaidOnDueDate PaymentScheduleMode = "due_date"
	// PaidStatementPlusDays pays a fixed number of days after the statement
	// issues, for a card whose autopay precedes the due date.
	PaidStatementPlusDays PaymentScheduleMode = "statement_plus_days"
)

// PaymentSchedule is when a card is paid. The zero value is the default mode,
// so an account that has never been configured needs no migration backfill.
type PaymentSchedule struct {
	Mode       PaymentScheduleMode
	OffsetDays int
}

// CardStatement is the billing-cycle detail a credit card's bank reports. Each
// field is independently nil when unreported: an unknown must reach the sweep
// as an unknown, where it degrades to the worst case, rather than being
// defaulted into a figure that reads as certain.
type CardStatement struct {
	Balance  *float64
	IssuedAt *time.Time
	DueAt    *time.Time
}

// PaymentDate resolves when this card's statement payment leaves checking,
// reporting false when the inputs its mode depends on are not all reported.
//
// This module owns the rule because it owns the schedule — the sweep asks
// rather than carrying its own copy, the same way it asks about staleness.
func (a Account) PaymentDate() (time.Time, bool) {
	if a.Statement == nil {
		return time.Time{}, false
	}
	if a.PaymentSchedule.Mode == PaidStatementPlusDays {
		if a.Statement.IssuedAt == nil {
			return time.Time{}, false
		}
		return a.Statement.IssuedAt.AddDate(0, 0, a.PaymentSchedule.OffsetDays), true
	}
	if a.Statement.DueAt == nil {
		return time.Time{}, false
	}
	return *a.Statement.DueAt, true
}

// Valid reports whether this schedule is one the model understands. An unknown
// mode or a negative offset is refused rather than stored: a schedule decides
// when real money is expected to leave, and a silently-coerced one would move
// the sweep's number without the user ever saying so.
func (p PaymentSchedule) Valid() bool {
	if p.Mode != PaidOnDueDate && p.Mode != PaidStatementPlusDays {
		return false
	}
	return p.OffsetDays >= 0
}
