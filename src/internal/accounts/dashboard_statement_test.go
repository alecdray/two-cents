package accounts

import (
	"testing"
	"time"

	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
)

func TestDashboardSurfacesCardStatementFacts(t *testing.T) {
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
	cardID := cards[0].ID

	t.Run("a card carries its payment schedule so the row can offer the control", func(t *testing.T) {
		if err := svc.SetPaymentSchedule(ctx, cardID, PaymentSchedule{Mode: PaidStatementPlusDays, OffsetDays: 3}); err != nil {
			t.Fatalf("SetPaymentSchedule: %v", err)
		}
		row := cardRow(t, svc, ctx, cardID)
		if row.PaymentSchedule.Mode != PaidStatementPlusDays || row.PaymentSchedule.OffsetDays != 3 {
			t.Errorf("row schedule = %+v, want the stored statement-plus-3-days", row.PaymentSchedule)
		}
	})

	t.Run("a card whose login serves no statements says so on the row", func(t *testing.T) {
		if err := svc.repo().SetAccountStatementsUnavailable(ctx, cardID, true); err != nil {
			t.Fatalf("mark unavailable: %v", err)
		}
		row := cardRow(t, svc, ctx, cardID)
		if !row.StatementsUnavailable {
			t.Errorf("row does not surface the statement gap, want it beside the card it affects")
		}
		if row.NeedsReconnect {
			t.Errorf("row is flagged needs-reconnect; a missing product must not read as a broken login")
		}
	})

	t.Run("a statement paid down shows what the card will actually take", func(t *testing.T) {
		billed := 500.0
		paidDownTo := 120.0
		if err := svc.repo().SetAccountStatementsUnavailable(ctx, cardID, false); err != nil {
			t.Fatalf("clear: %v", err)
		}
		due := time.Now().AddDate(0, 0, 12)
		if _, err := svc.repo().SetAccountStatement(ctx, cardID, &CardStatement{Balance: &billed, DueAt: &due}); err != nil {
			t.Fatalf("set statement: %v", err)
		}
		// The balance has been paid down below what was billed. The sweep caps
		// the obligation at the balance, so the row must say the same thing —
		// one definition of what this card will take, not two.
		acct, err := svc.repo().GetAccount(ctx, cardID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		acct.Balance.Money.Amount = paidDownTo
		if _, err := svc.repo().UpdateAccount(ctx, acct); err != nil {
			t.Fatalf("update balance: %v", err)
		}

		row := cardRow(t, svc, ctx, cardID)
		if row.StatementBilled == nil || *row.StatementBilled != paidDownTo {
			t.Errorf("row billed = %v, want the capped %v the sweep reserves", row.StatementBilled, paidDownTo)
		}
	})

	t.Run("a card shows when its next payment is expected to leave", func(t *testing.T) {
		due := time.Now().AddDate(0, 0, 12)
		billed := 500.0
		if err := svc.repo().SetAccountStatementsUnavailable(ctx, cardID, false); err != nil {
			t.Fatalf("clear: %v", err)
		}
		// Restore a balance above the billed figure; the subtest above paid it
		// down, and the row now caps what it shows at the balance.
		acct, err := svc.repo().GetAccount(ctx, cardID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		acct.Balance.Money.Amount = 840
		if _, err := svc.repo().UpdateAccount(ctx, acct); err != nil {
			t.Fatalf("restore balance: %v", err)
		}
		if err := svc.SetPaymentSchedule(ctx, cardID, PaymentSchedule{Mode: PaidOnDueDate}); err != nil {
			t.Fatalf("SetPaymentSchedule: %v", err)
		}
		if _, err := svc.repo().SetAccountStatement(ctx, cardID, &CardStatement{Balance: &billed, DueAt: &due}); err != nil {
			t.Fatalf("set statement: %v", err)
		}
		row := cardRow(t, svc, ctx, cardID)
		if row.StatementBilled == nil || *row.StatementBilled != 500 {
			t.Errorf("row billed = %v, want 500", row.StatementBilled)
		}
		if row.PaymentDue == nil || !row.PaymentDue.Equal(due) {
			t.Errorf("row payment due = %v, want the resolved %s", row.PaymentDue, due)
		}
	})
}

func cardRow(t *testing.T, svc *Service, ctx contextx.ContextX, id string) AccountRow {
	t.Helper()
	dash, err := svc.Dashboard(ctx)
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	for _, r := range append(append([]AccountRow{}, dash.Cash...), dash.Credit...) {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("card row %s not found in dashboard", id)
	return AccountRow{}
}
