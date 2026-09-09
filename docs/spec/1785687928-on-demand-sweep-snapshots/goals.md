# Goals

What has to be true when this work merges, and the design decisions that got locked
down before any of it was built. The boundary of the work is in
[`scope.md`](scope.md); the durable rationale is
[ADR-0022](../../adr/0022-on-demand-navigable-sweep-snapshots.md).

## Outcomes

1. **The sweep can be run whenever the user wants one.** A **Run now** action on
   `/sweep` computes a fresh recommendation and lands the user on it. A plain page
   read still computes nothing.
2. **Nothing a run produced is ever lost.** Each run appends an immutable snapshot;
   the monthly job appends alongside manual runs instead of overwriting them.
3. **The history is navigable.** `/sweep` opens on the newest snapshot, steps
   older/newer, and every snapshot is deep-linkable and labelled with its date *and*
   time.
4. **A snapshot's label provably matches the figures it holds** — its instant is the
   instant the computation measured against, not the moment the row was written.
5. **The sweep refuses to advise on a balance it cannot vouch for.** A stale checking
   balance produces a needs-attention snapshot rather than a number resting on data a
   stuck sync left behind.
6. **The docs describe the new model, not the old one** — the module docs, the domain
   glossary and derivation card, and the ADR index no longer call a recommendation
   "monthly."

## Decisions locked during Spec

Each of these was reached by working a concrete scenario, and each closes off an
alternative that would otherwise look reasonable at Implement time.

- **Staleness has exactly one definition, and `accounts` owns it.** `accounts` exports
  the predicate; the overview and the sweep are both callers. *Rejected:* a second
  threshold in `sweep` — two numbers that could drift apart is worse than one that is
  imperfect for one caller.
- **The stale-checking rule is uniform across callers.** A sync stuck for longer than
  the threshold means the 7th appends a needs-attention snapshot instead of that
  month's baseline number, and that is correct: `Compute` must not learn who called
  it, and on-demand running is exactly what makes the missed baseline recoverable.
  *Rejected:* blocking only manual runs; refreshing balances before computing (the
  sweep would reach the provider, crossing ADR-0020's boundary).
- **Stale savings does not block**, for the same reason an unknown savings balance
  does not — savings is not a term in the formula.
- **Snapshots record nothing about what triggered them.** The job and the button run
  the same computation and produce the same kind of result. "Monthly" survives only
  as a *cadence*, never as a kind of recommendation. *Rejected:* an origin/trigger
  field, which would add an axis the formula never reads.
- **Every run appends — no de-duplication, no throttle.** A snapshot records that the
  user asked at a given instant and what the answer was; collapsing identical
  consecutive runs would destroy exactly that, and would stop a snapshot's instant
  from being the instant it was computed. *Rejected:* collapsing repeats; a cooldown
  on Run now.

## Carried into Implement

- The ADR was drafted as 0021 and **renumbered to 0022** — `main` took 0021 for the
  fault-isolating sync pass while this branch sat unrebased. The branch is now rebased
  onto that work, which is what put the stale-balance question on the table at all.
- **Migration:** `sweep_recommendation` moves from a single `id = 'default'` upserted
  row to one row per snapshot. At most one row can exist, so the pre-existing one is
  either carried forward as the first historical snapshot or dropped; decide at
  Implement.
- **`accounts` must export its staleness predicate**, and the `staleAfter`
  doc-comment's rationale should be rewritten to justify a domain rule rather than
  "what the overview presents as current."
- The **no-future-dated-transaction invariant** behind the month-to-date window's
  upper bound now has its doc-comment at the query site in `sweep/service.go`, which
  is where ADR-0022 points for it. It was missing when the ADR was drafted; the ADR
  should not be the only place it is written down.
