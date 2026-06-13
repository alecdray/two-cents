# Two Cents

A personal finance app that pulls a user's own bank transactions and balances (via Teller) to make spending legible: aggregation, budgeting, and month tracking.

## Language

**Connection**:
A linked bank enrollment (one login at one institution) through which the bank provider exposes one or more Accounts. Carries a state including **needs-reconnect**, surfaced when the provider reports the enrollment must be re-authenticated.

**Account**:
One financial account owned by the user (e.g. checking, savings, a credit card), connected via the bank provider. Has a **kind** — `cash` or `credit` — seeded from the bank's type but user-overridable, which drives the overview (cash adds to total cash, credit to total debt). Separately carries a **counts-as-savings** flag (default on for bank-type savings); a Transfer into a savings-flagged Account is a Savings contribution. Investment/brokerage accounts are out of scope for v1.

**Net cash**:
Total balance across all `cash` Accounts (savings included — savings balances are spendable assets) minus total balance owed across all `credit` Accounts.

**Transaction**:
A single money movement on one Account, as reported by the bank. Carries two independent fields: a **Classification** and (only when classified as Spending) a **Category**.

**Classification**:
What a Transaction *is*: **Income**, **Spending**, or **Transfer**. Decides whether it counts toward income, spending, or neither. The user can switch any Transaction to Income, Transfer, or a spending Category; choosing a spending Category sets Classification to Spending.

**Income**:
An inflow Transaction classified as Income (e.g. a paycheck) — explicitly **not** a Transfer (savings/checking moves are Transfers, not Income). A **refund or reimbursement** is **not** Income: it is recorded as **negative Spending** in its Category, so spend-by-category stays truthful. Net income in a wrap = total Income − total Spending (Spending already net of refunds).

**Category**:
The spending bucket of a Transaction (e.g. Groceries, Dining, Rent). Only meaningful when Classification is Spending. Drawn from a curated **built-in taxonomy** (Teller's raw categories are mapped onto it to seed auto-assignment) plus user-defined **custom categories** alongside. Has a stable id: renaming is free. Deleting **archives** (hidden from new budgets and the picker, but past Transactions and historical wraps keep it intact) rather than destroying. Merging categories is out of scope for v1.

**Transfer**:
A Transaction that moves money between two Accounts the user owns. It is **neither Income nor Spending** and is excluded from both aggregations. Detected in two layers: (1) **classification** from the bank-provided transaction `type` (e.g. `transfer`, `card_payment`) on a single Transaction, no pairing needed; (2) **destination/subtype** resolved by pairing the inflow leg on another connected Account (exact amount, ±3 days). If the destination Account isn't connected, the Transfer's destination stays unknown (and it cannot count as a Savings contribution) until the user marks it. The user can correct any Transaction's classification manually as the fallback.
_Avoid_: "internal payment", "move"

**Savings contribution**:
A Transfer whose destination is a designated savings Account. This is how savings is measured — money actually moved into savings.

**Credit-card payment**:
A Transfer whose destination is a credit Account. Excluded from Spending; the original purchases on the card are the real Spending, counted once.

**Rule**:
A user-defined mapping from a **substring of the cleaned counterparty (merchant) name** to a full categorization outcome — a Classification, plus a Category when that outcome is Spending. Applies to future *and* existing Transactions, except any the user has manually overridden (those always win). Lets the user teach the app once (e.g. "Amazon → Shopping", "ACME PAYROLL → Income").
_Avoid_: matching on raw `description`

**Budget**:
A user's plan for one calendar month: an **income target**, a **savings target**, and optional per-**Category** spending limits. Resets each month; no rollover.

**Everything else** (residual budget):
The spending budget not allocated to any Category limit, computed as `income target − sum(category limits) − savings target`. Unbudgeted-category spending and uncategorized Spending both draw from it. Total spending budget for the month = income target − savings target.

**Month wrap**:
The end-of-month summary for a calendar month: net income, savings contributed, and spending by Category, each vs. budget. A Transaction belongs to a month by its **transaction date** (not posted date). A wrap is **settling** for as long as the month still contains any pending Transaction, and becomes final once all have posted — there is no separate grace period. A wrap is **partial** when its month has incomplete coverage (the connect month, or backfilled months at the edge of the provider's history window); it shows its numbers but flags them as possibly incomplete. Past wraps are navigable historically.

**Pace target**:
The sustainable spending rate for the rest of the current month — remaining budget spread evenly over the days left (today included), surfaced per Category, for Everything else, and as a total, as a daily and weekly figure. Forward-looking guidance; applies to Spending only. Income and savings are shown as progress toward target, not as a pace.

## Relationships

- An **Account** has many **Transactions**
- A **Transfer** links exactly two **Accounts** (source and destination), both owned by the user
- A **Savings contribution** and a **Credit-card payment** are each a kind of **Transfer**

## Flagged ambiguities

- "spending" must exclude **Transfers** — paying a credit card or moving money to savings is not spending. The real Spending is the originating purchase, counted once.
