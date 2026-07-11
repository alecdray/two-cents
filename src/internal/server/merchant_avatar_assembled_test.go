package server_test

// Assembled tests for the leading transaction-row avatar. They drive the real
// sync + render path through transactions.NewService and the transactions HTTP
// handlers, so they prove the emergent behaviour that only holds once the
// merchant-logo cache, the render, and the image endpoint are composed — not the
// unit behaviour each module already covers on its own.

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alecdray/two-cents/src/internal/accounts"
	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/categorization"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/db"
	"github.com/alecdray/two-cents/src/internal/plaid"
	"github.com/alecdray/two-cents/src/internal/transactions"
	txnAdapters "github.com/alecdray/two-cents/src/internal/transactions/adapters"
)

// mapLogoFetcher is a network-free logo fetcher: it returns canned raster bytes
// for URLs it knows and a no-logo result for anything else, standing in for the
// SSRF-constrained fetcher wired at the composition root.
type mapLogoFetcher struct {
	bytesByURL map[string][]byte
}

func (f *mapLogoFetcher) FetchLogo(_ contextx.ContextX, logoURL string) ([]byte, string, error) {
	if b, ok := f.bytesByURL[logoURL]; ok {
		return b, "image/png", nil
	}
	return nil, "", nil
}

// failingLogoFetcher errors on every URL, standing in for a bank CDN that is down
// or slow — the warm step must swallow this and never touch a row or balance.
type failingLogoFetcher struct{ attempts int }

func (f *failingLogoFetcher) FetchLogo(_ contextx.ContextX, _ string) ([]byte, string, error) {
	f.attempts++
	return nil, "", errFetchDown
}

var errFetchDown = &fetchError{}

type fetchError struct{}

func (*fetchError) Error() string { return "simulated logo fetch failure" }

// forbiddenLogoFetcher fails the test if it is ever asked to fetch — a render must
// read the cache only and never reach out for a logo.
type forbiddenLogoFetcher struct {
	t     *testing.T
	calls int
}

func (f *forbiddenLogoFetcher) FetchLogo(_ contextx.ContextX, logoURL string) ([]byte, string, error) {
	f.calls++
	f.t.Errorf("rendering a transaction row fetched a logo (%q); the render must read local cache only", logoURL)
	return nil, "", nil
}

// logoKeyOf recomputes the cache key the pipeline serves under: the hex SHA-256 of
// the logo URL. Computed here so a test can address the image endpoint by key.
func logoKeyOf(logoURL string) string {
	sum := sha256.Sum256([]byte(logoURL))
	return hex.EncodeToString(sum[:])
}

// logoTxn is a bank transaction on the fixed checking account, optionally carrying
// a merchant logo URL. Distinct days keep the rendered order deterministic.
func logoTxn(id, merchant string, day int, amount float64, logoURL string) banking.Transaction {
	return banking.Transaction{
		ID:        id,
		AccountID: "p-check",
		Date:      time.Date(2026, 6, day, 0, 0, 0, 0, time.UTC),
		Amount:    banking.Money{Amount: amount, Currency: "USD"},
		Merchant:  merchant,
		LogoURL:   logoURL,
	}
}

// setupAvatarScenario connects one bank whose backfill is the given transactions,
// runs one sync through the given logo fetcher, and returns the wired services so a
// test can render the list or hit the image endpoint over the resulting cache.
func setupAvatarScenario(t *testing.T, fetcher transactions.LogoFetcher, txns []banking.Transaction) (*db.DB, *recordingProvider, *accounts.Service, *transactions.Service, *categorization.Service) {
	t.Helper()
	database := newTestDB(t)
	ctx := testCtx()

	provider := &recordingProvider{
		accounts:     []banking.Account{cashProviderAccount("p-check", "Everyday Checking")},
		transactions: txns,
	}
	accountsSvc := accounts.NewService(database, provider, testKey)
	categorizationSvc := categorization.NewService(database, nil)
	txnSvc := transactions.NewService(database, provider, accountsSvc, categorizationSvc, fetcher)

	if _, err := accountsSvc.RegisterConnection(ctx, "access-token", "item-id"); err != nil {
		t.Fatalf("RegisterConnection: %v", err)
	}
	if err := txnSvc.SyncTransactions(ctx); err != nil {
		t.Fatalf("SyncTransactions: %v", err)
	}
	return database, provider, accountsSvc, txnSvc, categorizationSvc
}

// TestEveryRenderedRowShowsExactlyOneAvatarNeverBlank drives the real /transactions
// render over a mixed set — one merchant with a positively cached logo, one with no
// logo URL, one whose logo the fetcher refused — and proves every row carries
// exactly one avatar: the cached merchant shows its served image, the others fall
// back to the category glyph, and no row is ever blank.
func TestEveryRenderedRowShowsExactlyOneAvatarNeverBlank(t *testing.T) {
	const cachedURL = "https://cdn.example/cached.png"
	const refusedURL = "https://cdn.example/refused.png"
	fetcher := &mapLogoFetcher{bytesByURL: map[string][]byte{cachedURL: []byte("\x89PNG cached")}}

	txns := []banking.Transaction{
		logoTxn("t-cached", "Acme Coffee", 3, 12.50, cachedURL),
		logoTxn("t-nologo", "Corner Bodega", 2, 8.00, ""),
		logoTxn("t-refused", "Sketchy Shop", 1, 5.00, refusedURL),
	}

	_, _, accountsSvc, txnSvc, categorizationSvc := setupAvatarScenario(t, fetcher, txns)

	code, body := getPage(t, txnSvc, accountsSvc, categorizationSvc)
	if code != http.StatusOK {
		t.Fatalf("render status = %d, want 200", code)
	}

	rows := strings.Count(body, `data-testid="transactions-row"`)
	if rows != 3 {
		t.Fatalf("rendered %d rows, want 3; the read setup is wrong", rows)
	}
	// The container div carries data-testid="merchant-avatar" (the image/glyph
	// children use suffixed testids), so this counts one avatar per row.
	if avatars := strings.Count(body, `data-testid="merchant-avatar"`); avatars != rows {
		t.Errorf("rendered %d avatars for %d rows; every row must render exactly one avatar", avatars, rows)
	}
	images := strings.Count(body, `data-testid="merchant-avatar-image"`)
	glyphs := strings.Count(body, `data-testid="merchant-avatar-glyph"`)
	if images != 1 {
		t.Errorf("rendered %d merchant images, want 1 (only the cached merchant)", images)
	}
	if glyphs != 2 {
		t.Errorf("rendered %d glyphs, want 2 (the no-logo and refused rows)", glyphs)
	}
	// Exactly one avatar per row, and each is either an image or a glyph — never blank.
	if images+glyphs != rows {
		t.Errorf("image(%d)+glyph(%d) != rows(%d); a row rendered a blank avatar", images, glyphs, rows)
	}

	served := transactions.MerchantLogoRoutePrefix + logoKeyOf(cachedURL)
	if !strings.Contains(body, `src="`+served+`"`) {
		t.Errorf("cached merchant image src is not the served origin path %q:\n%s", served, body)
	}
}

// TestRenderedLogoAvatarsStayOnOurOwnOrigin proves the privacy property of the
// assembled render: a cached row's image is served from our own origin, and the
// rendered markup names no bank CDN host, so a browser painting the list never
// issues a logo request to a third party.
func TestRenderedLogoAvatarsStayOnOurOwnOrigin(t *testing.T) {
	const cachedURL = "https://cdn.example/logo.png"
	fetcher := &mapLogoFetcher{bytesByURL: map[string][]byte{cachedURL: []byte("\x89PNG")}}
	txns := []banking.Transaction{logoTxn("t-cached", "Acme", 1, 9.0, cachedURL)}

	_, _, accountsSvc, txnSvc, categorizationSvc := setupAvatarScenario(t, fetcher, txns)

	_, body := getPage(t, txnSvc, accountsSvc, categorizationSvc)

	served := transactions.MerchantLogoRoutePrefix + logoKeyOf(cachedURL)
	if !strings.HasPrefix(served, "/") {
		t.Fatalf("served logo URL %q is not origin-relative", served)
	}
	if !strings.Contains(body, `src="`+served+`"`) {
		t.Fatalf("cached row did not render the origin-served image %q", served)
	}
	if strings.Contains(body, "cdn.example") {
		t.Errorf("rendered markup references the bank CDN host; a cached logo must be served from our own origin only:\n%s", body)
	}
}

// TestRenderingRowsNeverFetchesALogo proves the render is cache-only: after a sync
// has warmed the cache, a second service wired to a fetcher that fails the test if
// called renders the same list successfully — the cached image still appears and
// the fetcher is never touched.
func TestRenderingRowsNeverFetchesALogo(t *testing.T) {
	const cachedURL = "https://cdn.example/cached.png"
	warmFetcher := &mapLogoFetcher{bytesByURL: map[string][]byte{cachedURL: []byte("\x89PNG")}}
	txns := []banking.Transaction{
		logoTxn("t-cached", "Acme", 2, 12.0, cachedURL),
		logoTxn("t-nologo", "Bodega", 1, 4.0, ""),
	}

	database, provider, accountsSvc, _, categorizationSvc := setupAvatarScenario(t, warmFetcher, txns)

	// A fresh service over the same warmed cache whose fetcher must never be invoked
	// during a render. It is never synced — only used to serve the page.
	forbid := &forbiddenLogoFetcher{t: t}
	renderSvc := transactions.NewService(database, provider, accountsSvc, categorizationSvc, forbid)

	code, body := getPage(t, renderSvc, accountsSvc, categorizationSvc)
	if code != http.StatusOK {
		t.Fatalf("render status = %d, want 200 (a render must succeed without any fetch)", code)
	}
	if forbid.calls != 0 {
		t.Errorf("render invoked the logo fetcher %d time(s); it must read the cache only", forbid.calls)
	}
	served := transactions.MerchantLogoRoutePrefix + logoKeyOf(cachedURL)
	if !strings.Contains(body, `src="`+served+`"`) {
		t.Errorf("cache-only render dropped the already-cached image %q", served)
	}
}

// TestFailingLogoFetchLeavesRowsAndBalancesUnchanged proves the logo warm step is
// decoupled from sync correctness at the assembled level: syncing identical bank
// data through a fetcher that errors on every URL succeeds and writes exactly the
// same transactions and account balance as syncing with no fetcher at all.
func TestFailingLogoFetchLeavesRowsAndBalancesUnchanged(t *testing.T) {
	const logoURL = "https://cdn.example/logo.png"
	data := func() []banking.Transaction {
		return []banking.Transaction{
			logoTxn("t1", "Acme", 2, 84.32, logoURL),
			logoTxn("t2", "Payroll", 1, -2400.00, logoURL),
		}
	}

	// Baseline: no fetcher, so nothing is ever fetched.
	baseDB, _, _, _, _ := setupAvatarScenario(t, nil, data())
	baseTxns := readStoredTransactions(t, baseDB)
	baseBalance := readAccountBalance(t, baseDB)

	// The same data through an always-failing fetcher. setupAvatarScenario fatals if
	// the sync errored, so reaching here already proves the fetch failure never fails
	// the sync.
	failer := &failingLogoFetcher{}
	failDB, _, _, _, _ := setupAvatarScenario(t, failer, data())

	if failer.attempts == 0 {
		t.Fatal("the failing fetcher was never called; the warm step did not run and the test proves nothing")
	}
	if got := negativeCacheCount(t, failDB); got == 0 {
		t.Errorf("a failed fetch left no negative cache entry; the failure path did not run")
	}

	failTxns := readStoredTransactions(t, failDB)
	if len(failTxns) != len(baseTxns) {
		t.Fatalf("failing-fetch sync wrote %d transactions, want %d", len(failTxns), len(baseTxns))
	}
	for id, amt := range baseTxns {
		if failTxns[id] != amt {
			t.Errorf("transaction %s amount = %v with a failing fetch, want %v (a logo fetch must never alter a row)", id, failTxns[id], amt)
		}
	}
	if failBalance := readAccountBalance(t, failDB); failBalance != baseBalance {
		t.Errorf("account balance = %v with a failing fetch, want %v (a logo fetch must never alter a balance)", failBalance, baseBalance)
	}
}

// TestRefusedLogoIsNeverServedAndRowFallsBackToGlyph is the end-to-end containment
// path through the real SSRF-constrained fetcher: a transaction whose logo URL is
// on a host outside the in-code allowlist is refused (with no network I/O),
// negative-cached, its image endpoint 404s, and its row renders the category glyph.
func TestRefusedLogoIsNeverServedAndRowFallsBackToGlyph(t *testing.T) {
	const refusedURL = "https://evil.example.com/logo.png" // not the allowlisted CDN host
	txns := []banking.Transaction{logoTxn("t-evil", "Sketchy Shop", 1, 5.0, refusedURL)}

	// The concrete fetcher wired at the composition root. A non-allowlisted host is
	// refused before any request is issued, so this stays hermetic.
	database, _, accountsSvc, txnSvc, categorizationSvc := setupAvatarScenario(t, plaid.NewLogoFetcher(), txns)

	key := logoKeyOf(refusedURL)

	if negativeCacheCount(t, database) == 0 {
		t.Fatalf("the refused URL was not negative-cached; containment did not record the refusal")
	}

	// The image endpoint refuses to serve a refused (negative-cached) logo.
	handler := txnAdapters.NewHttpHandler(txnSvc, accountsSvc, categorizationSvc)
	req := httptest.NewRequest(http.MethodGet, transactions.MerchantLogoRoutePrefix+key, nil)
	req.SetPathValue("key", key)
	rec := httptest.NewRecorder()
	handler.GetMerchantLogo(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("image endpoint status = %d for a refused logo, want 404 (no bytes to serve)", rec.Code)
	}

	// The row still renders — as the category glyph, never the refused source.
	_, body := getPage(t, txnSvc, accountsSvc, categorizationSvc)
	if strings.Contains(body, "merchant-avatar-image") {
		t.Errorf("a refused logo rendered an image; the row must fall back to the glyph:\n%s", body)
	}
	if !strings.Contains(body, `data-testid="merchant-avatar-glyph"`) {
		t.Errorf("the refused-logo row rendered no glyph; a row must never be blank:\n%s", body)
	}
	if strings.Contains(body, "evil.example.com") {
		t.Errorf("rendered markup references the refused host; a refused source must never reach the page:\n%s", body)
	}
}

// readStoredTransactions returns the stored transactions as id -> signed amount.
func readStoredTransactions(t *testing.T, database *db.DB) map[string]float64 {
	t.Helper()
	rows, err := database.Sql().Query("SELECT id, amount_amount FROM transactions")
	if err != nil {
		t.Fatalf("read transactions: %v", err)
	}
	defer rows.Close()
	out := map[string]float64{}
	for rows.Next() {
		var id string
		var amount float64
		if err := rows.Scan(&id, &amount); err != nil {
			t.Fatalf("scan transaction: %v", err)
		}
		out[id] = amount
	}
	return out
}

// readAccountBalance returns the single connected account's stored balance amount.
func readAccountBalance(t *testing.T, database *db.DB) float64 {
	t.Helper()
	var amount float64
	if err := database.Sql().QueryRow("SELECT balance_amount FROM accounts").Scan(&amount); err != nil {
		t.Fatalf("read account balance: %v", err)
	}
	return amount
}

// negativeCacheCount reports how many logos were recorded as negative (no bytes).
func negativeCacheCount(t *testing.T, database *db.DB) int {
	t.Helper()
	var n int
	if err := database.Sql().QueryRow("SELECT count(*) FROM merchant_logo_cache WHERE image_bytes IS NULL").Scan(&n); err != nil {
		t.Fatalf("count negative cache entries: %v", err)
	}
	return n
}
