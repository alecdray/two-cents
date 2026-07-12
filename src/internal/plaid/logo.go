package plaid

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/alecdray/two-cents/src/internal/core/contextx"
)

// plaidLogoHost is the only host the merchant-logo fetcher will retrieve from: the
// Plaid CDN that serves the logo_url values on Plaid transactions. It is an in-code
// fact of this adapter, not a deployment knob, so it is a const here rather than
// configuration; the fetcher refuses every other host outright — an SSRF guard on the
// bank-sourced, in-principle attacker-influenceable URLs.
const plaidLogoHost = "plaid-merchant-logos.plaid.com"

// Logo fetch budgets: a merchant logo is a small raster, so a modest byte ceiling and
// a short timeout bound the work and let an oversized or slow response be refused as
// no-logo rather than buffered whole or waited on.
const (
	maxLogoBytes    = 512 * 1024
	logoFetchBudget = 5 * time.Second
)

// allowedLogoContentTypes is the raster image set the fetcher accepts. SVG is
// deliberately excluded: an SVG can carry script, so it is never fetched or served.
var allowedLogoContentTypes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/webp": true,
}

// LogoFetcher retrieves merchant-logo image bytes over HTTP under a strict policy —
// the concrete, SSRF-constrained implementation of the transactions module's logo
// fetch seam, wired at the composition root. It returns bytes ONLY for an https URL
// whose host is the allowlisted Plaid CDN, within the size and time budgets, and whose
// response is a raster image (png/jpeg/webp). A non-https URL, a non-allowlisted host,
// an over-size or too-slow response, and a non-raster type (including SVG) each yield a
// no-logo result and never bytes.
type LogoFetcher struct {
	httpClient *http.Client
}

// NewLogoFetcher builds the fetcher with the fetch-time budget applied to its client.
func NewLogoFetcher() *LogoFetcher {
	return &LogoFetcher{httpClient: &http.Client{Timeout: logoFetchBudget}}
}

// FetchLogo retrieves the logo at logoURL under the fetch policy. A URL that is not
// https, whose host is not the allowlisted CDN, that exceeds the budgets, or whose
// content type is not a raster image yields a no-logo result (nil bytes, empty type,
// nil error) — never bytes. A transport failure returns the error; the caller treats
// both the no-logo result and an error as a negative cache entry.
func (f *LogoFetcher) FetchLogo(ctx contextx.ContextX, logoURL string) ([]byte, string, error) {
	if !allowedLogoURL(logoURL) {
		return nil, "", nil
	}

	reqCtx, cancel := context.WithTimeout(ctx, logoFetchBudget)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, logoURL, nil)
	if err != nil {
		return nil, "", nil
	}

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", nil
	}

	contentType := normalizeContentType(resp.Header.Get("Content-Type"))
	if !allowedLogoContentTypes[contentType] {
		return nil, "", nil
	}

	// Read one byte past the ceiling so an over-size body is detected and refused
	// rather than buffered whole.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxLogoBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(body) == 0 || len(body) > maxLogoBytes {
		return nil, "", nil
	}
	return body, contentType, nil
}

// allowedLogoURL reports whether logoURL is an https URL on the allowlisted CDN host —
// the two structural guards applied before any request is issued.
func allowedLogoURL(logoURL string) bool {
	u, err := url.Parse(logoURL)
	if err != nil {
		return false
	}
	if u.Scheme != "https" {
		return false
	}
	return u.Hostname() == plaidLogoHost
}

// normalizeContentType strips any charset or other parameters and lowercases the media
// type, so "image/PNG; charset=binary" compares equal to "image/png".
func normalizeContentType(header string) string {
	mediaType := header
	if i := strings.IndexByte(mediaType, ';'); i >= 0 {
		mediaType = mediaType[:i]
	}
	return strings.ToLower(strings.TrimSpace(mediaType))
}
