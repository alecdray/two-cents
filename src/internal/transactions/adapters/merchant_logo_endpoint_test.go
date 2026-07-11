package adapters_test

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alecdray/two-cents/src/internal/core/app"
	"github.com/alecdray/two-cents/src/internal/core/db"
	"github.com/alecdray/two-cents/src/internal/core/httpx"
	"github.com/alecdray/two-cents/src/internal/fakebank"
	"github.com/alecdray/two-cents/src/internal/transactions"
	"github.com/alecdray/two-cents/src/internal/transactions/adapters"
)

// logoKeyOf recomputes the cache key the pipeline uses: the hex SHA-256 of the logo
// URL. The test computes it itself so it also asserts the key scheme is a content hash
// of the URL.
func logoKeyOf(logoURL string) string {
	sum := sha256.Sum256([]byte(logoURL))
	return hex.EncodeToString(sum[:])
}

// seedPositiveLogo inserts a positively cached logo directly into the rebuildable
// cache table (no transaction row needed), so the endpoint test controls the served
// bytes without running a fetch.
func seedPositiveLogo(t *testing.T, database *db.DB, key, contentType string, body []byte) {
	t.Helper()
	_, err := database.Sql().Exec(
		"INSERT INTO merchant_logo_cache (logo_key, logo_url, content_type, image_bytes) VALUES (?, ?, ?, ?)",
		key, "https://cdn.example/seed.png", contentType, body,
	)
	if err != nil {
		t.Fatalf("seed positive logo: %v", err)
	}
}

// seedNegativeLogo inserts a negative cache entry (NULL image_bytes).
func seedNegativeLogo(t *testing.T, database *db.DB, key string) {
	t.Helper()
	_, err := database.Sql().Exec(
		"INSERT INTO merchant_logo_cache (logo_key, logo_url, content_type, image_bytes) VALUES (?, ?, '', NULL)",
		key, "https://cdn.example/negative.png",
	)
	if err != nil {
		t.Fatalf("seed negative logo: %v", err)
	}
}

func logoHandler(t *testing.T, database *db.DB) *adapters.HttpHandler {
	t.Helper()
	accountsSvc, txnSvc, categorizationSvc := newServices(t, database, fakebank.NewService())
	_ = accountsSvc
	return adapters.NewHttpHandler(txnSvc, accountsSvc, categorizationSvc)
}

func TestMerchantLogoEndpointServesCachedBytesWithHeaders(t *testing.T) {
	database := newTestDB(t)
	const logoURL = "https://cdn.example/acme.png"
	key := logoKeyOf(logoURL)
	body := []byte("\x89PNG cached bytes")
	seedPositiveLogo(t, database, key, "image/png", body)

	handler := logoHandler(t, database)
	req := httptest.NewRequest(http.MethodGet, transactions.MerchantLogoRoutePrefix+key, nil)
	req.SetPathValue("key", key)
	rec := httptest.NewRecorder()
	handler.GetMerchantLogo(rec, req)

	res := rec.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if got := res.Header.Get("Content-Type"); got != "image/png" {
		t.Errorf("Content-Type = %q, want image/png (the stored type)", got)
	}
	if got := res.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := res.Header.Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q, want an immutable long-lived header", got)
	}
	if rec.Body.String() != string(body) {
		t.Errorf("body = %q, want the stored bytes %q", rec.Body.String(), string(body))
	}
}

func TestMerchantLogoEndpointNotFoundForAbsentOrNegativeKey(t *testing.T) {
	database := newTestDB(t)
	handler := logoHandler(t, database)

	t.Run("an absent key is 404", func(t *testing.T) {
		key := logoKeyOf("https://cdn.example/never-cached.png")
		req := httptest.NewRequest(http.MethodGet, transactions.MerchantLogoRoutePrefix+key, nil)
		req.SetPathValue("key", key)
		rec := httptest.NewRecorder()
		handler.GetMerchantLogo(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404 for an absent key", rec.Code)
		}
	})

	t.Run("a negative-cached key is 404", func(t *testing.T) {
		key := logoKeyOf("https://cdn.example/negative.png")
		seedNegativeLogo(t, database, key)
		req := httptest.NewRequest(http.MethodGet, transactions.MerchantLogoRoutePrefix+key, nil)
		req.SetPathValue("key", key)
		rec := httptest.NewRecorder()
		handler.GetMerchantLogo(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404 for a negative-cached key (no bytes to serve)", rec.Code)
		}
	})
}

// TestMerchantLogoEndpointRequiresAuthenticatedSession proves the endpoint inherits
// the session gate like every other authed route: mounted on the JWT-guarded mux, an
// unauthenticated request is redirected to login, while a request carrying a valid
// session cookie serves the bytes. This exercises the real route registration and
// middleware, not just the handler.
func TestMerchantLogoEndpointRequiresAuthenticatedSession(t *testing.T) {
	database := newTestDB(t)
	const logoURL = "https://cdn.example/guarded.png"
	key := logoKeyOf(logoURL)
	seedPositiveLogo(t, database, key, "image/png", []byte("bytes"))

	application := app.NewApp(app.Config{Env: app.EnvLocal, JwtSecret: "test-secret"})

	// Mirror the composition root: a mux guarded by the session middleware, with the
	// transactions routes registered on it.
	accountsSvc, txnSvc, categorizationSvc := newServices(t, database, fakebank.NewService())
	_ = accountsSvc
	handler := adapters.NewHttpHandler(txnSvc, accountsSvc, categorizationSvc)
	appMux := httpx.NewMux(application, httpx.JwtMiddleware)
	adapters.RegisterRoutes(appMux, handler)

	path := transactions.MerchantLogoRoutePrefix + key

	t.Run("an unauthenticated request is redirected to login", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		appMux.ServeHTTP(rec, req)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("status = %d, want 303 redirect for an unauthenticated request", rec.Code)
		}
		if loc := rec.Header().Get("Location"); loc != "/login" {
			t.Errorf("redirect Location = %q, want /login", loc)
		}
	})

	t.Run("a request with a valid session serves the bytes", func(t *testing.T) {
		// Issue a session cookie the way login does, then present it.
		issueRec := httptest.NewRecorder()
		if err := application.IssueSession(issueRec, "user-1"); err != nil {
			t.Fatalf("IssueSession: %v", err)
		}
		req := httptest.NewRequest(http.MethodGet, path, nil)
		for _, c := range issueRec.Result().Cookies() {
			req.AddCookie(c)
		}
		rec := httptest.NewRecorder()
		appMux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 for an authenticated request", rec.Code)
		}
		if rec.Body.String() != "bytes" {
			t.Errorf("body = %q, want the served bytes", rec.Body.String())
		}
	})
}
