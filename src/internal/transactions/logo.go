package transactions

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/alecdray/two-cents/src/internal/core/contextx"
)

// LogoFetcher fetches a merchant logo's image bytes from its bank-sourced URL. It is
// the SSRF-constrained seam the sync's cache-warm step fetches through: this module
// defines only the interface (so it stays provider-agnostic and is unit-tested with a
// fake that touches no network), while the concrete implementation lives with the
// provider adapter (its home for the CDN host) and is wired at the composition root.
type LogoFetcher interface {
	// FetchLogo returns the image bytes and content type for a logo URL, or a nil/
	// empty result when the URL yields no usable logo (not https, a disallowed host,
	// over the size or time budget, or a non-raster content type). The warm step
	// records a no-logo result, whether signalled by empty bytes or an error, as a
	// negative cache entry and never fails the sync over it.
	FetchLogo(ctx contextx.ContextX, logoURL string) (imageBytes []byte, contentType string, err error)
}

// MerchantLogoRoutePrefix is the origin path the cached-logo image endpoint serves
// under; a content-hash key appends to it. The served URL a row carries and the route
// the adapter mounts are both built from this one prefix, so they never drift. It is a
// top-level path (not under /transactions/) to avoid colliding with the
// /transactions/{id}/edit wildcard route.
const MerchantLogoRoutePrefix = "/merchant-logos/"

// merchantLogoKey is the cache key for a logo URL: the hex SHA-256 of the URL. Keying
// by a content hash of the URL makes the cache self-invalidating: when a merchant's
// stored logo URL changes, the new URL hashes to a new key that warms afresh, with no
// explicit invalidation and no stale serve.
func merchantLogoKey(logoURL string) string {
	sum := sha256.Sum256([]byte(logoURL))
	return hex.EncodeToString(sum[:])
}

// merchantLogoServedURL is the origin URL a row references for its cached logo: our own
// image endpoint, never the bank CDN.
func merchantLogoServedURL(key string) string {
	return MerchantLogoRoutePrefix + key
}
