# home

The dashboard's composing module. It owns no domain tables; it injects the
`budget`, `transactions`, `categorization`, and `accounts` services plus the
configured app timezone, fetches the month-scoped data, fills the pure `tracker`
and `reporting` projection inputs, and joins Category names back onto the
results to render the read-side dashboard.

## Month navigation ([ADR-0018](../../../docs/adr/0018-month-navigable-home.md))

The current-month Tracker and the retrospective month wraps are one
month-navigable surface. A horizontally-scrollable **month rail** at the top of
both the Tracker and the wrap pages carries one chip per month from the earliest
transaction's month through the current one, in chronological order — oldest on
the left, the current month (the newest; no future months) on the right. Each
chip links to that month's page — `/` for the current month (the Tracker),
`GET /wraps/{ym}` for any earlier month — and the chip for the month being viewed
is marked active. The server renders the full rail; a small client script scrolls
the active chip into view on load, so the current month at the right edge is
visible by default. There is no standalone list of months.

The current month keeps a single face: `GET /wraps/{ym}` for the current month
**redirects to `/`**, so a current-month drill's back-link returns to the Tracker
rather than a parallel current-month wrap. Only earlier months render a wrap.

## Pages

- `GET /{$}` — the current-month **Tracker** (the application's landing page). Two
  tiers: the **top metrics** — income and savings progress toward their targets
  (reach-a-target) — over a **Budget** section of uniform budget rows
  (stay-under-a-limit): each budgeted Category, then the everything-else residual,
  with **Total remaining** (their sum) as a gap-separated summary row below. Every budget row
  carries its net-spend-of-limit, the daily pace to hold it, its remaining, and a
  budget-used bar seated at its bottom edge (red when over). Forward-looking, so it
  carries **no Surplus** (a closed-month figure — see the wrap below). With no
  budget set it shows the month's actuals (spent / income / saved so far) and
  prompts to create one.
- `GET /wraps/{ym}` — a single month **wrap** (`ym` = `YYYY-MM`): net income, gross
  income, savings contributed, the **Surplus** figure (net income − savings
  contributed — see [glossary](../../../docs/domain/README.md)), and
  spend-by-Category — actuals only, never compared against a budget. Net income (income − spending) is a derived summary and is not
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
  Tracker's Category/everything-else figures (the Tracker's income/savings are
  read-only display metrics, not drills). Rows are editable
  through the shared transaction-editing modal
  ([ADR-0011](../../../docs/adr/0011-reusable-transaction-editing-modal.md)); the
  drill region carries the total header and the list and **self-refreshes** on the
  `transaction-changed` event ([ADR-0010](../../../docs/adr/0010-event-driven-cross-region-refresh.md)),
  re-querying and re-summing so the total stays reconciled — including when an edit
  moves a row out of the bucket.

The accounts overview lives at `/accounts`; this module owns `/`.

## Service methods

- `CurrentMonthTracker(ctx) (TrackerView, error)` — the two-tier Tracker view:
  income/savings progress plus the budget rows (Total remaining, each Category,
  everything-else). No Surplus (forward-looking).
- `MonthWrap(ctx, year, month) (WrapView, error)` — includes the Surplus figure
  (net income − savings contributed).

Both the Tracker and wrap pages also render the **month rail** — the span of every
month from the earliest transaction's month through the current, with the viewed
month marked active — built in the composing layer (see Month navigation).
- `SpendDrill(ctx, year, month, bucket) (DrillView, error)` — selects the month's
  transactions the bucket names and sums their reconciling total: signed-net Spending
  for a Category / `uncategorized` / `everything-else`, the Income legs for `income`
  (→ gross income), the savings-contribution source legs for `savings` (→ savings
  contributed). Reads the Budget config only for the `everything-else` residual;
  `income`/`savings` read no budget and carry no month restriction. The same method
  serves both the drill page and the region's `transaction-changed` self-refresh, so
  editing a row through the shared modal re-queries and re-sums the bucket from scratch.
