# Two-layer transfer detection

Transfers (money moved between the user's own accounts) must be excluded from spending and income or every aggregate double-counts. We detect them in two layers: (1) **classification** from the bank-provided transaction `type` (e.g. `transfer`, `card_payment`) on a single transaction — no pairing; (2) **destination/subtype** (savings contribution vs. credit-card payment vs. plain transfer) resolved by **pairing** the inflow leg on another connected account (exact amount, ±3-day window).

This split exists because of a hard constraint in Teller's schema: a transaction carries `type` and a `counterparty` of only `{name, type}`, but **no reference to the destination account** (`links` exposes only the source). So we can cheaply and reliably know *that* something is a transfer, but not *where it went* — and "where it went" is exactly what decides whether it counts as a savings contribution.

Consequences:
- A transfer to/from an account **not connected** to Two Cents can't have its destination resolved (we see one leg only); it stays a Transfer with unknown destination and **cannot count as a savings contribution** until the user marks it.
- Pairing is deliberately **conservative** (exact amount, ±3 days, leave ambiguous matches unmatched) because a *false* pair silently hides real spending — worse than a missed pair the user can correct.
- The user can always override any transaction's classification manually; that override wins over both layers.

Rejected: pure pairing to detect transfers (fragile, and unnecessary now that `type` classifies most); trusting `counterparty.name` alone (unstructured, no account identity).
