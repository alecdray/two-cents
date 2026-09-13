# A card reserves its unpaid statement, never its balance

The timeline places what a card's statement still owes — the statement balance less any payment
the bank reports against it since it issued — on the date the card's payment schedule resolves
to, bounded by the card's current balance. The **current balance stops being a payment record**
and becomes only what it actually is: a ceiling on what the card can possibly claim. For a card
whose bank reports no statement at all, it remains the whole obligation, a known dollar value
with no known date that the missing-date rule reserves immediately
([0026](0026-statement-detail-is-an-enhancement.md)).

This replaces the `min(statement balance, current balance)` cap that
[0024](0024-cash-flow-timeline-sweep.md) specified and chunk B shipped. The change is not that
the balance stops bounding the figure — it is *which* figure it bounds, and what else is now
doing the work it was wrongly asked to do.

**The cap was a proxy answering a question it could not see.** It conflated two facts that have
nothing to do with each other — *how much was billed*, and *whether it has been paid* — and used
the current balance to stand in for the second. That is the exact pathology 0024 diagnosed in the
model it replaced: a proxy whose compensations become load-bearing, where every attempt to make
it more accurate needs another adjustment rather than fewer.

**It bounded the wrong figure, and so leaked unbilled spend, which 0026 forbids.** A statement of
$1,000, $400 paid against it, and $200 spent since it issued: $600 is still owed on the statement,
the current balance is $800, and capping the *billed* figure reserves **$800** — carrying $200 of
this cycle's unbilled spend onto the timeline. 0026 decided that spend is dropped rather than
projected, because placing it means forecasting a billing cycle the bank never reported. So the
cap silently violated the decision it was partly meant to implement, in every partially-paid
cycle. Bounding the *unpaid* figure instead returns $600 and leaks nothing: the ceiling binds only
when the card genuinely owes less than the statement facts suggest.

**And it under-releases exactly where the model expects spending to be.** The cap can only
release a paid statement by way of a fallen balance, so it works best on a card that has stopped
being used — while 0024 *assumes* day-to-day spending runs through cards. With three weeks of
fresh spending sitting under a statement that was paid on its due date, the cap releases almost
nothing, and close to the full statement is held a second time until the next statement issues.
The user's checking is already lower by the payment; the sweep holds the same money again.

**The release has to be replaced, not deleted.** The cap is currently the only mechanism that
lets a paid statement go, so removing it alone would make the sweep strictly more conservative in
every case. What replaces it is a *reported fact* rather than a second proxy: the bank's last
payment amount and date, read from the same billing-cycle product that already supplies the
statement. A payment dated after the statement issued and covering it settles the obligation. This
keeps 0023's durable insight intact — the figure is **read, never inferred** — where deriving
payments from the transaction ledger would both rebuild a held figure badly and hand the sweep a
ledger dependency 0024 removed on purpose.

**Every way of not knowing runs conservative**, which is what lets this ship before a live login
has confirmed the payment fields populate. No payment reported, or an unparseable one → nothing is
subtracted and the full statement is reserved. A payment dated *on* the issue date → assumed to
have settled the prior cycle, so nothing is subtracted. Several payments since the statement issued
→ only the last is reported, so less is subtracted than was really paid. None of these can make the
number less conservative, so the feature degrades into the behaviour that precedes it — 0026's rule
for statement detail, applied to the payment facts within it.

**Cards still carry no occurrence match.** The reason is now stronger than the chunk boundary that
first drew the line: the bank reports the payment directly, so binding a card statement to a
checking transaction would re-derive, less reliably, a fact already in hand — and it would cost the
match record a second kind of occurrence key
([0027](0027-occurrence-matching-reconciles-the-schedule.md)).

## Rejected alternatives

- **Dropping the current balance from a card with a statement entirely.** The cleanest-sounding
  rule, and the first form of this decision: one mechanism, nothing to explain. Rejected because
  the balance is not only a proxy — *a card cannot owe more than its balance* is a fact, and
  without it a statement whose payment goes unreported is reserved in full against a card that
  demonstrably owes less. It also covers the case the provider cannot: only the **last** payment
  is reported, so two partial payments leave the unpaid figure too high, and the balance is what
  corrects it. The ceiling can never under-reserve — a balance below the unpaid statement means
  something reduced the debt, so the statement is not fully owed.
- **Deriving payments from the ledger** — an outflow from checking paired to that card as its
  transfer destination, which the app already resolves. It needs no new provider field, but it
  gives the sweep the ledger dependency 0024 removed, and it reconstructs from transactions a
  figure the bank holds exactly, breaking at the backfill edge as 0023 warned.
- **Occurrence matching for cards.** The original reading of 0024 — a card statement is a
  scheduled item with an observed amount, so it matches like one. It answers a question the
  provider already answers, and it is the most expensive of the three options.

## Consequences

- 0024's `min(statement balance, current balance)` no longer holds, and neither does the "capping
  releases a statement already paid" reasoning in the sweep's card branch. The balance bounds the
  **unpaid** figure, and releasing is the reported payment's job.
- `banking`'s `CardStatement` gains the last payment amount and date, `accounts` stores and
  refreshes them on the ordinary sync pass under the same `last_synced_at` stamp, and the
  liabilities decode widens from three reported fields to five. Loan APR and interest detail stay
  out, so `vision.md`'s narrowed non-goal narrows again by two billing-cycle facts rather than
  reopening.
- **Unconfirmed against a live login.** No deployed instance has yet shown that Chase, Amex and
  Capital One populate the payment fields. The degradation above is what makes shipping first
  legitimate rather than optimistic — an issuer that reports nothing gets the full statement
  reserved, which is the behaviour it would have had anyway. Confirming them is still owed, and
  their absence is not evidence the model is wrong.
- A partially-paid statement is now reserved for correctly rather than approximately, and a fully
  paid one is released without the card having to fall idle for the cap to notice. Where the
  payment facts are missing the ceiling still catches a paid-down card, so the behaviour before a
  live login confirms them is no worse than what chunk B shipped — it is the same bound, correctly
  applied.
