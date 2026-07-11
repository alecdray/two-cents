package plaid

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/alecdray/two-cents/src/internal/core/contextx"
)

func fetchCtx() contextx.ContextX {
	return contextx.NewContextX(context.Background())
}

// pointFetcherAt rewrites the fetcher's client so a request to the allowlisted CDN
// host is transparently served by the given test server instead of the real internet.
// It keeps the allowlist check (which runs against the original URL's host) honest
// while letting the test control the response — no real network I/O.
func pointFetcherAt(f *LogoFetcher, ts *httptest.Server) {
	base, _ := url.Parse(ts.URL)
	f.httpClient = &http.Client{
		Timeout: logoFetchBudget,
		Transport: rewriteTransport{
			host: base.Host,
			base: ts.Client().Transport,
		},
	}
}

// rewriteTransport redirects every request to the test server's host, so a URL on the
// allowlisted CDN host actually hits the httptest server.
type rewriteTransport struct {
	host string
	base http.RoundTripper
}

func (rt rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	req.URL.Host = rt.host
	base := rt.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

// pngServer returns a test server that answers with the given content type and body.
func imageServer(t *testing.T, contentType string, body []byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
}

const allowlistedURL = "https://" + plaidLogoHost + "/logo.png"

func TestFetchLogoReturnsRasterBytesFromAllowlistedHTTPSHost(t *testing.T) {
	want := []byte("\x89PNG\r\n\x1a\n fake png bytes")
	ts := imageServer(t, "image/png", want)
	defer ts.Close()

	f := NewLogoFetcher()
	pointFetcherAt(f, ts)

	got, contentType, err := f.FetchLogo(fetchCtx(), allowlistedURL)
	if err != nil {
		t.Fatalf("FetchLogo: %v", err)
	}
	if contentType != "image/png" {
		t.Errorf("content type = %q, want image/png", contentType)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("bytes = %q, want %q", got, want)
	}
}

func TestFetchLogoNormalizesContentTypeParameters(t *testing.T) {
	want := []byte("jpegbytes")
	ts := imageServer(t, "image/JPEG; charset=binary", want)
	defer ts.Close()

	f := NewLogoFetcher()
	pointFetcherAt(f, ts)

	got, contentType, err := f.FetchLogo(fetchCtx(), allowlistedURL)
	if err != nil {
		t.Fatalf("FetchLogo: %v", err)
	}
	if contentType != "image/jpeg" {
		t.Errorf("content type = %q, want normalized image/jpeg", contentType)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("bytes mismatch")
	}
}

// TestFetchLogoRefusesEachGuard proves the policy rejects everything outside it,
// yielding no bytes: a non-https scheme, a non-allowlisted host, and a non-raster
// content type (including SVG) each return an empty result. These guards need no
// network for the URL-structural cases.
func TestFetchLogoRefusesEachGuard(t *testing.T) {
	t.Run("a non-https URL yields no bytes", func(t *testing.T) {
		f := NewLogoFetcher()
		got, ct, err := f.FetchLogo(fetchCtx(), "http://"+plaidLogoHost+"/logo.png")
		assertNoLogo(t, got, ct, err)
	})

	t.Run("a non-allowlisted host yields no bytes", func(t *testing.T) {
		f := NewLogoFetcher()
		got, ct, err := f.FetchLogo(fetchCtx(), "https://evil.example.com/logo.png")
		assertNoLogo(t, got, ct, err)
	})

	t.Run("an unparseable URL yields no bytes", func(t *testing.T) {
		f := NewLogoFetcher()
		got, ct, err := f.FetchLogo(fetchCtx(), "://not a url")
		assertNoLogo(t, got, ct, err)
	})

	for _, svgType := range []string{"image/svg+xml", "text/html", "application/octet-stream"} {
		svgType := svgType
		t.Run("a non-raster content type ("+svgType+") yields no bytes", func(t *testing.T) {
			ts := imageServer(t, svgType, []byte("<svg/>"))
			defer ts.Close()
			f := NewLogoFetcher()
			pointFetcherAt(f, ts)
			got, ct, err := f.FetchLogo(fetchCtx(), allowlistedURL)
			assertNoLogo(t, got, ct, err)
		})
	}

	t.Run("a non-200 status yields no bytes", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer ts.Close()
		f := NewLogoFetcher()
		pointFetcherAt(f, ts)
		got, ct, err := f.FetchLogo(fetchCtx(), allowlistedURL)
		assertNoLogo(t, got, ct, err)
	})
}

func TestFetchLogoRefusesOversizeResponse(t *testing.T) {
	oversize := bytes.Repeat([]byte("A"), maxLogoBytes+10)
	ts := imageServer(t, "image/png", oversize)
	defer ts.Close()

	f := NewLogoFetcher()
	pointFetcherAt(f, ts)

	got, ct, err := f.FetchLogo(fetchCtx(), allowlistedURL)
	assertNoLogo(t, got, ct, err)
}

func TestFetchLogoRefusesEmptyBody(t *testing.T) {
	ts := imageServer(t, "image/png", nil)
	defer ts.Close()

	f := NewLogoFetcher()
	pointFetcherAt(f, ts)

	got, ct, err := f.FetchLogo(fetchCtx(), allowlistedURL)
	assertNoLogo(t, got, ct, err)
}

func TestFetchLogoRefusesTooSlowResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("late"))
	}))
	defer ts.Close()

	f := NewLogoFetcher()
	// A tight budget makes the slow server exceed it: the client times out and the
	// fetch is treated as no-logo (an error the caller negative-caches).
	base, _ := url.Parse(ts.URL)
	f.httpClient = &http.Client{
		Timeout:   10 * time.Millisecond,
		Transport: rewriteTransport{host: base.Host},
	}

	got, ct, err := f.FetchLogo(fetchCtx(), allowlistedURL)
	if len(got) != 0 || ct != "" {
		t.Errorf("a too-slow response returned bytes %q / type %q, want none", got, ct)
	}
	if err == nil {
		t.Errorf("a client timeout should surface an error (the caller negative-caches it)")
	}
}

// assertNoLogo asserts the fetcher returned a no-logo result and no error (a policy
// refusal, distinct from a transport error).
func assertNoLogo(t *testing.T, got []byte, contentType string, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("a policy refusal returned an error %v, want a clean no-logo result", err)
	}
	if len(got) != 0 {
		t.Errorf("returned %d bytes, want none", len(got))
	}
	if contentType != "" {
		t.Errorf("returned content type %q, want empty", contentType)
	}
}

// TestFetchLogoStructurallySatisfiesTheSeamSignature documents that the concrete
// fetcher's method signature matches the transactions seam without either package
// importing the other: a compile-time assertion against a locally-declared interface
// of the same shape stands in for the composition-root wiring.
func TestFetchLogoStructurallySatisfiesTheSeamSignature(t *testing.T) {
	type logoFetcher interface {
		FetchLogo(ctx contextx.ContextX, logoURL string) ([]byte, string, error)
	}
	var _ logoFetcher = NewLogoFetcher()
	if !strings.HasSuffix(allowlistedURL, ".png") {
		t.Fatal("unreachable")
	}
}
