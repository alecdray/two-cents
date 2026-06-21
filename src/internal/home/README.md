# home

The dashboard's composing module. It owns no domain tables; it injects the
`budget`, `transactions`, `categorization`, and `accounts` services plus the
configured app timezone, fetches the month-scoped data, fills the pure `tracker`
and `reporting` projection inputs, and joins Category names back onto the
results to render the read-side dashboard.

## Pages

- `GET /{$}` — the current-month **Tracker** (the application's landing page):
  per-Category remaining and pace, the everything-else line, total remaining and
  pace, and income/savings progress. With no budget set it shows the month's
  actuals and prompts to create one.
- `GET /wraps` — the **wraps list**: every month from the earliest transaction's
  month through the current month, most-recent first, each linking to its wrap
  with settling/final and partial badges.
- `GET /wraps/{ym}` — a single month **wrap** (`ym` = `YYYY-MM`): net income,
  savings contributed, and spend-by-Category — actuals only, never compared
  against a budget.
- `GET /wraps/{ym}/spend/{bucket}` — the spend **drill-down** ([ADR-0009](../../../docs/adr/0009-category-spend-drill-down.md)):
  the Spending transactions making up one bucket's net figure for the month,
  newest-first, with the net total in the header. `bucket` is a Category id,
  `uncategorized`, or `everything-else` (the budget residual — unbudgeted plus
  uncategorized Spending, rejected for any month but the current one). Linked
  from both the wrap's Category rows and the Tracker's Category/everything-else
  rows. Rows are editable through the shared transaction-editing modal
  ([ADR-0011](../../../docs/adr/0011-reusable-transaction-editing-modal.md)); the
  drill region carries the net-total header and the list and **self-refreshes** on
  the `transaction-changed` event ([ADR-0010](../../../docs/adr/0010-event-driven-cross-region-refresh.md)),
  re-querying and re-summing so the total stays reconciled to the rows — including
  when an edit moves a row out of the bucket.

The accounts overview lives at `/accounts`; this module owns `/`.

## Service methods

- `CurrentMonthTracker(ctx) (TrackerView, error)`
- `WrapList(ctx) ([]WrapSummary, error)`
- `MonthWrap(ctx, year, month) (WrapView, error)`
- `SpendDrill(ctx, year, month, bucket) (DrillView, error)` — buckets the month's
  Spending into the requested drill set and sums the net total; reads the Budget
  config only for the `everything-else` residual.
- `ReCategorizeInDrill(ctx, year, month, bucket, txnID, classification, categoryID) (DrillView, string, error)`
  — delegates the write to `transactions.ReCategorize`, then re-composes the drill
  so the region re-renders; the string is a coupling validation message (view left
  unchanged) rather than a server error.
