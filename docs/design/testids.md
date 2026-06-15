# Testids

Every templ component's top-level root element carries a `data-testid`. Testids are how Playwright tests, HTMX `hx-target="closest [data-testid='...']"` selectors, and ad-hoc DOM tooling locate elements without depending on Tailwind classes that change with styling.

## Naming

```
data-testid="<component>[-<postfix>]"
```

- **`<component>`** — kebab-case of the templ function name, with the `Frag` suffix dropped if present. `BudgetSummaryFrag` → `budget-summary`. `transactionRow` (private templ) → `transaction-row`.
- **`<postfix>`** — added only when needed to disambiguate. Required when:
  - The single root is rendered by different `if`/`else`/`switch` branches and each branch is a meaningfully different variant (`-spending` vs `-transfer`, `-empty` vs the populated form).
  - The same conceptual component appears with materially different variants (`account-card-cash` vs `account-card-credit`).
- A component with one unambiguous root takes the base name alone — no `-main`, no `-root`, no filler.

The postfix describes the **variant** of the root (which branch, which state) — `-empty`, `-spending`, `-transfer`, `-settling`, `-partial`. It is not the role of a sibling; siblings live under a wrapper (see "One root per component" below).

## Non-root elements

Descendants that need their own testid follow the same pattern, prefixed by the containing component:

```
data-testid="<component>-<role>"
```

`TransactionDetailPage` has a title heading inside it → `transaction-detail-page-title`. A submit button in `BudgetFormFrag` → `budget-form-submit`. The role names what the element does within the component; it is not derived from a separate component name.

When a sub-fragment is composed into **exactly one parent** and exists to serve that parent, "containing component" means the parent: the fragment's testid takes the parent's prefix, not its own. A `CategoryLimitsFrag` that lives only inside `BudgetFormFrag` declares `budget-form-category-limits`, not `category-limits`. The grep-the-codebase rule still applies — a testid's prefix doesn't always point to its declaring file.

## One root per component

A non-OOB templ component renders exactly one top-level root. If a component would emit several always-rendered siblings (a header next to a form, a heading next to a list), wrap them in a `<div>` and let the wrapper carry the testid — that wrapper is also the natural target for HTMX swaps, layout classes, and Alpine scopes, so the constraint pays for itself.

Two narrow exceptions:

- **Pure delegation** — a component that just calls into another templ (`@templates.Modal(...)`, `@templates.ForceCloseModal(...)`) doesn't get a testid; the testid belongs to whatever component actually owns the rendered root.
- **List emitters** — a component that emits a homogeneous list (e.g. a `for` loop of `<li>` items with no enclosing `<ul>`, where the caller supplies the wrapper) doesn't invent a wrapper just to host a testid; each item carries its own testid if needed.

Conditional branches (`if`/`else`/`switch` where exactly one root renders) are not multi-root — they are one root that varies by branch, and each branch gets its own variant postfix.

## Out-of-band swap targets

OOB swap fragments don't define their own HTML — they compose a shared region templ. The testid lives on that region in exactly one place, and is inherited by both the initial render and the OOB swap. See [oob-swaps.md](oob-swaps.md).

## Testids are not runtime selectors

`hx-target`, Alpine `x-ref` lookups, and other runtime selectors target the DOM by `id`, not by `data-testid`. Ids are the source of truth for what a region is named (see the "DOM ids belong to the templ that owns the region" principle); testids are an orthogonal channel for tests and debugging. Coupling runtime behavior to testids couples those two concerns and breaks any test that renames its own selector.

If `hx-target="closest [data-testid='...']"` would be the natural expression, give the target element an `id` (via a helper next to the templ) and use `hx-target="closest #..."` or `hx-target="#..."` instead.

## Registered testids

The grep-the-codebase rule is the source of truth; this list captures the testids that cross module boundaries as the e2e/HTMX contract.

### Primitives (`core/templates/`)

- `app-navbar` — the shared navigation strip, threaded into every page through the layout's navbar slot.
- `nav-home` — the navbar's link to the current-month Tracker home (`/`).
- `nav-accounts` — the navbar's link to the accounts overview (`/accounts`).
- `nav-transactions` — the navbar's link to the transactions page (`/transactions`).
- `nav-budget` — the navbar's link to the budget page (`/budget`).
- `nav-wraps` — the navbar's link to the wraps list (`/wraps`).
- `nav-categories` — the navbar's link to the categories page (`/categories`).
- `nav-rules` — the navbar's link to the rules page (`/rules`).

### Transactions (`transactions/adapters/views/`)

- `transactions-page` — the transactions page root and its shared swap region.
- `transactions-list` — the flat list of transaction rows.
- `transactions-row` — one transaction row.
- `transactions-row-merchant` — the row's merchant name.
- `transactions-row-account` — the row's account name.
- `transactions-row-amount` — the row's display-signed amount.
- `transactions-row-pending` — the pending marker, present only on pending rows.
- `txn-classification` — the row's classification chip (income/spending/transfer/needs-review).
- `txn-category-chip` — the row's assigned Category chip, present only when it carries a Category.
- `txn-needs-review` — the needs-review flag, present only on needs-review rows.
- `txn-categorize` — the per-row re-categorize picker form.
- `txn-categorize-classification` — the picker's outcome select.
- `txn-categorize-category` — the picker's Category select, revealed only for a Spending outcome.
- `txn-categorize-error` — the inline picker error (a Spending choice with no Category).
- `txn-transfer-destination` — the resolved transfer-destination chip on a Transfer row (savings contribution or plain transfer); present only when the destination is known/resolved.
- `txn-destination-unknown` — the flagged chip on an outflow Transfer whose destination is still unresolved and unmarked (the branch alternative to `txn-transfer-destination`).
- `txn-mark-destination` — the per-outflow-Transfer control that opens the mark/correct picker.
- `txn-destination-picker` — the transfer-destination picker form (destination account + subtype), present only on outflow Transfer rows.
- `txn-destination-picker-account` / `txn-destination-picker-subtype` — the picker's destination-account and subtype selects.
- `txn-destination-picker-submit` — the picker's submit control.
- `txn-destination-picker-error` — the inline picker error (not an outflow transfer, or an invalid subtype).
- `txn-destination-option-<accountId>` — one connected-account option in the destination select, keyed by account id.
- `transactions-sync` — the "Sync now" control.
- `transactions-sync-error` — the recoverable inline error shown when a sync fails.
- `transactions-empty-no-connections` — the empty state shown when no bank is connected.
- `transactions-empty-no-transactions` — the empty state shown when a bank is connected but nothing is synced yet.

### Categorization (`categorization/adapters/views/`)

- `categories-page` — the categories page root and its shared swap region.
- `categories-active` — the active-categories group.
- `categories-archived` — the archived-categories group (present only when any are archived).
- `category-create` — the new-custom-category form.
- `category-row` — one category row.
- `category-rename` — the inline rename submit on a category row.
- `category-archive` — the archive control on an active category row.
- `category-unarchive` — the restore control on an archived category row.
- `category-create-error` / `category-row-error` — the inline validation errors.
- `rules-page` — the rules page root and its shared swap region.
- `rules-list` — the list of rule rows.
- `rules-empty` — the empty state shown when no rules exist.
- `rules-feedback` — the "N transactions re-categorized" feedback after a rule mutation.
- `rule-create` — the new-rule form.
- `rule-row` — one rule row.
- `rule-edit` — the inline edit form on a rule row.
- `rule-delete` — the delete control on a rule row.
- `rule-create-error` / `rule-row-error` — the inline validation errors.

### Budget (`budget/adapters/views/`)

- `budget-page` — the budget editor page root and its shared swap region.
- `budget-income` — the monthly income target input.
- `budget-savings` — the monthly savings target input.
- `budget-limit-row` — one active-Category spending-limit row (its name + limit input).
- `budget-residual` — the computed "everything else" residual line.
- `budget-balance-banner` — the balanced / over-allocated verdict banner (text distinguishes the two).
- `budget-save` — the save control.
- `budget-error` — the inline validation error shown on a malformed amount.

### Home / dashboard (`home/adapters/views/`)

- `tracker-page` — the current-month Tracker page root (the application landing page at `/`).
- `tracker-needs-budget` — the actuals-only prompt to create a budget, shown when no budget is set.
- `tracker-category-row` — one budgeted-Category standing (name, remaining, pace).
- `tracker-over-budget` — the over-budget chip on a Category row, present only when net spend exceeds its limit.
- `tracker-everything-else` — the "everything else" residual remaining line.
- `tracker-total` — the total-remaining card (with the overall pace).
- `tracker-pace-daily` / `tracker-pace-weekly` — the daily and weekly pace within the total card.
- `tracker-income-progress` — the income-toward-target progress card.
- `tracker-savings-progress` — the savings-toward-target progress card.
- `wraps-page` — the wraps-list page root (`/wraps`).
- `wrap-row` — one month in the wraps list, linking to its wrap.
- `wrap-page` — a single month-wrap page root (`/wraps/{ym}`).
- `wrap-net-income` — the wrap's net-income line.
- `wrap-savings` — the wrap's savings-contributed line.
- `wrap-category-row` — one Category's net spend in the wrap's spend-by-Category table.
- `wrap-state` — the settling/final state badge (text distinguishes the two).
- `wrap-partial` — the partial badge, present only when the month sits at/before the backfill edge.

## Examples

```templ
// Single root, no postfix needed
templ BudgetSummaryFrag(budget budget.BudgetDTO) {
  <div data-testid="budget-summary" class="...">
    ...
  </div>
}

// Multiple roots via branching — each branch gets a postfix
templ TransactionClassificationFrag(txn transactions.TransactionDTO) {
  if txn.Classification == transactions.Spending {
    <div data-testid="transaction-classification-spending" class="...">...</div>
  } else {
    <div data-testid="transaction-classification-transfer" class="...">...</div>
  }
}

// Descendants carry the component prefix
templ BudgetFormFrag(...) {
  <form data-testid="budget-form" ...>
    ...
    <button data-testid="budget-form-submit" type="submit">Save</button>
  </form>
}
```
