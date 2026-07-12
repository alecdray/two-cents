package transactions

import (
	"fmt"
	"testing"

	"github.com/alecdray/two-cents/src/internal/accounts"
	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/db"
)

// fakeLogo is one canned fetch outcome the fake fetcher can return for a URL.
type fakeLogo struct {
	bytes       []byte
	contentType string
}

// fakeLogoFetcher is an in-package LogoFetcher stand-in: it touches no network, so no
// unit test performs real I/O. It returns canned bytes for URLs it knows, a transport
// error for URLs armed to fail, and a no-logo result for anything else — and records
// every URL it was asked to fetch, in order, so a test can assert recency ordering,
// per-pass bounding, and that a cached merchant is never fetched again.
type fakeLogoFetcher struct {
	byURL    map[string]fakeLogo
	failURLs map[string]bool
	fetched  []string
}

func newFakeLogoFetcher() *fakeLogoFetcher {
	return &fakeLogoFetcher{byURL: map[string]fakeLogo{}, failURLs: map[string]bool{}}
}

func (f *fakeLogoFetcher) FetchLogo(_ contextx.ContextX, logoURL string) ([]byte, string, error) {
	f.fetched = append(f.fetched, logoURL)
	if f.failURLs[logoURL] {
		return nil, "", fmt.Errorf("simulated logo fetch failure")
	}
	if lg, ok := f.byURL[logoURL]; ok {
		return lg.bytes, lg.contentType, nil
	}
	return nil, "", nil
}

// bankTxnWithLogo is bankTxn with a merchant logo URL attached, so the sync stores a
// row the logo-warm step will consider.
func bankTxnWithLogo(id, providerAccountID string, day int, amount float64, logoURL string) banking.Transaction {
	bt := bankTxn(id, providerAccountID, day, amount, false)
	bt.LogoURL = logoURL
	return bt
}

// logoCacheState reports whether a key is in the cache and, if so, whether it is a
// positive entry (has image bytes) or a negative one (image_bytes NULL).
func logoCacheState(t *testing.T, database *db.DB, key string) (exists, positive bool) {
	t.Helper()
	rows, err := database.Sql().Query("SELECT image_bytes IS NOT NULL FROM merchant_logo_cache WHERE logo_key = ?", key)
	if err != nil {
		t.Fatalf("query cache state: %v", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return false, false
	}
	if err := rows.Scan(&positive); err != nil {
		t.Fatalf("scan cache state: %v", err)
	}
	return true, positive
}

// syncWithLogos registers a connection whose backfill is the given transactions and
// returns a service wired to the fake fetcher plus that service, having run one sync.
func setupLogoSync(t *testing.T, database *db.DB, fetcher LogoFetcher, added []banking.Transaction) (*Service, *accounts.Service, *stubProvider) {
	t.Helper()
	const token = "tok-logo"
	provider := newStub()
	provider.accountsByToken[token] = []banking.Account{cashAccount("p-check", "Checking")}
	provider.syncByToken[token] = func(cursor string) (banking.TransactionChanges, error) {
		if cursor == "" {
			return banking.TransactionChanges{Added: added, Cursor: "c1"}, nil
		}
		return banking.TransactionChanges{Cursor: cursor}, nil
	}
	accountsSvc := accounts.NewService(database, provider, testKey)
	registerConnection(t, accountsSvc, token, "item-logo")
	svc := NewService(database, provider, accountsSvc, newCategorization(database), fetcher)
	return svc, accountsSvc, provider
}

// TestSyncWarmsCacheBoundedMostRecentFirstAndDrainsOverPasses proves the warm step
// fetches un-cached merchants across the whole stored set, most-recent-first, no more
// than one bounded batch per pass, and that a later pass fetches only the not-yet-
// cached remainder (never re-fetching an already-cached merchant).
func TestSyncWarmsCacheBoundedMostRecentFirstAndDrainsOverPasses(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	// One more distinct-logo merchant than a single pass fetches, each on its own day
	// so recency is unambiguous: higher day = more recent.
	total := maxLogosWarmedPerSync + 2
	fetcher := newFakeLogoFetcher()
	var added []banking.Transaction
	for i := 1; i <= total; i++ {
		u := fmt.Sprintf("https://cdn.example/logo-%03d.png", i)
		fetcher.byURL[u] = fakeLogo{bytes: []byte(fmt.Sprintf("png-%d", i)), contentType: "image/png"}
		added = append(added, bankTxnWithLogo(fmt.Sprintf("t%03d", i), "p-check", i, 10, u))
	}

	svc, _, _ := setupLogoSync(t, database, fetcher, added)
	if err := svc.SyncTransactions(ctx); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	t.Run("only a bounded batch is fetched in one pass", func(t *testing.T) {
		if len(fetcher.fetched) != maxLogosWarmedPerSync {
			t.Fatalf("first pass fetched %d, want the bound of %d", len(fetcher.fetched), maxLogosWarmedPerSync)
		}
	})

	t.Run("the batch is the most-recent merchants first", func(t *testing.T) {
		mostRecent := fmt.Sprintf("https://cdn.example/logo-%03d.png", total)
		if fetcher.fetched[0] != mostRecent {
			t.Errorf("first fetched = %q, want the most-recent merchant %q", fetcher.fetched[0], mostRecent)
		}
		// The two oldest (i=1, i=2) fall outside the first bounded batch.
		for _, oldest := range []string{"https://cdn.example/logo-001.png", "https://cdn.example/logo-002.png"} {
			for _, got := range fetcher.fetched {
				if got == oldest {
					t.Errorf("oldest merchant %q was fetched in the first bounded pass; the backlog should drain later", oldest)
				}
			}
		}
	})

	// A second pass: the earlier batch is already cached and must not be re-fetched;
	// only the remaining un-cached merchants are pulled.
	fetcher.fetched = nil
	if err := svc.SyncTransactions(ctx); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	t.Run("the next pass fetches only the not-yet-cached remainder", func(t *testing.T) {
		if len(fetcher.fetched) != 2 {
			t.Fatalf("second pass fetched %d, want the 2 remaining", len(fetcher.fetched))
		}
		want := map[string]bool{
			"https://cdn.example/logo-001.png": true,
			"https://cdn.example/logo-002.png": true,
		}
		for _, got := range fetcher.fetched {
			if !want[got] {
				t.Errorf("second pass fetched %q, which was already cached; a cached merchant must never be re-fetched", got)
			}
		}
	})

	t.Run("every merchant ends up positively cached", func(t *testing.T) {
		for i := 1; i <= total; i++ {
			u := fmt.Sprintf("https://cdn.example/logo-%03d.png", i)
			exists, positive := logoCacheState(t, database, merchantLogoKey(u))
			if !exists || !positive {
				t.Errorf("logo %d: exists=%v positive=%v, want cached positive", i, exists, positive)
			}
		}
	})
}

// TestSyncNegativeCachesUnusableLogosAndNeverRetries proves a logo URL that is absent,
// fails, or is refused becomes a negative cache entry and is not attempted again.
func TestSyncNegativeCachesUnusableLogosAndNeverRetries(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	const (
		goodURL   = "https://cdn.example/good.png"
		failURL   = "https://cdn.example/fail.png"
		refuseURL = "https://cdn.example/refused.png" // absent from byURL: a no-logo result
	)
	fetcher := newFakeLogoFetcher()
	fetcher.byURL[goodURL] = fakeLogo{bytes: []byte("png"), contentType: "image/png"}
	fetcher.failURLs[failURL] = true

	added := []banking.Transaction{
		bankTxnWithLogo("t-good", "p-check", 3, 10, goodURL),
		bankTxnWithLogo("t-fail", "p-check", 2, 10, failURL),
		bankTxnWithLogo("t-refuse", "p-check", 1, 10, refuseURL),
	}
	svc, _, _ := setupLogoSync(t, database, fetcher, added)
	if err := svc.SyncTransactions(ctx); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	t.Run("a usable logo is positive; a failed or refused one is negative", func(t *testing.T) {
		if exists, positive := logoCacheState(t, database, merchantLogoKey(goodURL)); !exists || !positive {
			t.Errorf("good logo: exists=%v positive=%v, want positive", exists, positive)
		}
		for name, u := range map[string]string{"fail": failURL, "refuse": refuseURL} {
			exists, positive := logoCacheState(t, database, merchantLogoKey(u))
			if !exists {
				t.Errorf("%s logo was not recorded; a no-logo result must be negative-cached", name)
			}
			if positive {
				t.Errorf("%s logo was cached positive, want negative (no bytes)", name)
			}
		}
	})

	t.Run("the next sync retries nothing already attempted", func(t *testing.T) {
		fetcher.fetched = nil
		if err := svc.SyncTransactions(ctx); err != nil {
			t.Fatalf("second sync: %v", err)
		}
		if len(fetcher.fetched) != 0 {
			t.Errorf("second pass fetched %v; a cached merchant (positive OR negative) must never be re-fetched", fetcher.fetched)
		}
	})
}

// TestWarmConsidersWholeStoredSetNotJustThisPullsDelta proves the warm step heals a
// merchant synced before a fetcher was wired: a later sync with no new delta still
// warms the pre-existing rows across the whole stored set.
func TestWarmConsidersWholeStoredSetNotJustThisPullsDelta(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	const logoURL = "https://cdn.example/preexisting.png"
	added := []banking.Transaction{bankTxnWithLogo("t-pre", "p-check", 5, 10, logoURL)}

	// First sync with no fetcher: the row is stored but nothing warms.
	noFetcherSvc, accountsSvc, provider := setupLogoSync(t, database, nil, added)
	if err := noFetcherSvc.SyncTransactions(ctx); err != nil {
		t.Fatalf("first sync (no fetcher): %v", err)
	}
	if exists, _ := logoCacheState(t, database, merchantLogoKey(logoURL)); exists {
		t.Fatalf("logo was cached with no fetcher wired; nothing should have warmed")
	}

	// A later service over the same DB, now with a fetcher. The pull resumes from the
	// stored cursor and returns no new delta, yet the warm step considers the whole
	// stored set and caches the pre-existing merchant.
	fetcher := newFakeLogoFetcher()
	fetcher.byURL[logoURL] = fakeLogo{bytes: []byte("png"), contentType: "image/png"}
	healingSvc := NewService(database, provider, accountsSvc, newCategorization(database), fetcher)
	if err := healingSvc.SyncTransactions(ctx); err != nil {
		t.Fatalf("second sync (with fetcher): %v", err)
	}
	if exists, positive := logoCacheState(t, database, merchantLogoKey(logoURL)); !exists || !positive {
		t.Errorf("pre-existing merchant not warmed: exists=%v positive=%v; warm must consider the whole stored set, not just this pull's delta", exists, positive)
	}
}

// TestWarmFailureNeverFailsSyncOrChangesRows proves the warm step is decoupled from
// sync correctness: a fetcher that errors on every URL leaves the sync green and every
// transaction row intact.
func TestWarmFailureNeverFailsSyncOrChangesRows(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	fetcher := newFakeLogoFetcher()
	const failURL = "https://cdn.example/always-fails.png"
	fetcher.failURLs[failURL] = true

	added := []banking.Transaction{
		bankTxnWithLogo("t1", "p-check", 2, 84.32, failURL),
		bankTxnWithLogo("t2", "p-check", 1, -2400.00, failURL),
	}
	svc, _, _ := setupLogoSync(t, database, fetcher, added)

	if err := svc.SyncTransactions(ctx); err != nil {
		t.Fatalf("a failing logo fetch must not fail the sync, got: %v", err)
	}

	if got := countTransactions(t, database); got != 2 {
		t.Errorf("stored %d transactions, want 2 (a logo fetch must never drop a row)", got)
	}
	var amount float64
	if err := database.Sql().QueryRow("SELECT amount_amount FROM transactions WHERE id = ?", "t2").Scan(&amount); err != nil {
		t.Fatalf("read row: %v", err)
	}
	if amount != -2400.00 {
		t.Errorf("t2 amount = %v, want -2400 unchanged (a logo fetch must never alter a row)", amount)
	}
	// The failure was recorded as a negative entry, not propagated.
	if exists, positive := logoCacheState(t, database, merchantLogoKey(failURL)); !exists || positive {
		t.Errorf("failed logo: exists=%v positive=%v, want a negative entry", exists, positive)
	}
}

// TestLookupYieldsServedURLOnlyWhenPositivelyCached proves the published contract: a
// row's MerchantLogoURL is our origin's served URL when — and only when — its logo is
// positively cached, and points at our endpoint, never the bank CDN.
func TestLookupYieldsServedURLOnlyWhenPositivelyCached(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	const (
		positiveURL = "https://cdn.example/positive.png"
		negativeURL = "https://cdn.example/negative.png" // fetch fails -> negative
	)
	fetcher := newFakeLogoFetcher()
	fetcher.byURL[positiveURL] = fakeLogo{bytes: []byte("png"), contentType: "image/png"}
	fetcher.failURLs[negativeURL] = true

	added := []banking.Transaction{
		bankTxnWithLogo("t-pos", "p-check", 3, 10, positiveURL),
		bankTxnWithLogo("t-neg", "p-check", 2, 10, negativeURL),
		bankTxn("t-nologo", "p-check", 1, 10, false), // no logo URL at all
	}
	svc, _, _ := setupLogoSync(t, database, fetcher, added)
	if err := svc.SyncTransactions(ctx); err != nil {
		t.Fatalf("sync: %v", err)
	}

	rows, err := svc.RecentTransactions(ctx, 10)
	if err != nil {
		t.Fatalf("RecentTransactions: %v", err)
	}
	byID := map[string]RecentTransaction{}
	for _, r := range rows {
		byID[r.ID] = r
	}

	wantServed := MerchantLogoRoutePrefix + merchantLogoKey(positiveURL)
	if got := byID["t-pos"].MerchantLogoURL; got != wantServed {
		t.Errorf("positively cached row served URL = %q, want %q", got, wantServed)
	}
	if got := byID["t-neg"].MerchantLogoURL; got != "" {
		t.Errorf("negative-cached row served URL = %q, want empty", got)
	}
	if got := byID["t-nologo"].MerchantLogoURL; got != "" {
		t.Errorf("no-logo row served URL = %q, want empty", got)
	}

	t.Run("the single-row read populates the same field", func(t *testing.T) {
		row, err := svc.RecentTransaction(ctx, "t-pos")
		if err != nil {
			t.Fatalf("RecentTransaction: %v", err)
		}
		if row.MerchantLogoURL != wantServed {
			t.Errorf("single-row served URL = %q, want %q", row.MerchantLogoURL, wantServed)
		}
	})
}

// TestMerchantLogoIsPureCacheRead proves the served-URL lookup source — the MerchantLogo
// method the image endpoint reads — returns bytes only for a positively cached key and
// nothing for an absent or negative key, without touching the provider or the fetcher.
func TestMerchantLogoIsPureCacheRead(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	const (
		positiveURL = "https://cdn.example/served.png"
		negativeURL = "https://cdn.example/missing.png"
	)
	fetcher := newFakeLogoFetcher()
	fetcher.byURL[positiveURL] = fakeLogo{bytes: []byte("the-bytes"), contentType: "image/jpeg"}
	fetcher.failURLs[negativeURL] = true

	added := []banking.Transaction{
		bankTxnWithLogo("t-pos", "p-check", 2, 10, positiveURL),
		bankTxnWithLogo("t-neg", "p-check", 1, 10, negativeURL),
	}
	svc, _, provider := setupLogoSync(t, database, fetcher, added)
	if err := svc.SyncTransactions(ctx); err != nil {
		t.Fatalf("sync: %v", err)
	}

	provider.syncWasCalled = false
	fetcher.fetched = nil

	logo, ok, err := svc.MerchantLogo(ctx, merchantLogoKey(positiveURL))
	if err != nil {
		t.Fatalf("MerchantLogo: %v", err)
	}
	if !ok {
		t.Fatalf("positively cached key not found")
	}
	if logo.ContentType != "image/jpeg" || string(logo.Bytes) != "the-bytes" {
		t.Errorf("served logo = %q/%q, want image/jpeg/the-bytes", logo.ContentType, string(logo.Bytes))
	}

	if _, ok, _ := svc.MerchantLogo(ctx, merchantLogoKey(negativeURL)); ok {
		t.Errorf("a negative-cached key was served; only a positive key must return bytes")
	}
	if _, ok, _ := svc.MerchantLogo(ctx, "nonexistent-key"); ok {
		t.Errorf("an absent key was served; only a positive key must return bytes")
	}

	if provider.syncWasCalled {
		t.Errorf("MerchantLogo called the provider; it must be a pure cache read")
	}
	if len(fetcher.fetched) != 0 {
		t.Errorf("MerchantLogo fetched a logo; it must be a pure cache read")
	}
}
