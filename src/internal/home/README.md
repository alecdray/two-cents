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

The accounts overview lives at `/accounts`; this module owns `/`.

## Service methods

- `CurrentMonthTracker(ctx) (TrackerView, error)`
- `WrapList(ctx) ([]WrapSummary, error)`
- `MonthWrap(ctx, year, month) (WrapView, error)`
