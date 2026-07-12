-- name: GetMerchantLogo :one
-- The cached image bytes and content type for a positively cached logo key. A
-- negative entry (NULL image_bytes) is excluded, so a cache miss and a negative key
-- both return no row: the image endpoint serves bytes only for a positively cached
-- key, and never for one recorded as having no usable logo.
SELECT content_type, image_bytes
FROM merchant_logo_cache
WHERE logo_key = ? AND image_bytes IS NOT NULL;

-- name: PutMerchantLogo :exec
-- Record a positively cached logo: its bytes and stored content type under the URL's
-- content-hash key. Idempotent by key so a re-warm rewrites the same key in place.
INSERT INTO merchant_logo_cache (logo_key, logo_url, content_type, image_bytes)
VALUES (?, ?, ?, ?)
ON CONFLICT (logo_key) DO UPDATE SET
    logo_url     = excluded.logo_url,
    content_type = excluded.content_type,
    image_bytes  = excluded.image_bytes,
    updated_at   = CURRENT_TIMESTAMP;

-- name: PutMerchantLogoMiss :exec
-- Record a negative cache entry: a logo URL that yielded no usable logo. image_bytes
-- stays NULL and content_type empty, so the warm step attempts the key exactly once
-- and the image endpoint never serves it. DO NOTHING on conflict so it never clobbers
-- an existing positive entry for the same key.
INSERT INTO merchant_logo_cache (logo_key, logo_url, content_type, image_bytes)
VALUES (?, ?, '', NULL)
ON CONFLICT (logo_key) DO NOTHING;

-- name: ListCachedLogoKeys :many
-- Every logo key already in the cache, positive or negative: the set the warm step
-- skips so no already-attempted merchant is fetched again on a later sync.
SELECT logo_key FROM merchant_logo_cache;

-- name: ListPositiveLogoKeys :many
-- Every positively cached logo key (a stored image). The read model consults this set
-- to fill a row's served logo URL only when the logo is actually cached, so a miss or
-- a negative entry leaves the row without a logo (its category glyph shows instead).
SELECT logo_key FROM merchant_logo_cache WHERE image_bytes IS NOT NULL;

-- name: ListMerchantLogoURLsByRecency :many
-- Each distinct non-empty merchant logo URL across the whole stored transaction set,
-- most recent first by the merchant's latest transaction date. The warm step walks
-- this order, skips already-cached keys, and fetches only a bounded batch, so a large
-- backlog drains over successive syncs with the current month's merchants warmed first.
SELECT logo_url
FROM transactions
WHERE logo_url != ''
GROUP BY logo_url
ORDER BY MAX(date) DESC;
