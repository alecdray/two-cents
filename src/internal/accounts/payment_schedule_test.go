package accounts

import (
	"testing"
	"time"
)

func TestAccountPaymentDate(t *testing.T) {
	loc := time.UTC
	issue := time.Date(2026, time.September, 3, 0, 0, 0, 0, loc)
	due := time.Date(2026, time.September, 28, 0, 0, 0, 0, loc)

	t.Run("defaults to the reported due date", func(t *testing.T) {
		a := Account{Statement: &CardStatement{IssuedAt: &issue, DueAt: &due}}

		got, ok := a.PaymentDate()
		if !ok {
			t.Fatalf("PaymentDate reported unknown, want the due date")
		}
		if !got.Equal(due) {
			t.Errorf("PaymentDate = %s, want the due date %s", got, due)
		}
	})

	t.Run("a card paid a fixed number of days after the statement issues uses that offset", func(t *testing.T) {
		a := Account{
			PaymentSchedule: PaymentSchedule{Mode: PaidStatementPlusDays, OffsetDays: 5},
			Statement:       &CardStatement{IssuedAt: &issue, DueAt: &due},
		}

		got, ok := a.PaymentDate()
		if !ok {
			t.Fatalf("PaymentDate reported unknown, want the offset date")
		}
		if want := issue.AddDate(0, 0, 5); !got.Equal(want) {
			t.Errorf("PaymentDate = %s, want statement issue plus 5 days (%s)", got, want)
		}
	})

	t.Run("an unreported input the mode depends on is unknown, never guessed", func(t *testing.T) {
		noDue := Account{Statement: &CardStatement{IssuedAt: &issue}}
		if _, ok := noDue.PaymentDate(); ok {
			t.Errorf("PaymentDate resolved with no due date reported, want unknown")
		}

		noIssue := Account{
			PaymentSchedule: PaymentSchedule{Mode: PaidStatementPlusDays, OffsetDays: 5},
			Statement:       &CardStatement{DueAt: &due},
		}
		if _, ok := noIssue.PaymentDate(); ok {
			t.Errorf("PaymentDate resolved with no issue date reported, want unknown")
		}
	})

	t.Run("a card with no statement at all is unknown", func(t *testing.T) {
		if _, ok := (Account{}).PaymentDate(); ok {
			t.Errorf("PaymentDate resolved with no statement, want unknown")
		}
	})
}
