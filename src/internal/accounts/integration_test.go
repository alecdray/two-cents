package accounts

import (
	"testing"

	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/core/cryptox"
)

// These tests exercise the assembled accounts module end-to-end: the real
// Service over a real (migrated, in-memory-file) SQLite DB through the real
// repo and a fake banking.BankProvider. They demonstrate whole-product
// properties that span registration, sync, the overview derivation, and
// token-at-rest protection — the helpers (newTestDB, testCtx, knownBalance,
// providerAccount, fakeProvider, testKey) are shared with service_test.go.

// closeAccount marks a stored account closed by direct repo manipulation —
// there is no service operation to close/hide an account yet, so we set the
// state through the repo to assemble the mixed-state fixture the overview must
// filter over.
func setAccountState(t *testing.T, svc *Service, account Account, state AccountState) {
	t.Helper()
	account.State = state
	if _, err := svc.repo().UpdateAccount(testCtx(), account); err != nil {
		t.Fatalf("set account %q state to %q: %v", account.ProviderAccountID, state, err)
	}
}

// TestNetCashExcludesHiddenClosedAndUnknown registers one connection whose
// accounts span all three kinds (cash, credit, other), a savings flag, and an
// unknown balance, then hides one account and closes another through the repo.
// It asserts each account is persisted with its provider subtype as the
// bank_type label, that the other-bucket account is still stored and listed,
// and that Overview — served from the real DB — totals cash (savings included)
// and credit debt and reports net cash = cash − debt over only the active,
// known-balance, non-other accounts.
func TestNetCashExcludesHiddenClosedAndUnknown(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	provider := &fakeProvider{accounts: []banking.Account{
		providerLabelledAccount("p-check", "Checking", banking.KindCash, "depository", "checking", false, knownBalance("p-check", 1000)),
		providerLabelledAccount("p-save", "Savings", banking.KindCash, "depository", "savings", true, knownBalance("p-save", 1500)),
		providerLabelledAccount("p-card", "Credit Card", banking.KindCredit, "credit", "credit card", false, knownBalance("p-card", 400)),
		// An other-bucket account (a known-balance mortgage): it must be stored
		// and listed, but excluded from the overview totals.
		providerLabelledAccount("p-loan", "Mortgage", banking.KindOther, "loan", "mortgage", false, knownBalance("p-loan", 250000)),
		// Unknown balance carrying a non-zero stale amount: it must be excluded
		// entirely. A non-zero amount makes "excluded" distinguishable from
		// "counted as zero" — if the code trusted the amount despite Known being
		// false, the totals below would shift by 7777.
		providerLabelledAccount("p-unknown", "Investment", banking.KindOther, "investment", "401k", false, banking.Balance{AccountID: "p-unknown", Known: false, Money: banking.Money{Amount: 7777, Currency: "USD"}}),
		// Will be hidden after registration.
		providerLabelledAccount("p-hidden", "Old Checking", banking.KindCash, "depository", "checking", false, knownBalance("p-hidden", 9999)),
		// Will be closed after registration.
		providerLabelledAccount("p-closed", "Old Card", banking.KindCredit, "credit", "credit card", false, knownBalance("p-closed", 8888)),
	}}

	svc := NewService(database, provider, testKey)
	conn, err := svc.RegisterConnection(ctx, "tok", "item-overview")
	if err != nil {
		t.Fatalf("RegisterConnection: %v", err)
	}

	accounts, err := svc.repo().ListAccountsByConnection(ctx, conn.ID)
	if err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	byProvider := map[string]Account{}
	for _, a := range accounts {
		byProvider[a.ProviderAccountID] = a
	}

	t.Run("each account stores its kind and the provider subtype as the bank_type label", func(t *testing.T) {
		cases := []struct {
			providerID   string
			wantKind     banking.AccountKind
			wantBankType string
		}{
			{"p-check", banking.KindCash, "checking"},
			{"p-save", banking.KindCash, "savings"},
			{"p-card", banking.KindCredit, "credit card"},
			{"p-loan", banking.KindOther, "mortgage"},
		}
		for _, c := range cases {
			a := byProvider[c.providerID]
			if a.Kind != c.wantKind {
				t.Errorf("%s kind = %q, want %q", c.providerID, a.Kind, c.wantKind)
			}
			if a.BankType != c.wantBankType {
				t.Errorf("%s bank_type label = %q, want %q (the provider subtype)", c.providerID, a.BankType, c.wantBankType)
			}
		}
	})

	t.Run("the other-bucket account is persisted and returned by the listing", func(t *testing.T) {
		loan, ok := byProvider["p-loan"]
		if !ok {
			t.Fatalf("other-bucket account p-loan was not persisted/listed")
		}
		if loan.Kind != banking.KindOther {
			t.Errorf("p-loan kind = %q, want other", loan.Kind)
		}
	})

	setAccountState(t, svc, byProvider["p-hidden"], AccountHidden)
	setAccountState(t, svc, byProvider["p-closed"], AccountClosed)

	ov, err := svc.Overview(ctx)
	if err != nil {
		t.Fatalf("Overview: %v", err)
	}

	// Cash = checking 1000 + savings 1500 = 2500 (hidden 9999 excluded,
	// unknown excluded — never zero, other-bucket loan excluded).
	if ov.TotalCash != 2500 {
		t.Errorf("total cash = %v, want 2500 (checking 1000 + savings 1500)", ov.TotalCash)
	}
	// Debt = card 400 (closed 8888 excluded).
	if ov.TotalDebt != 400 {
		t.Errorf("total debt = %v, want 400 (only the active card)", ov.TotalDebt)
	}
	if ov.NetCash != 2100 {
		t.Errorf("net cash = %v, want 2100 (2500 - 400); the other-bucket mortgage must be excluded", ov.NetCash)
	}
	if ov.Currency != "USD" {
		t.Errorf("currency = %q, want USD", ov.Currency)
	}
}

// TestResyncIsIdempotent runs SyncAccounts twice over unchanged provider data
// and asserts the assembled state is identical after the second run as after
// the first: no duplicate accounts and no reseed of the seeded (non-overridden)
// fields. A user override applied between the runs must also survive untouched.
func TestResyncIsIdempotent(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	provider := &fakeProvider{accounts: []banking.Account{
		providerAccount("p-check", "Checking", banking.KindCash, false, knownBalance("p-check", 500)),
		providerAccount("p-save", "Savings", banking.KindCash, true, knownBalance("p-save", 2000)),
		providerAccount("p-card", "Credit Card", banking.KindCredit, false, knownBalance("p-card", 300)),
	}}

	svc := NewService(database, provider, testKey)
	conn, err := svc.RegisterConnection(ctx, "tok", "item-sync")
	if err != nil {
		t.Fatalf("RegisterConnection: %v", err)
	}

	// User overrides checking's kind/savings so the no-reseed guarantee is
	// observable across both syncs.
	pre, err := svc.repo().ListAccountsByConnection(ctx, conn.ID)
	if err != nil {
		t.Fatalf("list pre: %v", err)
	}
	for _, a := range pre {
		if a.ProviderAccountID == "p-check" {
			a.Kind = banking.KindCredit
			a.KindOverridden = true
			a.CountsAsSavings = true
			a.SavingsOverridden = true
			if _, err := svc.repo().UpdateAccount(ctx, a); err != nil {
				t.Fatalf("override checking: %v", err)
			}
		}
	}

	// First sync over unchanged provider data.
	if err := svc.SyncAccounts(ctx); err != nil {
		t.Fatalf("SyncAccounts (first): %v", err)
	}
	first := snapshotByProvider(t, svc, conn.ID)

	// Second sync over the same unchanged provider data must be a no-op for the
	// seeded fields and must not create duplicates.
	if err := svc.SyncAccounts(ctx); err != nil {
		t.Fatalf("SyncAccounts (second): %v", err)
	}
	second := snapshotByProvider(t, svc, conn.ID)

	if len(first) != 3 {
		t.Fatalf("expected 3 accounts after first sync, got %d", len(first))
	}
	if len(second) != len(first) {
		t.Fatalf("account count changed across resync: first=%d second=%d (duplicates created)", len(first), len(second))
	}

	for providerID, a1 := range first {
		a2, ok := second[providerID]
		if !ok {
			t.Errorf("account %q present after first sync but missing after second", providerID)
			continue
		}
		// Identity must be stable — no delete+recreate.
		if a1.ID != a2.ID {
			t.Errorf("account %q row identity changed across resync: %q -> %q", providerID, a1.ID, a2.ID)
		}
		// Seeded / non-overridden fields must be identical (no reseed).
		if a1.Kind != a2.Kind {
			t.Errorf("account %q kind reseeded across resync: %q -> %q", providerID, a1.Kind, a2.Kind)
		}
		if a1.KindOverridden != a2.KindOverridden {
			t.Errorf("account %q kind-overridden flag changed: %v -> %v", providerID, a1.KindOverridden, a2.KindOverridden)
		}
		if a1.CountsAsSavings != a2.CountsAsSavings {
			t.Errorf("account %q counts-as-savings reseeded: %v -> %v", providerID, a1.CountsAsSavings, a2.CountsAsSavings)
		}
		if a1.SavingsOverridden != a2.SavingsOverridden {
			t.Errorf("account %q savings-overridden flag changed: %v -> %v", providerID, a1.SavingsOverridden, a2.SavingsOverridden)
		}
		if a1.Balance != a2.Balance {
			t.Errorf("account %q balance changed across resync of unchanged data: %+v -> %+v", providerID, a1.Balance, a2.Balance)
		}
	}

	// The user override specifically survived both syncs intact.
	check := second["p-check"]
	if check.Kind != banking.KindCredit || !check.CountsAsSavings || !check.KindOverridden || !check.SavingsOverridden {
		t.Errorf("user override was reseeded across resync: %+v", check)
	}
}

func snapshotByProvider(t *testing.T, svc *Service, connID string) map[string]Account {
	t.Helper()
	accounts, err := svc.repo().ListAccountsByConnection(testCtx(), connID)
	if err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	out := make(map[string]Account, len(accounts))
	for _, a := range accounts {
		out[a.ProviderAccountID] = a
	}
	return out
}

// TestStoredTokenIsEncrypted registers a connection and reads the raw
// access_token column straight out of the DB, asserting it is not the plaintext
// token. It then drives a sync through the service path (which decrypts the
// token to call the provider) and confirms the round trip works — proving the
// stored ciphertext decrypts back to the original the service needs.
func TestStoredTokenIsEncrypted(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	const accessToken = "super-secret-access-token-value"
	provider := &fakeProvider{accounts: []banking.Account{
		providerAccount("p-check", "Checking", banking.KindCash, false, knownBalance("p-check", 500)),
	}}

	svc := NewService(database, provider, testKey)
	conn, err := svc.RegisterConnection(ctx, accessToken, "item-token")
	if err != nil {
		t.Fatalf("RegisterConnection: %v", err)
	}

	// Read the RAW column bytes directly from the DB — bypass the repo so we see
	// exactly what is persisted at rest.
	var rawToken string
	row := database.Sql().QueryRow("SELECT access_token FROM connections WHERE id = ?", conn.ID)
	if err := row.Scan(&rawToken); err != nil {
		t.Fatalf("read raw access_token: %v", err)
	}

	if rawToken == accessToken {
		t.Fatalf("raw stored access_token is the plaintext token; it must be encrypted at rest")
	}
	if rawToken == "" {
		t.Fatalf("raw stored access_token is empty")
	}

	// The persisted value must decrypt back to the original under the config key.
	decrypted, err := cryptox.SymmetricDecrypt(rawToken, testKey)
	if err != nil {
		t.Fatalf("stored token does not decrypt under the config key: %v", err)
	}
	if decrypted != accessToken {
		t.Errorf("decrypted token = %q, want original %q", decrypted, accessToken)
	}

	// Observe the round trip through the live service flow: sync decrypts the
	// stored token and calls the provider with it. Assert the provider was
	// invoked with the original plaintext.
	provider.lastAccessToken = ""
	if err := svc.SyncAccounts(ctx); err != nil {
		t.Fatalf("SyncAccounts: %v", err)
	}
	if provider.lastAccessToken != accessToken {
		t.Errorf("service called the provider with token %q, want the decrypted original %q", provider.lastAccessToken, accessToken)
	}
}
