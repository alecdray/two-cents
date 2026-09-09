# Uncovered card debt in the sweep reserve

The cash-sweep reserve ([ADR-0020](../../adr/0020-monthly-cash-sweep-recommendation.md))
reserves what you *planned* to spend and never looks at what you actually owe. Because
month-to-date spend counts only what left checking, card spending stays reserved forward
from the budget — which holds while a month lands near its budget, and breaks the moment
it runs past: the budget term stops at the budget while the bill keeps climbing, and the
sweep goes on offering up money a card payment is about to claim. This work adds a third
reserve term for the part of the card balance the budget does not already cover, and
reverses ADR-0020's refusal to read a card balance — see
[ADR-0023](../../adr/0023-uncovered-card-debt-reserve.md) for why that refusal no longer
applies.

## In scope

- A **third reserve term**: the summed balances of the active credit Accounts, less the
  remaining budget reserve, floored at zero. Netting the budget reserve out is what
  prevents the double-count ADR-0020 rejected; the term is zero for anyone inside their
  budget, so the ordinary month's number does not move.
- Reading the credit balances the **accounts sync already stores**. No liabilities
  product, no statement balance, no due date, no new provider endpoint.
- **Every active credit Account counts, summed** — unlike checking and savings, where the
  derivation demands exactly one and ambiguity is needs-attention. Debt is additive.
- An **unknown or stale card balance becomes needs-attention**, on the same footing as
  checking, because the balance is now a term in the formula.
- Reconciling the canonical docs: ADR-0023, the `sweep` module docs, and the domain
  `README.md` derivation card and reserve entries.

## Out of scope

- **The liabilities product** — statement balance, due date, minimum payment, APR. Still a
  stated non-goal ([vision](../../product/vision.md)). The balance read here is the one the
  ordinary sync already holds.
- **A projected-income term.** Income already received is already in the live checking
  balance and already raises the sweep; reserving against income not yet arrived would
  advise moving money on the strength of a paycheck that has not landed.
- **Modelling card payments as their own transfer subtype.** Reading the balance removes
  the sweep's need for it — a payment reduces the balance and clears the reserve on its
  own. The app still cannot tell a card payment from any other plain transfer, which is
  worth fixing; it is now separable work, not a prerequisite.
- **Carrying a prior month's overspend forward**, and **reconstructing what is owed from
  the transaction ledger**. Both were considered and rejected in ADR-0023.
- The reserve's existing terms, the account derivation for checking and savings, the
  append-only snapshot model ([ADR-0022](../../adr/0022-on-demand-navigable-sweep-snapshots.md)),
  and the advisory-only boundary: the sweep still never moves money.
