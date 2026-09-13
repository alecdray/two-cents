# sweep

Owns the **cash-sweep recommendation** — an advisory dollar amount and direction
(checking → savings, or the reverse) that relocates only idle checking cash. Every
run computes against the state of the world at that instant and **appends a
snapshot**; the `/sweep` page reads the resulting history. Runs come from the
scheduled monthly job and from the user's on-demand action, and are the same
computation either way. Advisory only: the recommendation never moves money.

Why the number is a dated cash-flow timeline rather than a budget-derived reserve,
and why the peak of that timeline is the figure:
[ADR-0024](../../../docs/adr/0024-cash-flow-timeline-sweep.md). Why runs are
on-demand, append-only, and navigable, and why a stale checking balance blocks:
[ADR-0022](../../../docs/adr/0022-on-demand-navigable-sweep-snapshots.md). Domain
framing: [`docs/domain/README.md`](../../../docs/domain/README.md) (§Cash sweep
recommendation), whose derivation card is the canonical arithmetic.

## Entities

- **Recommendation** — one snapshot: the result of a single run, stamped with the
  instant it was computed against and immutable thereafter. Either a **numeric**
  result carrying the balances, the required checking figure, the safety margin, the
  suggested sweep and its direction, **and the timeline it was computed from** — or a
  **needs-attention** result carrying the list of reasons the number could not be
  produced. Snapshots carry nothing about what triggered the run, and every snapshot
  is retained.
- **TimelineEvent** — one dated movement through checking: a date, a label, a
  direction, a positive amount, the running total it produced, and whether it is the
  peak. Storing the running total and the peak is what lets the page show the
  derivation without recomputing it, which the append-only model requires.

## The number

Every expected movement is placed on a dated timeline running from the run instant to
**exactly one month** later. What must stay in checking is the **highest point the
running total reaches** over that window; the suggested sweep is the checking balance
less that figure, less a flat **safety margin** (`FIXED_SAFETY_MARGIN`, default $500).
Its sign is the direction, and it is **not floored** — a negative value is a
meaningful pull back from savings.

**The peak, not the final total.** The running total's end value is what the month
nets out to; its maximum is what must be present for the balance never to go
negative. Taking the maximum is also what confines an inflow to offsetting only what
follows it — a paycheck on the 30th cannot pay a bill due on the 20th.

The safety margin is headroom, not a term: it gives room before an undeclared outflow
becomes dangerous, and is not sized to cover one.

## What reaches the timeline

- **Every active credit Account** contributes what it owes. With no statement detail
  held, that is a known dollar value with no known date, so the missing-date rule
  places the **whole balance at the run instant** — the most conservative reading. A
  card owing nothing contributes no row.
- **Every active scheduled item** contributes its occurrences inside the window, from
  the `schedule` module. An occurrence still ahead lands on its own date. One dated
  earlier today has already come due: an **outflow** lands at the run instant, still
  owed, and an **inflow** is omitted, because it may never arrive.

The window looks only forward. An occurrence that fell due before today is not
reached back for: deciding whether a past occurrence was actually paid is a
*reconciliation* of what was declared against what happened, not a wider window.

Ordering is by date, and on the same date **outflows before inflows** — never assume
a deposit clears before a debit posted the same day.

## Account derivation & needs-attention

Checking is the single active cash Account with counts-as-savings false; savings the
single active counts-as-savings cash Account. Every active credit Account counts —
debt is additive, so no number of cards is ambiguous or a failure.

A needs-attention result lists **every** applicable reason, never just the first.
Every blocking reason is a missing **dollar value**: checking absent or ambiguous, a
checking balance unreported or stale, or any active card's balance unreported or
stale. A missing *date* never blocks — it degrades to the worst case, so the number
still forms and is simply more conservative.

Savings **never blocks**. It is not a term in the arithmetic, so absent, ambiguous,
unreported or stale all read as "unknown" on the snapshot. An empty schedule does not
block either: the timeline simply holds only the cards.

Staleness is `accounts`' rule and `accounts`' threshold ([ADR-0021](../../../docs/adr/0021-fault-isolating-sync-pass.md));
this module consumes it and never restates it. It is checked at the run instant, so
it applies identically to a scheduled and an on-demand run — a stuck sync costs the
7th its number rather than quietly degrading it.

## Boundaries

Reaches the bank only through `accounts` (derived checking/savings by the
counts-as-savings flag, [ADR-0008](../../../docs/adr/0008-account-kind-and-savings-overrides.md);
their current balances, the credit balances the cards contribute, and the statement
detail that dates them), and reads the declared activity through `schedule`. It
**reads no provider client**, and no loan APR or interest detail. It reads neither the
**budget** nor the **ledger**: the budget is whole-of-spending, so reserving it
alongside a declared outflow would hold the same money twice, and the timeline
carries what is owed or scheduled, never what was spent. It writes only its own
`sweep_recommendation` table. Calendar dates are reckoned in the
[configured app timezone](../../../docs/adr/0004-configured-app-timezone.md).

## Service

- `NewService(...)` — injected the peer services, the app timezone, and the safety
  margin at the composition root.
- `Compute(ctx) → Recommendation` — derives the accounts, loads the schedule, builds
  and evaluates the timeline, and returns the numeric or needs-attention result.
  Reads only; persists nothing. The clock is read **once** per run, in the app
  timezone, and threaded through the derivation, the horizon, and the stamped
  instant, so a snapshot cannot describe a state of the world that never existed.
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
  needs-attention snapshot. Instants, including the timeline's dates, come back in
  the [configured app timezone](../../../docs/adr/0004-configured-app-timezone.md).

`Recommendation.Horizon()` derives the far end of the window a snapshot measured from
its own stamped instant, so an old snapshot describes the window it actually covered
rather than one reckoned from today.

## Schedule and the page

A background job runs on the **7th** of each month at 00:00 in the configured app
timezone, appending a snapshot. The user's **Run now** action on `/sweep` appends one
the same way, landing on the fresh result; a plain page read still computes nothing.
Neither run knows about the other, and neither is privileged.

`/sweep` opens on the newest snapshot and steps older/newer through the history;
`/sweep/{id}` addresses one snapshot directly, and an unknown id is a 404. Labels
carry date **and** time of day, since manual runs make several snapshots a day
routine. Below the snapshot, the page renders the **sweep schedule** — the `schedule`
module's CRUD surface, which lives here because the sweep is its only consumer. The
two are independent swap regions: a schedule edit does not disturb the snapshot on
screen, because a snapshot records what was advised at an instant and re-running is
how the user sees the effect of a change.

## Persistence

- `sweep_recommendation` — one row per snapshot, inserted never updated, keyed by a
  generated id and ordered by the instant the run computed against (stamped from that
  instant, not from the write). Holds the balances (savings nullable, for "unknown"),
  the required-checking figure, the margin, the sweep and its direction, the
  needs-attention reasons as a JSON list, and the timeline as a JSON list of events.
  All rows are kept — no pruning, no retention window.
