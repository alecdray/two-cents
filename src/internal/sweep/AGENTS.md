# sweep — domain module

Rules: ../../../docs/architecture/archetypes/domain-module.md

Owns the **cash-sweep recommendation**: computes an advisory sweep amount + direction
and appends it as a snapshot for the `/sweep` page to read and navigate. Behaviour and
the read surface: [`README.md`](README.md). Domain authority:
[ADR-0020](../../../docs/adr/0020-monthly-cash-sweep-recommendation.md),
[ADR-0022](../../../docs/adr/0022-on-demand-navigable-sweep-snapshots.md),
[ADR-0023](../../../docs/adr/0023-uncovered-card-debt-reserve.md);
[`docs/domain/README.md`](../../../docs/domain/README.md) §Cash sweep recommendation,
whose derivation card is the canonical arithmetic — read it rather than re-deriving
the formula from here.

Invariants a refactor could silently break:

- **Each reserve term is floored at 0 on its own**, so an over-satisfied obligation
  (overspent, oversaved, or a cleared card) can never drag another term negative and
  manufacture a phantom surplus. The **card term is the one deliberate coupling**: it
  is computed *net of* the budget reserve, because the budget and the card balance are
  two views of the same upcoming money and reserving both double-counts it. Do not
  "simplify" it into an independent term. The sweep itself is **not** floored — a
  negative value is a meaningful pull — and money uses the app-wide outflow-positive
  sign convention.
- **The card balance is read, never inferred.** Charges-minus-payments over all time
  *is* the balance, so do not reconstruct it from the transaction ledger (it breaks at
  the backfill edge), and the sweep needs no notion of which transfers were card
  payments — a payment reduces the balance and clears the reserve on its own. No
  assumption about autopay timing is safe either: a snapshot can be produced at any
  instant, so any prior-month or "already paid" heuristic is wrong about half the time.
- **The clock is read once per run** and threaded through the derivation, the
  month-to-date window, and the stamped instant. Reading it again mid-compute lets a
  snapshot describe a state of the world that never existed.
- **`computed_at` is the compute instant, not the write.** It is the navigation key and
  the on-screen label, so it is stamped from `Compute`'s own `now`, never derived from
  the row's `updated_at` — a label must provably match the window it measured.
- **`Save` inserts, always.** Never an upsert, and no de-duplication: a run whose
  figures repeat the previous snapshot still appends, because the record is *that the
  question was asked at that instant*. All snapshots are retained. Reads go through
  `Snapshot`; nothing about what triggered a run is stored, so there is no privileged
  "monthly" snapshot.
- **Staleness is `accounts`' rule** ([ADR-0021](../../../docs/adr/0021-fault-isolating-sync-pass.md)):
  call the exported predicate, never define a second threshold that could drift from it.
- **The month-to-date window counts only Spending that actually left checking** —
  Transfers (card autopay *and* savings moves) and Income excluded, refunds netting it
  down — so card spend stays reserved forward from the budget. It is bucketed via
  `core/timex` in the configured app timezone, matching `budget`'s month bucketing; the
  no-future-dated-transaction invariant behind its upper bound is documented at the
  query site.
- **Checking and savings are derived, not designated**, and gated on `Balance.Known`
  **and** on the balance not being stale. Cards are different: every active one counts
  and they sum, with no single-account requirement, because debt is additive. The full
  blocking rules live in the derivation card.
- **Never moves money.** No provider transfer/payment call exists, and the budgeted
  savings target is *reserved* rather than swept — it stays in checking for the user to
  move.

Boundaries: imports `core/*`, `accounts`, `transactions`, `budget` — never a provider
client, and never a liabilities product (no statement balance, due date or APR; credit
balances come from the ordinary accounts sync). `repo.go` is the only file touching
`core/db/sqlc`, and its methods take and return this package's `Recommendation`, never
`sqlc.*`. A plain page read triggers no compute — only the explicit Run now action
does. The job's cron spec carries a `CRON_TZ=` prefix built from the configured app
timezone ([ADR-0004](../../../docs/adr/0004-configured-app-timezone.md)), not
server-local.
