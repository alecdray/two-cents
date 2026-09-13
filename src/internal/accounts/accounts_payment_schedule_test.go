package accounts

import (
	"testing"
	"time"

	"github.com/alecdray/two-cents/src/internal/banking"
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

func TestSetPaymentSchedule(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	provider := &fakeProvider{accounts: []banking.Account{
		providerAccount("p-card", "Sapphire", banking.KindCredit, false, knownBalance("p-card", 840)),
	}}
	svc := NewService(database, provider, testKey)

	conn, err := svc.RegisterConnection(ctx, "tok", "item-123")
	if err != nil {
		t.Fatalf("RegisterConnection: %v", err)
	}
	cards, err := svc.repo().ListAccountsByConnection(ctx, conn.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	card := cards[0]

	t.Run("a new card defaults to paying on the due date", func(t *testing.T) {
		if card.PaymentSchedule.Mode != PaidOnDueDate {
			t.Errorf("mode = %q, want the due-date default with no configuration", card.PaymentSchedule.Mode)
		}
	})

	t.Run("setting a statement-plus-days schedule stores the offset", func(t *testing.T) {
		if err := svc.SetPaymentSchedule(ctx, card.ID, PaymentSchedule{Mode: PaidStatementPlusDays, OffsetDays: 5}); err != nil {
			t.Fatalf("SetPaymentSchedule: %v", err)
		}
		got, err := svc.repo().GetAccount(ctx, card.ID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.PaymentSchedule.Mode != PaidStatementPlusDays || got.PaymentSchedule.OffsetDays != 5 {
			t.Errorf("schedule = %+v, want statement plus 5 days", got.PaymentSchedule)
		}
	})

	t.Run("a sync never overwrites the user's schedule", func(t *testing.T) {
		if err := svc.SyncAccounts(ctx); err != nil {
			t.Fatalf("SyncAccounts: %v", err)
		}
		got, err := svc.repo().GetAccount(ctx, card.ID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.PaymentSchedule.Mode != PaidStatementPlusDays || got.PaymentSchedule.OffsetDays != 5 {
			t.Errorf("schedule = %+v after sync, want the user's choice untouched", got.PaymentSchedule)
		}
	})

	t.Run("an unknown mode is refused rather than stored", func(t *testing.T) {
		if err := svc.SetPaymentSchedule(ctx, card.ID, PaymentSchedule{Mode: "whenever"}); err == nil {
			t.Errorf("SetPaymentSchedule accepted an unknown mode, want a rejection")
		}
	})

	t.Run("a negative offset is refused", func(t *testing.T) {
		if err := svc.SetPaymentSchedule(ctx, card.ID, PaymentSchedule{Mode: PaidStatementPlusDays, OffsetDays: -1}); err == nil {
			t.Errorf("SetPaymentSchedule accepted a negative offset, want a rejection")
		}
	})
}
