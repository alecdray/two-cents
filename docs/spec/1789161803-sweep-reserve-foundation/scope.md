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
  - Interest rate / APY — **not held today**, and the goal names interest explicitly

- **Credit cards** — every active credit account
  - Current balance = amount owed right now (synced)
  - Statement balance = what the last closed cycle billed — **not held today** (liabilities)
  - Payment due date — **not held today** (liabilities)
  - Only a statement whose due date falls inside the horizon goes on the timeline. Charges in
    the open cycle are due after it, so they are outside the window by construction.

- **Recurring items** — **new**, user-declared; the only source of dated checking activity
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
  - The horizon — roughly one month forward, so a run on the 7th covers the next month's
    card payment and the next month's rent

- **Configuration**
  - Fixed safety margin (default $500)

- **Budget** — **no longer an input to the sweep.** It is whole-of-spending, rent included
  ([ADR-0020](../../adr/0020-monthly-cash-sweep-recommendation.md)), so once rent is a
  declared recurring withdrawal the same money exists in two places. The timeline carries only
  observed and declared facts. The budget remains what it is everywhere else in the app — a
  spending plan for the Tracker

## Decided so far

1. The reserve is a dated cash-flow projection, not a budget proxy.
2. Required checking is the **cumulative maximum** over the horizon, floored at zero.
3. Card spending enters **only** as a statement on its due date — never as forecast spending.
4. Recurring checking activity is **user-declared** to start. No detection from the ledger, and
   not Plaid's recurring-transactions product (a paid add-on).
5. Income is declared through the **same structure** as bills, distinguished by direction and
   cadence. Dated inflows and undated ones cannot share a timeline.
6. The budget leaves the reserve entirely.
7. The **savings target becomes a declared recurring outflow** — a scheduled transfer to
   savings is a checking outflow like any other, and reserving it falls out of the timeline
   instead of being the special case [ADR-0020](../../adr/0020-monthly-cash-sweep-recommendation.md)
   made of it. Every timeline item is a fact, never an intention.

## Open questions

- **Ad-hoc debit spending** — groceries and coffee on the debit card are dated by nobody. Does
  the safety margin absorb them, or do they need a term?
- **Horizon length**, precisely. One month forward is the intent; whether it ends at a fixed
  offset or at a dated event needs settling, because truncation decides which card payment
  falls inside.
- **What blocks versus what degrades** — which missing inputs make a number impossible, and
  which produce a conservative number instead.
- **Savings interest** is named in the goal but no rate is held anywhere in the app.
