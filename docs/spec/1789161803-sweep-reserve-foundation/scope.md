# Re-deriving the sweep reserve from first principles

The cash-sweep reserve is re-derived from scratch. Its current shape
([ADR-0020](../../adr/0020-monthly-cash-sweep-recommendation.md),
[ADR-0023](../../adr/0023-uncovered-card-debt-reserve.md)) is a **proxy plus a chain of
compensations**, and each attempt to make it more accurate has required another
compensation rather than a better model. This work replaces the proxy with a formula
derived from a stated guarantee, accepts a written list of limitations, and only then asks
what data it needs — which is where Plaid's liabilities product enters, as a consequence of
the derivation rather than its premise.

## Why the current model cannot be extended

0020 holds back the month's unspent budget as a stand-in for one month of outflow. It never
asks *when* anything is due, and it never distinguishes money that leaves checking today
from money charged to a card that leaves in six weeks. Two compensations exist purely to
keep that stand-in standing:

- Its month-to-date window counts only spending that left **checking**, so card spend
  "stays reserved forward" — the budget term is the only place the formula can hold it.
- 0023 must then net the card balance back **out of** the budget term, because reserving
  both counts the same money twice.

Neither is about the domain; both prop up the approximation. A retrofit that removes one
requires reworking the other, and an earlier attempt at exactly that
(withdrawn before this document replaced it) produced a formula with four special cases,
a term whose sign was hard to predict, and a cross-month gap where income was credited
against a card bill *and* relied on to fund the following month. The problem was not the
individual fixes. It was fitting them to a model whose foundation never represented the
thing being computed.

## The foundational question

Before any arithmetic, the formula must answer one question, stated precisely enough to
test against:

> **How much of the cash now in checking can be moved to savings without having to be moved
> back?**

Every term in the reserve should be justifiable as an answer to that, and any term that is
not is a compensation. Making it testable means settling three things the current model
leaves implicit: **over what horizon** "having to be moved back" is measured, **what
counts as an obligation** within it, and **what guarantee** is being offered at the
horizon's edge.

## Properties the formula must have

Derived from the failures found while grilling the retrofit. These are the acceptance
criteria for the new formula, and a derivation that cannot demonstrate one of them has not
answered the question above.

1. **Exactly once.** Every obligation is reserved once. The budget is a *plan* for spending;
   a card balance is a *record* of spending; a transaction is a *fact*. Where these describe
   the same dollar, the formula must hold it a single time by construction — not by
   subtracting one view from another after the fact.
2. **Nothing missed.** No real obligation falls outside the reserve because no term happened
   to cover it.
3. **Timing is represented.** An obligation falling due after money arrives is not the same
   as one falling due before it. A model that cannot tell them apart cannot answer the
   question.
4. **No unlanded income against an earlier obligation.** Income not yet received may never
   reduce what is held for something due before that income arrives.
5. **No cross-period cancellation.** Income already relied on to cover one obligation cannot
   also discount another. This is the gap that killed the retrofit, and it is invisible
   inside a single-month horizon — which is itself evidence the horizon needs stating.
6. **Valid at any instant.** Snapshots are produced on demand
   ([ADR-0022](../../adr/0022-on-demand-navigable-sweep-snapshots.md)), so no term may
   assume a day of the month, a billing-cycle position, or that autopay has or has not run.
7. **Degrades safely, and never below the status quo.** Missing or unknown data must never
   produce a more optimistic number — and must never produce a *worse* one than the user
   gets today. A fallback that is merely "conservative" can still be a regression, which is
   how the retrofit treated a card whose bank reports no statement.
8. **Stable.** Repeated runs against unchanged facts must not oscillate between advising a
   sweep and advising a pull.
9. **Self-explaining.** Every figure that produced the number is on screen and the arithmetic
   is reconstructable from the snapshot alone.
10. **Advisory.** Nothing moves money; the budgeted savings transfer stays reserved for the
    user to make.

## Questions the derivation must answer

Open — they are the agenda for the formula session, not decisions taken here.

- **What is the horizon**, and what is promised at its edge? "Until the next income arrives"
  and "one month" are different answers, and property 5 only has meaning once one is chosen.
- **What is an obligation?** Money already owed (a card balance), money planned but not yet
  spent (the budget), and money scheduled to move (a savings transfer) are not obviously the
  same kind of thing, and the current model treats all three identically.
- **Is future spending an obligation at all**, or a reason to keep a float? The answer
  decides whether the budget belongs in the reserve or somewhere else entirely.
- **How does income enter** — as a dated inflow the model reasons over, or as a discount
  applied to an obligation? The retrofit did the latter and property 5 is what it cost.
- **What must be known for a number to form at all**, and what is a needs-attention result
  rather than a degraded one?
- **What limitations are accepted**, stated up front rather than discovered later. A model
  with a written limit is finished; one with an unwritten limit is not.

## In scope

- The reserve's derivation, its stated guarantee, its accepted limitations, and the ADR that
  records why this shape replaces 0020's and 0023's.
- Whatever data the derivation turns out to require — including, if it earns its place, card
  statement detail from Plaid's liabilities product, subject to the coverage check below.
- Reconciling the canonical docs the new model touches.

## Out of scope

- **Moving money.** The recommendation stays advisory.
- **The account derivation** (single active checking, single active savings, credit accounts
  summed), the append-only snapshot timeline, and the `/sweep` navigation model. Unless the
  derivation forces a change, these are untouched — and if it does, that is a finding to
  record, not a licence to redesign them here.
- **Sweep multi-account aggregation**, still deferred ([backlog](../../backlog/features.md)).
- **Loan and mortgage liabilities, and APR / interest detail.** Out regardless of what the
  derivation needs.

## Gate before any implementation

If the derivation requires card statement detail, two assumptions about the real Plaid
account must be checked **before** code is written, because both are unverified and either
one failing changes the answer:

1. **Institution coverage** — Plaid's liabilities support is narrower than its transactions
   support. Do the linked issuers report a statement balance and a due date at all?
2. **Item consent** — the app links Items with `transactions` only (`PLAID_PRODUCTS`), and
   the reconnect path omits products because Plaid requires update-mode tokens to. Whether
   existing Items serve `/liabilities/get` without being re-linked is account-specific and
   unknown. If they do not, **re-linking every connection** is part of the work.
