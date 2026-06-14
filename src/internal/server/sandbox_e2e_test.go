//go:build sandbox

// This file holds a live integration test that drives the real wired backend
// stack against Plaid's Sandbox: the plaid client through the banking seam,
// into the accounts service, through cryptox and the sqlc-backed SQLite DB, and
// out to the overview. It is excluded from the normal build by the `sandbox`
// build tag and only runs with `go test -tags=sandbox`. It needs
// PLAID_CLIENT_ID and PLAID_SECRET in the environment (the matching Sandbox
// secret) and skips when they are absent, so the default suite stays hermetic.
//
//	set -a && source .env && set +a
//	go test -tags=sandbox ./src/internal/server/ -run TestSandbox -v -count=1 -timeout 180s
//
// It lives in package server — the composition root — because that is the only
// non-plaid package allowed to import plaid (the provider-isolation test exempts
// it). Putting it in package accounts would break that seam.
package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alecdray/two-cents/src/internal/accounts"
	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/cryptox"
	"github.com/alecdray/two-cents/src/internal/core/db"
	"github.com/alecdray/two-cents/src/internal/plaid"

	"github.com/pressly/goose/v3"

	_ "github.com/mattn/go-sqlite3"
)

const (
	sandboxOrigin = "https://sandbox.plaid.com"
	// sandboxInstitution is Plaid's standard Sandbox test bank.
	sandboxInstitution = "ins_109508"
	// sandboxEncryptionKey is a valid 32-byte (AES-256) hex key. It is set in the
	// test rather than read from the environment, since the repo's .env does not
	// carry one.
	sandboxEncryptionKey = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
)

// sandboxCreds reads the app credentials from the environment, skipping the
// test when either is absent so the suite stays green without secrets. The
// values are never logged.
func sandboxCreds(t *testing.T) (clientID, secret string) {
	t.Helper()
	clientID = os.Getenv("PLAID_CLIENT_ID")
	secret = os.Getenv("PLAID_SECRET")
	if clientID == "" || secret == "" {
		t.Skip("PLAID_CLIENT_ID/PLAID_SECRET not set; skipping live Sandbox test")
	}
	return clientID, secret
}

// newSandboxDB opens a fresh temp-file SQLite database with the production
// migrations applied, returning the wrapped *db.DB the accounts service expects.
func newSandboxDB(t *testing.T) *db.DB {
	t.Helper()

	migrationsDir, err := filepath.Abs("../../../db/migrations")
	if err != nil {
		t.Fatalf("resolve migrations dir: %v", err)
	}

	dbPath := filepath.Join(t.TempDir(), "sandbox.db")
	sqlDB, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("set dialect: %v", err)
	}
	if err := goose.Up(sqlDB, migrationsDir); err != nil {
		t.Fatalf("goose up: %v", err)
	}
	return db.WrapSqlDB(sqlDB)
}

// postSandbox issues a raw client_id+secret authenticated POST to a Sandbox
// endpoint and decodes the JSON response into out. Used only to bootstrap and
// manipulate a test Item (mint, exchange, reset_login); it deliberately does
// not go through the plaid Client. Credentials are never logged.
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
		// The error body can echo back the request; scrub the credentials before
		// surfacing it so they never reach the test log.
		safe := strings.NewReplacer(clientID, "<client_id>", secret, "<secret>").Replace(string(msg))
		t.Fatalf("%s returned %d: %s", path, resp.StatusCode, safe)
	}
	if out != nil {
		if err := json.Unmarshal(msg, out); err != nil {
			t.Fatalf("decoding %s response: %v", path, err)
		}
	}
}

// bootstrapAccessToken mints a fresh Sandbox Item and returns an access token
// for it, by creating a public token for the test institution and exchanging
// it. These two calls are test scaffolding; the data calls under test go
// through the wired stack.
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
		ItemID      string `json:"item_id"`
	}
	postSandbox(t, ctx, clientID, secret, "/item/public_token/exchange", map[string]any{
		"public_token": created.PublicToken,
	}, &exchanged)
	if exchanged.AccessToken == "" {
		t.Fatal("sandbox did not return an access_token")
	}
	return exchanged.AccessToken
}

// TestSandboxConnectSyncOverviewAndReconnect drives the whole wired backend
// against live Plaid Sandbox data: it mints a real Item, registers a connection
// (persisting an encrypted token and the accounts to a real SQLite DB),
// refreshes balances via sync, derives the overview, and finally forces a
// login-required state to prove the connection flips to needs-reconnect.
func TestSandboxConnectSyncOverviewAndReconnect(t *testing.T) {
	clientID, secret := sandboxCreds(t)
	ctx := contextx.NewContextX(context.Background())

	accessToken := bootstrapAccessToken(t, ctx, clientID, secret)

	// Wire the system like production but pointed at sandbox: plaid client ->
	// banking seam (plaid.Service) -> accounts service over a real migrated DB.
	client, err := plaid.NewClient(clientID, secret, plaid.WithOrigin(sandboxOrigin))
	if err != nil {
		t.Fatalf("building plaid client: %v", err)
	}
	bankProvider := plaid.NewService(client)

	database := newSandboxDB(t)
	svc := accounts.NewService(database, bankProvider, sandboxEncryptionKey)

	// The Sandbox item id is not needed to drive the stack; a placeholder keeps
	// the provider item id column populated. The provider item id is opaque to
	// the accounts service.
	const providerItemID = "sandbox-item"

	// --- 1+2. Register the connection: persists a Connection + Accounts to the
	// real DB, each with a sane kind + balance, and an encrypted access token. ---
	conn, err := svc.RegisterConnection(ctx, accessToken, providerItemID)
	if err != nil {
		t.Fatalf("RegisterConnection over live sandbox: %v", err)
	}
	if conn.ID == "" {
		t.Fatal("RegisterConnection returned a connection with no id")
	}

	registered, err := accounts.NewRepo(database.Queries()).ListAccountsByConnection(ctx, conn.ID)
	if err != nil {
		t.Fatalf("list accounts after register: %v", err)
	}
	if len(registered) == 0 {
		t.Fatal("expected at least one account persisted from the sandbox institution")
	}
	for _, a := range registered {
		if a.Name == "" || a.ProviderAccountID == "" {
			t.Errorf("persisted account missing name/provider id: %+v", a)
		}
		if a.Kind != banking.KindCash && a.Kind != banking.KindCredit {
			t.Errorf("account %q has unexpected kind %q", a.Name, a.Kind)
		}
		if a.Balance.Known && a.Balance.Money.Currency == "" {
			t.Errorf("account %q has a known balance with no currency", a.Name)
		}
		t.Logf("account: name=%q kind=%s known=%t balance=%.2f %s",
			a.Name, a.Kind, a.Balance.Known, a.Balance.Money.Amount, a.Balance.Money.Currency)
	}

	// The stored access_token column must be ciphertext (not the plaintext), yet
	// decrypt back to the original under the config key.
	var rawToken string
	row := database.Sql().QueryRow("SELECT access_token FROM connections WHERE id = ?", conn.ID)
	if err := row.Scan(&rawToken); err != nil {
		t.Fatalf("read raw access_token: %v", err)
	}
	if rawToken == "" {
		t.Fatal("raw stored access_token is empty")
	}
	if rawToken == accessToken {
		t.Fatal("raw stored access_token is the plaintext token; it must be encrypted at rest")
	}
	decrypted, err := cryptox.SymmetricDecrypt(rawToken, sandboxEncryptionKey)
	if err != nil {
		t.Fatalf("stored token does not decrypt under the config key: %v", err)
	}
	if decrypted != accessToken {
		t.Error("decrypted stored token does not match the original access token")
	}
	t.Logf("connection persisted: id=%s state=%s accounts=%d; stored token is encrypted and round-trips",
		conn.ID, conn.State, len(registered))

	// --- 3. SyncAccounts: balances refresh with no error, accounts still present.
	// Sandbox products can be briefly not-ready; retry on PRODUCT_NOT_READY. ---
	syncWithRetry(t, svc, ctx)

	synced, err := accounts.NewRepo(database.Queries()).ListAccountsByConnection(ctx, conn.ID)
	if err != nil {
		t.Fatalf("list accounts after sync: %v", err)
	}
	if len(synced) != len(registered) {
		t.Errorf("account count changed across sync: registered=%d synced=%d", len(registered), len(synced))
	}
	knownAfterSync := 0
	for _, a := range synced {
		if a.LastSyncedAt == nil {
			t.Errorf("account %q has no last-synced timestamp after sync", a.Name)
		}
		if a.Balance.Known {
			knownAfterSync++
		}
	}
	t.Logf("after sync: %d accounts present, %d with a known balance", len(synced), knownAfterSync)

	// --- 4. Overview: total cash / credit debt / net cash from the real
	// balances, internally consistent (net == cash - debt). ---
	ov, err := svc.Overview(ctx)
	if err != nil {
		t.Fatalf("Overview: %v", err)
	}
	if ov.TotalCash < 0 {
		t.Errorf("total cash is negative: %.2f", ov.TotalCash)
	}
	if ov.TotalDebt < 0 {
		t.Errorf("total debt is negative: %.2f", ov.TotalDebt)
	}
	if ov.NetCash != ov.TotalCash-ov.TotalDebt {
		t.Errorf("net cash not internally consistent: net=%.2f cash=%.2f debt=%.2f", ov.NetCash, ov.TotalCash, ov.TotalDebt)
	}
	if knownAfterSync > 0 && ov.Currency == "" {
		t.Error("overview has no currency despite known balances")
	}
	t.Logf("overview: total cash=%.2f, credit debt=%.2f, net cash=%.2f %s",
		ov.TotalCash, ov.TotalDebt, ov.NetCash, ov.Currency)

	// --- 5. Reconnect path: force ITEM_LOGIN_REQUIRED via reset_login, then sync
	// again and assert the connection transitions to needs-reconnect. This is the
	// live proof that plaid maps ITEM_LOGIN_REQUIRED -> banking.ErrReauthRequired
	// -> the accounts needs-reconnect flip. The sync should fail fast (no retry). ---
	postSandbox(t, ctx, clientID, secret, "/sandbox/item/reset_login", map[string]any{
		"access_token": accessToken,
	}, nil)

	if err := svc.SyncAccounts(ctx); err != nil {
		t.Fatalf("SyncAccounts after reset_login returned an error; expected a clean needs-reconnect transition: %v", err)
	}

	after, err := accounts.NewRepo(database.Queries()).ListConnections(ctx)
	if err != nil {
		t.Fatalf("list connections after reset_login sync: %v", err)
	}
	var found bool
	for _, c := range after {
		if c.ID != conn.ID {
			continue
		}
		found = true
		if c.State != accounts.ConnectionNeedsReconnect {
			t.Errorf("connection state = %q after reset_login, want %q", c.State, accounts.ConnectionNeedsReconnect)
		}
		t.Logf("after reset_login sync: connection state=%s (needs-reconnect as expected)", c.State)
	}
	if !found {
		t.Fatalf("connection %s missing after reset_login sync", conn.ID)
	}
}

// syncWithRetry calls the real sync through the wired stack, retrying while
// Plaid reports the product is still preparing (a normal transient state for a
// freshly linked Sandbox Item). It fails the test if sync never succeeds within
// the budget, since a clean sync is required for the rest of the assertions.
func syncWithRetry(t *testing.T, svc *accounts.Service, ctx contextx.ContextX) {
	t.Helper()
	for attempt := 0; attempt < 10; attempt++ {
		err := svc.SyncAccounts(ctx)
		if err == nil {
			return
		}
		if !strings.Contains(err.Error(), "PRODUCT_NOT_READY") {
			t.Fatalf("SyncAccounts: %v", err)
		}
		t.Logf("sandbox product not ready (attempt %d), retrying…", attempt+1)
		time.Sleep(2 * time.Second)
	}
	t.Fatal("sandbox product never became ready within the retry budget")
}
