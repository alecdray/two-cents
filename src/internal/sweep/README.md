# sweep

Owns the **cash-sweep recommendation** — an advisory dollar amount and direction
(checking → savings, or the reverse) that relocates only idle checking cash. Every
run computes against the state of the world at that instant and **appends a
snapshot**; the `/sweep` page reads the resulting timeline. Runs come from the
scheduled monthly job and from the user's on-demand action, and are the same
computation either way. Advisory only: the recommendation never moves money, and the
user's own budgeted savings transfer is reserved for them, never swept.

Why the number is shaped the way it is (the reserve model, the persisted-snapshot
choice, the 7th-of-month schedule): [ADR-0020](../../../docs/adr/0020-monthly-cash-sweep-recommendation.md).
Why runs are on-demand, append-only, and navigable, and why a stale checking balance
blocks: [ADR-0022](../../../docs/adr/0022-on-demand-navigable-sweep-snapshots.md).
Domain framing: [`docs/domain/README.md`](../../../docs/domain/README.md)
(§Cash sweep recommendation).

## Entities

- **Recommendation** — one snapshot: the result of a single run, stamped with the
  instant it was computed against and immutable thereafter. Either a **numeric**
  result carrying every figure that produced it — current checking, current savings
  (or "unknown"), total spending budget, month-to-date spending from checking,
  savings target, month-to-date savings contributed, the reserve, the safety margin,
  the suggested sweep, and its direction — or a **needs-attention** result carrying
  the list of reasons the number could not be produced. Snapshots carry nothing about
  what triggered the run, and every snapshot is retained.

## The number

The suggested sweep is current checking minus a **reserve** — the month's unspent
budget, the budgeted savings not yet moved, and the uncovered card debt — minus a flat
**safety margin**; its sign is the direction. The card term exists because the budget
reserves what you *planned* to spend while the card records what you *did*: past the
budget, the budget term stops and the bill does not
([ADR-0023](../../../docs/adr/0023-uncovered-card-debt-reserve.md)). The exact formula, the
derivation inputs, and the rationale are in
[ADR-0020](../../../docs/adr/0020-monthly-cash-sweep-recommendation.md) and the
derivation card in [`docs/domain/README.md`](../../../docs/domain/README.md) (§Cash
sweep recommendation). `fixed_safety_margin` is a config constant
(`FIXED_SAFETY_MARGIN`, default $500).

## Boundaries

Reaches the bank only through peer services — `accounts` (derived checking/savings
by the counts-as-savings flag, [ADR-0008](../../../docs/adr/0008-account-kind-and-savings-overrides.md);
their current balances, and the credit balances the card term reserves against),
`budget` (the spending/savings targets), and `transactions`
(the month-to-date checking activity, savings-contribution transfers via the
transfer-subtype detection, [ADR-0003](../../../docs/adr/0003-two-layer-transfer-detection.md)).
It **reads no provider client**, and no liabilities product — no statement balance,
no due date, no APR ([ADR-0023](../../../docs/adr/0023-uncovered-card-debt-reserve.md)
reverses ADR-0020's wider no-card-balance rule, keeping the narrower one). It writes
only its own `sweep_recommendation` table. Month reckoning uses the
[configured app timezone](../../../docs/adr/0004-configured-app-timezone.md).

## Service

- `NewService(...)` — injected the peer services, the app timezone, and the safety
  margin at the composition root.
- `Compute(ctx) → Recommendation` — derives the accounts, gathers the inputs, and
  returns the numeric or needs-attention result. Reads only; persists nothing. The
  clock is read **once** per run and threaded through the derivation, the
  month-to-date window, and the stamped instant, so a snapshot cannot describe a
  state of the world that never existed.
- `Run(ctx) → Recommendation` — compute, then append. The single path that produces
  a snapshot: the monthly job and the page's action both call it, and it cannot tell
  them apart.
- `Save(ctx, Recommendation)` — append the snapshot. Never replaces a previous one;
  a run whose figures repeat the last snapshot still appends. The id is assigned
  before saving, so the caller knows the new snapshot's address.
- `Snapshot(ctx, id) → (Snapshot, found)` — one snapshot positioned in the history,
  with the ids to step older and newer (empty at the ends). An empty id selects the
  newest, which is what a plain page load wants; an id that is not in the history is
  `found == false`, never a silent fall back to a different snapshot. Before any run
  it reports not-found — the first-run empty state, distinct from a stored
  needs-attention snapshot. Instants come back in the
  [configured app timezone](../../../docs/adr/0004-configured-app-timezone.md).

This is the whole read surface the page uses; the repo keeps the narrower reads
`Snapshot` is built from.

## Account derivation & needs-attention

Checking is the single active cash Account with counts-as-savings false; savings the
single active counts-as-savings cash Account. Every active credit Account counts, and
they sum — debt is additive, so no count of cards is ambiguous or a failure. Ambiguous
(more than one) or absent checking/savings, or an **unknown or stale balance on
checking or on any card**, yields a needs-attention result listing **every** applicable
reason. A missing budget is
*not* needs-attention (its terms are zero, a numeric result still forms); an unknown
**or stale savings balance** is *not* blocking (savings is not a formula term) — the
figure shows "unknown".

Staleness is `accounts`' rule and `accounts`' threshold ([ADR-0021](../../../docs/adr/0021-fault-isolating-sync-pass.md));
this module consumes it and never restates it. It is checked at the run instant, so
it applies identically to a scheduled and an on-demand run — a stuck sync costs the
7th its number rather than quietly degrading it.

## Schedule and the page

A background job runs on the **7th** of each month at 00:00 in the configured app
timezone, appending a snapshot. The user's **Run now** action on `/sweep` appends one
the same way, landing on the fresh result; a plain page read still computes nothing.
Neither run knows about the other, and neither is privileged.

`/sweep` opens on the newest snapshot and steps older/newer through the history;
`/sweep/{id}` addresses one snapshot directly, and an unknown id is a 404. Labels
carry date **and** time of day, since manual runs make several snapshots a day
routine.

## Persistence

- `sweep_recommendation` — one row per snapshot, inserted never updated, keyed by a
  generated id and ordered by the instant the run computed against (stamped from that
  instant, not from the write). Holds every numeric figure (savings balance nullable,
  for "unknown"; card balance defaulting to 0 on snapshots that predate the term)
  plus the needs-attention reasons as a JSON list. All rows are kept —
  no pruning, no retention window.
