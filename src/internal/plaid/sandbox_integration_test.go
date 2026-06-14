//go:build sandbox

// This file holds a live integration test against Plaid's Sandbox. It is
// excluded from the normal test build by the `sandbox` build tag and only runs
// with `go test -tags=sandbox`. It needs PLAID_CLIENT_ID and PLAID_SECRET in
// the environment (the matching Sandbox secret) and skips when they are absent.
//
//	set -a && source .env && set +a
//	go test -tags=sandbox ./src/internal/plaid/ -run TestSandbox -v
//
// Its purpose is to confirm the wire structs and conversions in this package
// decode Plaid's *real* Sandbox responses — the recorded fixtures elsewhere in
// the package are hand-built from Plaid's documented schemas, and only a live
// call proves they match reality.
package plaid

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
)

const (
	sandboxOrigin = "https://sandbox.plaid.com"
	// sandboxInstitution is Plaid's standard Sandbox test bank.
	sandboxInstitution = "ins_109508"
)

// sandboxCreds reads the app credentials from the environment, skipping the
// test when either is absent so the suite stays green without secrets.
func sandboxCreds(t *testing.T) (clientID, secret string) {
	t.Helper()
	clientID = os.Getenv("PLAID_CLIENT_ID")
	secret = os.Getenv("PLAID_SECRET")
	if clientID == "" || secret == "" {
		t.Skip("PLAID_CLIENT_ID/PLAID_SECRET not set; skipping live Sandbox test")
	}
	return clientID, secret
}

// bootstrapAccessToken mints a fresh Sandbox Item and returns an access token
// for it, by creating a public token for the test institution and exchanging
// it. These two calls are test scaffolding; the data calls under test go
// through the Service.
func bootstrapAccessToken(t *testing.T, ctx contextx.ContextX, clientID, secret string) string {
	t.Helper()

	var created struct {
		PublicToken string `json:"public_token"`
	}
	postSandbox(t, ctx, clientID, secret, "/sandbox/public_token/create", map[string]any{
		"institution_id":   sandboxInstitution,
		"initial_products": []string{"transactions"},
	}, &created)
	if created.PublicToken == "" {
		t.Fatal("sandbox did not return a public_token")
	}

	var exchanged struct {
		AccessToken string `json:"access_token"`
	}
	postSandbox(t, ctx, clientID, secret, "/item/public_token/exchange", map[string]any{
		"public_token": created.PublicToken,
	}, &exchanged)
	if exchanged.AccessToken == "" {
		t.Fatal("sandbox did not return an access_token")
	}
	return exchanged.AccessToken
}

// postSandbox issues a raw client_id+secret authenticated POST to a Sandbox
// endpoint and decodes the JSON response into out. Used only to bootstrap a
// test Item; it deliberately does not send an access_token.
func postSandbox(t *testing.T, ctx contextx.ContextX, clientID, secret, path string, body map[string]any, out any) {
	t.Helper()
	payload := map[string]any{"client_id": clientID, "secret": secret}
	for k, v := range body {
		payload[k] = v
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshalling %s request: %v", path, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sandboxOrigin+path, strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("building %s request: %v", path, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("calling %s: %v", path, err)
	}
	defer resp.Body.Close()
	msg, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s returned %d: %s", path, resp.StatusCode, string(msg))
	}
	if err := json.Unmarshal(msg, out); err != nil {
		t.Fatalf("decoding %s response: %v", path, err)
	}
}

// TestSandboxLiveProviderRoundTrip links a Sandbox Item and drives the real
// Service against the live Sandbox API, asserting the responses decode into
// sane domain values. A clean run is the evidence that the package's wire
// structs match Plaid's real shapes.
func TestSandboxLiveProviderRoundTrip(t *testing.T) {
	clientID, secret := sandboxCreds(t)
	ctx := testContext()

	accessToken := bootstrapAccessToken(t, ctx, clientID, secret)

	client, err := NewClient(clientID, secret, WithOrigin(sandboxOrigin))
	if err != nil {
		t.Fatalf("building client: %v", err)
	}
	svc := NewService(client)

	t.Run("accounts decode into domain accounts", func(t *testing.T) {
		accounts, err := svc.ListAccounts(ctx, accessToken)
		if err != nil {
			t.Fatalf("listing accounts: %v", err)
		}
		if len(accounts) == 0 {
			t.Fatal("expected at least one account from the sandbox institution")
		}
		sawOther := false
		for _, a := range accounts {
			if a.ID == "" || a.Name == "" {
				t.Errorf("account missing id/name: %+v", a)
			}
			switch a.Kind {
			case banking.KindCash, banking.KindCredit:
			case banking.KindOther:
				sawOther = true
			default:
				t.Errorf("account %q has unexpected kind %q", a.Name, a.Kind)
			}
			if a.Subtype == "" {
				t.Errorf("account %q has an empty subtype label; the bank's subtype must flow through", a.Name)
			}
			if a.Balance.Known && a.Balance.Money.Currency == "" {
				t.Errorf("account %q has a known balance with no currency", a.Name)
			}
			t.Logf("account: name=%q kind=%s type=%q subtype=%q", a.Name, a.Kind, a.Type, a.Subtype)
		}
		// The sandbox institution exposes loan/investment accounts, which map to
		// the other bucket; at least one must classify as other.
		if !sawOther {
			t.Error("expected at least one account classified as the other bucket (loans/investments) from the sandbox institution")
		}
		t.Logf("decoded %d accounts", len(accounts))
	})

	t.Run("balances decode and at least one is known", func(t *testing.T) {
		balances, err := svc.GetBalances(ctx, accessToken)
		if err != nil {
			t.Fatalf("getting balances: %v", err)
		}
		if len(balances) == 0 {
			t.Fatal("expected at least one balance")
		}
		anyKnown := false
		for _, b := range balances {
			if b.AccountID == "" {
				t.Errorf("balance missing account id: %+v", b)
			}
			if b.Known {
				anyKnown = true
				if b.Money.Currency == "" {
					t.Errorf("known balance for %q has no currency", b.AccountID)
				}
			}
		}
		if !anyKnown {
			t.Error("expected at least one known balance from the sandbox institution")
		}
	})

	t.Run("transaction sync decodes and advances the cursor", func(t *testing.T) {
		changes, ok := syncWithRetry(t, svc, ctx, accessToken)
		if !ok {
			t.Skip("sandbox transactions not ready within the retry budget; skipping (not a failure)")
		}
		if changes.Cursor == "" {
			t.Error("expected a non-empty cursor after the first sync")
		}
		for _, txn := range append(changes.Added, changes.Modified...) {
			if txn.ID == "" || txn.AccountID == "" {
				t.Errorf("transaction missing id/account: %+v", txn)
			}
			if txn.Date.IsZero() {
				t.Errorf("transaction %q has a zero date", txn.ID)
			}
			if txn.Amount.Currency == "" {
				t.Errorf("transaction %q has no currency", txn.ID)
			}
		}
		t.Logf("decoded %d added, %d modified, %d removed; cursor=%q",
			len(changes.Added), len(changes.Modified), len(changes.RemovedIDs), changes.Cursor)
	})
}

// syncWithRetry calls the real sync, retrying while Plaid reports the product
// is still preparing (a normal transient state for a freshly linked Sandbox
// Item). It returns ok=false if the data never became ready in the budget.
func syncWithRetry(t *testing.T, svc *Service, ctx contextx.ContextX, accessToken string) (banking.TransactionChanges, bool) {
	t.Helper()
	for attempt := 0; attempt < 8; attempt++ {
		changes, err := svc.SyncTransactions(ctx, accessToken, "")
		if err == nil {
			return changes, true
		}
		if !strings.Contains(err.Error(), "PRODUCT_NOT_READY") {
			t.Fatalf("syncing transactions: %v", err)
		}
		t.Logf("transactions not ready (attempt %d), retrying…", attempt+1)
		time.Sleep(2 * time.Second)
	}
	return banking.TransactionChanges{}, false
}
