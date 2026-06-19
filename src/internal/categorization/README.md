# categorization

Owns how a money movement is bucketed: the **classification taxonomy** (the
built-in spending Categories plus user-created custom ones, archive-not-delete),
the user **Rules** (cleaned-merchant substring → outcome), and the pure
`ResolveCategorization` decision engine. It is the *decider* — it returns a
`Decision`, it never writes Transaction rows.

See the domain model: [`docs/domain/README.md`](../../../docs/domain/README.md)
(§Categorization).

## Boundaries

Imports `core/*` and `banking` (for `banking.Category`) only — never the
`transactions` module and never a provider client such as `plaid`. The decision
type (`Decision`) and `Classification` are defined here, the decider, so the
later `transactions → categorization` edge carries no reverse dependency. The
architecture test `TestCategorizationDependencyDirection` fails the build if
either forbidden import appears.

The one cross-domain write — re-categorize matching transactions when a Rule
changes — goes through the injected `ReapplyCategorization` func seam, wired at
the composition root, so this module stays free of any `transactions` import
(the same dependency-inversion pattern as accounts' `BackfillTransactions`).

## The engine (`categorization.go`, pure)

`ResolveCategorization(in) → Decision{Classification, CategoryID}` applies the
precedence ladder, first match wins:

1. **Manual override** present → use it (defensive; callers pre-skip overridden rows).
2. **Rule** whose substring is a case-insensitive substring of the cleaned
   merchant → its outcome. The longest matching substring wins; ties break to the
   most-recently-edited Rule. A spending Rule whose target Category is archived is
   skipped.
3. **Bank category** (Plaid `personal_finance_category` primary): a transfer
   signal (`TRANSFER_IN`/`TRANSFER_OUT`/`LOAN_PAYMENTS`) → Transfer; `INCOME` →
   Income; else the static PFC→Category map → Spending + that built-in Category
   (skipped if archived). A spending-mapped inflow is a refund — negative
   Spending — never re-routed to Income.
4. **Direction fallback**: outflow (amount > 0) → uncategorized Spending; inflow →
   needs-review (never auto-Income).

`CleanMerchantName(merchant, counterparty)` prefers the provider-cleaned merchant
and otherwise normalizes the raw counterparty (uppercase-fold, strip trailing
store numbers / numeric ids, collapse whitespace). Rules match this cleaned value,
never the raw counterparty.

## Categories

The twelve built-in spending Categories are seeded by the migration with stable
string ids (`food_and_drink`, `general_merchandise`, …) that the in-code PFC map
references. Operations: `CreateCategory` (custom), `RenameCategory` (built-in and
custom, id stable), `ArchiveCategory` / `UnarchiveCategory`,
`ListCategories(includeArchived)`. A name must be non-blank and unique
(case-insensitive) → otherwise a `ValidationError`. There is no hard delete:
archiving drops a Category from pickers and future auto-assignment, while existing
rows keep their assignment and revive on un-archive. None of these operations
re-categorize transactions.

## Rules

A Rule maps a merchant substring onto an outcome: a spending Rule carries a
Category; income/transfer Rules carry none. Operations: `CreateRule`,
`EditRule`, `DeleteRule`, `ListRules`. The substring must be non-blank and a
spending Rule requires a Category → otherwise a `ValidationError`. Each mutation,
**after** committing its own write, calls the `ReapplyCategorization` seam with
the affected substring(s) (old∪new on edit) and surfaces the returned
"N transactions re-categorized" count.

## UI

- `GET /categories` — active and archived Categories listed separately, with
  create / inline rename / archive / unarchive, htmx fragment swaps, inline
  validation.
- `GET /rules` — the Rules list with create / inline edit / delete, each
  surfacing the re-categorized count, htmx fragment swaps, inline errors.

Both are linked from the shared navbar.

## Persistence

- `categories` — `id` (stable string PK), `name`, `builtin`, `archived`,
  timestamps. The twelve built-ins are seeded in the migration.
- `rules` — `id`, `merchant_substring`, `classification`
  (`income`/`spending`/`transfer`), nullable `category_id` (FK → `categories.id`,
  set only for spending), timestamps (`updated_at` is the recency tiebreak).
