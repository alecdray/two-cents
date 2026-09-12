# Sweep reserve — scope

## Goal

Provide a sweep number, on demand, telling the user how much money to move **into or out of
savings** to maintain a safe checking balance while maximizing interest earned in savings.

## The model

A **cash-flow timeline**, not a budget-derived reserve. Every expected inflow and outflow is
placed on a dated timeline from the run instant to the end of a horizon. The amount that must
stay in checking is the worst point on that timeline:

```
required_checking = max( 0,  max over t in horizon of [ outflows(≤ t) − inflows(≤ t) ] )
sweep             = current_checking − required_checking − safety_margin
```

Positive sweep → move to savings. Negative → pull back from savings.

**Cumulative, then take the maximum — never a sum of netted buckets.** Summing lets a later
surplus cancel an earlier shortfall, which is money travelling backwards in time: a paycheck
on the 30th cannot pay a bill due on the 20th. Taking the running maximum is what makes
income offset only what comes after it.

## Entities

Each attribute notes where it comes from, and flags anything we do not hold today.

- **Checking account** — the single active cash account not marked counts-as-savings
  - Current balance (synced; may be unknown, may be stale)

- **Savings account** — the single active cash account marked counts-as-savings
  - Current balance (synced; may be unknown or stale — today this does not block)
  - No interest rate is held or needed. "Maximizing interest" is served by sweeping more,
    not by reasoning about rates

- **Credit cards** — every active credit account
  - Current balance = amount owed right now (synced)
  - Statement balance = what the last closed cycle billed — **not held today** (liabilities)
  - Payment due date — **not held today** (liabilities)
  - Only a statement whose due date falls inside the horizon goes on the timeline. Charges in
    the open cycle are due after it, so they are outside the window by construction.

- **Scheduled items** — **new**, user-declared; collectively *the sweep schedule*, and the
  only source of dated checking activity
  - Name (e.g. "Rent", "Paycheck")
  - Direction — out (a bill) or in (income)
  - **Conservative amount** — the *maximum* expected for an outflow, the *minimum* expected
    for an inflow. The safe direction flips with the sign: over-stating a bill holds extra
    cash, while over-stating a paycheck discounts real debt against money that may not arrive
  - Cadence — **monthly** (day of month, clamped to the last day in short months) or
    **biweekly** (anchor date, every 14 days). Not semi-monthly, not weekly
  - Scoped to recurring activity **in the checking account only**. Card spending is not
    declared here; it reaches the timeline as a statement, once, on its due date
  - **Only actual scheduled transfers are declared, never intentions.** A standing transfer
    happens whether or not the sweep says anything, so the cash must be in checking on its
    date. An aspiration must not sit on a timeline of dated facts — the sweep achieves it by
    moving surplus, and declaring it would hold money back from savings so it can be moved to
    savings

- **Time**
  - The run instant — a snapshot can be produced at any moment
  - The horizon — **exactly one month** from the run instant, rolling, not the calendar month.
    A run on the 7th covers through the 7th of next month

- **Configuration**
  - Fixed safety margin (default $500) — headroom, not a term. It gives the user room before
    an undeclared outflow becomes dangerous; it is **not** sized to cover one and is not
    intended to

- **Budget** — **no longer an input to the sweep.** It is whole-of-spending, rent included
  ([ADR-0020](../../adr/0020-monthly-cash-sweep-recommendation.md)), so once rent is a
  scheduled outflow the same money exists in two places. The timeline carries only
  observed and declared facts. The budget remains what it is everywhere else in the app — a
  spending plan for the Tracker

## Decided so far

1. The reserve is a dated cash-flow projection, not a budget proxy.
2. Required checking is the **cumulative maximum** over the horizon, floored at zero.
3. Card spending enters **only** as a statement on its due date — never as forecast spending.
4. Recurring checking activity is **user-declared** to start, as *scheduled items*. No
   detection from the ledger, and not Plaid's recurring-transactions product (a paid add-on).
   The name avoids "transaction", which the app already uses for a bank-reported fact — the
   two must be discussed together once occurrences are matched to real transactions.
5. Income is declared through the **same structure** as bills, distinguished by direction and
   cadence. Dated inflows and undated ones cannot share a timeline.
6. The budget leaves the reserve entirely.
7. The **savings target becomes a scheduled outflow** — a scheduled transfer to
   savings is a checking outflow like any other, and reserving it falls out of the timeline
   instead of being the special case [ADR-0020](../../adr/0020-monthly-cash-sweep-recommendation.md)
   made of it. Every timeline item is a fact, never an intention.
8. The horizon is **exactly one month** from the run instant, rolling — the length at which
   the window holds exactly one instance of every monthly item. A longer window was considered
   and rejected: its only effect is to pull in a *second* instance of monthly items for runs
   late in the month, and it pulls in the outflow without the income that covers it, so the
   number would lurch upward in the last week of every month for no real reason. A monthly
   bill is never genuinely past the edge — its previous instance was inside the window on an
   earlier run.
9. Undeclared spending straight from checking is **the user's to manage, not the sweep's**.
   The safety margin gives headroom so an exception is not immediately dangerous; it does not
   solve the problem and does not intend to.
10. Missing **dollar values fail hard**; missing **dates degrade to the worst case**.
11. A scheduled item's occurrences are **matched to real transactions**, manually and by
    best-effort automatic resolution, so an occurrence that has already landed leaves the
    timeline instead of being reserved twice alongside the balance that already reflects it.

## Missing data

- **A missing dollar value is a hard failure.** The run produces a needs-attention result
  naming every reason, never a number built on a figure we do not have.
- **A missing date degrades to the worst case** rather than failing: an outflow with no known
  date is placed at the **run instant** (due immediately), and an inflow with no known date at
  the **end of the horizon** (as late as possible, which is equivalent to leaving it out).
  Both push `required_checking` up, never down.
- These compose to handle a card whose bank does not report statements: the current balance is
  a known dollar value with no due date, so the **whole balance lands at the run instant**. The
  number still forms, and it is the most conservative reading. A card whose *balance* is
  unknown is a hard failure, as it is today.
- Existing staleness blocking is retained ([ADR-0021](../../adr/0021-fault-isolating-sync-pass.md),
  [ADR-0023](../../adr/0023-uncovered-card-debt-reserve.md)): a balance that has not refreshed
  is treated as missing rather than trusted.

## Accepted limitations

Stated up front, not discovered later. Each is a known cost of the model, not a defect.

- **Day-to-day spending is assumed to run through cards**, where it reaches the timeline as a
  statement on a real due date. Spending straight from checking — debit, cash withdrawals — is
  treated as the exception. A user who routinely spends that way gets a number that
  under-reserves, and the remedy is to declare it or to accept the margin as the only cushion.
- **A dated fact beats a forecast, so nothing is forecast.** The model never predicts spending.
  It only places money that is already owed, already scheduled, or already declared.
- **The horizon truncates, and no length removes that.** An outflow beyond the window is not
  reserved for, so a sweep today can be reversed by a run next week. It self-corrects, since
  the model is advisory and re-runnable. Lengthening the window does not fix it — it only
  moves the cut, and moving the cut past an outflow without also passing the income that
  covers it makes the answer worse, not safer. One month is chosen because it is the length at
  which every monthly item appears exactly once.
