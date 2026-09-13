# Chunks

[`scope.md`](scope.md) and [`spec.md`](spec.md) describe the whole model;
[ADR-0024](../../adr/0024-cash-flow-timeline-sweep.md) records why. That is more than one
branch's work, so it ships in three chunks.

**The missing-data rule is what makes this sliceable.** Because an unknown degrades to the
worst case rather than failing, each chunk produces a working sweep on its own and later chunks
only make the number better informed. Nothing here is a half-built feature waiting on the next
piece.

| chunk | delivers | cards meanwhile | occurrences meanwhile |
|---|---|---|---|
| **A** — timeline core | the `schedule` module, the timeline derivation, the new snapshot, the `/sweep` page | whole current balance at the run instant | unmatched, so the window looks only forward and a late one is not reserved |
| **B** — statement ingestion | the provider seam, stored statement detail, its sync, the per-card payment schedule | real amounts on real dates | unchanged |
| **C** — occurrence matching | the match record, manual association, best-effort automatic resolution | unchanged | matched occurrences leave the timeline |

**A is first and the other two are independent of each other.** A alone is a complete
replacement for [ADR-0020](../../adr/0020-monthly-cash-sweep-recommendation.md)'s reserve model
and [ADR-0023](../../adr/0023-uncovered-card-debt-reserve.md), and it reaches no provider — so
the open question about re-establishing bank logins (see [`spec.md`](spec.md)) lands in B, where
it belongs, instead of blocking a working sweep.

A's card degradation errs toward holding **more** cash: a balance with no known due date falls
due immediately, which is the worst case. Its occurrence handling errs the other way — an
occurrence that already fell due is not reserved for, because reserving it would mean deciding
it went unpaid, and nothing in A can tell. That is a *reconciliation* question, which is what
chunk C is; performing it in A would double every monthly bill for the whole month. The safety
margin is the headroom in the meantime.

## This folder's role

This is the design record for the whole model, and the spec record for **chunk A**, which this
branch implements. B and C each get their own `docs/spec/<timestamp>-<name>/` folder on their
own branch, referencing this one rather than restating it — a deliberate departure from
one-folder-per-chunk, because three folders repeating the same model would drift against each
other.

## Chunk A boundary

**In:** the `schedule` domain module (scheduled items: name, direction, conservative amount,
monthly or biweekly cadence, active flag) with its CRUD surface on `/sweep`; the timeline
builder and its cumulative-maximum evaluator; the reshaped snapshot including the stored
timeline; the migration, which drops snapshots predating this model; the rewritten `/sweep`
page; removal of the sweep's dependency on `budget` and `transactions`.

**Out:** every provider change, stored statement detail, the per-card payment schedule (B); the
match record and any automatic resolution (C). Also out: the module `README`/`AGENTS.md` and
`vision.md` reconciliation for B and C's behaviour — each chunk reconciles its own.
