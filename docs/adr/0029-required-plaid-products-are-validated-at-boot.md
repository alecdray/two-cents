# Required Plaid products are validated at boot

`PLAID_PRODUCTS` is checked at startup against a fixed table of products the app's shipped features
unconditionally depend on, and a configured list missing one refuses to start — extending
[0025](0025-live-bank-access-is-an-explicit-act.md)'s "loud failure at boot" rule from the Plaid
*environment* to the Plaid *product list*.

**Why.** A deployment can omit a required product and never know it. The card-statement feature
([0026](0026-statement-detail-is-an-enhancement.md)) reads the `liabilities` product; a deployment
still configured with the documented `transactions`-only default never requests consent for it, so
every existing bank Item returns a consent error the client deliberately treats as a benign per-card
fact rather than a failure — logged nowhere, indistinguishable from every connected bank genuinely
not supporting statement dates. 0026 was right that a missing product shouldn't alarm at the
*connection* level; it says nothing about the *deployment* never having asked for the product in the
first place, which is a configuration bug, not a bank fact, and belongs with 0025's other
config-drift failures.

**What's required, and why unconditionally.** Statement detail is shipped product scope, not a
speculative feature ([`vision.md`](../product/vision.md) names billing-cycle facts as in-scope) — so
`transactions` and `liabilities` are both required, regardless of whether the deployment's accounts
happen to include a credit card yet, mirroring 0025's refusal to make the environment check "smarter"
by branching on state a fresh boot can't yet know.

**Consequences.**
- A deployment with insufficient `PLAID_PRODUCTS` now fails to start, same posture 0025 already took
  for `PLAID_ENV`.
- This check catches configuration drift, not stale per-Item consent: an existing bank connection
  still needs Link's update-mode reconnect before Plaid actually serves the newly-required product,
  and a login that keeps failing after that is still 0026's per-card degrade path, not this one.
- The required-products table is a second map living beside `plaidOrigins`, so a future feature that
  adds a new provider dependency declares it there instead of trusting deployment docs to stay
  in sync.

**Rejected: hardcoding `PLAID_PRODUCTS` instead of validating it.** Removes the actual bug class
(nothing checks configured products against what the code needs) while losing real flexibility — the
Plaid plan this app runs on ([`vision.md`](../product/vision.md)) bills per product per Item, so a
fixed list forecloses ever trimming it in an environment that doesn't need everything.
