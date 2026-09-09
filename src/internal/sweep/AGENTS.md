# sweep — domain module

Rules: ../../../docs/architecture/archetypes/domain-module.md

Owns the **cash-sweep recommendation**: computes an advisory sweep amount + direction
and appends it as a snapshot for the `/sweep` page to read and navigate. Domain
authority: [ADR-0020](../../../docs/adr/0020-monthly-cash-sweep-recommendation.md),
[ADR-0022](../../../docs/adr/0022-on-demand-navigable-sweep-snapshots.md);
[`docs/domain/README.md`](../../../docs/domain/README.md) §Cash sweep recommendation.

Module-specific notes:
- **The number is a reserve model** — exact formula, inputs, and rationale:
  [ADR-0020](../../../docs/adr/0020-monthly-cash-sweep-recommendation.md) and the
  derivation card in [`docs/domain/README.md`](../../../docs/domain/README.md)
  (§Cash sweep recommendation). Invariants a refactor must preserve: the two reserve
  components (unspent budget, unmet savings target) are each floored at 0
  **independently** — an over-satisfied obligation (overspent, or oversaved) must not
  drag the other term negative and manufacture a phantom surplus; the sweep itself is
  **not** floored (it may be negative — a pull); money uses the app-wide
  outflow-positive sign convention.
- **Reads no card/liability balance, no provider client.** The cycle's card spend is
  reserved *forward* from the budget, never read from the card — so there is no
  `/liabilities` or credit-balance read here ([ADR-0020](../../../docs/adr/0020-monthly-cash-sweep-recommendation.md)
  explains why the rejected "subtract the card balance" shape double-counted).
- **Whole-of-spending, scope-matched.** `total_spending_budget` (income − savings,
  from `budget`) and `mtd_spending_from_checking` are both whole-of-spending (rent
  included); the MTD figure counts only Spending that actually left checking
  (Transfers — card autopay *and* savings moves — and Income excluded, refunds net
  it down). No fixed/variable split.
- **Never moves money.** No provider transfer/payment call exists. The budgeted
  savings target is *reserved* (added into `reserve`, subtracting from the sweep),
  never folded into the swept amount — it stays in checking for the user to move.
- **Accounts derived, not designated.** Checking = the single active `cash` Account
  with counts-as-savings false; savings = the single active counts-as-savings `cash`
  Account (via `accounts.ActiveCashAccounts`). Ambiguous/absent either side → a
  needs-attention result. The checking pointer is gated on `Balance.Known` **and on
  the balance not being stale**, so an unknown *or* stale checking balance blocks
  (needs-attention); neither blocks on the savings side (savings is non-load-bearing —
  numeric result, figure shows "unknown", never counted as 0). A missing budget is
  **not** needs-attention. When more than one reason applies, **all** are listed (no
  precedence).
- **Staleness is `accounts`' rule, consumed here.** The threshold and its rationale
  live in `accounts` ([ADR-0021](../../../docs/adr/0021-fault-isolating-sync-pass.md));
  this module calls the exported predicate and never defines a second one — two
  thresholds that could drift would be worse than none. Evaluated at the run instant,
  so the rule is identical for the scheduled and the on-demand run: `Compute` does not
  know its caller, and the 7th appending a needs-attention snapshot is the intended
  outcome, not a degraded number.
- **Append-only snapshots, not a live projection.** Each run is computed and
  stored, never recomputed on render. `Save` **inserts** a
  snapshot keyed by a generated id — never an upsert, and no de-duplication: a run
  whose figures repeat the previous snapshot still appends, because the record is
  *that the question was asked at that instant*. Reads go through `Snapshot`, which
  positions one snapshot in the history; before any run it reports not-found — the
  first-run empty state, distinct from a stored needs-attention snapshot. Reasons are
  stored as a JSON list.
- **`computed_at` is the compute instant, not the write.** It is both the navigation
  key and the on-screen label, so it is stamped from `Compute`'s own `now` rather than
  derived from the row's `updated_at` at save time — a label must provably match the
  month-to-date window it measured. All snapshots are retained; there is no pruning.
- **Nothing records what triggered a run.** The job and the action produce the same
  kind of result, so there is no origin/trigger field and no privileged "monthly"
  snapshot — snapshots are distinguished by their instant alone.
- **Scheduled on the 7th, app timezone.** The job's spec carries a `CRON_TZ=` prefix
  built from the configured app timezone ([ADR-0004](../../../docs/adr/0004-configured-app-timezone.md)),
  not server-local. Month window = 1st 00:00 (configured zone) → run instant, via
  `core/timex` (the same reckoning `budget` uses, so the MTD window matches the
  budget's month bucketing).
- **Reads peers, writes only its table.** Imports `core/*`, `accounts`,
  `transactions`, `budget` — never a provider client. `repo.go` is the only file
  touching `core/db/sqlc`; its methods take/return this package's `Recommendation`,
  never `sqlc.*`. A plain page read still triggers no compute — only the explicit
  Run now action does.
