# sweep

Owns the monthly **cash-sweep recommendation** — an advisory dollar amount and
direction (checking → savings, or the reverse) that relocates only idle checking
cash. A scheduled job computes it once a month and **persists the latest snapshot**;
the `/sweep` page reads it. Advisory only: the recommendation never moves money, and
the user's own budgeted savings transfer is reserved for them, never swept.

Why the number is shaped the way it is (the reserve model, the persisted-snapshot
choice, the 7th-of-month schedule): [ADR-0020](../../../docs/adr/0020-monthly-cash-sweep-recommendation.md).
Domain framing: [`docs/domain/README.md`](../../../docs/domain/README.md)
(§Cash sweep recommendation).

## Entities

- **Recommendation** — the persisted monthly snapshot. Either a **numeric** result
  carrying every figure that produced it — current checking, current savings (or
  "unknown"), total spending budget, month-to-date spending from checking, savings
  target, month-to-date savings contributed, the reserve, the safety margin, the
  suggested sweep, and its direction — or a **needs-attention** result carrying the
  list of reasons the number could not be produced. Only the latest is kept.

## The number

`suggested_sweep = current_checking − reserve − fixed_safety_margin`, where
`reserve = max(0, total_spending_budget − mtd_spending_from_checking) + max(0,
savings_target − mtd_savings_contributed)` and each reserve component is floored at
zero **independently**. Direction is the sign of the sweep. Exact definitions and
rationale live in [ADR-0020](../../../docs/adr/0020-monthly-cash-sweep-recommendation.md);
`fixed_safety_margin` is a config constant (`FIXED_SAFETY_MARGIN`, default $500).

## Boundaries

Reaches the bank only through peer services — `accounts` (derived checking/savings
by the counts-as-savings flag, [ADR-0008](../../../docs/adr/0008-account-kind-and-savings-overrides.md);
their current balances), `budget` (the spending/savings targets), and `transactions`
(the month-to-date checking activity, savings-contribution transfers via the
transfer-subtype detection, [ADR-0003](../../../docs/adr/0003-two-layer-transfer-detection.md)).
It **reads no provider client and no card/liability balance**, and writes only its
own `sweep_recommendation` table. Month reckoning uses the
[configured app timezone](../../../docs/adr/0004-configured-app-timezone.md).

## Service

- `NewService(...)` — injected the peer services, the app timezone, and the safety
  margin at the composition root.
- `Compute(ctx) → Recommendation` — derives the accounts, gathers the inputs, and
  returns the numeric or needs-attention result. Reads only; persists nothing.
- `SaveLatest(ctx, Recommendation)` / `LoadLatest(ctx) → (Recommendation, found)` —
  store and read the single latest snapshot. `found == false` before any run has
  stored one, distinct from a needs-attention result.

## Account derivation & needs-attention

Checking is the single active cash Account with counts-as-savings false; savings the
single active counts-as-savings cash Account. Ambiguous (more than one) or absent
either side, or an **unknown checking balance**, yields a needs-attention result
listing **every** applicable reason. A missing budget is *not* needs-attention (its
terms are zero, a numeric result still forms); an **unknown savings balance** is
*not* blocking (savings is not a formula term) — the figure shows "unknown".

## Schedule

A background job runs on the **7th** of each month at 00:00 in the configured app
timezone, computing and replacing the latest. Re-runs replace rather than
accumulate. No on-demand compute in v1 — the `/sweep` page only reads the latest.

## Persistence

- `sweep_recommendation` — single row (`id` fixed `'default'`), upserted on each run.
  Holds every numeric figure (savings balance nullable, for "unknown") plus the
  needs-attention reasons as a JSON list.
