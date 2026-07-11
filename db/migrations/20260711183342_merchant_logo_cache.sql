-- +goose Up
-- The merchant-logo cache: image bytes fetched once from a transaction's bank-sourced
-- logo URL and served from our own origin, so a transaction row shows a merchant logo
-- without the browser ever hotlinking a third-party CDN (ADR-0019). Keyed by a content
-- hash of the logo URL, so a merchant's changed URL is simply a new key that warms
-- afresh with no explicit invalidation and no stale serve. A row whose image_bytes is
-- NULL is a negative entry: the URL yielded no usable logo (absent, failed, or refused
-- by the fetch policy), recorded so the warm step attempts the key exactly once instead
-- of retrying a dead fetch every sync. Holds no user state, only a cache of bank data,
-- so it carries no foreign keys and is rebuildable — it can be dropped and rewarmed.
-- +goose StatementBegin
CREATE TABLE merchant_logo_cache (
    logo_key     TEXT PRIMARY KEY,
    logo_url     TEXT NOT NULL,
    content_type TEXT NOT NULL DEFAULT '',
    image_bytes  BLOB,
    created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE merchant_logo_cache;
-- +goose StatementEnd
