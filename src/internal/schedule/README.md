# schedule

Owns the **sweep schedule** — the recurring movements through checking that the user
declares so the cash sweep can place them on a dated timeline. Bills, income, and
standing transfers to savings are all the same shape, distinguished by direction.

Why the activity is declared rather than detected, and why the amount is a
*conservative* figure rather than an average:
[ADR-0024](../../../docs/adr/0024-cash-flow-timeline-sweep.md). Domain framing:
[`docs/domain/README.md`](../../../docs/domain/README.md) (§Scheduled item).

## Entities

- **Item** — one declared recurring movement: a name, a direction (`out` or `in`), a
  conservative amount, a cadence, and the date field that cadence uses —
  `day_of_month` for monthly, an `anchor_date` for biweekly. An inactive Item is kept
  in storage but contributes nothing, which is how a commitment is retired without
  losing what was declared.

The amount is the figure you would not want to be short of: the **maximum** expected
for an outflow, the **minimum** expected for an inflow. The safe direction flips with
the sign — over-stating a bill holds extra cash, while over-stating a paycheck
discounts real debt against money that may not arrive.

Only genuinely scheduled movements are declared, **never intentions**. A savings
*target* is not a scheduled item; a standing transfer to savings is. An aspiration on
a timeline of dated facts would have the sweep hold money back from savings so the
user could move it to savings.

Card spending is never declared here. A card reaches the timeline through the
`accounts` balance the sweep reads, not through a declaration.

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

## Boundaries

A dependency-graph **leaf**: it imports `core/*` and nothing else under
`src/internal/`. What it holds is what the user said, so reading the ledger or the
accounts list would mean inferring what it is supposed to be told. It writes only its
own `schedule_items` table, and has no HTTP adapter of its own — the CRUD surface
lives on `/sweep`, because the sweep is its only consumer and a declared item means
nothing anywhere else in the app.

## Service

- `NewService(db)` — built at the composition root; takes no peer service.
- `List(ctx) → []Item` — the whole declared schedule, inactive items included. What
  the management surface renders.
- `ActiveItems(ctx) → []Item` — only the items that currently belong on a timeline.
  The read seam the sweep consumes.
- `Create(ctx, Item) → Item` — validates and stores, returning the Item with its
  assigned id.
- `Update(ctx, Item)` — validates and overwrites the stored Item with the same id.
  The active flag rides the same call, so switching an item off is an ordinary edit.
- `Delete(ctx, id)` — removes it from the schedule.

A declaration that does not make sense — nameless, a non-positive amount, a cadence
with no date to go with it — is a `ValidationError` and nothing is stored. Adapters
render its message inline via `IsValidationError`. The amount must be positive
because **direction carries the sign**; a negative amount paired with a direction
would let one declaration mean two opposite things.

## Persistence

- `schedule_items` — one row per declared Item. `day_of_month` and `anchor_date` are
  each nullable because only one applies, and a table CHECK enforces the pairing, so
  a stored row can never describe a cadence it has no date for.
