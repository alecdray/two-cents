# home

The dashboard's composing module. It owns no domain tables; it injects the
`budget`, `transactions`, `categorization`, and `accounts` services plus the
configured app timezone, fetches the month-scoped data, fills the pure `tracker`
and `reporting` projection inputs, and joins Category names back onto the
results to render the read-side dashboard.

## Pages

- `GET /{$}` — the current-month **Tracker** (the application's landing page):
  per-Category remaining and pace, the everything-else line, total remaining and
  pace, and income/savings progress. Each Category, everything-else, and the total
  row carries a budget-used bar seated at its bottom edge. With no budget set it
  shows the month's actuals and prompts to create one.
- `GET /wraps` — the **wraps list**: every month from the earliest transaction's
  month through the current month, most-recent first, each linking to its wrap
  with settling/final and partial badges.
- `GET /wraps/{ym}` — a single month **wrap** (`ym` = `YYYY-MM`): net income, gross
  income, savings contributed, and spend-by-Category — actuals only, never compared
  against a budget. Net income (income − spending) is a derived summary and is not
  itself a drill; the gross **Income** figure, **Savings contributed**, and each
  Category row drill in. Below spend-by-Category an inline editable list shows the
  month's whole transaction set (every classification); its rows open the shared
  modal and an edit re-renders the wrap's figures via the `transaction-changed` event
  ([ADR-0012](../../../docs/adr/0012-wrap-income-savings-and-month-list-drill-ins.md)).
  The same GET also serves the figure region's self-refresh.
- `GET /wraps/{ym}/spend/{bucket}` — the **drill-down** ([ADR-0009](../../../docs/adr/0009-category-spend-drill-down.md),
  [ADR-0012](../../../docs/adr/0012-wrap-income-savings-and-month-list-drill-ins.md)):
  the transactions making up one bucket's figure for the month, newest-first, with
  the reconciling total in the header. `bucket` is a Category id, `uncategorized`,
  `everything-else` (the budget residual — unbudgeted plus uncategorized Spending,
  rejected for any month but the current one), `income` (the month's Income legs,
  summing to gross income), or `savings` (the savings-contribution source legs,
  summing to savings contributed); income/savings read no budget and carry no month
  restriction. Linked from the wrap's Income/Savings/Category figures and the
  Tracker's income/savings/Category/everything-else figures. Rows are editable
  through the shared transaction-editing modal
  ([ADR-0011](../../../docs/adr/0011-reusable-transaction-editing-modal.md)); the
  drill region carries the total header and the list and **self-refreshes** on the
  `transaction-changed` event ([ADR-0010](../../../docs/adr/0010-event-driven-cross-region-refresh.md)),
  re-querying and re-summing so the total stays reconciled — including when an edit
  moves a row out of the bucket.

The accounts overview lives at `/accounts`; this module owns `/`.

## Service methods

- `CurrentMonthTracker(ctx) (TrackerView, error)`
- `WrapList(ctx) ([]WrapSummary, error)`
- `MonthWrap(ctx, year, month) (WrapView, error)`
- `SpendDrill(ctx, year, month, bucket) (DrillView, error)` — selects the month's
  transactions the bucket names and sums their reconciling total: signed-net Spending
  for a Category / `uncategorized` / `everything-else`, the Income legs for `income`
  (→ gross income), the savings-contribution source legs for `savings` (→ savings
  contributed). Reads the Budget config only for the `everything-else` residual;
  `income`/`savings` read no budget and carry no month restriction. The same method
  serves both the drill page and the region's `transaction-changed` self-refresh, so
  editing a row through the shared modal re-queries and re-sums the bucket from scratch.
