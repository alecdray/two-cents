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

## How it landed

All six outcomes shipped, and the decisions above held — none was revisited under
contact with the code. What is worth recording is where implementation *added* to
the design rather than diverging from it:

- **The clock is read once per run.** `Compute` previously called it twice — once for
  the account derivation and once for the month-to-date window. Harmless when the only
  output was a monthly figure; not harmless once the same instant also becomes the
  snapshot's label and its staleness check. It is now read once and threaded through,
  so a snapshot cannot describe a state of the world that never existed.
- **Stale checking is its own reason, not the existing catch-all.** An unknown checking
  balance already produced "checking undetermined", and folding staleness in there would
  have told the user to go designate an account when the account is perfectly well
  identified and the sync is what needs fixing.
- **The snapshot id is assigned by the service, not the repo**, following the convention
  `accounts` and `categorization` already use — and it means the run that produced a
  snapshot knows its address without reading it back, which is what lets Run now land
  the user on the result.
- **The table was rebuilt rather than altered.** `computed_at` has to be `NOT NULL`, and
  SQLite cannot add a NOT NULL column with a non-constant default. The one pre-existing
  row is carried forward as the first historical snapshot (the call left open at Spec),
  so the page does not fall back to the empty state on deploy.
- **Navigation is a pure function over the ordered history**, which kept the stepping
  logic out of the handler and testable on its own.
- **`accounts` exports `Account.BalanceStale`**, and `staleAfter`'s doc-comment now
  justifies a domain rule rather than "what the overview presents as current".
- **The invariant doc-comment** behind the month-to-date window's upper bound was
  written at the query site, where ADR-0022 points for it.

Coverage: unit tests at each layer (derivation, the run instant, append-only
persistence, navigation, and the page's render paths), plus an e2e feature exercising
the one thing units cannot reach — the real Run now round-trip, stepping back to an
earlier snapshot, and a stale balance blocking a number. Gate green: `go build ./...`,
`go test ./src/...`, and `task test/e2e` (82 passing).

One pre-existing gap was closed on the way past: the sweep's testids were never
registered in `docs/design/testids.md`, so the new controls are registered along with
the ones that were already there.
