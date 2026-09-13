package accounts

import (
	"errors"
	"strings"
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

func TestSyncAccountsClearsAStatementTheBankStopsReporting(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	issued := time.Date(2026, time.September, 3, 0, 0, 0, 0, time.UTC)
	due := time.Date(2026, time.September, 28, 0, 0, 0, 0, time.UTC)
	provider := &fakeProvider{
		accounts: []banking.Account{
			providerAccount("p-card", "Sapphire", banking.KindCredit, false, knownBalance("p-card", 840)),
		},
		statements: []banking.CardStatement{{
			AccountID: "p-card", Known: true,
			Balance:  banking.Money{Amount: 500, Currency: "USD"},
			IssuedAt: &issued, DueAt: &due,
		}},
	}
	svc := NewService(database, provider, testKey)

	conn, err := svc.RegisterConnection(ctx, "tok", "item-123")
	if err != nil {
		t.Fatalf("RegisterConnection: %v", err)
	}
	if err := svc.SyncAccounts(ctx); err != nil {
		t.Fatalf("first SyncAccounts: %v", err)
	}

	// The bank stops reporting this card's cycle. The stored figure is now an
	// arbitrarily old one, and because it caps the card's contribution it would
	// make the sweep hold back LESS than the balance — with nothing on the row
	// to say why. It must not survive a pass that did not report it.
	provider.statements = nil
	if err := svc.SyncAccounts(ctx); err != nil {
		t.Fatalf("second SyncAccounts: %v", err)
	}

	cards, err := svc.repo().ListAccountsByConnection(ctx, conn.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if cards[0].Statement != nil {
		t.Errorf("statement = %+v, want nil — an unreported cycle degrades to the whole balance, not a stale figure", cards[0].Statement)
	}
}

func TestSyncAccountsIsolatesAFailingStatementRead(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	issued := time.Date(2026, time.September, 3, 0, 0, 0, 0, time.UTC)
	due := time.Date(2026, time.September, 28, 0, 0, 0, 0, time.UTC)
	provider := &fakeProvider{
		accounts: []banking.Account{
			providerAccount("p-card", "Sapphire", banking.KindCredit, false, knownBalance("p-card", 840)),
		},
		statements: []banking.CardStatement{{
			AccountID: "p-card", Known: true,
			Balance:  banking.Money{Amount: 500, Currency: "USD"},
			IssuedAt: &issued, DueAt: &due,
		}},
	}
	svc := NewService(database, provider, testKey)

	broken, err := svc.RegisterConnection(ctx, "tok-broken", "item-broken")
	if err != nil {
		t.Fatalf("RegisterConnection broken: %v", err)
	}
	if err := svc.SyncAccounts(ctx); err != nil {
		t.Fatalf("seed sync: %v", err)
	}

	// A second, healthy login. The first one's statement read now fails in a way
	// that is neither a re-auth nor a missing product — a transient provider
	// fault, which must not deny the other connection its refresh.
	healthy, err := svc.RegisterConnection(ctx, "tok-healthy", "item-healthy")
	if err != nil {
		t.Fatalf("RegisterConnection healthy: %v", err)
	}
	// Scoped to the statement read: this login's accounts and balances still
	// serve, so what is being isolated is the statement step itself rather than
	// the whole connection (which the balances-stage test already covers).
	provider.statementErrByToken = map[string]error{"tok-broken": errors.New("provider exploded")}

	syncErr := svc.SyncAccounts(ctx)
	if syncErr == nil {
		t.Fatalf("SyncAccounts = nil, want the failure reported")
	}
	if !strings.Contains(syncErr.Error(), broken.ID) {
		t.Errorf("error %q does not name the failing connection %s", syncErr, broken.ID)
	}

	// The failing login keeps the statement it already had — a stale one beats
	// none, and nothing about this failure says the cycle changed.
	brokenCards, err := svc.repo().ListAccountsByConnection(ctx, broken.ID)
	if err != nil {
		t.Fatalf("list broken: %v", err)
	}
	if brokenCards[0].Statement == nil {
		t.Errorf("a failed read cleared the stored statement, want it left intact")
	}

	// The healthy login still synced.
	healthyCards, err := svc.repo().ListAccountsByConnection(ctx, healthy.ID)
	if err != nil {
		t.Fatalf("list healthy: %v", err)
	}
	if len(healthyCards) == 0 || healthyCards[0].Statement == nil {
		t.Errorf("the healthy connection did not get its statement; one login's failure denied another its refresh")
	}
}
