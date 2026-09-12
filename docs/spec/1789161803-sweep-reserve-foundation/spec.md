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
    statement unknown
        → outflow  current_balance  at now ; done

    resolve the payment date from the card's payment schedule:
        due_date            (default)  → next_payment_due_date
        statement_plus_days(n)         → statement_issue_date + n days
        either input unknown           → now

    then, as for a scheduled item:
        already matched to a transaction → drop it
        date beyond the horizon          → contributes nothing
        date in the future               → outflow at that date
        date in the past, unmatched      → outflow at now (imminent or overdue)
    amount is always  min(statement_balance, current_balance)

for each active scheduled item:
    project its occurrences (below), from one cadence interval before now through horizon
    drop any occurrence already matched to a transaction — the balance reflects it
    remaining occurrence in the future → place it on its own date
    remaining occurrence in the past:
        direction out → place it at now   (still owed, and overdue)
        direction in  → omit              (it may never arrive — the worst case)
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

## Projecting a scheduled item

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
- **No scheduled items declared at all** → the timeline holds only card statements. The number
  is still produced; it is simply less informed.

## Entities

### Scheduled item — new

User-declared; collectively **the sweep schedule**. The only source of dated checking
activity, covering bills, income, and standing transfers to savings alike.

It is deliberately not called a transaction: the app already uses `Transaction` for a
bank-reported fact, and matching (below) makes us discuss the two in the same breath.

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

Owned by a new **`schedule` domain module** — its own entities, tables, and CRUD surface, in
the shape `budget` already uses for user-managed configuration.

### Occurrence matching — new

An occurrence is one dated instance of a scheduled item, identified by its item and its date.
A **match** binds an occurrence to the real `Transaction` that satisfied it.

Without it the model double-counts at every boundary: rent declared for the 1st, run on the
1st, already posted — the balance is $3,000 lower *and* the timeline places another $3,000.

| field | meaning |
|---|---|
| scheduled item + occurrence date | the occurrence being matched; at most one match each |
| transaction id | the real transaction that satisfied it; at most one occurrence each |
| source | `manual` or `auto` |

**Manual association is the guaranteed path**; automatic resolution is best-effort on top,
matching on account, direction, amount proximity and date proximity. Income is the easy case —
an Income-classified inflow of roughly the declared size near the expected date. This follows
the app's existing grain: *"the API category is a default, never the truth"*
([vision](../../product/vision.md)) — a **manual match is never overwritten by an automatic
one**, and it survives re-sync, exactly as a categorization override does.

Auto-resolution runs as a step in the sync pass, beside the categorization sweep and transfer
pairing that already work this way, reached through an injected seam so `transactions` and
`schedule` do not import each other.

**A consequence worth noting, not building yet:** once occurrences carry their real
transactions, a declared amount can be compared with what actually landed — so a rent increase
or a changed paycheck surfaces as drift instead of silently rotting.

### Card statement — new, on the provider seam

A card statement behaves as a **scheduled item whose amount is observed rather than declared**:
the provider supplies the figure exactly, and the date comes from the card's payment schedule.
The same matching, past/future and horizon rules apply to it.

`banking` gains a `CardStatement` value (account id, a `Known` flag, statement balance,
statement issue date, next payment due date) and one method,
`GetCardStatements(ctx, accessToken)`. Named for what is taken — billing-cycle facts for
credit cards — rather than for the provider's product.

The `plaid` client satisfies it from `/liabilities/get`, reading the `credit` array only. The
`student` and `mortgage` arrays and the `aprs` field are not decoded, so **loan and APR detail
remain a non-goal** and `vision.md`'s entry narrows rather than disappearing. An Item with no
supported credit account is a normal empty result, not a failure.

### Payment schedule — new, a per-card user setting

Autopay pulls when it is configured to, not when the bill is due, and the provider exposes no
autopay date. Two configurations occur in practice, so a credit Account carries one of:

| mode | payment date |
|---|---|
| `due_date` (**default**) | the reported next payment due date |
| `statement_plus_days(n)` | the statement issue date plus `n` days |

This sits with the existing per-account user overrides (kind, counts-as-savings, custom name)
rather than being a new concept.

The default is **the one deliberately optimistic assumption in the model**. The due date is an
upper bound — autopay can only pull earlier — so a card that pulls early and has not been
configured under-reserves by up to its statement, with the safety margin as the only headroom.
It is accepted because paying on the due date is the common configuration and the alternative
is discarding the due date for every card until each is declared.

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
a synced statement, or a scheduled item. Its dependencies become `accounts` and
`schedule`, and the month-to-date SQL it drove in `transactions` loses its only caller.
`schedule` reads transactions for matching, so the dependency moves rather than vanishing —
but it lands in the module that needs the ledger, not in the one doing the arithmetic.

The account derivation is unchanged: checking is the single active cash Account not marked
counts-as-savings, savings the single active one that is, and every active credit Account
counts.

## Surfaces

- **`/sweep`** — the number and direction, then the timeline that produced it as a dated list
  with a running-total column, so the peak is visible as the row that set the figure.
  Needs-attention renders the full reason list.
- **The schedule** — a CRUD surface for declaring scheduled items, and the place to confirm or
  correct a match. It belongs on `/sweep`, since the sweep is its only consumer and it is
  meaningless outside it.

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
new shape. **They are dropped in the migration** rather than rendered as a legacy shape beside
the new one — they are advisory history with no ongoing use, and keeping two irreconcilable
snapshot shapes on one navigable timeline costs more than the history is worth.

## What the coverage check is actually for

Whether the provider reports statement detail does **not** decide whether the model works: the
missing-data rule places a known balance with an unknown due date at `now`, so a number forms
either way. It decides **how much to build**.

- **If no linked issuer reports statements**, the provider seam, the stored statement detail,
  the extra sync step and the statement UI would all be built to return nothing, and every card
  would take the worst-case path regardless. That slice should not be built.
- **If existing bank logins cannot serve statement detail without being re-established**, then
  re-establishing every one of them is user-facing work this spec does not otherwise account
  for.

Both are cheap to answer against the real account, and neither is structural — the timeline,
the arithmetic and the schedule are unaffected by the answer.

### Result

**Institution coverage: confirmed.** Checked against the live provider account (production).
Every linked issuer reports the liabilities product:

| issuer | institution | liabilities |
|---|---|---|
| Chase | `ins_56` | yes |
| American Express | `ins_10` | yes |
| Capital One | `ins_128026` | yes |

For scale, 4,401 US institutions support liabilities against 9,785 for transactions — narrower,
but the shortfall is small banks and credit unions, not card issuers. Apple Card (Goldman) is
the one notable absence, and is not linked here. **The statement-ingestion slice is worth
building.**

**Login consent: open, and the signal is unfavourable.** All three issuers are OAuth
institutions, where adding a product to an existing login generally means sending the user back
through the bank's own consent screen rather than simply calling the new endpoint. That points
toward re-establishing each login being part of this work, but it is not proven: settling it
requires a live credential, which only the deployed instance holds — the local database is seed
data with no real login. **Plan for re-establishing logins; confirm before building it.**

## Testing

- **Timeline builder** — monthly projection including the short-month clamp; biweekly
  projection stepping from an anchor both directions; a payment date outside the window
  contributing nothing; both payment-schedule modes, including `statement_plus_days` resolving
  to a past date and landing at `now`; a card with no statement at all; the statement capped at
  the current balance, so an already-paid statement releases without the model knowing a
  payment happened.
- **Evaluator** — the peak is taken, not the final total (a case where they differ, which is
  the whole model); same-day ordering putting the outflow first; an all-inflow timeline giving
  zero; a negative sweep surviving unfloored.
- **Needs-attention** — each blocking reason alone and several together; a missing statement
  *not* blocking, standing beside an unknown balance that does.
- **Persistence** — snapshot round-trip including the stored timeline.
- **`schedule`** — CRUD, and an inactive item leaving the timeline.
- **Matching** — a matched occurrence dropping out of the timeline; an unmatched past outflow
  landing at `now` while an unmatched past inflow is omitted; an automatic match declining to
  overwrite a manual one.
- **e2e** — a bill due before the next paycheck raising the number, the same bill dated after
  it lowering the number, and a card with no statement detail still producing a result.
