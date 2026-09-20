# Spec — occurrence matching

The record, the reconciliation, and the failure rules. Boundary and decisions are in
[`scope.md`](scope.md); the rationale is [ADR-0027](../../adr/0027-occurrence-matching-reconciles-the-schedule.md).
The model this extends is [ADR-0024](../../adr/0024-cash-flow-timeline-sweep.md) and the
chunk-A spec ([`../1789161803-sweep-reserve-foundation/spec.md`](../1789161803-sweep-reserve-foundation/spec.md)).

## What changes in the computation

Both the card branch and the scheduled-item branch move, for unrelated reasons. The evaluator,
the horizon, the needs-attention rules and the safety margin are untouched.

```
for each active credit Account:
    no statement at all  → outflow of the whole current balance at now ; done   (unchanged)
    resolve the payment date from the card's payment schedule                   (unchanged)
    amount = min(statement balance − payment reported against it since issue,   <- CHANGED
                 current balance)
             (a payment qualifies only if dated strictly after the issue date;
              nothing reported, or dated on the issue date → subtract nothing.
              the balance is a ceiling on what can be claimed, never a payment
              record — it bounds the UNPAID figure, not the billed one)
    amount <= 0 → no row (nothing billed owes nothing)                          (unchanged)
```

```
for each active scheduled item:
    window = [ today − one cadence interval , horizon's date − 1 day ]     <- CHANGED
    for each occurrence the item projects across that window:
        occurrence is matched   → drop it; it is settled and owes nothing  <- NEW
        occurrence still ahead  → place it on its own date
        occurrence today or earlier:
            direction out → place it at now, carrying its original due date
            direction in  → omit          (it may never arrive — the worst case)
```

**The lookback is one cadence interval, per item, not a wider shared window.** A monthly item
then offers the window exactly two candidate occurrences — the one before today and the one
after — and a biweekly item one before and two or three after. One is the largest lookback that
cannot stack: at two intervals an occurrence that will never be matched (paid from an account
the app cannot see, or declared after it had already been paid) would be reserved twice, at
three, three times, and the number would drift upward forever with no event to correct it. At
one interval the same failure costs a single occurrence and is corrected by the next one
falling due, or by the user confirming a match.

The interval is the cadence's own calendar step — one calendar month, or 14 days — so it is
`schedule`'s to compute, not the sweep's to assume. The sweep decides *how far back to look*;
the module that owns the cadence answers *where that lands*.

**An occurrence placed at `now` keeps its original date for display.** With a lookback, "Rent,
at the run instant" may mean due today or due eleven days ago, and those read very differently
to someone asking why the peak is where it is. The timeline event gains an optional
originally-due date, filled only when the event was moved; a snapshot renders it as the reason
the row sits where it does. New field, new JSON tag — the existing tags are the storage
contract and none of them change.

## Entities

### Occurrence match — new

An occurrence is one dated instance of a scheduled item, identified by its item and its date; it
has no row of its own until something is decided about it. A **match** is that decision.

| field | meaning |
|---|---|
| item id + occurrence date | the occurrence being decided; at most one record each |
| transaction id | the transaction that satisfied it — **nullable**; at most one occurrence each |
| source | `manual` or `auto` |

**A match's transaction is always on checking — manual or automatic.** The schedule is checking
activity by definition, so money that moved against a declared occurrence left a row there; a
matched transaction on any other account would mean the occurrence drops against money that never
left the account the sweep reasons about. When no checking row can be found, that is not a gap in
the matching UI — it is the signal that the item is declared wrong, or that the occurrence was not
paid. Neither is silenced here.

Three states, and the empty one is not a record:

- **no record** — nothing has been decided. The occurrence is placed on the timeline.
- **matched** — a transaction satisfied it. The occurrence is dropped.
- **manually cleared** (`source = manual`, no transaction) — the user has asserted this
  occurrence is *not* satisfied by anything the resolver offered. The occurrence is placed, and
  automatic resolution leaves it alone.

The cleared state exists because without it, clearing a wrong automatic match would last until
the next sync pass re-made it. Rejecting a match is a decision about the occurrence and is
stored as one, in the same slot and with the same precedence as an accepted one.

**Manual is never overwritten by automatic** — the grain the app already uses for a
categorization override ([vision](../../product/vision.md): *"the API category is a default,
never the truth"*). Automatic *is* overwritten by automatic: resolution re-resolves from
scratch over its window every pass, exactly as transfer-destination pairing re-resolves stored
legs, so a better candidate arriving later supersedes a worse one and no stale automatic
decision survives on the strength of having been made first.

**A match whose transaction no longer exists is dropped**, manual or automatic, and its
occurrence returns to the timeline. A provider `removed` deletes transaction rows, so a match
can be orphaned by an ordinary sync; leaving one in place would hide an obligation behind a
transaction that is gone — the only direction this model may never degrade in. Manual's
guarantee is that automatic will not overwrite it, not that it outlives its referent.

Owned by `schedule`, which owns the occurrence. Storage is `schedule_occurrence_matches`, keyed
`(item_id, occurrence_date)`, with a unique index on `transaction_id` where it is non-null (a
transaction settles at most one obligation) and `ON DELETE CASCADE` from `schedule_items` (a
deleted item's decisions have nothing left to be about).

### Card statements — no match, new arithmetic

Cards carry **no occurrence match**, and the reason is that they need none: the bank reports the
payment on the same billing-cycle product that reports the statement, so a match record would
re-derive a fact already held, less reliably, and would cost this chunk's central entity a second
kind of occurrence key.

What they do get is the repair of the `min(statement balance, current balance)` cap
([ADR-0028](../../adr/0028-a-card-reserves-its-unpaid-statement.md)). The cap bounded the
**billed** figure and stood in for a payment it could not see, which did two things wrong: it
carried **unbilled spend** onto the timeline whenever a statement was partly paid (statement
$1,000, $400 paid, $200 spent since → it reserves the $800 balance rather than the $600 owed, and
[ADR-0026](../../adr/0026-statement-detail-is-an-enhancement.md) says that $200 may not be
placed), and it left a fallen balance as the only way a paid statement could be released — so on
a card in active use, which is the case the model assumes, almost nothing was released.

The bound itself survives, moved onto the **unpaid** figure, where it leaks nothing and means
what it says: a card cannot owe more than its balance. It can never under-reserve — a balance
below the unpaid statement means something reduced the debt — and it covers what the provider
cannot, since only the *last* payment is reported and two partial payments would otherwise leave
the unpaid figure too high.

`banking`'s `CardStatement` therefore gains the **last payment amount and date**, `accounts`
stores and refreshes them on the ordinary sync pass under the same `last_synced_at` stamp, and
the liabilities decode widens from three reported fields to five. Loan APR and interest detail
stay out.

Every unknown subtracts nothing and reserves the full statement, so the change degrades into the
behaviour that precedes it and does not wait on a live login to confirm the fields populate.

## Automatic resolution

**Where it runs.** A step in `SyncTransactions`, after the categorization sweep and the
transfer pairing, inside the pass's fault isolation ([ADR-0021](../../adr/0021-fault-isolating-sync-pass.md)):
tagged, collected, and never ending the pass. It must follow categorization, because the income
case reads the resolved classification. It re-resolves its whole window each pass rather than
this pull's delta — the same self-healing shape as the categorization sweep, so a match a failed
pass never made resolves on the next one instead of waiting for a backfill.

**Its window is the occurrences that could already have been satisfied**: from one cadence
interval before today through today. A future occurrence has no transaction to find.

**The criteria.** A candidate is admissible only if *all* hold:

- it is on the **checking** account (the rule above, which binds automatic and manual alike);
- its **direction** agrees with the item's (outflow positive, per the app-wide convention);
- its date is within **±5 calendar days** of the occurrence, and nearer to this occurrence than
  to any other of the same item;
- it is not already matched to another occurrence;
- and its **amount** clears one of two tests:
  - within **5% or $5 of the declared figure, whichever is larger** — the narrow test, which
    stands on the amount alone; or
  - it shares a **merchant with this item's most recent manually-confirmed match**, in which
    case the amount need only fall between **half and double** the declared figure — but only
    while that merchant is **distinctive**: a descriptor two declared items have both learned
    identifies neither of them, and both fall back to the narrow test.

The distinctiveness guard is not a refinement; it is what keeps the second test from being the
most dangerous rule in the design. The case that most needs a merchant signal is the one where
several bills leave checking through one bill-pay under a single descriptor — and that is exactly
the case where learning the descriptor swaps a narrow amount test for a wide one on a row that
identifies the wrong obligation. Ambiguity only catches it when both rows fall in the window; a
late-posting occurrence leaves the wrong row alone in it, admissible and unopposed.

The second test is what makes automatic resolution work at all for a conservatively-declared
item. `Amount` is the *maximum* expected for an outflow, so a $200 declaration against a $63
utility bill is the declaration working exactly as intended — and no amount-only rule can accept
it without accepting far too much. A merchant the user has already confirmed for this item is
evidence of a different kind, and it follows the app's own grain: a manual decision teaches the
automatic path, as a categorization override does. It needs no new field and no new UI — the
merchant is read from the match the user already made.

**Ambiguity declines.** Two admissible candidates that the tie-breaks (nearest date, then
nearest amount) cannot separate produce **no match**, not a guess. This is the whole tuning
posture: a missed match leaves an occurrence on the timeline and over-reserves, which is the
model's safe direction and is visible to the user as a row they can settle in one click. A false
match silently drops an obligation and under-reserves, which is the one direction
[ADR-0024](../../adr/0024-cash-flow-timeline-sweep.md)'s asymmetry does not permit. Coverage is
the user's to complete by hand; precision is not.

## Dependency shape

`schedule` gains the ledger it needs without importing it, and `transactions` learns nothing
about the schedule. Two injected seams, each pointing away from the other module, both wired at
the composition root — the shape the codebase already uses for the rule-change re-categorize and
the counts-as-savings re-pair.

```
transactions.SyncTransactions ──(seam: "resolve occurrence matches")──▶ schedule.Service
schedule.Service ──(port: candidate transactions for a window)──▶ adapter at the root
                                                                   └─ accounts (derive checking)
                                                                   └─ transactions (query range)
```

**The checking derivation goes home to `accounts`.** Matching is checking-only, so the adapter has
to identify checking — and that rule (*exactly one active cash Account with `CountsAsSavings`
false*) currently lives as a private function inside `sweep`, beside the three needs-attention
reasons. Only the *identification* moves: `accounts` owns the `counts-as-savings` flag, so the
question "which account is checking" is its to answer, and both `sweep` and the matching adapter
ask it. Whether that account's balance is known, fresh, and designated at all stays with `sweep`,
because those are three different fixes it names apart for the user and they mean nothing to
`accounts`. The alternative was a second copy of a domain rule in the composition root, which
`data-model.md` rules out for exactly this reason: cross-module reads flow through the owning
module's service.

The trigger seam carries **no shared types** — a context and an error — so neither side needs
the other's vocabulary. The candidate port is declared by `schedule` in terms of its own
candidate value (account-scoped already, an id, a date, a signed amount, a merchant), and the
root's adapter is the only code that holds both `accounts` and `transactions`. `schedule`
therefore stays an *import* leaf and `TestScheduleLeafPurity` stands unchanged — but its
`AGENTS.md` claim that reading the ledger would mean "inferring what it is supposed to be told"
no longer describes the module and must be rewritten: reconciling a declaration against the
ledger is precisely what it is now for.

`sweep` asks `schedule` for the active items **and which of their occurrences are settled** over
the window — dates, never transaction ids. It has no use for what settled an occurrence, and
taking the id would hand it a ledger reference it must then be trusted not to follow.
`TestSweepReadsNeitherBudgetNorLedger` stands unchanged and unweakened.

## Surfaces

Everything lives in the schedule region of `/sweep`, which already swaps independently of the
snapshot. A snapshot is a historical record and gains no controls — matching changes what the
*next* run computes, never what a stored one did.

- Each active item lists the occurrences in its matching window with their state: settled (with
  the transaction that settled it, and whether the app or the user decided), outstanding, or
  cleared.
- An outstanding occurrence offers the admissible candidates the resolver found, and beyond them
  any **checking** transaction — the guaranteed path is not restricted to what automatic
  resolution could see, but it is restricted to where the money can have gone.
- A settled occurrence can be confirmed (promoting an automatic match to manual, freezing it) or
  cleared (the stored decision above).
- Each control emits the cross-region refresh event ([ADR-0010](../../adr/0010-event-driven-cross-region-refresh.md))
  the region already uses; nothing here triggers a compute, in keeping with
  [ADR-0022](../../adr/0022-on-demand-navigable-sweep-snapshots.md) — only the explicit Run now
  action does.

## Degradation

Every row runs the conservative way, which is what makes an incompletely-matched schedule safe
rather than merely tolerable.

| condition | effect |
|---|---|
| no match record | occurrence placed — over-reserves if it was in fact paid |
| resolver finds nothing | same; the user settles it by hand |
| candidates ambiguous | no match; occurrence placed |
| matched transaction deleted by a sync | match dropped, occurrence placed |
| item newly declared over an already-paid occurrence | occurrence placed until matched |
| matching step fails in the sync pass | pass continues, tagged; last pass's matches stand |
| a **wrong** automatic match | the one unsafe case — occurrence dropped though unpaid. Guarded by precision-first criteria, and correctable by clearing |

Nothing here can block a run: matching has no dollar value of its own, and a blocked run is
still exactly the three checking reasons plus a card balance.

## Testing

- **Timeline** — a matched occurrence dropping out; an unmatched past outflow landing at `now`
  carrying its original due date; an unmatched past inflow still omitted; a monthly item
  offering exactly two occurrences across the lookback window and a biweekly one offering
  exactly one before today; the lookback not reaching a second interval back.
- **Match record** — manual beats automatic on write; automatic replaces automatic; a manual
  clear surviving a resolution pass; a match orphaned by a deleted transaction dropping and its
  occurrence returning; the one-occurrence-per-transaction constraint.
- **Resolution** — each criterion rejecting on its own; the narrow amount band accepting and the
  conservative over-declaration being rejected *without* the learned merchant and accepted
  *with* it; ambiguity declining; a candidate already matched elsewhere excluded; re-resolution
  from scratch superseding a worse automatic match.
- **Isolation** — `TestScheduleLeafPurity` and `TestSweepReadsNeitherBudgetNorLedger` still
  pass, which is the assertion that the seams are seams.
- **Cards** — a partly-paid statement reserving the unpaid remainder and *not* the balance (the
  unbilled-spend leak, which is the case the old cap got wrong); a fully paid statement releasing
  while the card is still being spent on; a payment dated *on* the issue date subtracting nothing;
  absent payment facts reserving the full statement, and the balance still bounding it when the
  card has been paid down; a card with no statement still taking the whole-balance-at-now path.
- **e2e** — a declared bill that fell due yesterday raising the number, the same bill matched to
  a transaction lowering it again, and clearing an automatic match raising it back; a card whose
  statement has been paid no longer holding that money twice.

## As shipped

Three departures from the design above, each decided during the build and recorded
here so the spec matches what merged.

- **The ledger port reads rows, not ids.** `ExistingTransactions(ids) []string` could
  not answer the learned-merchant question: the merchant behind a confirmed match is
  read from the transaction, and a confirmed match may be older than any window
  resolution looks at. One method answers both — `TransactionsByID(ids) []Candidate`,
  where absence is the orphan drop and the merchant is the learned signal.
- **No cross-region event.** §Surfaces said each control emits the ADR-0010 refresh
  event "the region already uses". The schedule region uses none, and deliberately:
  a schedule edit does not refresh the snapshot, because a snapshot records what was
  advised at an instant ([ADR-0022](../../adr/0022-on-demand-navigable-sweep-snapshots.md)).
  Under ADR-0010's own rule the acting handler owns the region it swaps, which is the
  direct-swap case, so the occurrence controls swap the schedule region exactly as the
  existing schedule mutations do.
- **Resolution seeds its claimed set from stored decisions, not just the pass.** A
  transaction already settling an occurrence was offered to a second one, and the
  partial unique index rejected the write mid-pass. The constraint is global, so the
  claimed set is read from storage before the pass rather than accumulated during it.

The learned-merchant band is unchanged at half to double. Its illustration in
[ADR-0027](../../adr/0027-occurrence-matching-reconciles-the-schedule.md) — a $63 bill
against a $200 declaration — falls outside that band and is not achievable; the ADR
records the decision, and the decision it records is that the criteria are "narrow
enough that a conservatively-declared amount will often fail them". The module README's
copy of the example was corrected to one the band admits.
