# Two Cents — Domain Model

The canonical home for the domain **language** and its decomposition into bounded **domains**. A domain owns *behavior* — who is allowed to change a thing — not data; entities are shared (any domain may read them), but exactly one domain writes each field.

Companion docs: cross-cutting data-shape decisions in [`../architecture/data-model.md`](../architecture/data-model.md); architectural rationale in [`../adr/`](../adr/); the v1 feature set in [`../prd.md`](../prd.md). Card notation: `Inputs:`/`Rules:` use dot notation (`Account.kind`, `Transaction.providerId`) to name exactly what a card reads — input notation, not schema. Cross-domain composition is written `Domain.CardName` (e.g. `Categorization.ResolveCategorization`); these are domain-stable references, not service names.

## Entities

The shared nouns. Each is cross-domain — any domain may read it; field ownership is named in the domains below.

| Entity | What it is |
|---|---|
| **Connection** | A linked bank enrollment (one login at one institution); the provider exposes one or more Accounts through it. |
| **Account** | One financial account the user owns (checking, savings, credit card), sourced via a Connection. |
| **Transaction** | A single money movement on one Account, as reported by the bank — the anchor unit of the domain. |
| **Category** | A spending bucket (Groceries, Rent…), from a built-in taxonomy plus user-defined customs; stable id, archive-not-delete. |
| **Rule** | A user mapping from a cleaned-merchant substring to a categorization outcome, applied to future + existing transactions. |
| **Budget** | The user's single rolling plan — income target, savings target, optional per-Category limits — applied to the **current month**. Optional; persists and carries forward, not recreated monthly. |

Relationships: an Account has many Transactions; a Transfer (a Transaction classified Transfer) links two Accounts the user owns — source and destination; a Budget targets one calendar month. Teller / BankProvider is an external boundary, not an entity — it sources Connections/Accounts/Transactions but persists no identity of its own.

## Glossary

The confusable and system-specific terms — disambiguated.

| Term | Meaning |
|---|---|
| **Classification** | One axis of a Transaction: Income / Spending / Transfer — decides whether it counts as income, spending, or neither. |
| **Category (axis)** | The other axis: the spending bucket, meaningful **only** when Classification = Spending. One picker sets both; choosing a Category sets Classification = Spending. |
| **Transfer** | A Transaction moving money between two Accounts the user owns; excluded from both income and spending. *Not* "internal payment" / "move". |
| **Transfer subtype** | Whether a Transfer's outflow leg is a **Savings contribution** (destination counts-as-savings) or a **plain Transfer** (everything else, incl. credit-card payments). Only Savings contribution is counted anywhere; resolved by pairing. |
| **Savings contribution** | The **outflow leg** of a Transfer whose destination is a counts-as-savings Account — how saving is *measured* (movement, not leftover). The matching inflow leg stays a plain Transfer. |
| **Credit-card payment** | A Transfer whose destination is a credit Account — treated as a **plain Transfer**, not Spending and *not* a counted subtype. The card purchases are the real spending (counted once); the payment is assumed principal — we don't split out interest. |
| **Income** | An inflow classified Income (e.g. a paycheck), explicitly *not* a Transfer. A refund/reimbursement is **not** Income — it is negative Spending in its Category. |
| **Net cash** | Σ cash-Account balances (savings included) − Σ credit-Account balances owed. A *position*. |
| **Net income** | Within a wrap: total Income − total Spending (Spending already net of refunds). A *flow*. |
| **kind** | Per-Account axis: `cash` or `credit`; drives the overview. Seeded from the bank type, user-overridable. |
| **counts-as-savings** | Per-Account flag, orthogonal to `kind`; default on for bank-type savings. Marks a Transfer's destination as a Savings contribution. |
| **needs-reconnect** | Connection state surfaced when the provider reports the enrollment must be re-authenticated. |
| **pending / orphaned-pending** | A Transaction not yet posted; orphaned-pending = pending, unseen on a later sync, older than ~5 days → reaped. |
| **manual override** | A user's sticky correction to a Transaction's Classification/Category; survives re-sync and beats any Rule. |
| **precedence** | Categorization order: manual override > Rule > bank-`type` > uncategorized. |
| **"Everything else" (residual)** | income target − Σ(category limits) − savings target; unbudgeted-category and uncategorized Spending both draw from it. |
| **Pace target** | `max(0, remaining) ÷ days-left-inclusive` (weekly = daily × 7); forward spending guidance, Spending only. Derived. |
| **Month wrap** | The end-of-month summary for a calendar month; a Transaction belongs to a month by **transaction date**, not posted date. **Actuals only** — net income, savings, spend-by-Category; budget comparison is the current-month tracker's job, not the wrap's. Derived. |
| **settling / final** | Wrap states: *settling* while any of the month's Transactions is still pending; *final* once all have posted. No separate grace period. Derived. |
| **partial** | A wrap whose month has incomplete coverage — the connect month, or backfilled months at the edge of the provider's history window. Derived. |

A Rule matches the **cleaned counterparty (merchant) name**, never the raw bank `description`.

## Domains

Derived by **mutation-owner** (who may change a thing) and **cohesion of vocabulary**. The four below each own their own tables; most operations write only within their own domain, and the few boundary-crossing writes are tabled in the [ledger](#cross-domain-write-ledger). Each domain section runs **state machines → policies → operations**, per the convention that lifecycle, pure rules, and side-effecting workflows are distinct card types.

---

### Accounts

**Owns (writes):** Connection rows + state; Account rows, balances, `kind`, `counts-as-savings`, hidden state. → `accounts` module.
**Cross-domain:** its `kind`/`counts-as-savings` are *read* by Categorization (transfer subtype), Tracker, Reporting. Exposes overview inputs (total cash, total debt, net cash) via its service. No cross-domain writes.

```
State machine: Connection
Domain:        Accounts
States:        active, needs-reconnect, disconnected
Transitions:
  active          ──[provider reports auth required]──> needs-reconnect
  needs-reconnect ──[user re-authenticates]──────────> active
  active          ──[user disconnects]───────────────> disconnected
  needs-reconnect ──[user disconnects]───────────────> disconnected
Notes: disconnected is terminal and cascades its Accounts to closed. A needs-reconnect
       Connection keeps its Accounts and history; only sync is paused.
```

```
State machine: Account
Domain:        Accounts
States:        active, hidden, closed
Transitions:
  active ──[user hides]───────────────> hidden
  hidden ──[user unhides]─────────────> active
  active ──[connection disconnected]──> closed
  hidden ──[connection disconnected]──> closed
Notes: hidden drops the Account from the overview and pickers but retains its Transactions
       and history. closed is terminal (Connection gone); balances stop refreshing. An
       Account is never hard-deleted — Transactions and historical wraps stay intact.
```

```
Policy:  SeedAccountKind
Domain:  Accounts
Trigger: an Account first appears from the BankProvider (ConnectBank / SyncAccounts)
Inputs:  Account.bankType
Rules:   credit / credit-card type → `credit`; everything else → `cash`
Output:  a default `kind` (user may later override via SetAccountKind)
```

```
Policy:  SeedCountsAsSavings
Domain:  Accounts
Trigger: an Account first appears from the BankProvider
Inputs:  Account.bankType
Rules:   bank-type savings → flag on; otherwise off
Output:  a default `counts-as-savings` flag (user may later toggle)
```

```
Operation: ConnectBank
Domain:    Accounts
Policies:  SeedAccountKind, SeedCountsAsSavings (per discovered Account)
Steps:
  1. Accept the provider enrollment handed off from the Teller connect flow
  2. Create the Connection (state active)
  3. List Accounts via BankProvider; per Account create it, seed kind + counts-as-savings, load initial balance
Side effects: none cross-domain — the initial transaction backfill is triggered by the
              connect-callback orchestrator, not by Accounts (see Sync orchestration)
Output:    the new Connection + its Accounts
```

```
Operation: SyncAccounts
Domain:    Accounts
Policies:  SeedAccountKind, SeedCountsAsSavings (only for Accounts newly appearing)
Steps:
  1. For each active / needs-reconnect Connection, fetch balances + enrollment health via BankProvider
  2. Provider reports auth required → set Connection needs-reconnect; else ensure active
  3. Update each Account's balance and last-synced timestamp
  4. New Account under an existing Connection → create + seed
Side effects: may flag a Connection needs-reconnect (surfaced in UI)
Output:    refreshed balances + connection states
```

```
Operation: SetAccountKind
Domain:    Accounts
Policies:  (none)
Steps:
  1. Set Account.kind to the user's choice (cash / credit)
  2. Mark it user-overridden so future syncs do not reseed
Side effects: shifts overview totals (read-side)
Output:    updated Account
```

```
Operation: ToggleCountsAsSavings
Domain:    Accounts
Policies:  (none)
Steps:
  1. Set Account.counts-as-savings; mark user-overridden
Side effects: changes which Transfers resolve as Savings contributions (read by Categorization + Reporting)
Output:    updated Account
```

```
Operation: ResolveReconnect
Domain:    Accounts
Policies:  (none)
Steps:
  1. User completes provider re-auth
  2. Set Connection active
Side effects: resumes sync — the next sync pass (cron or on-demand) refreshes balances and
              pulls transactions via the orchestrator; Accounts does not call Transactions
Output:    Connection active
```

```
Operation: HideAccount / UnhideAccount
Domain:    Accounts
Policies:  (none)
Steps:
  1. Set Account hidden / active
Side effects: Account enters/leaves overview + pickers; Transactions and history unaffected
Output:    updated Account
```

```
Operation: Disconnect
Domain:    Accounts
Policies:  (none)
Steps:
  1. Set Connection disconnected
  2. Set its Accounts closed; stop balance refresh
  3. Retain all Transactions + history
Side effects: Accounts leave the overview; their Transactions remain in past wraps
Output:    disconnected Connection
```

---

### Transactions

**Owns (writes):** Transaction rows; both axes — Classification + Category — with manual overrides; and a Transfer's destination Account + subtype, carried on the source/outflow leg. → `transactions` module; hosts the sync task. The **only writer of `Transaction.Classification`/`Category`**, even though the *decision* comes from Categorization. (A Transfer is two rows — the source/outflow leg is canonical and carries the subtype; the destination/inflow leg is its excluded mirror.)
**Cross-domain:** *reads* `Categorization.ResolveCategorization` and Accounts' `kind`/`counts-as-savings`; writes only its own rows.

```
State machine: Transaction
Domain:        Transactions
States:        pending, posted, reaped
Transitions:
  ∅       ──[sync sees new pending]──> pending
  ∅       ──[sync sees new posted]───> posted
  pending ──[same id now posted]─────> posted
  pending ──[unseen > ~5 days]───────> reaped
Notes: identity is the stable provider id; pending→posted updates in place (amount/date/
       merchant may shift). reaped removes a pending that never posted and stopped
       appearing (a dropped authorization). A posted Transaction is never reaped. Manual
       overrides survive the pending→posted update.
```

```
Policy:  DedupeKey
Domain:  Transactions
Trigger: each row arriving from a SyncTransactions pull
Inputs:  Transaction.providerId, the set of known providerIds
Rules:   providerId known → update-in-place; unknown → insert. The id is stable across
         pending→posted (rare significant-change cases surface as insert + later reap).
Output:  insert | update, keyed by providerId
```

```
Policy:  PendingReconcileMatch
Domain:  Transactions
Trigger: an update-in-place whose state moved pending → posted
Inputs:  existing Transaction (incl. manualOverride), incoming provider row
Rules:   overwrite bank-sourced fields (amount, dates, merchant, status); preserve any
         manual override on Classification/Category; if not overridden, re-run auto-categorization
Output:  the reconciled Transaction
```

```
Policy:  OrphanedPendingEligibility
Domain:  Transactions
Trigger: end of a SyncTransactions pass
Inputs:  each pending Transaction.lastSeenAt + age, the current pull's providerId set
Rules:   pending AND absent from the latest pull AND older than ~5 days → eligible to reap
Output:  the set of pending Transactions to reap
```

```
Operation: SyncTransactions
Domain:    Transactions
Policies:  DedupeKey, PendingReconcileMatch, OrphanedPendingEligibility;
           Categorization.ResolveCategorization (per new / uncategorized row)
Steps:
  1. Call Accounts.SyncAccounts first (balances + connection health) — see Sync orchestration
  2. For each active Account, pull transactions since the stored cursor via BankProvider
  3. Per row: DedupeKey → insert or update-in-place (PendingReconcileMatch on pending→posted)
  4. Auto-categorize new + still-uncategorized rows via Categorization.ResolveCategorization (overrides untouched)
  5. OrphanedPendingEligibility → reap dropped pendings
  6. Advance each Account's sync cursor + last-synced timestamp
Side effects: balances/overview refreshed (step 1); pending/reaped changes; categorized rows
Output:    counts of inserted / updated / reaped
```

```
Operation: ReCategorize
Domain:    Transactions
Policies:  (uses the user's explicit choice, not ResolveCategorization)
Steps:
  1. Set Classification and/or Category to the user's pick (a spending Category sets
     Classification=Spending; choosing Income/Transfer clears Category)
  2. Mark the Transaction manually-overridden — sticky; future syncs and rules never revert it
Side effects: shifts spend/income aggregates (read by Tracker, Reporting)
Output:    updated Transaction
```

```
Operation: ApplyCategorization
Domain:    Transactions
Policies:  Categorization.ResolveCategorization
Trigger:   invoked by Categorization after a Rule or Category change
Steps:
  1. Select affected, non-overridden Transactions (matching a changed Rule's merchant, etc.)
  2. Re-run ResolveCategorization per row; write the new Classification/Category
  3. Skip any manually-overridden Transaction (override always wins)
Side effects: shifts aggregates; never touches overridden rows
Output:    count re-categorized
```

```
Operation: MarkTransferDestination
Domain:    Transactions
Policies:  (uses the user's explicit choice; ResolveTransferSubtype is the auto path)
Trigger:   user marks/corrects a Transfer whose destination is unconnected or was mis-paired
Steps:
  1. On the source (outflow) leg, set destination Account + subtype (Savings contribution | plain Transfer)
  2. Mark it manually-overridden — sticky; auto-pairing never reverts it
Side effects: a Savings-contribution mark changes SavingsContributed (read by Tracker, Reporting)
Output:    updated Transfer leg
```

---

### Categorization

**Owns (writes):** the Category taxonomy (built-in + custom; archive-not-delete) and Rules. → `categorization` module. **Never writes Transaction rows.**
**Cross-domain:** *reads* Accounts (`kind`/savings flag) for subtype; its `ResolveCategorization` policy is read by Transactions. Its only cross-domain *write* is indirect, via `Transactions.ApplyCategorization`.

```
State machine: Category
Domain:        Categorization
States:        active, archived
Transitions:
  active   ──[user archives]────> archived
  archived ──[user un-archives]─> active
Notes: reversible. archived hides the Category from new budgets and the picker, but past
       Transactions and historical wraps keep it. Stable ids — renaming never transitions
       state. Built-in and custom Categories both archive; no hard delete in v1; merging out of scope.
```

```
Policy:  CleanMerchantName
Domain:  Categorization
Trigger: categorizing a Transaction (sync auto-categorize or ApplyCategorization)
Inputs:  Transaction.counterparty.name (provider-reported)
Rules:   normalize (strip store numbers, trailing ids, casing) → a stable cleaned merchant.
         Rules and display use this — never the raw bank description.
Output:  cleaned merchant name
```

```
Policy:  ResolveCategorization
Domain:  Categorization
Trigger: a Transaction needs a Classification/Category (sync auto-categorize, ApplyCategorization)
Inputs:  Transaction.manualOverride?, cleaned merchant, matching Rules,
         Transaction.bankType / providerCategory, the built-in taxonomy mapping
Rules (precedence, first match wins):
  1. manual override present → use it (callers pre-skip these; defensive)
  2. a Rule whose substring matches the cleaned merchant → the Rule's outcome
       (multiple matches → longest matching substring wins; equal length → most-recently-edited Rule)
  3. bank `type` signals a transfer (transfer, card_payment, …) → Transfer (ADR-0003 layer 1)
  4. bank category maps onto the taxonomy → that Classification + Category
       (an inflow mapping to a spending Category becomes negative Spending — an auto-detected
        refund, no pairing needed; a clear income signal → Income)
  5. nothing derivable from 1–4 → fall back on the amount's direction:
       outflow → Spending, Category uncategorized;  inflow → uncategorized / needs-review (never auto-Income)
Output:  a (Classification, Category?) decision — Category set only when Classification=Spending
Notes:   pure; writes nothing. Direction-prior (5) is the last resort, reached only when 1–4
         derive nothing — it never guesses an inflow into Income. A refund detected via (4) or
         set via ReCategorize is negative Spending, never Income; pairing an inflow to a prior
         same-merchant outflow is a post-v1 gap.
```

```
Policy:  ResolveTransferSubtype
Domain:  Categorization
Trigger: an outflow Transaction classified Transfer (layer 1) needs its destination/subtype (ADR-0003 layer 2)
Inputs:  the outflow (source) leg (amount, date, source Account); candidate inflow legs on other
         connected Accounts; each Account.kind + counts-as-savings
Rules:   pair an inflow leg on another connected Account, exact amount within ±3 days → destination known.
         destination counts-as-savings → source-leg subtype = Savings contribution;
         otherwise (incl. destination kind=credit — a credit-card payment) → plain Transfer, no counted subtype;
         no connected match → destination unknown (cannot count as savings until the user marks it).
         The paired inflow leg stays a plain Transfer — the excluded mirror, never the carrier.
Output:  for the source leg: destination Account (or unknown) + subtype (Savings contribution | plain Transfer)
Notes:   pure read across Accounts; writes nothing. Subtype lives on the source (outflow) leg only,
         so aggregations count it once. Best-effort — user can correct via MarkTransferDestination.
```

```
Operation: CreateCategory / RenameCategory
Domain:    Categorization
Policies:  (none)
Steps:
  1. Create a custom Category, or rename one — rename keeps the stable id
Side effects: none on Transactions (rename is id-stable, no re-categorization needed)
Output:    the Category
```

```
Operation: ArchiveCategory / UnarchiveCategory
Domain:    Categorization
Policies:  (none)
Steps:
  1. Toggle the Category active ⇄ archived
  2. Existing Transactions keep their assignment to an archived Category — history is preserved;
     archive only removes it from new budgets and the picker
Side effects: changes the Category set offered to budgets / picker; no Transaction writes
Output:    the Category
```

```
Operation: CreateRule / EditRule / DeleteRule
Domain:    Categorization
Policies:  Categorization.ResolveCategorization (via the triggered ApplyCategorization)
Steps:
  1. Write / edit / delete the Rule (substring of cleaned merchant → outcome)
  2. Trigger Transactions.ApplyCategorization over non-overridden Transactions whose cleaned merchant
     matches the rule's substring — the union of old + new substring on edit. It re-runs
     ResolveCategorization from scratch (no provenance tracked): a remaining rule wins, else it
     falls to bank-type / direction. Future rows pick the rule up on the next sync.
Side effects: re-categorizes existing matching, non-overridden Transactions (cross-domain, via Transactions)
Output:    the Rule + count affected
```

---

### Budget

**Owns (writes):** the single, persistent Budget **config** — income target, savings target, per-Category limits. Optional; one config, *not* one row per month. → `budget` module. No state machine.
**Cross-domain:** *reads* Categories (to attach limits); its config is read by the current-month Tracker only. No cross-domain writes.

Applies to the **current month**: the live tracker measures this month's actuals against the config. *No rollover* means unspent amounts never accumulate — each new month the limits reset to full against the same config, which **persists and carries forward** until the user edits it. Budgets do **not** apply to past months; history is the actuals-only Monthly wrap.

```
Policy:  ComputeResidual
Domain:  Budget
Trigger: reading or editing the Budget config
Inputs:  Budget.incomeTarget, Σ(Budget.categoryLimits), Budget.savingsTarget
Rules:   residual ("Everything else") = incomeTarget − Σ(categoryLimits) − savingsTarget;
         total spending budget = incomeTarget − savingsTarget
Output:  residual + total spending budget (negative → surfaced as over-allocated)
Notes:   pure; consumed by the current-month Tracker for the "Everything else" line.
```

```
Policy:  BalanceCheck
Domain:  Budget
Trigger: editing the Budget config
Inputs:  Σ(Budget.categoryLimits), Budget.savingsTarget, Budget.incomeTarget
Rules:   Σ(categoryLimits) + savingsTarget > incomeTarget → over-allocated
Output:  balanced | over-allocated (surfaced, not enforced — an unbalanced plan may be saved)
```

```
Operation: SetBudget / EditBudget
Domain:    Budget
Policies:  ComputeResidual, BalanceCheck
Steps:
  1. Set / edit income target, savings target, per-Category limits on the single rolling config
  2. Run BalanceCheck → surface balanced / over-allocated (non-blocking)
  3. Persist — the config carries forward across months; unspent never rolls over
Side effects: none cross-domain; read by the current-month Tracker
Output:    the Budget config
```

## Derived projections (not domains)

These mutate nothing — pure read-models. With no mutation-owner they are **not domains**: they have **no operations and no state machines**, because there is nothing to change and no lifecycle to advance. Their logic is real and mapped below as **derivation cards** — pure `inputs → output`, side-effect-free, the read-side analogue of a policy. Both → `utility` modules (`tracker`, `reporting`): no `Service`, no `repo`, no DB.

**How they get their data.** A projection imports no domain module (utility modules are dependency-graph leaves). A **composing layer** — the `home`/dashboard domain module, which injects the `accounts` / `transactions` / `budget` / `categorization` services — fetches the data and **passes it in** as arguments; the projection returns a view model the composer renders. So "Inputs: Transactions, Budget" below means *computed from* that data, not *fetched by* the projection. The projections own no `adapters/`, so the overview / tracker / wrap **pages** are served by that composing module.

**Shared basis — `MonthAssignment`:** a Transaction belongs to the calendar **month of its transaction date** (not posted date). Both projections bucket by this; spend/income sums are over Spending/Income Transactions in the month, Transfers excluded, refunds counted as negative Spending.

**Time basis.** "Today," "days left in the month," and "the current month" are reckoned in a single **configured app timezone** — a persisted setting, default **EST** — stable across devices and available to background jobs (cron sync, reaper), unlike a per-request browser zone. `MonthAssignment` uses the bank's calendar transaction date as-is; since both the buckets and "today" are calendar-date-based in one zone, no boundary off-by-one opens up.

**Budget is optional, and current-month only.** Budget-relative derivations — `Remaining`, `EverythingElseRemaining`, `PaceTarget`, `OverBudgetFlag`, and the vs-target progress bars — are **Tracker-only and defined only when a Budget config exists**; with none, the tracker shows actuals (spend / income / savings so far) and prompts to set one. Reporting wraps are **always actuals** — net income, savings, spend-by-Category — and never compare against a budget.

### Tracker — current-month, forward-looking  *(budget-relative cards are defined only when a Budget config exists)*

```
Derivation: Remaining
Projection:  Tracker
Trigger:     rendering the current-month tracker
Inputs:      Budget.categoryLimits[c]; Σ net Spending in the month for Category c
Rules:       remaining[c] = limit[c] − netSpend[c]  (netSpend nets refunds)
Output:      remaining amount per budgeted Category (and the total across Categories)
```

```
Derivation: EverythingElseRemaining
Projection:  Tracker
Trigger:     rendering the current-month tracker
Inputs:      Budget.ComputeResidual (the "Everything else" residual);
             Σ Spending drawing on the residual = unbudgeted-Category spend + uncategorized spend
Rules:       everythingElseRemaining = residual − residualSpend
Output:      remaining for the "Everything else" bucket
```

```
Derivation: PaceTarget
Projection:  Tracker
Trigger:     rendering the current-month tracker
Inputs:      a remaining figure (per Category, Everything else, or total); days-left-inclusive (today counts)
Rules:       daily = max(0, remaining) ÷ days-left-inclusive;  weekly = daily × 7
Output:      daily + weekly pace, per Category / Everything else / total
Notes:       Spending only — Income and savings are shown as progress, never as a pace.
```

```
Derivation: IncomeProgress / SavingsProgress
Projection:  Tracker
Trigger:     rendering the current-month tracker
Inputs:      Budget.incomeTarget + Σ Income in month;  Budget.savingsTarget + Σ Savings contributions in month
Rules:       progress = so-far vs target (a ratio + the two figures); savings so-far = Σ Transfers
             resolved as Savings contributions this month
Output:      income progress, savings progress
Notes:       progress toward a target, not a pace.
```

```
Derivation: OverBudgetFlag
Projection:  Tracker
Trigger:     rendering the current-month tracker
Inputs:      netSpend[c], Budget.categoryLimits[c]
Rules:       netSpend[c] > limit[c] → over budget
Output:      per-Category over-budget flag
```

### Reporting / Month wrap — any month, retrospective

```
Derivation: NetIncome
Projection:  Reporting
Trigger:     rendering a month wrap
Inputs:      Σ Income in month; Σ Spending in month (net of refunds)
Rules:       netIncome = totalIncome − totalSpending   (Transfers excluded both sides)
Output:      net income for the month
```

```
Derivation: SavingsContributed
Projection:  Reporting
Trigger:     rendering a month wrap
Inputs:      source-leg Transfers in month with subtype = Savings contribution
Rules:       sum their amounts — source leg only; the mirror inflow leg is a plain Transfer, never counted
Output:      total savings contributed that month  (vs Budget.savingsTarget)
```

```
Derivation: SpendByCategory
Projection:  Reporting
Trigger:     rendering a month wrap, or a period spend report
Inputs:      Spending Transactions in the period (net of refunds) grouped by Category
Rules:       per Category, actual = Σ net spend
Output:      spend-by-Category actuals (same aggregation backs any date-range report) — no vs-budget; wraps are actuals-only
```

```
Derivation: WrapState
Projection:  Reporting
Trigger:     rendering a month wrap
Inputs:      the pending/posted state of every Transaction assigned to the month
Rules:       any still pending → settling;  all posted → final  (no separate grace period)
Output:      settling | final
Notes:       a *derived* state, recomputed each read — not a stored wrap entity, not a state machine.
```

```
Derivation: PartialFlag
Projection:  Reporting
Trigger:     rendering a month wrap
Inputs:      the month vs the connect date and the provider's history window
Rules:       the connect month, or a backfilled month at the edge of the history window (incomplete coverage) → partial
Output:      partial flag (the wrap shows its numbers but marks them possibly incomplete)
```

## External boundary (not a domain)

- **Teller / BankProvider** — the anti-corruption layer behind the `BankProvider` interface ([ADR-0002](../adr/0002-bankprovider-abstraction.md)); the `teller` external-client translates wire shapes into our `Account`/`Transaction` types and is the only code that talks to the bank network. It persists nothing and owns no domain fields — it is *called by* `Accounts.SyncAccounts` and `Transactions.SyncTransactions`.

## Cross-domain write ledger

Every write below crosses a domain boundary and therefore lives in an **operation**, never a policy.

| Write | Owning operation | Why it crosses |
|---|---|---|
| Account balances + Connection state | `Accounts.SyncAccounts` | Data originates in the BankProvider (external), applied by the field's owner. |
| Transaction rows (insert/update/reconcile/reap) | `Transactions.SyncTransactions` | Same sync pass; Transactions owns the rows. |
| `Transaction.Classification`/`Category` (auto) | `Transactions.SyncTransactions` → calls `Categorization.ResolveCategorization` | The decision is Categorization's; the field is Transactions'. |
| `Transaction.Classification`/`Category` (rule/category change) | `Transactions.ApplyCategorization`, triggered by Categorization Create/Edit/Delete | Categorization owns Rules; Transactions owns the field it must update. |

The guiding invariant: **Categorization decides, Transactions writes.** Categorization never writes a Transaction row; Transactions never invents a categorization rule.

## Sync orchestration

**Dependency direction: Transactions → Accounts, one-way.** Transactions needs the account list to pull for; Accounts knows nothing of Transactions. This keeps the module graph acyclic (wax's composition is a strict DAG) — `accounts` is a leaf, `transactions` imports `accounts.Service`. **Accounts operations never call Transactions.**

A full sync writes *both* Accounts (balances, connection state) and Transactions (rows), accounts-first. **Resolved:** `SyncAccounts` is owned by Accounts; the recurring sync (cron in `transactions/task.go`) and any on-demand sync call `Accounts.SyncAccounts` first, then pull/dedupe/reconcile their own rows. Each domain still writes only its own tables.

**Connect and reconnect are orchestrated from the Transactions side, never from Accounts.** The Teller connect-callback handler calls `Accounts.ConnectBank` (persist Connection + Accounts), then `Transactions.SyncTransactions` for the initial backfill. `ResolveReconnect` merely flips the Connection active; the next sync pass catches it up. The orchestrator lives where both services are in scope — transactions' adapter or a thin composition seam — because only the Transactions→Accounts direction may hold both.
