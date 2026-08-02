# sweep — domain module

Rules: ../../../docs/architecture/archetypes/domain-module.md

Owns the monthly **cash-sweep recommendation**: computes an advisory sweep amount +
direction and persists the latest snapshot for the `/sweep` page to read. Domain
authority: [ADR-0020](../../../docs/adr/0020-monthly-cash-sweep-recommendation.md);
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
  needs-attention result. The checking pointer is gated on `Balance.Known`, so an
  **unknown checking balance blocks** (needs-attention); an **unknown savings
  balance does not** (savings is non-load-bearing — numeric result, figure shows
  "unknown", never counted as 0). A missing budget is **not** needs-attention. When
  more than one reason applies, **all** are listed (no precedence).
- **Persisted latest, not a live projection.** Unlike the Tracker/wrap (recomputed
  live by `home`), this is computed by a scheduled job and stored. `SaveLatest`
  upserts a single row (`id` `'default'`); `LoadLatest` returns `found=false` before
  any run — distinct from a needs-attention snapshot. Re-runs replace. Reasons are
  stored as a JSON list to keep the single-row shape.
- **Scheduled on the 7th, app timezone.** The job's spec carries a `CRON_TZ=` prefix
  built from the configured app timezone ([ADR-0004](../../../docs/adr/0004-configured-app-timezone.md)),
  not server-local. Month window = 1st 00:00 (configured zone) → run instant, via
  `core/timex` (the same reckoning `budget` uses, so the MTD window matches the
  budget's month bucketing).
- **Reads peers, writes only its table.** Imports `core/*`, `accounts`,
  `transactions`, `budget` — never a provider client. `repo.go` is the only file
  touching `core/db/sqlc`; its methods take/return this package's `Recommendation`,
  never `sqlc.*`. Adapters read only `LoadLatest` — the page never triggers a
  compute (no on-demand compute in v1).
