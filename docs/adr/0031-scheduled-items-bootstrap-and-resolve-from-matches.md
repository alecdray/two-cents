# Scheduled items may bootstrap from a transaction, and their date and amount may resolve from matches

A scheduled item can now be created directly from a transaction — prefilled and
matched in the same action — and both its date and its amount may resolve from
that item's matched transaction history instead of staying what was typed at
creation. The name and cadence remain the user's to declare; everything else
may now be evidence rather than declaration.

**Creating from a transaction skips the second match [ADR-0027](0027-occurrence-matching-reconciles-the-schedule.md)
already expected.** 0027 accepted one manual match as the price of admission
before the learned merchant takes over matching a fluctuating amount. Creating
the item from the transaction that would have been that first match — its
direction, amount, and date seed the declaration, and the transaction is
recorded as the item's first match in the same step — makes the schedule
accurate from the item's first occurrence rather than its second, with nothing
new to decide about matching itself.

**A monthly cadence may declare no date.** [0024](0024-cash-flow-timeline-sweep.md)
already treats a missing *date* as a case that degrades to the worst case
rather than blocks, for a card whose bank reports no due date. The same reading
now extends to a monthly scheduled item with no fixed real-world date — a bill
paid only when it arrives, never on autopay, for which a declared day was a
barrier to entering the item at all and a source of missed matches once
entered. Dateless, an item's occurrence is the calendar month itself rather
than one projected day in it — still a pure function of the declaration and
today's date, exactly as a dated occurrence is — and, while unmatched, takes
0024's existing missing-date branch exactly as a dated item overdue and
unmatched already does: an **outflow** holds at the run instant, an
**inflow** is omitted, because it may never arrive. Dateless changes nothing
about that asymmetry; it only changes how often an item takes that branch —
a dateless item takes it for its whole open month rather than only once
overdue. It closes the moment a match lands; the next
calendar month is simply the next occurrence `Occurrences` projects, no
different from how a dated item's next month arrives. **Biweekly stays
dated-only**: a dateless cycle has to stay a pure function of the item and
today's date, with no dependency on match history, and there is no real
no-fixed-date case among this schedule's biweekly items (income) the way there
is among monthly ones (bills) — so the complexity of a second, non-point
meaning for a biweekly anchor isn't earned here.

**The declared amount may resolve to trailing evidence instead.** A
conservative figure typed once goes stale the moment a fluctuating bill's cost
moves; left unset, an item's amount instead reads live as the maximum
(outflow) or minimum (inflow) of its last three months of matches — the same
conservative reading the declared figure was always reaching for, now measured
rather than guessed, and computed fresh on every read rather than stored, the
same discipline `Occurrences` already keeps. A user-set figure is never
touched by that computation: it stands until the user clears it back.

**The schedule's editing surface is reachable from a second place, the same way
the Rule editor already is.** [0016](0016-rule-editor-modal-and-cross-modal-return.md)
established the pattern for exactly this shape: a shared modal editor, opened
from its own page and from the transaction editor, with an opaque reopen
handle bringing the user back to whichever origin opened it. Creating a
scheduled item from a transaction reuses that pattern rather than inventing a
second one — the transaction editor never learns what a scheduled item is, and
`schedule` still has no HTTP surface of its own; `sweep` still owns the one it
renders, now linked to from a second origin.

## Rejected alternatives

- **Declaring the matching criteria** — a merchant-contains string, an amount
  range, an explicit "any date" — instead of relying on the learned merchant.
  Rejected for the reason 0027 already gave the same alternative: a hand-typed
  descriptor is more error-prone than one read off a transaction the user
  picked, and the learned merchant already tolerates a swinging amount once a
  first match exists. Creating from a transaction removes the remaining
  objection to that design — there is no longer a wait for a second occurrence
  to get it.
- **A general N-week/month/year cadence engine**, so an item could be declared
  with no fixed date without a special case. The existing monthly and biweekly
  cadences already cover every recurrence this schedule needs; dateless is a
  variant of monthly, not a new cadence.
- **A dateless biweekly form**, anchored to the last match date rather than a
  declared point. Rejected because it isn't a pure function of the item and
  the window the way every other projection in this module is — it would
  depend on match history to know where a cycle starts — and there is no real
  no-fixed-date case among this schedule's biweekly items to justify it.

## Consequences

- `schedule_items`' cadence-date CHECK constraint relaxes: `day_of_month` may
  now be null, meaning dateless, for a monthly item only; a biweekly row still
  requires its `anchor_date`.
- `amount` becomes nullable: null means computed live from the item's trailing
  three months of matches on every read, exactly like `Occurrences` — never
  cached, never written by any automatic process. A non-null amount is the
  user's override and stands until they clear it back to null.
- `schedule` gains a creation path that takes a transaction directly, seeding
  the item and recording its first match in one operation — the one exception
  to a declaration being validated before anything is stored, since here the
  declaration and its first evidence arrive together.
- A transaction may now show which scheduled item it is matched to. `schedule`
  is read by a second module for this, the mirror of how `categorization` is
  read by `transactions` for governing Rules — schedule still writes only its
  own tables and still infers no declaration.
