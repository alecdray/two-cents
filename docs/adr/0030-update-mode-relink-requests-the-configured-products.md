# Update-mode relink requests the configured products

Every update-mode `/link/token/create` request — driven by both "Reconnect" and "Ask this bank
again" — sends the app's configured Plaid products as `additional_consented_products`, the field
Plaid reads to grant a product an existing Item hasn't consented to yet. Update mode's other field,
`products`, is rejected outright alongside an `access_token`, so a client that sends neither can never
add a product to a login after the fact — only re-run the consent the login already has.

**Why.** [0029](0029-required-plaid-products-are-validated-at-boot.md) makes the deployment declare
every product its features need, but a login created before a product was added carries only its
original consent — Plaid, not this app, decides what an Item has granted. Relink was already the
resolution path for exactly this gap ([0026](0026-statement-detail-is-an-enhancement.md) names
re-consent through Link as the remedy for a refused product), but the request it sent never asked
for anything beyond the existing login, so clicking either control silently re-confirmed the old
consent and left the product still missing.

**What is requested.** The full configured product list, not a computed "missing" subset — this app
holds no record of what a given Item has already consented to (that is Plaid's state, not ours), and
Plaid's docs describe a product already granted through this flow as simply staying granted rather
than erroring. Reusing the one existing config value keeps this a single source of truth rather than
a second list that could drift from it.

**Consequences.**
- Both relink controls now request the same set, since they already drive one shared update-mode
  code path — no per-control distinction needed.
- This is the piece that makes [0029](0029-required-plaid-products-are-validated-at-boot.md)'s boot
  check actionable: fixing a deployment's `PLAID_PRODUCTS` still requires each existing connection to
  relink once before the new product is actually granted, and this is what makes that relink work
  rather than silently no-op.
- **The field and its constraints are confirmed against Plaid's docs**
  ([Link - Update mode](https://plaid.com/docs/link/update-mode/)), not guessed: it is the documented
  way to add consent to an existing Item, and it cannot carry a product already listed in `products`
  or `required_if_supported_products` — moot here since update mode omits both.
- **Unconfirmed against a live login**, in the same sense [0028](0028-a-card-reserves-its-unpaid-statement.md)
  flagged its payment fields: no deployed instance has yet shown a real institution's update-mode flow
  actually honoring the request and returning statement data afterward. If an issuer doesn't, the card
  stays `statements_unavailable` after relinking and a fresh connect (not a relink) remains the
  fallback — that gap is not this decision's to close.
