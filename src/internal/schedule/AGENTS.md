# schedule — domain module

Rules: ../../../docs/architecture/archetypes/domain-module.md

Owns the **sweep schedule**: the user-declared recurring movements through checking
that the cash-flow timeline is built from, and the **occurrence matches** that
reconcile those declarations against the ledger. Behaviour and the read surface:
[`README.md`](README.md). Domain authority:
[ADR-0024](../../../docs/adr/0024-cash-flow-timeline-sweep.md),
[ADR-0027](../../../docs/adr/0027-occurrence-matching-reconciles-the-schedule.md);
[`docs/domain/README.md`](../../../docs/domain/README.md) §Scheduled item and
§Occurrence match.

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
- **A manual match is never overwritten by an automatic one** — the same grain as a
  categorization override. Automatic *is* overwritten by automatic: resolution
  re-resolves its whole window from scratch every pass, the way transfer pairing
  re-resolves stored legs, so a better candidate arriving later supersedes a worse
  one and no automatic decision survives on the strength of having been made first.
- **Clearing is a stored decision, not the absence of one.** A manual record with no
  transaction means the user asserts nothing satisfied this occurrence: it is still
  placed on the timeline, and automatic resolution leaves it alone. Without the
  record, re-resolution would simply re-make the match the user just rejected.
- **There is no "settled, but nothing to point at".** The schedule is checking
  activity by definition, so money that moved against a declared occurrence left a
  row in the ledger; a missing row means the declaration is wrong or the bill went
  unpaid, and neither may be silenced by a control that drops the occurrence. Hence
  **every match is on checking, manual or automatic** — a match elsewhere would drop
  an occurrence against money that never left the account the sweep reasons about.
- **A match whose transaction no longer exists is dropped, manual or not**, and its
  occurrence returns to the timeline. A provider `removed` deletes rows, so an
  ordinary sync can orphan a match, and an orphan hides an obligation behind
  something that is gone. Manual's guarantee is that automatic will not overwrite it,
  never that it outlives its referent.
- **Automatic resolution is tuned for precision, never coverage**, and an ambiguous
  candidate set **declines** rather than guessing. A miss leaves the occurrence on
  the timeline and over-reserves, which is visible and one click from settled; a
  false match drops an obligation silently and under-reserves, which is the single
  direction this model forbids.
- **A learned merchant is evidence only while it is distinctive.** Matching a later
  occurrence on the merchant of a prior manually-confirmed match is what makes a
  *conservatively* declared amount matchable at all — but a descriptor two declared
  items have both learned identifies neither, and both fall back to the narrow amount
  test. Without that guard, several bills leaving through one bill-pay teach each
  other a shared descriptor and widen their amount band at the same moment.

Boundaries: an **import** leaf — imports `core/*` and no other module under
`src/internal/`, guarded by `TestScheduleLeafPurity`. That is no longer the same claim
as reading nothing: reconciling a declaration against the ledger is now this module's
job, and it reaches the ledger through a **port it declares in its own vocabulary**,
whose adapter lives at the composition root and is the only code holding both
`accounts` (to identify checking) and `transactions` (to query the range). What it must
still never do is *infer a declaration* — the items themselves remain what the user
said, never what the ledger suggests. Automatic resolution is triggered by a seam
`transactions` fires during its sync pass, carrying no shared types in either
direction, so neither module learns the other's vocabulary. `repo.go` is the only file
touching `core/db/sqlc`, and its methods take and return this package's `Item` and
match types, never `sqlc.*`. There is no `adapters/`: the CRUD and matching surfaces
are rendered by `sweep`'s adapters on `/sweep`, the only place a declared item means
anything.
