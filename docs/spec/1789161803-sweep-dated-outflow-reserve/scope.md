# A dated-outflow reserve: statement timing against expected income

The cash-sweep reserve ([ADR-0020](../../adr/0020-monthly-cash-sweep-recommendation.md),
[ADR-0023](../../adr/0023-uncovered-card-debt-reserve.md)) holds back roughly one month's
budget as a **proxy** for one month's outflow, and never asks *when* anything is actually
due. That proxy is why the budget term nets against spending **from checking only** — card
spend has to stay reserved forward because the formula has no other way to hold it — and
why ADR-0023's card term must then be netted back out of the budget term to undo the
resulting double-count. The whole arrangement approximates a set of dated obligations the
bank can state exactly.

This work reads those dates. Plaid's liabilities product supplies, per credit Account, the
**last statement balance** and the **next payment due date**; the budget and the
transaction ledger already supply expected income. With both, the reserve stops being a
proxy and becomes what it always meant: **everything scheduled to leave checking before
income next arrives.** A statement due before payday is reserved from today's checking; a
statement due after it is covered by income that will have landed. Rationale:
[ADR-0024](../../adr/0024-dated-outflow-sweep-reserve.md).

## In scope

- **The liabilities read, narrowed to credit cards.** A `GetCardStatements` method on the
  `banking.BankProvider` seam returning statement balance, statement issue date, next
  payment due date, minimum payment, and the overdue flag per credit Account. Named for
  what is taken, not for the provider's product.
- **Ownership in `accounts`.** The statement detail is per-account, refreshed on the
  existing sync pass inside its per-connection fault isolation
  ([ADR-0021](../../adr/0021-fault-isolating-sync-pass.md)), and stored on the `accounts`
  row beside the balance. The provider call is made only for connections holding at least
  one active credit Account.
- **The reserve reformulation.** The budget term nets against **all-account** month-to-date
  spending rather than checking-only; the card obligation splits by due date into the part
  due before expected income arrives and the part due after; expected income
  (`budgeted_income − mtd_income_received`, assumed to land on the month's last day)
  offsets only the latter. ADR-0023's net-of-budget coupling is removed — it existed to
  compensate for the proxy.
- **A conservative fallback for cards without statement data.** Plaid's liabilities
  coverage is not universal. A card reporting no statement has its **whole current balance**
  treated as due before income: the number still forms, and the fallback can only ever
  over-reserve, never under-reserve.
- **Snapshot and page.** `sweep_recommendation` carries the new figures; the `/sweep`
  breakdown shows the split; `/accounts` credit rows show statement balance and due date.
- Reconciling the canonical docs: ADR-0024, the `sweep` and `accounts` module docs, the
  `banking` seam rules, the domain `README.md` reserve entries and derivation card, the
  cross-cutting data model, and the `vision.md` non-goal.

## Out of scope

- **The rest of the liabilities product.** Student loans and mortgages are not read at all;
  APRs and the interest breakdown are not decoded or stored, and remain a stated non-goal
  ([vision](../../product/vision.md)). The non-goal narrows; it is not deleted.
- **A dated income forecast.** Expected income is the budget's figure less what has already
  arrived, assumed to land on the **month's last day** — the latest plausible date, so the
  assumption can only under-state available cash. Inferring a real payday from the ledger's
  income cadence, reading Plaid's recurring-transactions product, and a user-configured pay
  schedule were all considered and rejected for this chunk.
- **Any autopay model.** The app still cannot tell a card payment from any other plain
  transfer, and still does not need to: a payment lowers the balance and the statement
  balance, clearing the reserve on its own ([ADR-0023](../../adr/0023-uncovered-card-debt-reserve.md)).
- **Payments and money movement.** The recommendation stays advisory; nothing here moves
  money, and the budgeted savings transfer is still reserved for the user to make.
- **Statement-close webhooks.** Statement detail refreshes on the ordinary sync cadence.
- The checking/savings account derivation, the append-only snapshot timeline
  ([ADR-0022](../../adr/0022-on-demand-navigable-sweep-snapshots.md)), the savings reserve
  term, the fixed safety margin, and sweep multi-account aggregation (still deferred,
  [backlog](../../backlog/features.md)).
