# Spec

The arithmetic, the data, and the failure rules. Goal, model and accepted limitations are in
[`scope.md`](scope.md).

## The computation

Read the clock **once** per run. `now` is the run instant; the horizon is `now` plus one
calendar month in the [configured app timezone](../../adr/0004-configured-app-timezone.md),
clamped when the day does not exist (31 Jan → 28 Feb). The window is `[now, horizon]`,
inclusive at both ends.

**1. Build the timeline.** Every event is a dated signed amount, following the app's
outflow-positive convention.

```
for each active credit Account:
    statement known and due date known and due date in window
        → outflow  min(statement_balance, current_balance)  on the due date
    statement known, due date unknown
        → outflow  min(statement_balance, current_balance)  at now
    statement unknown
        → outflow  current_balance                          at now
    (a due date outside the window contributes nothing)

for each active recurring item:
    project its occurrences into the window (below)
    direction out → outflow of its amount on each occurrence
    direction in  → inflow  of its amount on each occurrence
```

**2. Evaluate it.** Sort by date; on the same date, **outflows before inflows** — never assume
a deposit clears before a debit posted the same day. Then walk it, keeping a running total and
the highest point that total reaches:

```
running = 0 ; required = 0
for each event in order:
    running += outflow  (or −= inflow)
    required = max(required, running)

required_checking = required            (never below 0; it starts there)
suggested_sweep   = checking_balance − required_checking − safety_margin
direction         = sign(suggested_sweep)
```

`suggested_sweep` is **not floored**: a negative value is a meaningful pull back from savings.

**The maximum, not the final total.** The running total's end value is what the month nets
out to; its *peak* is what must be in the account to survive the month. Taking the peak is
what stops a later inflow cancelling an earlier outflow — a paycheck on the 30th cannot pay a
bill due on the 20th.

## Projecting a recurring item

```
monthly(day_of_month)   the single occurrence of that day inside the window,
                        clamped to the last day of a month that is too short
                        (day 31 → 30 Apr, → 28 Feb)

biweekly(anchor_date)   every occurrence of anchor + 14n falling inside the window,
                        stepping forward or backward from the anchor as needed
```

A one-month window holds **exactly one** occurrence of a monthly item — the property the
horizon was chosen for — and two or three of a biweekly one, depending on where the run lands.

## Missing data

Per [`scope.md`](scope.md): a missing dollar value fails hard, a missing date degrades to the
worst case. Concretely, a run produces a **needs-attention** result — listing *every*
applicable reason, never just the first — when:

- checking is absent, or ambiguous (more than one candidate account)
- the checking balance is unknown, or stale
- any active credit Account's balance is unknown, or stale

Staleness is `accounts`' rule and threshold ([ADR-0021](../../adr/0021-fault-isolating-sync-pass.md));
this module consumes the predicate and never defines a second one.

Everything else degrades rather than blocks:

- **No statement detail** (the bank does not report liabilities, or has not yet) → the card's
  whole current balance lands at `now`. The most conservative reading, and the reason no
  special case is needed for a bank without liabilities coverage.
- **Savings absent, ambiguous, unknown or stale** → shown as unknown. Savings is not a term in
  the formula, so it never blocks.
- **No recurring items declared at all** → the timeline holds only card statements. The number
  is still produced; it is simply less informed.

## Entities

### Recurring item — new

User-declared. The only source of dated checking activity, covering bills, income, and
standing transfers to savings alike.

| field | meaning |
|---|---|
| `name` | "Rent", "Paycheck", "Savings transfer" |
| `direction` | `out` or `in` |
| `amount` | the **conservative** figure: the *maximum* expected for an outflow, the *minimum* for an inflow |
| `cadence` | `monthly` or `biweekly` |
| `day_of_month` | 1–31, for `monthly` |
| `anchor_date` | any past occurrence, for `biweekly` |
| `active` | inactive items are kept but leave the timeline |

`amount` is named for its meaning, not its arithmetic, because the safe direction flips with
the sign — over-stating a bill holds extra cash, while over-stating a paycheck discounts real
debt against money that may not arrive. A field called "maximum" invites someone to later
"fix" income to use an average.

Only **actual scheduled transfers** are declared, never intentions
([`scope.md`](scope.md)). Card spending is never declared here; it reaches the timeline as a
statement, once, on its due date.

Owned by a new **`recurring` domain module** — its own entity, its own table, its own CRUD
surface, in the shape `budget` already uses for user-managed configuration.

### Card statement — new, on the provider seam

`banking` gains a `CardStatement` value (account id, a `Known` flag, statement balance,
statement issue date, next payment due date) and one method,
`GetCardStatements(ctx, accessToken)`. Named for what is taken — billing-cycle facts for
credit cards — rather than for the provider's product.

The `plaid` client satisfies it from `/liabilities/get`, reading the `credit` array only. The
`student` and `mortgage` arrays and the `aprs` field are not decoded, so **loan and APR detail
remain a non-goal** and `vision.md`'s entry narrows rather than disappearing. An Item with no
supported credit account is a normal empty result, not a failure.

`accounts` stores the detail on the account row and refreshes it on the existing sync pass,
inside that pass's per-connection fault isolation
([ADR-0021](../../adr/0021-fault-isolating-sync-pass.md)), calling the provider only for
connections holding at least one active credit Account. It inherits the same `last_synced_at`
stamp, so statement detail is governed by the one staleness rule rather than a second.

### Sweep snapshot — reshaped

A snapshot carries the checking and savings balances, `required_checking`, the safety margin,
the suggested sweep and its direction, the needs-attention reasons, and **the timeline it was
computed from** — the dated events, each with its label, direction and amount. Storing the
timeline is what makes a snapshot self-explaining: the page can show the derivation without
recomputing it, which the append-only model requires
([ADR-0022](../../adr/0022-on-demand-navigable-sweep-snapshots.md)).

## What `sweep` stops depending on

The module currently reads `budget` and `transactions` for the budget targets and the
month-to-date figures. **Under this model it reads neither.** Every input is a synced balance,
a synced statement, or a declared recurring item. Its dependencies become `accounts` and
`recurring`, and the month-to-date SQL it drove in `transactions` loses its only caller.

The account derivation is unchanged: checking is the single active cash Account not marked
counts-as-savings, savings the single active one that is, and every active credit Account
counts.

## Surfaces

- **`/sweep`** — the number and direction, then the timeline that produced it as a dated list
  with a running-total column, so the peak is visible as the row that set the figure.
  Needs-attention renders the full reason list.
- **Recurring items** — a CRUD surface for declaring them. It belongs on `/sweep`, since the
  sweep is their only consumer and they are meaningless outside it.

## Prior decisions this replaces

- [ADR-0020](../../adr/0020-monthly-cash-sweep-recommendation.md)'s reserve model, its
  checking-only month-to-date window, and its treatment of the savings target as a reserved
  term. The advisory-only boundary and the account derivation survive.
- [ADR-0023](../../adr/0023-uncovered-card-debt-reserve.md) entirely: the card term, its
  net-of-budget coupling, and its premise that a card balance is the whole obligation. Its
  durable insight survives in a different form — the balance is still read and never inferred,
  and the model still needs no notion of autopay timing, because a payment lowers both the
  balance and the statement balance on its own.
- [ADR-0022](../../adr/0022-on-demand-navigable-sweep-snapshots.md) is untouched: on-demand
  runs, the append-only timeline, and the stale-balance block all carry over.

Pre-existing snapshots were computed under the old reserve model and share no figures with the
new shape. **Recommendation: drop them in the migration** rather than render a legacy shape
alongside the new one — they are advisory history with no ongoing use, and keeping two
irreconcilable snapshot shapes on one navigable timeline costs more than the history is worth.

## Gate before implementation

The coverage spike in [`scope.md`](scope.md) still applies: whether the linked issuers report
statements at all, and whether existing Items serve `/liabilities/get` without being re-linked
(`PLAID_PRODUCTS` is `transactions` today, and the reconnect path omits products because Plaid
requires update-mode tokens to). If Items must be re-linked, that is part of this work.

Note what the spike no longer decides: the missing-data rule means the sweep produces a number
either way. Coverage changes how *good* the number is, not whether there is one.

## Testing

- **Timeline builder** — monthly projection including the short-month clamp; biweekly
  projection stepping from an anchor both directions; a due date outside the window
  contributing nothing; the three card cases (dated statement, undated statement, no
  statement); the statement capped at the current balance, so an already-paid statement
  releases without the model knowing a payment happened.
- **Evaluator** — the peak is taken, not the final total (a case where they differ, which is
  the whole model); same-day ordering putting the outflow first; an all-inflow timeline giving
  zero; a negative sweep surviving unfloored.
- **Needs-attention** — each blocking reason alone and several together; a missing statement
  *not* blocking, standing beside an unknown balance that does.
- **Persistence** — snapshot round-trip including the stored timeline.
- **`recurring`** — CRUD, and an inactive item leaving the timeline.
- **e2e** — a bill due before the next paycheck raising the number, the same bill dated after
  it lowering the number, and a card with no statement detail still producing a result.
