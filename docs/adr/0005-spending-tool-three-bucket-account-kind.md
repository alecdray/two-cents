# Two Cents is a spending tool — three-bucket account kind

Two Cents is a **spending / cash-flow tool**, not a net-worth tracker. Its job is to make where-the-money-goes legible: spendable balances, card debt, and the flows in between. It deliberately does **not** try to be a complete picture of household wealth. This decision records that framing and the account model that follows from it.

Every Account carries a **kind** with one of three values — `cash`, `credit`, or `other`:

- **`cash`** — depository accounts (checking, savings, CD, money market, cash management, and depository-type HSAs). These hold spendable money.
- **`credit`** — credit cards. These are card debt.
- **`other`** — loans, mortgage, and investment / retirement / brokerage accounts (including investment-type HSAs). These are pulled and listed so the user can see them, but they are **excluded from net cash**.

The kind is seeded from the provider's reported account type — depository → `cash`, credit → `credit`, everything else → `other` — and the user may override it. **Net cash = Σ cash balances (savings included) − Σ credit balances owed**, excluding `other` accounts (alongside the existing exclusions for hidden / closed accounts and any with an unknown balance).

**Why a third bucket instead of the prior `cash | credit` split.** The original model assumed every connected account was either spendable cash or card debt. But a single bank login routinely surfaces a 401(k), a brokerage account, a car loan, or a mortgage. Forcing those into `cash` would inflate net cash with money that can't be spent (and, for loans, would treat a debt as an asset); forcing them into `credit` would misrepresent a retirement balance as a card balance. Either way the headline number — the one the whole overview exists to show — would lie. A third bucket lets us **store and display these accounts honestly without letting them distort the spending view**.

**Why `other` is excluded from net cash rather than dropped.** Because this is a spending tool, the overview's central figure must answer "how much spendable money do I have, net of what I owe on cards?" An investment or loan balance answers a different question (net worth), so it must not move that figure. But silently discarding the accounts would be worse than useless: the user connected a bank and expects to see everything that came with it. So `other` accounts are kept — tracked, listed, balance-refreshed — and simply held out of the net-cash math. This keeps net cash a clean cash-flow position while still acknowledging the full set of linked accounts.

Consequences: the overview's net-cash sum walks only `cash` and `credit` accounts; `other` is presented separately and never folded in. Seeding gains a default rule for the long tail of non-depository, non-credit account types. Because kind is user-overridable, a user who *does* think of a particular account as spendable can move it into `cash` — the default just keeps net cash honest out of the box. We pull only balances for `other` accounts; holdings-level investment data and per-loan liability detail remain post-v1 (see [scope](../scope.md)).

Rejected: keeping the `cash | credit` binary and dropping every other account (loses accounts the user can plainly see in their bank and expects represented). Rejected: folding investments/loans into net cash to show a net-worth-style total (turns a spending tool into a net-worth tracker and makes the headline figure answer the wrong question). Rejected: a per-account boolean "count in net cash?" instead of a typed kind (a flag carries no vocabulary about *why* an account is excluded, and the three named buckets map cleanly onto how provider account types already partition).
