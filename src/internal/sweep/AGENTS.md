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

- **Take the cumulative maximum, never the sum.** The running total's end value is
  what the month nets out to; its *peak* is what must be present for the balance
  never to go negative. The peak is also what confines an inflow to offsetting only
  what follows it — summing netted periods lets a paycheck on the 30th pay a bill due
  on the 20th. This is structural, not a rule the arithmetic remembers, so do not
  "simplify" the evaluator into a sum. The sweep itself is **not** floored — a
  negative value is a meaningful pull — and money uses the app-wide outflow-positive
  sign convention.
- **The horizon is exactly one month, and the occurrence window looks only forward.**
  One month is the length at which the window holds exactly one occurrence of every
  monthly item; the window ends the day *before* the horizon's own date so a run
  landing on an item's day does not count it twice. Lengthening the horizon never
  removes truncation — it moves the cut, and moving it past an outflow without also
  passing the income that covers it makes the answer worse. Reaching *backwards* is
  not a wider window either: deciding whether an occurrence that already fell due was
  paid is a reconciliation of declaration against ledger, and doing it here — with no way
  to tell a paid occurrence from an unpaid one — would reserve every monthly item
  twice for the whole month.
- **Same-day ordering puts outflows before inflows.** Never assume a deposit clears
  before a debit posted the same day.
- **Unknowns are asymmetric on purpose.** A missing dollar value fails hard
  (needs-attention, listing *every* applicable reason); a missing date degrades to
  the worst case — an outflow at the run instant, an inflow omitted. Every unknown
  makes the number more conservative and none makes it less, which is what makes an
  incompletely-informed run safe rather than merely tolerable.
- **The card balance is read, never inferred.** Charges-minus-payments over all time
  *is* the balance, so do not reconstruct it from the transaction ledger (it breaks
  at the backfill edge), and no notion of autopay timing is needed — a payment
  reduces the balance on its own.
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
- **Checking is derived, not designated**, and gated on `Balance.Known` **and** on
  the balance not being stale, with each failure its own reason — designating an
  account, getting a bank to report a balance, and getting a sync working are three
  different fixes. Savings **never blocks**: it is not a term, so every way of not
  knowing it reads as "unknown". Cards all count and each contributes its own row.
- **Never moves money.** No provider transfer/payment call exists.

Boundaries: imports `core/*`, `accounts`, `schedule` — never a provider client, and
never a liabilities product (no statement balance, due date or APR; credit balances
come from the ordinary accounts sync). It reads neither `budget` nor `transactions`,
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
