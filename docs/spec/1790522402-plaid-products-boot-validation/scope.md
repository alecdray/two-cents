# Validate PLAID_PRODUCTS at boot

Production's `PLAID_PRODUCTS` was left at the documented default (`transactions`), so `liabilities`
was never requested and every existing bank Item was linked without consent for it — the app then
spent months silently marking every card `statements_unavailable` (ADR-0026's designed degrade path)
with no logged reason, indistinguishable from every connected bank genuinely lacking the feature.
This chunk extends [ADR-0025](../../adr/0025-live-bank-access-is-an-explicit-act.md)'s "loud failure
at boot" rule from `PLAID_ENV` to `PLAID_PRODUCTS`: a deployment whose configured products don't
cover what the code unconditionally depends on refuses to start, so the gap is caught at boot rather
than reasoned about per-card, after the fact, from a flag that can't tell "never asked" from "bank
won't serve it."
