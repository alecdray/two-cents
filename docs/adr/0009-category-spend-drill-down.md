# Category spend drill-down

Drilling from a spend-by-Category figure into the transactions behind it is a **dedicated read view owned by the dashboard composer**, nested under the month wrap's URL (`/wraps/{ym}/spend/{bucket}`), not a filter on the general transactions activity list. One `{bucket}` selector serves all three cases — a Category id, the uncategorized bucket, and the budget residual ("everything else"). Rows are editable in place (re-categorize), and an edit re-renders the **whole drill region** so the net-total header stays reconciled to the list.

Why a dedicated view rather than reusing the activity list:

- **The list must reconcile to the figure it came from.** The set is defined by the wrap's spend-by-Category aggregation — Spending only, bucketed by transaction-date month, refunds counted as negatives, transfers and income excluded. The general activity list has different semantics (a flat recent-N across all time and classifications, no month basis); reusing it would let the "transactions that make up the total" silently diverge from the total. General transactions filtering/search is separately deferred — this view does not start it.
- **Both surfaces drill into one view.** The retrospective wrap (any month) and the current-month Tracker both render per-Category figures; a single composer-owned view plus the `{bucket}` selector serves both, since the composer already builds both.

Two consequences fall out of the residual bucket:

- **"Everything else" is budget-derived, so it is current-month only.** The residual is unbudgeted-plus-uncategorized Spending, which needs the Budget config; the Budget applies only to the current month, so the view rejects the `everything-else` bucket for any other month. Because the residual reads the Budget, its bucketing lives in the **composer, not the actuals-only reporting projection** — that projection must never read a budget. The drill view still displays actuals only (no vs-budget comparison), so the wrap's actuals-only invariant holds even for this bucket.
- **An in-place re-categorization can move a row out of its bucket**, so the edit re-renders the region (list + net total) rather than swapping the single row, keeping the header honest; a Spending-without-Category validation error retargets inline to the row instead. The edit delegates to the Transactions re-categorize operation — the composer decides nothing about categorization itself (Categorization decides, Transactions writes).

Rejected: reusing the activity list as a filtered view (semantic drift from the figure, and pulls deferred general-filtering scope forward); a single-row swap on edit (leaves the net total stale); read-only rows (loses the verify-and-fix loop the drill is for).
