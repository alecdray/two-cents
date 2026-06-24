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

- **Pure delegation** — a component that just calls into another templ (e.g. a module's modal fragment that only invokes `@templates.Modal(...)`) doesn't get a testid; the testid belongs to whatever component actually owns the rendered root.
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

- `app-navbar` — the shared fixed bottom navigation bar, threaded into every page through the layout's navbar slot.
- `nav-home` — the navbar's link to the current-month Tracker home (`/`).
- `nav-accounts` — the navbar's link to the accounts overview (`/accounts`).
- `nav-transactions` — the navbar's link to the transactions page (`/transactions`).
- `nav-budget` — the navbar's link to the budget page (`/budget`).
- `nav-wraps` — the navbar's link to the wraps list (`/wraps`).
- `nav-categories` — the navbar's link to the categories page (`/categories`).
- `nav-rules` — the navbar's link to the rules page (`/rules`).
- `nav-more` — the bar's overflow control; opens the More sheet holding the secondary destinations and sign-out.
- `more-sheet` — the navbar's overflow `<dialog>`, opened from `nav-more`; contains `nav-wraps`, `nav-categories`, `nav-rules`, and `nav-logout`.
- `nav-logout` — the sign-out control inside the More sheet (a plain, non-boosted navigation to `/logout`).
- `request-progress-bar` — the app-wide pending indicator: a thin top bar shown while any HTMX request is in flight, mounted once in the shared layout ([ADR-0015](../adr/0015-app-wide-request-feedback.md)).
- `modal-container` — the one per-page mount point a modal swaps into out-of-band.
- `modal` — the modal shell's open `<dialog>`; its close control is the role=button labelled "Close".

### Transactions (`transactions/adapters/views/`)

- `transactions-page` — the transactions page root and its shared swap region.
- `transactions-list` — the flat list of transaction rows.
- `transactions-row` — one transaction row; the whole row is the click target that opens the shared editing modal.
- `transactions-row-merchant` — the row's merchant name.
- `transactions-row-account` — the row's account name.
- `transactions-row-amount` — the row's display-signed amount.
- `transactions-row-pending` — the pending marker, present only on pending rows.
- `txn-classification` — the row's classification chip (income/spending/transfer/needs-review).
- `txn-category-chip` — the row's assigned Category chip, present only when it carries a Category.
- `txn-needs-review` — the needs-review flag, present only on needs-review rows.
- `txn-transfer-destination` — the resolved transfer-destination chip on a Transfer row (savings contribution or plain transfer); present only when the destination is known/resolved.
- `txn-destination-unknown` — the flagged chip on an outflow Transfer whose destination is still unresolved and unmarked (the branch alternative to `txn-transfer-destination`).
- `transactions-refresh-listener` — the hidden element that re-fetches the list region on `transaction-changed` (carries the active search + view).
- `transactions-sync` — the "Sync now" control.
- `transactions-sync-error` — the recoverable inline error shown when a sync fails.
- `transactions-empty-no-connections` — the empty state shown when no bank is connected.
- `transactions-empty-no-transactions` — the empty state shown when a bank is connected but nothing is synced yet.

The transaction-editing modal body, served into the shared shell by the edit endpoint and reused by every surface that lists transactions ([ADR-0011](../adr/0011-reusable-transaction-editing-modal.md)):

- `transaction-editor` — the editor body region (the swap target the Save re-renders in place).
- `transaction-editor-merchant` — the editor header's merchant name (which transaction is being edited).
- `transaction-editor-counterparty` — the bank's counterparty, shown only when it differs from the merchant.
- `transaction-editor-source` — the Auto / Manual badge (whether the categorization is the auto guess or a sticky override).
- `transaction-editor-bank-category` — the raw bank category line (the signal behind auto-categorization), shown only when the bank supplied one.
- `txn-edit` — the editor's single form (classification/Category plus, for an outflow row, the transfer controls).
- `txn-edit-submit` — the editor's single Save control (issues both writes in turn).
- `txn-categorize-classification` — the outcome select.
- `txn-categorize-category` — the Category select, revealed only for a Spending outcome.
- `txn-categorize-error` — the inline coupling error (a Spending choice with no Category).
- `txn-destination-picker` — the transfer-destination controls (destination account + subtype); rendered for an outflow row, shown only when the outcome is Transfer.
- `txn-destination-picker-account` / `txn-destination-picker-subtype` — the destination-account and subtype selects.
- `txn-destination-picker-error` — the inline transfer error (not an outflow transfer, or an invalid subtype).
- `txn-destination-option-<accountId>` — one connected-account option in the destination select, keyed by account id.

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
- `budget-limit-row` — one shown Category spending-limit row (its name + limit input). Only Categories with a limit show on load; the rest are added via the add-category control.
- `budget-add-category` — the select that adds an editable limit row for a Category not currently shown (present only while any unshown Category remains).
- `budget-remove-limit` — the per-row control that hides a limit row and clears its limit (returning the Category to unbudgeted).
- `budget-residual` — the computed "everything else" residual line (recomputed live on the client as income, savings, or any limit changes).
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
- `tracker-income-progress` — the income-toward-target progress card; drills into the current month's income.
- `tracker-savings-progress` — the savings-toward-target progress card; drills into the current month's savings contributions.
- `wraps-page` — the wraps-list page root (`/wraps`).
- `wrap-row` — one month in the wraps list, linking to its wrap.
- `wrap-page` — a single month-wrap page root (`/wraps/{ym}`).
- `wrap-figure-region` — the wrap's self-refreshing region (every figure + the full-month list); rendered on load and returned for the `transaction-changed` self-refresh.
- `wrap-net-income` — the wrap's net-income line (a derived summary; not a drill).
- `wrap-income` — the wrap's gross-income figure; links into the income drill-down.
- `wrap-savings` — the wrap's savings-contributed figure; links into the savings drill-down.
- `wrap-category-row` — one Category's net spend in the wrap's spend-by-Category table; links into the spend drill-down.
- `wrap-month-list` — the inline full-month transaction list (present only when the month has transactions).
- `wrap-month-list-empty` — the empty state shown when the month has no transactions.
- `wrap-month-row` — one row of the full-month list; the whole row is the click target that opens the shared editing modal.
- `wrap-month-row-merchant` / `wrap-month-row-amount` — the row's merchant and ledger-signed amount (inflow positive, outflow negative).
- `wrap-figures-refresh-listener` — the hidden element that re-fetches the wrap figure region on `transaction-changed`.
- `wrap-state` — the settling/final state badge (text distinguishes the two).
- `wrap-partial` — the partial badge, present only when the month sits at/before the backfill edge.
- `spend-drill-page` — the spend drill-down page root (`/wraps/{ym}/spend/{bucket}`).
- `spend-drill-region` — the drill's self-refreshing region (label, total, list); rendered on load and returned for the `transaction-changed` self-refresh.
- `spend-drill-back` — the back-link to the month's wrap.
- `spend-drill-label` — the bucket label (Category name, "Uncategorized", or "Everything else").
- `spend-drill-total` — the bucket's net total, the figure the listed rows sum to.
- `spend-drill-list` — the list of drilled transaction rows (present only when the bucket is non-empty).
- `spend-drill-empty` — the empty state shown when the bucket has no transactions this month.
- `spend-drill-row` — one drilled transaction row; the whole row is the click target that opens the shared editing modal.
- `spend-drill-row-merchant` — the row's merchant name.
- `spend-drill-row-amount` — the row's net-signed amount (wrap convention: spending positive).
- `spend-drill-row-pending` — the pending marker, present only on pending rows.
- `spend-drill-row-category` — the row's Category chip ("Uncategorized" when it carries none).
- `spend-drill-refresh-listener` — the hidden element that re-fetches the drill region on `transaction-changed` (re-query + re-sum).

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
