# On-demand, navigable sweep snapshots

The cash-sweep recommendation ([ADR-0020](../../adr/0020-monthly-cash-sweep-recommendation.md))
today is a single stored row that a monthly job overwrites, with no way to produce one
between runs and nothing kept once it is replaced. This work makes the sweep **runnable at
any time** and turns storage into an **append-only history of point-in-time snapshots** the
`/sweep` page can navigate. Each snapshot computes *now* — live balances and month-to-date
activity through the run instant — and is stamped with that moment; the monthly job keeps
firing but now **appends** a snapshot instead of replacing the latest. Rationale, the
persistence-model change, and the rejected alternatives are captured in
[ADR-0022](../../adr/0022-on-demand-navigable-sweep-snapshots.md).

## In scope

- A **Run now** action on `/sweep` that computes a fresh recommendation and appends it as a
  new snapshot, landing the user on that result.
- **Append-only persistence**: each snapshot is its own immutable row keyed by a generated
  id and ordered by its `computed_at` instant. All snapshots are retained (single-user, low
  volume — no pruning).
- **Navigation** on `/sweep`: the page defaults to the latest snapshot and steps older/newer
  through the history; each snapshot is deep-linkable. The first-run empty state stays until
  a first snapshot exists.
- The monthly job **appends** rather than upserts; its cadence and computation are otherwise
  unchanged. Snapshots record nothing about what triggered them — the job and the action
  produce the same kind of result.
- A **stale checking balance becomes a needs-attention reason**, applied uniformly to every
  caller, with the staleness rule itself exported from `accounts` (its owner) rather than
  redefined here.
- Snapshot labels gain **date + time** (manual runs make multiple-per-day snapshots
  possible), where the current view shows month only.
- Reconciling the canonical docs the change touches: ADR-0022, the `sweep` module
  `README.md`/`AGENTS.md`/package doc, and the domain `README.md` sweep entries — including
  retiring "monthly" as a property of a recommendation.

## Out of scope

- **Backdated / as-of compute.** A snapshot always reflects *now*; reconstructing balances
  and MTD as of an arbitrary past date is not supported (Plaid balances are point-in-time).
- **Executing or scheduling transfers.** The recommendation stays advisory; it never moves
  money.
- **Editing or deleting snapshots.** History is append-only and immutable.
- **The reserve model and account derivation** ([ADR-0020](../../adr/0020-monthly-cash-sweep-recommendation.md)):
  the formula, inputs, needs-attention rules, and multi-account aggregation deferral are
  unchanged. `Compute` already reads *now* and is reused as-is.
- **Retention limits / pruning.**
- **De-duplicating or throttling runs.** Every run appends, including one whose figures repeat
  the previous snapshot; the button is not rate-limited.
- **Changing the staleness threshold or how staleness is detected** ([ADR-0021](../../adr/0021-fault-isolating-sync-pass.md)).
  This work consumes the existing rule and makes it callable from outside `accounts`; it does
  not re-tune it, and stale *savings* keeps its existing non-blocking treatment.
