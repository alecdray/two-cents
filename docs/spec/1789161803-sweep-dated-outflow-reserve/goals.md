# Goals

What has to be true when this work merges, and the decisions settled before any of it was
built. The boundary is in [`scope.md`](scope.md); the arithmetic is in [`spec.md`](spec.md);
the durable rationale is [ADR-0024](../../adr/0024-dated-outflow-sweep-reserve.md).

## Outcomes

1. **Timing changes the number.** A statement due after expected income arrives no longer
   demands cash in checking today; one due before it does. Today's formula cannot tell the
   two apart, and holds the same amount either way.
2. **Less idle cash in checking.** For the ordinary month — a statement due in the next
   cycle, income still to arrive — the reserve falls, and the sweep moves the difference to
   savings.
3. **The honest direction is honoured too.** When a large statement falls due *before*
   payday, the reserve **rises**. Today's proxy hides that case inside the budget term; a
   more accurate number is the goal, not a larger sweep.
4. **Each dollar is reserved exactly once, by construction rather than by correction.**
   With card charges consuming the budget directly, the budget term covers what has not
   been spent and the card terms cover what has — so ADR-0023's net-of-budget coupling is
   no longer needed and is removed.
5. **The sweep still forms a number when the bank is quiet.** A card whose institution does
   not report statements falls back to its full current balance treated as due before
   income — conservative, never optimistic, and never a dead end.
6. **The end-of-month case is unchanged.** Once income has landed and no statement is due
   before month end, the reformulated reserve converges on today's. The new machinery is
   inert exactly where today's model is already right.

## Decisions settled during Spec

- **Expected income comes from the budget, not from a forecast.**
  `max(0, budgeted_income − mtd_income_received)`, both figures already available
  (`budget.GetBudget`, `transactions.IncomeTransactionsInRange`). *Rejected:* Plaid's
  recurring-transactions product (a second new product for a figure we already hold),
  cadence inference from the ledger (we would own the failure modes for irregular pay), and
  a user-configured pay schedule (a settings surface that goes stale).
- **Expected income is assumed to arrive on the month's last day.** The latest plausible
  date, so the assumption can only under-state available cash. It also makes the model
  self-consistent: end-of-month income cannot fund this month's spending, which is why the
  budget term is *not* offset by it — only obligations falling due after month end are.
- **This reverses ADR-0023's rejection of a projected-income term, and the reversal is the
  point.** ADR-0023 refused it because it "advises moving money on the strength of an
  unlanded paycheck" and "lets one term cancel the others." Neither holds here: expected
  income is credited only against obligations due *after* it lands, so no term funded from
  today's checking is reduced by money that has not arrived, and no term is cancelled —
  each is still floored at zero on its own.
- **The budget term nets against all-account spending, not checking-only.** This is the
  load-bearing change. ADR-0020's checking-only window exists so card spend "stays reserved
  forward" from the budget; that is the proxy, and ADR-0023's net-of-budget card term
  exists to undo the double-count it creates. Once card charges consume the budget
  directly, both compensations disappear together.
- **Keep the current balance as the total card obligation; the statement only splits it by
  date.** Reserving only the statement would drop open-cycle charges from the reserve
  entirely — they are real debt, merely not yet billed. ADR-0023's "the balance is read,
  never inferred" survives intact; the statement adds a schedule, not a substitute.
- **A missing statement degrades conservatively rather than blocking.** *Rejected:*
  needs-attention on the ADR-0023 footing — a bank that never supports liabilities would
  leave the sweep permanently dead with nothing the user could do about it. Also *rejected:*
  falling back to ADR-0023's term per-card, which would leave two reserve models coexisting
  and make the on-screen breakdown unexplainable.
- **An unknown or stale card *balance* still blocks.** Unchanged from ADR-0023: the balance
  is still a term, and a wrong one errs in the direction that risks an overdraft.
- **The seam takes card statements, not liabilities.** `GetCardStatements` returns
  billing-cycle facts for credit Accounts. Student loans, mortgages and APRs are not read,
  so the `vision.md` non-goal narrows to the interest/APR detail it was always really about
  rather than being struck out.
- **`accounts` owns the statement detail; no new module.** It is per-account state,
  refreshed on the pass that already refreshes balances, under the fault isolation
  ([ADR-0021](../../adr/0021-fault-isolating-sync-pass.md)) and the staleness rule that
  module already owns. A `liabilities` module would re-implement all three for five fields.

## Canonical docs to reconcile during Implement

ADR-0024 records the decision now; the current-state docs below describe behaviour that
does not exist until phase 2, and are updated there rather than written ahead of the code:

- `vision.md` — narrow the liabilities non-goal to APR / interest detail.
- `src/internal/sweep/README.md` + `AGENTS.md` — the reserve terms, the boundary sentence
  that currently reads "never a liabilities product", and the net-of-budget invariant.
- `src/internal/accounts/README.md` + `AGENTS.md` — statement detail, its sync, its fallback.
- `src/internal/banking/AGENTS.md` — the new seam method and its naming rule.
- `docs/domain/README.md` — the Reserve and Uncovered card debt glossary entries, the new
  Expected income and Statement balance entries, and the derivation card.
- `docs/architecture/data-model.md` — the new `accounts` and `sweep_recommendation` columns.
- `docs/backlog/features.md` — drop liabilities from the explicitly-deferred list.
