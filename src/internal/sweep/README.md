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
less that figure, less a flat **safety margin** — a stored value, editable from the
page and read fresh on every run
([ADR-0032](../../../docs/adr/0032-safety-margin-is-a-stored-value.md)).
Its sign is the direction, and it is **not floored** — a negative value is a
meaningful pull back from savings.

**The peak, not the final total.** The running total's end value is what the month
nets out to; its maximum is what must be present for the balance never to go
negative. Taking the maximum is also what confines an inflow to offsetting only what
follows it — a paycheck on the 30th cannot pay a bill due on the 20th.

The safety margin is headroom, not a term: it gives room before an undeclared outflow
becomes dangerous, and is not sized to cover one.

## What reaches the timeline

- **Every active credit Account** contributes its statement's **unpaid remainder** —
  the billed figure less any payment the bank reports dated strictly after the
  statement issued, and never more than the card's current balance — on the date its
  payment schedule resolves to: the reported due date by default, or the statement's
  issue date plus a fixed number of days. A resolved date already past lands at the run
  instant; one beyond the horizon contributes nothing; and a card still owing nothing
  contributes no row.

  A card whose bank reports **no statement**, or whose schedule has no date to work
  from, is a known dollar value with no known date, so the missing-date rule places
  the **whole current balance at the run instant** — the most conservative reading.

  **The current balance is a ceiling, not a payment record**
  ([ADR-0028](../../../docs/adr/0028-a-card-reserves-its-unpaid-statement.md)). It bounds the
  *unpaid* figure, never the billed one: a cap on the billed figure would carry this
  cycle's unbilled spend onto the timeline, which
  [ADR-0026](../../../docs/adr/0026-statement-detail-is-an-enhancement.md) forbids.
  Releasing is the reported payment's job; the balance only stops the sweep reserving
  more than the card can claim. Every way of not knowing the payment
  subtracts nothing, so the figure degrades to the full statement under that ceiling.
- **Every active scheduled item** contributes its occurrences inside the window, from
  the `schedule` module — except the ones already **settled**, which carry a match to
  the transaction that satisfied them and never reach the timeline at all. An
  occurrence still ahead lands on its own date. One that has already come due and is
  unmatched is still owed: an **outflow** lands at the run instant carrying the date it
  was originally due, and an **inflow** is omitted, because it may never arrive. A
  **dateless** item ([ADR-0031](../../../docs/adr/0031-scheduled-items-bootstrap-and-resolve-from-matches.md))
  has no "still ahead" — its occurrence is always either settled or already due, so it
  always takes this same branch while unmatched, for as long as its cadence interval
  stays open.

The window reaches back **one cadence interval** per item, which the match record makes
safe — a reconciliation of what was declared against what actually happened, rather
than a wider window. One interval and no more: an occurrence that will never be matched
stays a single finite error that the next one replaces, instead of stacking with nothing
able to bring the number down. This module is told *which* occurrences are settled and
never *what* settled them; it reads no ledger.

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
block either: the timeline simply holds only the cards. Nor does a scheduled item
with **no amount figure at all** — no declared override and no matched history to
compute one from
([ADR-0031](../../../docs/adr/0031-scheduled-items-bootstrap-and-resolve-from-matches.md)):
it contributes nothing rather than blocking, since the item genuinely has no evidence
yet of what it costs, unlike an account balance the bank simply hasn't reported.

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
`sweep_recommendation` and `sweep_settings` tables. Calendar dates are reckoned in the
[configured app timezone](../../../docs/adr/0004-configured-app-timezone.md).

## Service

- `NewService(...)` — injected the peer services and the app timezone at the
  composition root. The safety margin is **not** a construction-time input — it is
  stored and read fresh per call, same as every other persisted value.
- `Compute(ctx) → Recommendation` — derives the accounts, loads the schedule, builds
  and evaluates the timeline, and returns the numeric or needs-attention result.
  Reads only; persists nothing. The clock and the **stored safety margin** are each
  read **once** per run and threaded through the derivation, the horizon, and the
  stamped instant, so a snapshot cannot describe a state of the world — or a margin
  — that never existed at that moment
  ([ADR-0032](../../../docs/adr/0032-safety-margin-is-a-stored-value.md)).
- `SafetyMargin(ctx) → float64` — the stored value, exactly as `Compute` would read
  it. What a plain page read shows and the edit control prefills its input from.
- `SetSafetyMargin(ctx, float64)` — updates the stored value in place. Takes effect
  on the *next* run; never touches an already-saved snapshot's own
  `FixedSafetyMargin`. `0` is valid (a deliberate no-cushion choice); negative is a
  `ValidationError`.
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
module's CRUD and matching surface, which lives here because the sweep is its only
*rendering* consumer. The two are independent swap regions: a schedule edit does not
disturb the snapshot on screen, because a snapshot records what was advised at an
instant and re-running is how the user sees the effect of a change.

The safety margin figure in the breakdown is itself a small inline edit control
([ADR-0032](../../../docs/adr/0032-safety-margin-is-a-stored-value.md)):
the plain figure carries no qualifier — just the dollar amount, like every other
breakdown row. Clicking it opens a one-field form in place, prefilled with the
current stored value; saving validates and updates it. The control then
re-renders itself from the new value. It never disturbs the snapshot on screen —
the displayed recommendation keeps the margin it actually ran with, and the new
figure takes effect on the next run.

The schedule renders as a **compact list**, built for a quick read of what's declared
and its current status. Each row's amount is labeled by which of four states it's in
— crossing whether an override is set against whether a computed figure exists at all
([ADR-0031](../../../docs/adr/0031-scheduled-items-bootstrap-and-resolve-from-matches.md)):
**forecast** (no override, a computed figure exists — the common case once an item has
history), **no estimate** (no override, nothing computed yet — a brand-new item with
no matches in its trailing window, prompting the user to set one), **manual** (an
override, and nothing computed to compare it to), and **manual override** (an override
that is now superseding a computed figure that does exist). Each item's full form and
its occurrence history — settled (and whether the app or the user decided it),
outstanding, or cleared, with the controls to settle one, confirm an automatic match,
or clear it — sit behind a **modal editor** opened per item, reusing the shared modal
shell and the
cross-modal-return pattern the Rule editor established
([ADR-0011](../../../docs/adr/0011-reusable-transaction-editing-modal.md),
[ADR-0016](../../../docs/adr/0016-rule-editor-modal-and-cross-modal-return.md),
[ADR-0031](../../../docs/adr/0031-scheduled-items-bootstrap-and-resolve-from-matches.md)).
The editor's amount field holds only a declared override — blank for an item left to
compute, with its current forecast shown beside it — so saving an unrelated edit never
freezes a computed figure into an override.
That editor is a second origin's destination too: the transaction editor links into it
by URL, passing an opaque reopen handle back to itself, to create or attach a
schedule item from a transaction without either page importing the other's views.
Matching changes what the *next* run computes, never what a stored snapshot did, so
the snapshot region gains no controls.

Every save, delete, or match decision made through that modal announces the
`schedule-changed` event ([ADR-0010](../../../docs/adr/0010-event-driven-cross-region-refresh.md)),
so this list and the `/transactions` list's per-row match badge both self-refresh
without a reload, regardless of which origin opened the modal or which of the two
lists is the one currently loaded. This region's own hidden listener re-fetches it
from `GET /sweep/schedule` — the plumbing that keeps this page current even when the
mutating response went to a transaction's editor instead of back here.

## Persistence

- `sweep_recommendation` — one row per snapshot, inserted never updated, keyed by a
  generated id and ordered by the instant the run computed against (stamped from that
  instant, not from the write). Holds the balances (savings nullable, for "unknown"),
  the required-checking figure, the margin, the sweep and its direction, the
  needs-attention reasons as a JSON list, and the timeline as a JSON list of events.
  All rows are kept — no pruning, no retention window.
- `sweep_settings` — a single row holding the safety margin
  ([ADR-0032](../../../docs/adr/0032-safety-margin-is-a-stored-value.md)), seeded by
  migration at $500. Updated in place, never appended — unlike a recommendation, a
  setting has no history worth keeping.
