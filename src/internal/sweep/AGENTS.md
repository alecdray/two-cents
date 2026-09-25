# sweep — domain module

Rules: ../../../docs/architecture/archetypes/domain-module.md

Owns the **cash-sweep recommendation**: places every expected movement through
checking on a dated timeline, holds back the worst point it reaches, and appends the
result as a snapshot for the `/sweep` page to read and navigate. Behaviour and the
read surface: [`README.md`](README.md). Domain authority:
[ADR-0024](../../../docs/adr/0024-cash-flow-timeline-sweep.md),
[ADR-0022](../../../docs/adr/0022-on-demand-navigable-sweep-snapshots.md);
[`docs/domain/README.md`](../../../docs/domain/README.md) §Cash sweep recommendation,
whose derivation card is the canonical arithmetic — read it rather than re-deriving
the formula from here.

Invariants a refactor could silently break:

- **Take the cumulative maximum, never the sum.** Why the peak rather than the total
  is derived in the [Cash sweep recommendation card](../../../docs/domain/README.md);
  what matters here is that it is structural, not a rule the arithmetic remembers, so
  do not "simplify" the evaluator into a sum. The sweep itself is **not** floored — a
  negative value is a meaningful pull — and money uses the app-wide outflow-positive
  sign convention.
- **The horizon is exactly one month; the occurrence window reaches back exactly one
  cadence interval.** One month is the length at which the window holds exactly one
  occurrence of every monthly item; the window ends the day *before* the horizon's own
  date so a run landing on an item's day does not count it twice. Lengthening the
  horizon never removes truncation — it moves the cut, and moving it past an outflow
  without also passing the income that covers it makes the answer worse. Reaching
  *backwards* is a different thing entirely: it is a reconciliation of declaration
  against ledger, safe only because a settled occurrence carries a match and drops out
  before it is placed ([ADR-0027](../../../docs/adr/0027-occurrence-matching-reconciles-the-schedule.md)).
  One interval, per item, and no more — at two, an occurrence that will never be
  matched is reserved twice, at three, three times, with no event able to bring the
  number back down. Without matching, the same rule would reserve every monthly item
  twice for the whole month.
- **A settled occurrence is known by its match, and the match is `schedule`'s.** This
  module asks *which* occurrences are settled and never *what* settled them: taking the
  transaction id would hand it a ledger reference it must then be trusted not to follow,
  and the ledger edge below is guarded precisely because re-adding it is the
  natural-looking way back to the superseded model.
- **Same-day ordering puts outflows before inflows.** Never assume a deposit clears
  before a debit posted the same day.
- **Unknowns are asymmetric on purpose.** A missing dollar value fails hard
  (needs-attention, listing *every* applicable reason); a missing date degrades to
  the worst case — an outflow at the run instant, an inflow omitted. Every unknown
  makes the number more conservative and none makes it less, which is what makes an
  incompletely-informed run safe rather than merely tolerable.
- **A card reserves its unpaid statement, bounded by its balance.** The contribution is
  the billed figure less any payment the bank reports dated *strictly after* the
  statement issued, capped at the current balance; for a card with no statement at all
  the missing-date rule reserves the whole balance. **The cap belongs on the unpaid
  figure, never the billed one**
  ([ADR-0028](../../../docs/adr/0028-a-card-reserves-its-unpaid-statement.md)): capping
  the billed figure carries unbilled spend onto the timeline, which
  [ADR-0026](../../../docs/adr/0026-statement-detail-is-an-enhancement.md) forbids. The
  balance is a **ceiling, never a payment record**: releasing is the reported payment's
  job, and the ceiling only stops the sweep reserving more than the card can claim — it
  cannot under-reserve, since a balance below the unpaid statement means something
  reduced the debt. Both figures stay **read, never inferred**: charges-minus-payments
  over all time *is* the balance and the bank reports the payment directly, so neither
  is reconstructed from the transaction ledger, which breaks at the backfill edge. Every
  unknown subtracts nothing, so the figure degrades to the full statement under that
  ceiling.
- **The clock is read once per run**, in the app timezone, and threaded through the
  derivation, the horizon, and the stamped instant. The zone is load-bearing, not
  cosmetic: which day "the 1st" is, and whether an occurrence has passed, are
  questions only a zone can answer.
- **`computed_at` is the compute instant, not the write.** It is the navigation key,
  the on-screen label, and the start of the window the snapshot's timeline covers, so
  it is stamped from `Compute`'s own `now`, never derived from the row's
  `updated_at`.
- **`Save` inserts, always.** Never an upsert, and no de-duplication: a run whose
  figures repeat the previous snapshot still appends, because the record is *that the
  question was asked at that instant*. All snapshots are retained. Reads go through
  `Snapshot`; nothing about what triggered a run is stored, so there is no privileged
  "monthly" snapshot.
- **A snapshot stores its own timeline**, running totals and peak flag included, so
  the page explains the arithmetic without recomputing it. The JSON field tags in
  `repo.go` are the storage contract — renaming a Go field is free, changing a tag
  orphans every snapshot already written.
- **Staleness is `accounts`' rule** ([ADR-0021](../../../docs/adr/0021-fault-isolating-sync-pass.md)):
  call the exported predicate, never define a second threshold that could drift.
- **Checking is derived, not designated** — *which* account it is comes from
  `accounts`, which owns the counts-as-savings flag, while the gate on `Balance.Known`
  **and** on the balance not being stale stays here, with each failure its own reason — designating an
  account, getting a bank to report a balance, and getting a sync working are three
  different fixes. Savings **never blocks**: it is not a term, so every way of not
  knowing it reads as "unknown". Cards all count, but not all place a row. Every such omission is an obligation genuinely absent from the window — nothing owed, or nothing falling due inside it — never a figure being dropped.
- **Never moves money.** No provider transfer/payment call exists.

Boundaries: imports `core/*`, `accounts`, `schedule` — never a provider client, and
never loan APR or interest detail. Statement balance, issue date and due date are read
from `accounts`, which refreshes them on the ordinary sync pass — the sweep asks that
module, never a provider. It reads neither `budget` nor `transactions`,
guarded by `TestSweepReadsNeitherBudgetNorLedger`: the budget is whole-of-spending,
so reserving it beside a declared outflow holds the same money twice, and the
timeline carries what is owed or scheduled, never what was spent. `repo.go` is the
only file touching `core/db/sqlc`, and its methods take and return this package's
`Recommendation`, never `sqlc.*`. A plain page read triggers no compute — only the
explicit Run now action does. The adapters also render `schedule`'s CRUD surface on
`/sweep` (the sweep is its only consumer, and a page may not import a peer module's
views), in a region that swaps independently of the snapshot. The job's cron spec
carries a `CRON_TZ=` prefix built from the configured app timezone
([ADR-0004](../../../docs/adr/0004-configured-app-timezone.md)), not server-local.
