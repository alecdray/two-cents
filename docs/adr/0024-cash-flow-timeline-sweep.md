# A cash-flow timeline for the cash sweep

The sweep stops computing a budget-derived reserve. It places every expected inflow and
outflow on a **dated timeline** covering one month from the run instant, and the amount that
must stay in checking is the **highest point the running total reaches** over that window.
This supersedes [ADR-0020](0020-monthly-cash-sweep-recommendation.md)'s reserve model and
[ADR-0023](0023-uncovered-card-debt-reserve.md) in full. The advisory-only boundary, the
account derivation, and the append-only snapshot timeline
([ADR-0022](0022-on-demand-navigable-sweep-snapshots.md)) are unchanged.

**The reserve model was a proxy, and its compensations were load-bearing.** 0020 held back the
month's unspent budget as a stand-in for a month of outflow, and never asked when anything was
due. Two adjustments existed only to keep that stand-in upright: its month-to-date window
counted solely spending that left *checking*, so card spend stayed reserved forward from the
budget, and 0023 then netted the card balance back *out of* the budget term so the same money
was not held twice. Neither describes the domain. Every attempt to make the model
timing-aware required further compensations rather than fewer — which is the evidence that the
foundation, not the terms, was wrong.

**The peak, not the total.** The running total's end value is what a month nets out to; its
maximum is what must be present for the balance never to go negative, and that is the question
the sweep is actually asking. Taking the maximum is also what confines income to offsetting
only what follows it: netting periods and summing them lets a later surplus cancel an earlier
shortfall, which is a paycheck on the 30th paying a bill due on the 20th. The property is
structural rather than a rule the arithmetic has to remember.

**The budget leaves the sweep.** It is whole-of-spending, rent included, so the moment rent is
declared as a dated outflow the same money exists in two places — the same double-count, in
new clothes. The timeline carries only facts: a synced balance, a synced statement, a declared
commitment. The budget remains the spending plan it is everywhere else in the app.

**Card spending reaches the timeline once, as a statement on its due date** — never as
forecast spending. This preserves what 0023 got right: the balance is read and never inferred,
and no assumption about autopay timing is needed, because a payment lowers the balance and the
statement balance together. It also means the sweep now reads card statement detail, so
liabilities narrows from a non-goal to a narrower one — loan and APR detail stay out.

**Dated activity is user-declared, because nothing else knows it.** No ledger record says rent
falls due on the 1st, and the provider's recurring-transactions product is a paid add-on. The
user declares **scheduled items** — a name, a direction, a cadence, and a *conservative*
amount, whose safe direction flips with the sign: the maximum expected for an outflow, the
minimum for an inflow. Over-stating a bill holds extra cash; over-stating a paycheck discounts
real debt against money that may not arrive. Only genuinely scheduled movements are declared,
never intentions — an aspiration on a timeline of dated facts would have the sweep hold money
back from savings so the user could move it to savings.

**Autopay pulls when it is configured to, not when the bill is due.** Two configurations occur
in practice — on the due date, or a fixed number of days after the statement issues — and no
provider reports which. The due date is therefore only an *upper bound* on when the money
leaves, so a card carries a user-declared payment schedule covering both shapes. Defaulting to
the due date is the one deliberately optimistic assumption in the model: a card that pulls
early and has not been configured under-reserves by up to its statement, and the safety margin
is the only headroom. It is accepted because due-date payment is the common configuration, and
the alternative — treating every issued statement as payable immediately until each card is
declared — discards the due date's value entirely and costs the interest the sweep exists to
earn.

**Occurrences are matched to real transactions.** Otherwise the model double-counts at every
boundary: a bill declared for the 1st, a run on the 1st, already paid — the balance is lower
*and* the timeline places it again. Matching is manual by guarantee and automatic by
best effort, with a manual match never overwritten by an automatic one, the same grain as a
categorization override.

**One month exactly, because that is the length at which every monthly item occurs once.** A
longer window was tried and rejected: its only effect is to pull a *second* occurrence of
monthly items into runs late in the month, and it pulls in the outflow without the income that
covers it — so the number would lurch upward in the last week of every month. Lengthening a
horizon never removes truncation; it moves the cut, and moving the cut past an outflow but not
past its covering inflow makes the answer worse rather than safer.

**Unknowns are deliberately asymmetric.** A missing dollar value is a hard failure, because a
number built on a figure we do not hold is confidently wrong. A missing *date* degrades to the
worst case instead: an outflow lands at the run instant, an inflow at the far end of the
horizon. Every unknown therefore makes the answer more conservative and none makes it less —
and a card whose bank reports no statement needs no special case, since a known balance with
an unknown due date simply lands immediately.

**What the model does not do, by design.** It assumes day-to-day spending runs through cards,
where it arrives as a dated statement rather than an undated guess. Spending straight from
checking is treated as the exception and is the user's to manage; the safety margin is
headroom that keeps an exception from being immediately dangerous, not a term sized to absorb
one. And the model forecasts nothing — it places only money already owed, already scheduled,
or already declared.

## Rejected alternatives

- **Extending 0023's card term with due dates.** The narrower change, and it keeps both prior
  ADRs intact — but that term is zero for anyone inside their budget, so the timing logic
  would be inert in every ordinary month and fire only during an overspend.
- **Deriving expected income from the budget** (its income figure less what has arrived),
  assumed to land on the month's last day. Needs no new data, but it is an amount without a
  date wearing one, and it over-reserves by the size of a whole statement whenever a bill
  falls due before the real pay date.
- **Inferring recurring activity from the ledger**, or buying the provider's
  recurring-transactions product. The first means owning cadence detection and its failure
  modes before anything works at all; the second is a paid add-on. Declaring by hand is viable
  because only *checking* activity needs declaring, which is a handful of entries.
- **A single reserve figure without a timeline.** Any such figure has to answer "due when?"
  with an average, and the whole failure of the prior model is that it answered with none.

## Consequences

- 0020's reserve model, its checking-only spending window, and its treatment of the savings
  target as a reserved term no longer hold; 0023 is superseded outright. The savings target
  becomes an ordinary scheduled outflow, so reserving it falls out of the model rather than
  being a special case.
- The sweep no longer reads the budget at all. It reads balances, statements, and the
  schedule; the ledger dependency moves to the schedule, which needs it for matching.
- A snapshot stores the timeline it was computed from, so it explains its own arithmetic
  without recomputation. Snapshots predating this model share no figures with it and are
  discarded.
- Whether the provider reports statements for a given card changes how well-informed the
  number is, not whether there is one.
- A card statement is structurally a scheduled item whose amount is observed rather than
  declared, so matching, the past/future rules and the horizon apply to it unchanged.
