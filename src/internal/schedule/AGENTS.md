# schedule — domain module

Rules: ../../../docs/architecture/archetypes/domain-module.md

Owns the **sweep schedule**: the user-declared recurring movements through checking
that the cash-flow timeline is built from. Behaviour and the read surface:
[`README.md`](README.md). Domain authority:
[ADR-0024](../../../docs/adr/0024-cash-flow-timeline-sweep.md);
[`docs/domain/README.md`](../../../docs/domain/README.md) §Scheduled item.

Invariants a refactor could silently break:

- **`Amount` is the *conservative* figure** — the maximum expected for an outflow,
  the minimum for an inflow — and is named for its meaning rather than its
  arithmetic. The safe direction flips with the sign, so a field called "maximum"
  would invite someone to later "fix" income to use an average, which is the one
  direction that makes the sweep unsafe. It is validated positive because
  **direction carries the sign**.
- **Only actual scheduled movements are declared, never intentions.** A standing
  transfer to savings belongs here; a savings *target* does not. An aspiration on a
  timeline of dated facts would have the sweep hold money back from savings so the
  user could move it to savings.
- **`Occurrences` is a pure projection of the declaration.** It reports when an item
  falls, and nothing about whether an occurrence has passed, been paid, or should be
  reserved for — those are the consumer's decisions. Adding a `now` to it would move
  sweep policy into this module.
- **Monthly clamps, biweekly steps in calendar days.** A day the target month is too
  short for lands on that month's last day (never overflowing into the next), and a
  biweekly step is 14 calendar days, so an occurrence keeps its weekday across a
  daylight-saving transition. Calendar math is `core/timex`' — call it rather than
  growing a second copy that could drift.
- **The anchor is any occurrence, not a start date.** A biweekly item's projection
  steps backwards as readily as forwards, so an anchor in the future is as valid as
  one in the past, and the anchor is re-read as a calendar date in the window's zone
  rather than trusted as a stored instant.
- **An inactive item is kept, not deleted.** It leaves the timeline but stays
  declared, which is how a commitment is retired without losing what it said.

Boundaries: a dependency-graph **leaf** — imports `core/*` and no other module under
`src/internal/`, guarded by `TestScheduleLeafPurity`. It holds what the user
declared, so reading the ledger or the accounts list would mean inferring what it is
supposed to be told. `repo.go` is the only file touching `core/db/sqlc`, and its
methods take and return this package's `Item`, never `sqlc.*`. There is no
`adapters/`: the CRUD surface is rendered by `sweep`'s adapters on `/sweep`, the only
place a declared item means anything.
