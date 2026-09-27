# Update-mode relink actually requests new products

"Reconnect" and "Ask this bank again" both mint an update-mode Plaid Link token, but the request
carries nothing beyond the login's `access_token` — Plaid forbids `products` alongside it, and the
client never sends the field Plaid actually reads for this case, `additional_consented_products`. So
neither control can grant a product added after the Item was first linked: a bank flagged
`statements_unavailable` because the deployment's `PLAID_PRODUCTS` config once omitted `liabilities`
(ADR-0029) stays flagged forever, even after the config is fixed, because update mode re-runs only
the login the user already consented to. This chunk sends the configured `PLAID_PRODUCTS`
(`c.link.Products`) as `additional_consented_products` on every update-mode request, so both remedies
actually ask the bank for whatever the app now requires that the login hasn't granted yet.
