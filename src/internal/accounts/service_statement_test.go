package accounts

import (
	"testing"
	"time"

	"github.com/alecdray/two-cents/src/internal/banking"
)

func TestSyncAccountsStoresCardStatements(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	issued := time.Date(2026, time.September, 3, 0, 0, 0, 0, time.UTC)
	due := time.Date(2026, time.September, 28, 0, 0, 0, 0, time.UTC)

	provider := &fakeProvider{
		accounts: []banking.Account{
			providerAccount("p-card", "Sapphire", banking.KindCredit, false, knownBalance("p-card", 840)),
		},
		statements: []banking.CardStatement{{
			AccountID: "p-card",
			Known:     true,
			Balance:   banking.Money{Amount: 500, Currency: "USD"},
			IssuedAt:  &issued,
			DueAt:     &due,
		}},
	}
	svc := NewService(database, provider, testKey)

	conn, err := svc.RegisterConnection(ctx, "tok", "item-123")
	if err != nil {
		t.Fatalf("RegisterConnection: %v", err)
	}
	if err := svc.SyncAccounts(ctx); err != nil {
		t.Fatalf("SyncAccounts: %v", err)
	}

	got, err := svc.repo().ListAccountsByConnection(ctx, conn.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("accounts = %d, want 1", len(got))
	}
	card := got[0]
	if card.Statement == nil {
		t.Fatalf("card holds no statement, want the reported one")
	}
	if card.Statement.Balance == nil || *card.Statement.Balance != 500 {
		t.Errorf("statement balance = %v, want 500", card.Statement.Balance)
	}
	if card.Statement.DueAt == nil || !card.Statement.DueAt.Equal(due) {
		t.Errorf("statement due = %v, want %s", card.Statement.DueAt, due)
	}
}

func TestSyncAccountsSkipsStatementsForCashOnlyConnections(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	provider := &fakeProvider{accounts: []banking.Account{
		providerAccount("p-check", "Checking", banking.KindCash, false, knownBalance("p-check", 500)),
	}}
	svc := NewService(database, provider, testKey)

	if _, err := svc.RegisterConnection(ctx, "tok", "item-123"); err != nil {
		t.Fatalf("RegisterConnection: %v", err)
	}
	if err := svc.SyncAccounts(ctx); err != nil {
		t.Fatalf("SyncAccounts: %v", err)
	}

	if provider.statementCalls != 0 {
		t.Errorf("GetCardStatements called %d times for a cash-only connection, want 0", provider.statementCalls)
	}
}

func TestSyncAccountsRecordsStatementsUnavailableWithoutBreakingTheConnection(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	provider := &fakeProvider{
		accounts: []banking.Account{
			providerAccount("p-card", "Sapphire", banking.KindCredit, false, knownBalance("p-card", 840)),
		},
		statementErr: banking.ErrStatementsUnavailable,
	}
	svc := NewService(database, provider, testKey)

	conn, err := svc.RegisterConnection(ctx, "tok", "item-123")
	if err != nil {
		t.Fatalf("RegisterConnection: %v", err)
	}

	// A login that serves balances but not statements is not a failed sync.
	if err := svc.SyncAccounts(ctx); err != nil {
		t.Fatalf("SyncAccounts = %v, want nil — the connection served everything it could", err)
	}

	got, err := svc.repo().GetConnection(ctx, conn.ID)
	if err != nil {
		t.Fatalf("GetConnection: %v", err)
	}
	if got.State != ConnectionActive {
		t.Errorf("connection state = %s, want active — a missing product is not a broken login", got.State)
	}

	cards, err := svc.repo().ListAccountsByConnection(ctx, conn.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !cards[0].StatementsUnavailable {
		t.Errorf("card does not record that statements are unavailable, want the per-card fact")
	}
}

func TestSyncAccountsClearsStatementsUnavailableOnceServed(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	provider := &fakeProvider{
		accounts: []banking.Account{
			providerAccount("p-card", "Sapphire", banking.KindCredit, false, knownBalance("p-card", 840)),
		},
		statementErr: banking.ErrStatementsUnavailable,
	}
	svc := NewService(database, provider, testKey)

	conn, err := svc.RegisterConnection(ctx, "tok", "item-123")
	if err != nil {
		t.Fatalf("RegisterConnection: %v", err)
	}
	if err := svc.SyncAccounts(ctx); err != nil {
		t.Fatalf("first SyncAccounts: %v", err)
	}

	// Consent granted: the next pass serves a statement.
	billed := 500.0
	provider.statementErr = nil
	provider.statements = []banking.CardStatement{{AccountID: "p-card", Known: true, Balance: banking.Money{Amount: billed, Currency: "USD"}}}
	if err := svc.SyncAccounts(ctx); err != nil {
		t.Fatalf("second SyncAccounts: %v", err)
	}

	cards, err := svc.repo().ListAccountsByConnection(ctx, conn.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if cards[0].StatementsUnavailable {
		t.Errorf("card still records statements unavailable after one was served")
	}
}
