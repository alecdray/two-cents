# schedule

Owns the **sweep schedule** — the recurring movements through checking that the user
declares so the cash sweep can place them on a dated timeline. Bills, income, and
standing transfers to savings are all the same shape, distinguished by direction.

Why the activity is declared rather than detected, and why the amount is a
*conservative* figure rather than an average:
[ADR-0024](../../../docs/adr/0024-cash-flow-timeline-sweep.md). Why an occurrence is
reconciled against the ledger, and why automatic matching declines rather than guesses:
[ADR-0027](../../../docs/adr/0027-occurrence-matching-reconciles-the-schedule.md).
Domain framing: [`docs/domain/README.md`](../../../docs/domain/README.md)
(§Scheduled item, §Occurrence match).

## Entities

- **Item** — one declared recurring movement: a name, a direction (`out` or `in`), a
  conservative amount, a cadence, and the date field that cadence uses —
  `day_of_month` for monthly, an `anchor_date` for biweekly. An inactive Item is kept
  in storage but contributes nothing, which is how a commitment is retired without
  losing what was declared.
- **Occurrence match** — the stored decision about one occurrence, keyed by the item and
  the occurrence's date: the transaction that satisfied it, or the user's assertion that
  nothing did. Three states, and the empty one is not a record — *no record* (placed on
  the timeline), *matched* (settled, dropped), and *cleared* (a manual record with no
  transaction: still placed, and automatic resolution leaves it alone). A **manual
  decision is never overwritten by an automatic one**; automatic supersedes automatic,
  because resolution re-resolves from scratch every pass.

The amount is the figure you would not want to be short of — the
[conservative amount](../../../docs/domain/README.md), which is canonical for what the
term means and why it is named that way.

Only genuinely scheduled movements are declared, never intentions: a standing transfer
to savings belongs here, a savings *target* does not. Why that line is load-bearing is
an invariant — see [`AGENTS.md`](AGENTS.md).

Card spending is never declared here. A card reaches the timeline through the statement
`accounts` holds, not through a declaration — and it carries no occurrence match either,
because its bank reports the payment directly
([ADR-0028](../../../docs/adr/0028-a-card-reserves-its-unpaid-statement.md)).

Every match is on **checking**. The schedule is checking activity by definition, so a
genuinely settled occurrence always has a row there to point at; no row means the item
is declared wrong or the bill went unpaid, and neither is something to silence. A match
whose transaction is later deleted is dropped — manual or not — and its occurrence
returns to the timeline, so every way of not holding a good match reserves *more*.

## Occurrences

`Item.Occurrences(from, to)` projects the dates an Item falls on inside an inclusive
window, in ascending order:

- **monthly** — the item's day in each month the window touches, clamped to the last
  day of a month too short to hold it, so a 31st item falls on the 30th in April and
  the 28th (or 29th) in February.
- **biweekly** — every `anchor ± 14n` in the window. The anchor is *any* occurrence,
  past or future, not a start date, so the projection steps in whichever direction
  the window lies. Stepping is in calendar days, so an occurrence keeps its weekday
  across a daylight-saving transition.

It is a pure projection of the declaration. Whether an occurrence has already
happened, and what to do about it, is the sweep's decision — this module says only
when the item falls.

## Matching

An occurrence is settled by pointing at the transaction that satisfied it. **Manual
association is the guaranteed path**; automatic resolution is best effort on top, and it
runs as a step of the bank sync pass rather than on a schedule of its own.

Automatic resolution is tuned for **precision, never coverage**. A candidate must be on
checking, agree in direction, fall within a few days of the occurrence and nearer to it
than to any other of the same item, not already be matched elsewhere, and clear one of
two amount tests: within a narrow band of the declared figure, or — where the item has a
prior manually-confirmed match whose merchant is **distinctive** — merely within a sane
multiple of it, matching on the merchant instead. An ambiguous candidate set produces
**no match**, not a guess: a miss leaves the occurrence on the timeline and over-reserves,
which is visible and one click from settled, while a false match drops an obligation
silently.

The learned merchant is what makes a conservatively declared amount matchable at all —
a $200 declaration against a $140 bill is the declaration working as intended, and the
narrow band will not accept it. The
sanity band the merchant unlocks is half to double the declared figure, so a bill that
lands far under its declaration stays the user's to settle by hand. The distinctiveness guard is not a refinement: several bills
leaving through one bill-pay share a descriptor, and learning it would identify the wrong
obligation while widening the amount band at the same moment.

## Boundaries

An **import** leaf: it imports `core/*` and nothing else under `src/internal/`. Reconciling
declarations against the ledger is its job, and
it reaches the ledger through a port declared in its own vocabulary, whose adapter sits at
the composition root and is the only code holding both `accounts` (to identify checking)
and `transactions` (to query the range). What it must never do is *infer a declaration*:
the items stay what the user said. It writes only its own tables, and has no HTTP adapter
— the CRUD and matching surfaces live on `/sweep`, because the sweep is their only
consumer and a declared item means nothing anywhere else in the app.

## Service

`NewService(db, ledger, location)` is built at the composition root over the ledger port
automatic resolution reconciles against (nil resolves nothing) and the app timezone
occurrences are dated in. It takes no peer *service*: the port is declared here, and its
adapter lives at the root.

Ordinary CRUD over declared items behaves as expected, with two notes: the active flag
rides the update call, so switching an item off is an ordinary edit, and deleting an item
also removes every decision recorded against its occurrences. The methods that carry
contract beyond that:

- `ActiveItems` — only the items that currently belong on a timeline. The read seam the
  sweep consumes.
- `MatchingWindow` — every active item's occurrences that could already have been
  satisfied, each carrying the decision standing against it and, while unsettled, the rows
  it could be settled by. One ledger read serves the whole page.
- `MatchOccurrence` / `ClearOccurrence` / `ConfirmOccurrence` — the three decisions a user
  makes: this transaction settled it, nothing did, or the automatic match is right and is
  now theirs.
- `ResolveOccurrenceMatches` — the best-effort pass the sync triggers.
- `SettledOccurrences` — which occurrences are settled, **dates only**. The read the sweep
  consumes; it never learns *what* settled one.

A declaration that does not make sense — nameless, a non-positive amount, a cadence
with no date to go with it — is a `ValidationError` and nothing is stored. Adapters
render its message inline via `IsValidationError`. The amount must be positive
because **direction carries the sign**; a negative amount paired with a direction
would let one declaration mean two opposite things.

## Persistence

- `schedule_items` — one row per declared Item. `day_of_month` and `anchor_date` are
  each nullable because only one applies, and a table CHECK enforces the pairing, so
  a stored row can never describe a cadence it has no date for.
- `schedule_occurrence_matches` — one row per *decided* occurrence, keyed by item and
  occurrence date. The transaction id is nullable on purpose: a null with a manual
  source is the user's recorded assertion that nothing satisfied the occurrence, which
  is a different fact from no row at all. A unique index on the transaction id keeps one
  transaction from settling two obligations, and rows cascade from `schedule_items` —
  a deleted item's decisions have nothing left to be about.
