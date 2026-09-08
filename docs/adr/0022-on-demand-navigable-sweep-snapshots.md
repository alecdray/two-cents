# On-demand, navigable sweep snapshots

The cash-sweep recommendation becomes **runnable at any time** and its storage becomes an
**append-only history of point-in-time snapshots** the `/sweep` page can navigate. This
revises two deliberate v1 choices in [ADR-0020](0020-monthly-cash-sweep-recommendation.md):
"no on-demand compute" and "a single latest snapshot the monthly job overwrites." The
**reserve model, account derivation, needs-attention rules, and the read-no-card-balance
boundary are unchanged** — only *when* a recommendation is produced and *how many* are kept.

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

**Navigation reuses the month-navigable spirit, adapted to irregular instants.** Like the
month-navigable home ([ADR-0018](0018-month-navigable-home.md)), `/sweep` defaults to the
newest snapshot and lets the user step back through history, and each snapshot is
deep-linkable. But sweep snapshots fall on irregular timestamps rather than neat month
buckets, so the selector is **older/newer stepping keyed on `computed_at`** with a
date-and-time label, not a rail of month chips. The label gains time-of-day because manual
runs make more than one snapshot per day possible — where the single-snapshot view showed only
the month. The first-run empty state stays until a first snapshot exists, and a
needs-attention run is a real, navigable snapshot (still distinct from "nothing stored yet").

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
- **A month-chip rail** as in ADR-0018. Snapshots aren't one-per-month; forcing them into month
  buckets would hide multiple same-month runs, which the manual action makes routine.

## Consequences

- ADR-0020's "no on-demand compute" and "single latest, upserted, re-runs replace" statements
  are superseded by this ADR; its formula, inputs, account derivation, and boundaries stand.
- `/sweep` gains a write action (Run now) alongside its read, and a navigation control. The
  page still triggers no compute on a plain read — only the explicit action does.
- The `sweep` module `README.md`/`AGENTS.md`/package doc and the domain `README.md` sweep
  entries move from "monthly, single persisted snapshot" to "append-only history of on-demand
  or monthly snapshots."
