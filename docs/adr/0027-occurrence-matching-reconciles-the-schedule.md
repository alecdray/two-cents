# Occurrence matching reconciles the schedule against the ledger

A declared occurrence and the transaction that satisfied it are bound by a stored **match**, and
an occurrence carrying one leaves the timeline before it is placed. With that record in hand the
occurrence window stops looking only forward: it reaches back **one cadence interval**, so an
occurrence that fell due and was never satisfied is finally reserved for. This completes the
model in [0024](0024-cash-flow-timeline-sweep.md), which specified matching but could not ship it
in the same slice as the timeline it protects.

**The forward-only window was a placeholder, and this is what it was waiting for.** 0024 records
the reasoning already: reaching back is not a wider window but a *reconciliation* of what was
declared against what happened, and performing it with no way to tell a paid occurrence from an
unpaid one would reserve every monthly item twice, every day, for the whole month. The cost of
the placeholder was that a genuinely late bill was not reserved for at all. A match record
removes the ambiguity the placeholder existed to avoid, so the reversal is the original design
arriving rather than a change of mind.

**One interval, and not more, because an unmatchable occurrence must stay a finite error.** Some
occurrences will never be matched — a bill paid from an account the app cannot see, an item
declared the week after it was last paid. At a one-interval lookback that costs a single
occurrence, and the next one falling due replaces it rather than adding to it. At two it costs
two, at three, three, and the number climbs with nothing in the model able to bring it down.
Lengthening the lookback has the same shape as lengthening the horizon, which 0024 already
rejected for the same reason: it does not remove the truncation, it moves it — and here it moves
it somewhere with no self-correcting event.

**Automatic resolution is tuned for precision, never coverage.** The two failure directions are
not comparable. A missed match leaves an occurrence on the timeline: the number is too high, the
error is visible as a row the user can settle in one click, and it runs the way every other
unknown in this model runs. A false match drops an obligation nothing will place again: the
number is too low, silently, which is the single direction 0024's asymmetry forbids. So an
ambiguous candidate set declines to match rather than choosing, and the criteria are narrow
enough that a conservatively-declared amount will often fail them. Completing the coverage is the
user's, by hand — that is what "manual is the guaranteed path" means, and it is why the manual
path is built first rather than as a fallback.

**A manual decision teaches the automatic path, rather than merely outranking it.** A declared
`Amount` is deliberately the *maximum* expected for an outflow, so an amount-only rule must
either reject the variable bills the conservative figure exists for, or widen until it accepts
things it should not. The way out is evidence of a different kind: once the user has manually
matched an occurrence of an item, that transaction's merchant is known to belong to it, and a
later occurrence may match on the merchant with only a sanity check on the amount. This is the
grain the app already runs on — *the API category is a default, never the truth* — and it needs
no new declared field, which matters because a field the user must fill in to make matching work
is a field that will be empty.

**A learned merchant is only evidence while it is distinctive.** The signal has one failure mode
and it is severe: several bills leaving checking through a single bill-pay share one descriptor,
so the item learns a merchant identifying the wrong obligation as readily as the right one — and
learning it *widens* the amount test at the same moment. That is a false match built by the
mechanism meant to prevent them, and the ambiguity rule does not catch it when the right row posts
late and leaves the wrong one alone in the window. So a descriptor two declared items have both
learned identifies neither, and both fall back to matching on the amount alone. The guard costs
the bill-pay case its automatic matching, which is the case least able to support it — and that is
the right trade, because the alternative there is not "no match" but "the wrong match".

**There is no "settled, but nothing to point at".** A match record can say a transaction
satisfied an occurrence, or that the user asserts none did — and deliberately not that an
occurrence is settled with no transaction behind it. The schedule is *checking activity by
definition*, so money that moved against a declared occurrence left a row in the ledger, and
every genuinely settled occurrence therefore has something to point at. A user reaching for a
"settled anyway" control means one of two things instead: the item is declared wrong (it does
not move through checking, or it no longer recurs), which is fixed on the declaration and stays
fixed; or the occurrence was not in fact paid, which is precisely what the sweep should be
holding money for. The one honest case — a recurring bill that simply did not occur this month —
is answered by the model's own asymmetry: we do not *know* it did not happen, so it is reserved
for and rolls off one cadence later. A dismiss control would buy that single case at the price of
a monthly way to silence a wrong declaration, and it would be the only user-facing control in the
model whose effect is to reserve *less*.

**Clearing a match is a stored decision, not the absence of one.** Automatic resolution
re-resolves from scratch every pass, so "no record" cannot mean "the user rejected this" — the
next pass would simply make the match again. A rejection is recorded in the same slot, with the
same manual precedence, as an acceptance: the occurrence is placed on the timeline and automatic
resolution leaves it alone.

**A match whose transaction is gone is dropped, manual or not.** A provider `removed` deletes
rows, so an ordinary sync can orphan a match, and an orphaned match hides an obligation behind a
transaction that no longer exists — under-reserving, silently. Manual's guarantee is that
automatic will not overwrite it; it was never that the decision outlives the fact it was about.
Every remaining way of not holding a good match therefore reserves *more*, which is what keeps a
partly-matched schedule safe rather than merely tolerable.

**Card statements carry no match.** Not because they are outside the slice — the reconciliation
they needed is real, and [0028](0028-a-card-reserves-its-unpaid-statement.md) makes it in the same
branch — but because the bank answers their question directly. A card's payment is a reported fact
on the same billing-cycle product that supplies the statement, so binding a statement to a checking
transaction would re-derive, less reliably, something already in hand, and would cost this record a
second kind of occurrence key. A scheduled item has no such reporter: nothing but the ledger can
say whether a declared bill was paid, which is why the match record exists at all.

## Rejected alternatives

- **A declared merchant hint on the scheduled item.** Makes automatic matching work from the
  first occurrence rather than the second, at the cost of a field the user must fill in
  correctly, against a bank descriptor they have to go and look up. The learned merchant costs
  one manual match — which the design already expects — and cannot be wrong about a descriptor it
  read from a transaction the user picked.
- **A wider amount band instead of the merchant signal.** Any band loose enough for a $200
  declaration to accept a $63 bill accepts most of the month's other outflows too, and it buys
  coverage with precision in the one direction the model cannot spend.
- **Letting the manual path reach beyond checking.** Tempting, because the guaranteed path
  should not be hemmed in by what automatic resolution can see — and it is not, within checking.
  But a match on another account drops an occurrence against money that never left the account
  the sweep reasons about, and it is precisely how a wrong declaration gets papered over once a
  month instead of fixed once. Restricting it is what turns "every settled occurrence has a row
  to point at" from an assertion into something the design enforces.
- **Reconciling in the sweep instead of a stored record.** The sweep would have to read the
  ledger, re-deciding every run and remembering nothing — so the user could not correct a wrong
  decision, which is the whole point of the manual path.

## Consequences

- 0024's "the window looks only forward" no longer holds, and neither does the `sweep`
  invariant that repeats it. The occurrence window now reaches back one cadence interval, and
  the reason reaching back was refused is the reason it is now safe.
- **The two decisions above pull against each other, deliberately.** Before this record, a
  matching miss cost nothing, because a past occurrence was not placed at all; the lookback
  charges a full occurrence of over-reserve per miss, continuously, until the next one falls due
  — and the precision-first posture is the one that produces misses. The trade is taken with open
  eyes: a miss is visible on `/sweep` as an outstanding occurrence and is one click from settled,
  while the alternative posture pays for coverage in silent under-reserves nobody is prompted to
  look for. It does mean a small recurring reconciliation is part of using the schedule, rather
  than an optional extra.
- The sweep holds back **more** in exactly one case — an occurrence that fell due and is
  unmatched — and is unchanged in every other. This is the opposite direction to
  [0026](0026-statement-detail-is-an-enhancement.md)'s one concession.
- `schedule` stops being a module that only holds what it was told: reconciling a declaration
  against the ledger is now its job. It reaches the ledger through an injected port and remains
  an *import* leaf, and the sweep learns which occurrences are settled without learning what
  settled them — so it still reads neither budget nor ledger.
- A timeline event moved to the run instant now carries the date it was originally due, because
  with a lookback "at the run instant" no longer implies "due today".
- The card double-count this design first pointed at is closed by
  [0028](0028-a-card-reserves-its-unpaid-statement.md), not by extending this record to cards.
  Matching and the card's arithmetic ship together and stay separate decisions.
- Declaration drift — comparing what was declared against what actually landed — becomes
  buildable, and is still not built.
