# A dated-outflow sweep reserve

The sweep reserve stops approximating one month of outflow and becomes **what is actually
scheduled to leave checking before income next arrives**. Three changes, which only make
sense together: the budget term nets against **all-account** month-to-date spending rather
than checking-only; the card obligation **splits by next payment due date**; and expected
income — the budget's income figure less what has already arrived, assumed to land on the
month's last day — offsets **only the part due after it lands**. The account derivation, the
safety margin, the append-only snapshot timeline
([ADR-0022](0022-on-demand-navigable-sweep-snapshots.md)) and the advisory-only boundary are
unchanged. This supersedes [ADR-0020](0020-monthly-cash-sweep-recommendation.md)'s reserve
model and absorbs [ADR-0023](0023-uncovered-card-debt-reserve.md)'s card term.

**The old reserve was a proxy, and both of its compensations existed to prop the proxy up.**
0020 holds back the month's unspent budget as a stand-in for a month of outflow, and never
asks when anything is due. To make that stand-in hold for a card user, its month-to-date
window counts only spending that left *checking*, so card spend "stays reserved forward" —
the budget term is the only place the formula can hold it. 0023 then has to net the card
balance back out of the budget term, because reserving both is counting the same money
twice. Neither adjustment is about the domain; both are about the approximation. Once card
charges consume the budget when they are made, the budget term covers what has not been
spent and the card terms cover what has, each dollar held exactly once **by construction**.
The net-of-budget coupling 0023 calls "the one deliberate coupling" is then not a rule to
preserve but a correction with nothing left to correct, and it goes.

**Timing is the thing the proxy cannot see, and it is worth real money in both directions.**
The same $1,800 statement is a different obligation depending on whether it falls due before
or after payday: due before, it must sit in checking today; due after, income that has not
yet landed will cover it. 0020's formula holds the identical amount either way. Reading the
due date lowers the reserve for the ordinary month — a statement falling in the next cycle —
and **raises** it when a heavy statement lands before payday, which is the case the proxy was
quietly under-reserving. A more accurate number is the goal; a larger sweep is only its usual
consequence.

**Reserving against expected income reverses 0023's rejection of exactly that, and the
reversal turns on where the credit is applied.** 0023 refused a projected-income term on two
grounds: that it "advises moving money on the strength of an unlanded paycheck", and that it
"lets one term cancel the others". Both were right about the shape it rejected, where income
would offset the reserve as a whole. Neither survives crediting income solely against
obligations due *after* it arrives: nothing funded from today's checking is reduced by money
that has not landed, and no term is cancelled — each is still floored at zero on its own.

**Expected income is the budget's, and it is assumed to arrive on the month's last day.**
`budgeted_income − mtd_income_received` uses two figures the app already holds; no forecast,
no second provider product, no pay-schedule setting to go stale. The last day is the latest
plausible arrival, so the assumption can only ever under-state available cash — and it is
what makes the model self-consistent, since income landing then cannot fund spending that
happened during the month. That is precisely why the budget and savings terms are *not*
offset by it.

**The statement schedules the debt; it does not replace it.** The total card obligation is
still the summed current balances — 0023's "the balance is read, never inferred" is intact,
and open-cycle charges are real debt merely not yet billed. The statement balance only says
which slice of that total is due first, capped at the card's current balance — which is what
keeps 0023's autopay-blindness intact, since a settled payment drops the balance and releases
the reserve without the sweep needing to know a payment happened. A card whose institution does not report statements
(Plaid's liabilities coverage is not universal) has its **whole balance treated as due
first**: a conservative fallback that keeps a number on screen, because a bank that will
never support the product must not be able to kill the recommendation permanently. An
unknown or stale card *balance* still blocks, unchanged.

**This narrows a non-goal rather than striking it out.** Statement balance and due date are
billing-cycle facts about credit cards; loan APR and the interest breakdown remain a
non-goal, and student and mortgage liabilities are not read at all. The seam takes card
statements, named for what is taken rather than for the provider's product. The detail is
per-account state refreshed on the pass that already refreshes balances, so it inherits that
pass's fault isolation and its single staleness rule
([ADR-0021](0021-fault-isolating-sync-pass.md)) instead of introducing second copies of both.

## Rejected alternatives

- **Gating 0023's card term by due date and changing nothing else.** Smaller, and it keeps
  0020 and 0023 intact — but 0023's term is zero for anyone inside their budget, so the
  timing logic would be inert in every ordinary month and fire only during an overspend.
- **Reserving only the statement balance**, dropping open-cycle charges from the reserve.
  Reserves less, by forgetting debt that exists.
- **Reserving only the minimum payment** for bills due before payday. Minimizes checking
  hardest, and only works by carrying a revolving balance and paying interest for it.
- **A real payday**, inferred from the ledger's income cadence, taken from Plaid's
  recurring-transactions product, or configured by the user. More faithful than month-end,
  and each buys it with inference we would own, a second new product, or a setting that goes
  stale. Month-end errs in the safe direction; a dated income forecast is separable work if
  the coarseness proves to cost anything.
- **Needs-attention when a card reports no statement**, on 0023's footing for an unknown
  balance. Consistent, but leaves an unrecoverable state for a bank that never supports
  liabilities. **Falling back to 0023's term per-card** was likewise rejected: two reserve
  models coexisting make the on-screen breakdown unexplainable.

## Consequences

- 0020's reserve model and its checking-only month-to-date window no longer hold; 0023's
  card term survives as the *total* it defines, but not as its net-of-budget shape. The
  sweep's "never a liabilities product" boundary is replaced by a narrower one: card
  statement detail, no loan or APR detail.
- The three reserve terms are floored independently **and** are independent in content again
  — the deliberate coupling 0023 introduced is gone.
- With income landed and nothing due before month end, the reserve converges on 0020+0023's
  number. The new terms change the answer only where timing is what the proxy could not see.
- A snapshot now carries expected income, income received, and the due-first / due-later
  split, so a stored recommendation still explains its own arithmetic.
