# Monthly cash-sweep recommendation

Two Cents computes a monthly **advisory** cash-sweep recommendation — one dollar
amount and a direction (checking → savings, or savings → checking) that relocates
only genuinely idle checking cash, the surplus above everything already spoken for.
It only surfaces the number and its reasoning; it **never moves money**, and it
deliberately leaves the user's own budgeted savings transfer for them to make.

**The number is a reserve model.** `suggested_sweep = current_checking − reserve −
fixed_safety_margin`, where `reserve = max(0, total_spending_budget −
mtd_spending_from_checking) + max(0, savings_target − mtd_savings_contributed)` and
each reserve component is floored at zero independently (an over-satisfied
obligation must not manufacture a phantom surplus). Because the total spending
budget is income minus savings, the reserve collapses to *about one month's income
minus what has already left checking this month* — keep that in checking, sweep the
rest above a flat margin. The margin is a single tunable cushion, not a rounded
buffer.

We **rejected** the alternative "projected checking minus the upcoming card
balance" shape: it double-counted the cycle's card spend — subtracting the current
card balance *and* reserving the budgeted spend for the same upcoming cycle. The
reserve model reserves that spend forward from the budget instead, so it reads **no
card or liability balance at all** — no new provider endpoint, and one fewer moving
part. The trade-off is that a cycle whose real card spend diverges materially from
budget is approximated; reconciling the two is deferred.

**Whole-of-spending, no fixed/variable split.** The budget term and the
month-to-date spend it nets against are both whole-of-spending (rent included), so
they stay scope-matched; splitting fixed from variable is deferred and unnecessary
for the math. The month-to-date spend counts only what actually left checking, so
card spend (a Transfer, not Spending) stays reserved forward and savings already
moved self-adjusts through the lower checking balance.

**Accounts are derived, not designated.** Checking is the single active cash
account that is not counts-as-savings; savings is the single active counts-as-savings
cash account ([ADR-0008](0008-account-kind-and-savings-overrides.md)). When either
is ambiguous or absent the result is a **needs-attention** state naming every
reason, never a guessed account. A missing budget is not needs-attention (its terms
go to zero); an unknown *savings balance* is shown as "unknown" but does not block,
since savings is not a term in the formula.

**A persisted monthly snapshot, not a live projection.** Unlike the Tracker and
month wraps (recomputed on every render), this recommendation is computed by a
scheduled job and stored; the view reads the latest. The job runs on the **7th** in
the [configured app timezone](0004-configured-app-timezone.md): by then every card
has closed and autopaid on the 1st and settled, with ample runway before the next
cycle, so a static monthly schedule suffices — no statement-close webhook. Re-runs
replace the latest rather than accumulate. There is no on-demand compute in v1.

**Deferred: multi-account aggregation.** The current derivation requires exactly
one checking and one savings account; two or more of either produces a
needs-attention result. A user with multiple checking or savings accounts would
benefit from aggregation (sum balances; union account IDs for the MTD queries)
rather than a hard failure. Deferred to keep v1 simple; the fix is contained to
`service.go` and the MTD SQL queries.
