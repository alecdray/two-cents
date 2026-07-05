# Month-navigable home

The home surface unifies the current-month Tracker and the retrospective month
wraps into one month-navigable view: a scrollable rail of month chips selects the
month, showing the budget-relative Tracker for the current month and that month's
actuals-only wrap
([0012](0012-wrap-income-savings-and-month-list-drill-ins.md)) for any earlier
month. The standalone wraps *list* and its navigation entry are removed — the
rail is the sole month navigator.

Each month shows the projection that fits it rather than forcing one shape onto
both: the Tracker is inherently current-month and forward-looking — its pace
figures only mean anything for a month still in progress — while the wrap is
retrospective actuals. The carousel is just the selector between them.

The current month is the newest reachable month; the carousel does not advance
into the future — there is nothing there yet, and the budget is a single rolling
config, not a per-month plan. The older edge is the earliest transaction's month,
the same span the wraps list covered.

Reusing the existing per-month wrap address for a past month (and the application
root for the current month) leaves the drill-downs and their self-refresh wiring
([0009](0009-category-spend-drill-down.md),
[0010](0010-event-driven-cross-region-refresh.md)) untouched; only the list entry
point is retired. The current month keeps a single canonical face: its own wrap
address redirects to the root, so a drill opened from the Tracker returns there
rather than to a parallel current-month wrap. A single standing navigator in place of a separate list page
also keeps the bottom-bar overflow lean
([0014](0014-bottom-bar-navigation.md)).

Consequence: the wraps list page, the month-summary read behind it, and the
overflow link to it go away, while a deep link to a specific month still resolves.
A richer, per-row-summarized wraps list — a prior backlog idea — is mooted by the
removal.
