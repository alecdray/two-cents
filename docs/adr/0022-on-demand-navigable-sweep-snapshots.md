# On-demand, navigable sweep snapshots

The cash-sweep recommendation becomes **runnable at any time** and its storage becomes an
**append-only history of point-in-time snapshots** the `/sweep` page can navigate. This
revises two deliberate v1 choices in [ADR-0020](0020-monthly-cash-sweep-recommendation.md):
"no on-demand compute" and "a single latest snapshot the monthly job overwrites." The
**reserve model, account derivation, and the read-no-card-balance boundary are unchanged** —
what changes is *when* a recommendation is produced, *how many* are kept, and one addition to
the needs-attention rules (a stale checking balance, below).

**A snapshot always reflects *now*.** Running the sweep — whether from the monthly job or the
new on-demand action — reads live balances and month-to-date activity through the run instant
and stamps the result with that moment. There is no backdated, as-of-a-past-date compute:
Plaid reports only a current balance, so a historical snapshot's inputs aren't reliably
reconstructable, and inventing them would produce advice that never actually held. The number
the user gets is always the number for the state of the world when they asked. `Compute`
already reads *now* and is reused verbatim.

**The derivation is unchanged — anytime compute is already free.** Nothing in the reserve
arithmetic or the month-to-date window needs touching to run at an arbitrary instant. Balances
and the budget are read live, and the MTD window is computed off `Compute`'s own `now`, so the
month-to-date figures already mean "this month through the run instant" whenever the call
happens. This rests on the existing invariant that no transaction is future-dated — which is why
the MTD query's upper bound is the month's end rather than `now` yet still means "through now"
(the invariant and its consequence for the bound live in a doc-comment at the query site in
`sweep/service.go`, its durable home; it is referenced here, not restated). A refactor must
not "tighten" that upper bound to `now` thinking it changes behavior: it does not, and the
comment guards against exactly that.

**`computed_at` is stamped from the compute instant, not the DB write.** Because the timestamp
is now the navigation key and the on-screen label — and manual runs make several snapshots a
day routine — a snapshot must carry the exact instant `Compute` measured against, not the
slightly-later moment the row is written. Today the value derives from the row's `updated_at`
default at save time; for append-only snapshots the run instant (`Compute`'s `now`) is stamped
as `computed_at` so a snapshot's label provably matches the month-to-date window it measured.

**On-demand compute, because the monthly cadence isn't always soon enough.** ADR-0020 deferred
on-demand compute reasoning that the 7th-of-month tick sufficed — by then every card has closed
and autopaid, with runway before the next cycle. That holds for the *baseline* number, but a
user who just made a large purchase, moved cash by hand, or reconnected an account wants to see
the sweep *now*, not wait for the 7th. On-demand was never expensive — `Compute` is a pure read
over data already in the app — so the only thing that kept it out was scope. A **Run now**
action on `/sweep` computes and appends a snapshot, landing the user on the fresh result.

**Append, not overwrite — because a snapshot is point-in-time advice worth keeping.** To be
*navigable* the history has to exist; overwriting destroys it. Each run is an immutable record
of what was advised, and against what figures, at a given instant — the same retrospective
character the month wraps have ([ADR-0018](0018-month-navigable-home.md)). The monthly job
therefore **appends** a snapshot on the 7th instead of replacing the latest, so an automatic
monthly baseline and any manual runs coexist in one timeline. All snapshots are **retained** —
a single-user app produces a handful a month, so full history is cheap and pruning is needless
complexity.

**Every snapshot is the same kind of thing.** A snapshot records nothing about what triggered
it: the job and the button run the identical computation and produce an identical result, so
the trigger is not a property of the advice and storing it would add an axis the formula never
reads and the user cannot act on. "Monthly" accordingly stops being a property of a
recommendation and becomes only a cadence that produces them — the domain language moves from
*the monthly recommendation* to *a snapshot, and the timeline of them*. Snapshots are
distinguished by their instant and nothing else. Every run appends, including one whose figures
are identical to the snapshot before it: an entry records that the user asked at that moment and
what the answer was, which is exactly what collapsing duplicates would destroy.

**Navigation reuses the month-navigable spirit, adapted to irregular instants.** Like the
month-navigable home ([ADR-0018](0018-month-navigable-home.md)), `/sweep` defaults to the
newest snapshot and lets the user step back through history, and each snapshot is
deep-linkable. But sweep snapshots fall on irregular timestamps rather than neat month
buckets, so the selector is **older/newer stepping keyed on `computed_at`** with a
date-and-time label, not a rail of month chips. The label gains time-of-day because manual
runs make more than one snapshot per day possible — where the single-snapshot view showed only
the month. The first-run empty state stays until a first snapshot exists, and a
needs-attention run is a real, navigable snapshot (still distinct from "nothing stored yet").

**A stale checking balance is needs-attention, not a footnote.** Running at an arbitrary
instant makes the freshness of the inputs a live question in a way a fixed monthly tick did
not: a user asks *now* precisely because something just changed, and the one thing that must
have caught up for the answer to mean anything is the checking balance the whole formula is
anchored on. A balance that has gone too long without refreshing therefore joins the
needs-attention reasons ([ADR-0020](0020-monthly-cash-sweep-recommendation.md)) rather than
quietly qualifying a number. This closes a real hole: a connection can fail to sync
indefinitely without ever reaching needs-reconnect, so nothing else on `/sweep` would have
said anything ([ADR-0021](0021-fault-isolating-sync-pass.md)). Staleness stays **one rule with
one definition, owned by `accounts`** and consumed by both the overview and the sweep — a
second threshold that could drift from the first would be worse than none. It follows the
existing asymmetry exactly: stale *checking* blocks, stale *savings* does not, for the same
reason an unknown savings balance does not — savings is not a term in the formula.

The rule is uniform across callers. The monthly job on the 7th can therefore append a
needs-attention snapshot instead of that month's baseline number, and that is the intended
outcome: `Compute` does not know who called it, and a result that varied by caller would be a
worse thing to own than a missing baseline. The baseline is also no longer lost — the user
fixes the connection and runs the sweep, which is what on-demand is *for*, and the history
keeps an honest record of the month the app could not advise.

**Data-model change: single upserted row → append-only table.** The `sweep_recommendation`
table drops the fixed `id = 'default'` upsert: each snapshot is inserted as its own row with a
generated UUID id (the app's `uuid.NewString()` convention) and its `computed_at` instant, the
ordering and navigation key. The columns are otherwise the numeric-or-needs-attention shape
ADR-0020 defined. The repo's single-latest surface (`SaveLatest`/`LoadLatest` upsert) becomes
append + read: `Save` inserts a new snapshot; `LoadLatest` reads the most recent by
`computed_at`; a by-id read and a list-for-navigation back the deep link and the older/newer
stepping. The lone pre-existing `'default'` row is carried forward as the first historical
snapshot (or dropped — at most one row exists, single-user), decided at Implement.

## Rejected alternatives

- **Live recompute on every page load, persist nothing.** Simplest, and it makes the number
  always-current — but it discards exactly what the request asks for: a *snapshot at a given
  time* and a navigable history. There would be no record of what was advised when.
- **Backdated as-of compute** (pick a past date, reconstruct its inputs). The data isn't
  there — Plaid balances are point-in-time — so past snapshots would rest on fabricated
  balances. A snapshot reflects *now*; history is accumulated by running over time, not
  reconstructed.
- **Capped retention** (last N, or a rolling window). Needless pruning logic for a single-user
  app that generates a handful of snapshots a month.
- **Collapsing identical consecutive snapshots** (a run matching the latest updates its instant
  instead of appending). Keeps every entry a real change, but breaks the append-only model and
  makes a snapshot's instant no longer the instant it was computed — the one thing the label and
  the navigation key both depend on.
- **Throttling Run now** (a minimum interval or a cooldown). Bounds duplicate entries at the
  source, but refuses the user a fresh number exactly when they want one, to solve a problem a
  single-user app does not have.
- **A month-chip rail** as in ADR-0018. Snapshots aren't one-per-month; forcing them into month
  buckets would hide multiple same-month runs, which the manual action makes routine.

## Consequences

- ADR-0020's "no on-demand compute" and "single latest, upserted, re-runs replace" statements
  are superseded by this ADR; its formula, inputs, account derivation, and boundaries stand.
- `/sweep` gains a write action (Run now) alongside its read, and a navigation control. The
  page still triggers no compute on a plain read — only the explicit action does.
- The `sweep` module `README.md`/`AGENTS.md`/package doc and the domain `README.md` sweep
  entries move from "monthly, single persisted snapshot" to "append-only history of snapshots,
  produced on demand or on the monthly cadence."
- `accounts` owns balance staleness as a domain rule rather than an overview presentation rule,
  and exposes it; `sweep` is its second consumer. The threshold and its rationale stay in one
  place ([ADR-0021](0021-fault-isolating-sync-pass.md)).
- A sync stuck failing now costs the sweep its answer rather than silently degrading it — a
  needs-attention snapshot on the 7th is a possible, and correct, outcome.
