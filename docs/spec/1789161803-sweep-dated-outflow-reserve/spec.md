# Spec

The arithmetic, the seam, the storage, and the failure rules. Boundary:
[`scope.md`](scope.md). Outcomes and settled decisions: [`goals.md`](goals.md). Rationale:
[ADR-0024](../../adr/0024-dated-outflow-sweep-reserve.md).

## The reserve

Read the clock once per run, as today. `month_end` is the last instant of the run's
calendar month in the [configured app timezone](../../adr/0004-configured-app-timezone.md);
expected income is assumed to arrive there.

```
expected_income  = max(0, budgeted_income − mtd_income_received)

budget_reserve   = max(0, total_spending_budget − mtd_spending_all_accounts)
savings_reserve  = max(0, savings_target − mtd_savings_contributed)

stmt_due_first   = Σ  statement_obligation(card)  over active credit Accounts
                   where statement_obligation(card) =
                       card.current_balance                            if statement unknown
                       card.current_balance                            if due date unknown
                       min(card.statement_balance, card.balance)       if due_date ≤ month_end
                       0                                               otherwise
card_after_pay   = max(0, total_card_balance − stmt_due_first)

card_reserve     = stmt_due_first + max(0, card_after_pay − expected_income)
reserve          = budget_reserve + savings_reserve + card_reserve
suggested_sweep  = current_checking − reserve − fixed_safety_margin
direction        = sign(suggested_sweep)
```

`suggested_sweep` is still not floored — a negative value is a meaningful pull from
savings. Money keeps the app-wide outflow-positive convention.

Two details in `statement_obligation` carry weight. **A statement with no due date takes the
fallback**, not the zero branch: the figure that says *when* is the one missing, so the safe
reading is "due first". And the statement is **capped at the card's current balance**, which
is what makes the term self-correcting under autopay — once a payment settles, the balance
drops below the closed statement and the reserve releases without the sweep ever needing to
know a payment happened ([ADR-0023](../../adr/0023-uncovered-card-debt-reserve.md)). Without
the cap, a paid statement would stay reserved until the next cycle closed. The cap also keeps
`stmt_due_first ≤ total_card_balance` per card, so `card_after_pay` cannot go negative; the
`max(0, …)` around it is belt-and-braces, not load-bearing.

**Three changes from [ADR-0023](../../adr/0023-uncovered-card-debt-reserve.md):**

1. `budget_reserve` nets against **all-account** month-to-date spending
   (`transactions.SpendingTransactionsInRange`), not checking-only
   (`SpendingByAccountInRange`). Card charges now consume the budget when they are made.
2. The card obligation **splits by due date**. The total is still the summed current
   balances — the statement schedules the debt, it does not replace it.
3. The **net-of-budget coupling is gone.** It compensated for card spend being reserved
   forward by a budget term that could not see it; change 1 removes the cause, so the
   correction goes with it. Each term is again floored at zero independently, and the
   terms are once more independent in content.

`expected_income` offsets **only** `card_after_pay`. It never touches `budget_reserve` or
`savings_reserve`: income landing on the month's last day cannot fund spending that
happens during the month, nor a savings transfer the user may make at any point in it.

## Worked examples

Budget: income $8,000, savings target $1,000 → `total_spending_budget` $7,000. Safety
margin $500.

**A — statement due before payday.** The 7th. $800 spent from checking, $1,200 charged on
cards this month. Cards total $3,000: statement $1,800 due the 20th, open cycle $1,200. No
income received yet.

```
expected_income = 8000 − 0            = 8,000
budget_reserve  = 7000 − (800+1200)   = 5,000
savings_reserve = 1000 − 0            = 1,000
stmt_due_first  = 1,800                          (due the 20th ≤ month end)
card_after_pay  = 3000 − 1800         = 1,200
card_reserve    = 1800 + max(0, 1200−8000) = 1,800
reserve = 7,800   →   checking must hold 8,300      (today's formula: 7,700)
```

**B — same month, statement due the 5th of next month.** Only the due date differs.

```
stmt_due_first  = 0
card_after_pay  = 3,000  →  max(0, 3000−8000) = 0
card_reserve    = 0
reserve = 6,000   →   checking must hold 6,500      (today's formula: 7,700)
```

**C — end of month, income landed.** The 30th. Income $8,000 received, savings $1,000
moved, $6,500 spent across all accounts ($2,000 from checking, $4,500 on cards). Cards
total $4,500, statement $2,200 due the 10th of next month.

```
expected_income = 8000 − 8000         = 0
budget_reserve  = 7000 − 6500         = 500
savings_reserve = 0
stmt_due_first  = 0
card_after_pay  = 4,500  →  max(0, 4500−0)   = 4,500
reserve = 5,000                                     (today's formula: 5,000)
```

C is the convergence case: with income landed and nothing due before month end, the
reformulation reproduces today's number. The new terms change the answer only where timing
is the thing today's proxy cannot see.

## Seam

`banking` gains one value type and one method. It stays a dependency-graph leaf, and the
type carries no provider vocabulary.

```go
// CardStatement is the billing-cycle detail for one credit account: what the last
// closed statement billed and when it must be paid. Known is false when the provider
// reports no statement for the account, so callers can distinguish "no statement" from
// a zero balance.
type CardStatement struct {
    AccountID          string
    Known              bool
    StatementBalance   Money      // meaningful only when Known
    StatementIssueDate *time.Time
    NextPaymentDueDate *time.Time
    MinimumPayment     Money
    IsOverdue          bool
}

// GetCardStatements returns the billing-cycle detail for the credit accounts exposed
// through the given bank login. Accounts the provider reports no statement for come
// back with Known false; accounts that are not credit accounts are not returned.
GetCardStatements(ctx contextx.ContextX, accessToken string) ([]CardStatement, error)
```

The `plaid` client satisfies it from `/liabilities/get`, reading only the `credit` array
and mapping `last_statement_balance`, `last_statement_issue_date`,
`next_payment_due_date`, `minimum_payment_amount`, `is_overdue`. The `student` and
`mortgage` arrays and the `aprs` field are **not decoded**. `fakebank` gains a matching
implementation so the e2e suite can drive every case.

Plaid returns `NO_LIABILITY_ACCOUNTS` / a product-not-supported error for an Item with no
supported credit account. That is a normal empty result, not a failure: it maps to an
empty slice, and every card on that connection takes the missing-statement fallback.

## Persistence

**`accounts`** gains, nullable and defaulted so existing rows are valid:

| column | type | note |
|---|---|---|
| `statement_balance_amount` | REAL | |
| `statement_balance_known` | INTEGER NOT NULL DEFAULT 0 | mirrors `balance_known` |
| `statement_issue_date` | DATE | |
| `next_payment_due_date` | DATE | |
| `minimum_payment_amount` | REAL | |
| `is_overdue` | INTEGER NOT NULL DEFAULT 0 | |

Existing rows start with `statement_balance_known = 0`, which is exactly the fallback case
— so the first sync after the migration is the only thing that changes behaviour, and an
un-synced app degrades conservatively rather than incorrectly.

**`sweep_recommendation`** gains `expected_income`, `mtd_income_received`,
`stmt_due_first`, `card_after_pay`, all `REAL NOT NULL DEFAULT 0`. Older snapshots read
back as zero, which is what they were computed with — the precedent set by
`20260911003115_sweep_card_balance.sql`. The existing `card_balance` column stays: it is
still the summed current balance.

## Sync

`accounts.SyncAccounts` calls `GetCardStatements` once per connection, **only when that
connection holds at least one active credit Account**, inside the existing per-connection
error isolation ([ADR-0021](../../adr/0021-fault-isolating-sync-pass.md)). A statement
fetch that fails leaves the connection's state untouched and the stored statement detail
unchanged, exactly as a balance fetch does; the joined error still reports it. Statement
detail refreshes on the same `last_synced_at` stamp as the balance, so it inherits the
staleness rule rather than introducing a second one.

## Needs-attention

Unchanged from [ADR-0023](../../adr/0023-uncovered-card-debt-reserve.md) except that
nothing new is added to it:

- Checking or savings absent or ambiguous → blocks (savings unknown/stale still does not).
- Checking balance unknown or stale → blocks.
- A card **balance** unknown or stale → blocks. Still true: the balance is still a term.
- A card **statement** missing → **does not block.** The card's full current balance is
  treated as due before income. Over-reserving is the safe direction, and a bank without
  liabilities coverage must not be able to kill the recommendation permanently.
- A missing budget is still not needs-attention. With no budget, `total_spending_budget`
  and `budgeted_income` are zero, so `expected_income` is zero and every card balance
  falls into `card_after_pay` unoffset — the reserve is the full card debt plus the margin.

## Surfaces

- **`/sweep`** — the breakdown gains `Expected income` and splits the card line into
  `Due before income` and `Owed after income`. The snapshot is the source, as always; the
  page computes nothing.
- **`/accounts`** — credit rows gain the statement balance and due date, with the overdue
  flag surfaced when set. A card with no statement detail says so rather than showing
  blanks, since it is the case that changes the sweep.

## Testing

Per [`docs/testing.md`](../../testing.md), unit tests at each layer plus e2e against the
real server with `BANK_PROVIDER=fake`.

- **Arithmetic** — the three worked examples above pinned as table cases, including C as
  the explicit convergence-with-ADR-0023 assertion; the missing-statement fallback; a card
  with a due date exactly on `month_end` (inclusive, so it counts as due first); a
  statement larger than the current balance (a payment posted after statement close),
  which must be capped at the balance so an autopaid statement releases its reserve;
  a statement whose due date the provider omits, taking the fallback rather than the zero
  branch.
- **Derivation** — blocking rules, with the case that a missing statement does *not* block
  standing beside the case that an unknown balance does.
- **Persistence** — round-trip of the new snapshot figures, and an older snapshot reading
  back as zeros.
- **`accounts` sync** — statement fetch skipped for a connection with no credit account;
  a statement fetch failure isolated to its connection and leaving stored detail intact.
- **`plaid` client** — `/liabilities/get` decoding from a testdata fixture; the
  no-liability-accounts response mapping to an empty slice, not an error.
- **e2e** — a statement due before month end raising the reserve, and the same statement
  dated into the next month lowering it. The seeder gains statement fields, defaulting to
  unknown so every existing scenario keeps its current behaviour.
