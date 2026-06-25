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
`EditRule`, `DeleteRule`, `ListRules`, and `RulesMatching` (read). The substring
must be non-blank and a spending Rule requires a Category → otherwise a
`ValidationError`. Each mutation, **after** committing its own write, calls the
`ReapplyCategorization` seam with the affected substring(s) (old∪new on edit) and
surfaces the returned "N transactions re-categorized" count.

`RulesMatching(merchant)` reports the Rules that match a merchant, reusing the
engine's cleaning + precedence so it can never disagree with what actually
categorizes: it cleans the merchant the same way, returns every Rule whose
substring matches, and marks the one the ladder would pick (the winner). The
transaction editor calls it to surface "which Rules govern this transaction"
([ADR-0016](../../../docs/adr/0016-rule-editor-modal-and-cross-modal-return.md)).

## UI

- `GET /categories` — active and archived Categories listed separately, with
  create / inline rename / archive / unarchive, htmx fragment swaps, inline
  validation.
- `GET /rules` — the Rules list (read-only rows + a "New rule" opener). Create and
  edit open the shared **rule editor modal** (the second consumer of the modal
  shell, [ADR-0011](../../../docs/adr/0011-reusable-transaction-editing-modal.md));
  delete is a per-row control. A save validates inline in the open modal; on
  success the handler closes the modal (out-of-band) and re-renders the list region
  in place with the re-categorized count, and announces `transaction-changed` so
  transaction views elsewhere self-refresh
  ([ADR-0010](../../../docs/adr/0010-event-driven-cross-region-refresh.md), [ADR-0016](../../../docs/adr/0016-rule-editor-modal-and-cross-modal-return.md)).
- The rule editor modal also opens from the transaction editor, prefilled to
  create a Rule from that transaction or to edit a matching one. When opened that
  way the caller hands it an opaque same-origin **return handle**; on a successful
  save *or on dismissal* the modal echoes the handle back to re-mount the caller's
  modal, refreshed — the module never learns what that origin is
  ([ADR-0016](../../../docs/adr/0016-rule-editor-modal-and-cross-modal-return.md)).
  A non-same-origin handle is rejected. Opened from the Rules page it carries no
  handle and closes natively.

Both pages are linked from the shared navbar.

## Persistence

- `categories` — `id` (stable string PK), `name`, `builtin`, `archived`,
  timestamps. The twelve built-ins are seeded in the migration.
- `rules` — `id`, `merchant_substring`, `classification`
  (`income`/`spending`/`transfer`), nullable `category_id` (FK → `categories.id`,
  set only for spending), timestamps (`updated_at` is the recency tiebreak).
