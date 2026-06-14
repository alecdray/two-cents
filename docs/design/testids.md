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
- `nav-overview` — the navbar's link to the accounts overview (`/`).
- `nav-transactions` — the navbar's link to the transactions page (`/transactions`).

### Transactions (`transactions/adapters/views/`)

- `transactions-page` — the transactions page root and its shared swap region.
- `transactions-list` — the flat list of transaction rows.
- `transactions-row` — one transaction row.
- `transactions-row-merchant` — the row's merchant name.
- `transactions-row-account` — the row's account name.
- `transactions-row-amount` — the row's display-signed amount.
- `transactions-row-pending` — the pending marker, present only on pending rows.
- `transactions-sync` — the "Sync now" control.
- `transactions-sync-error` — the recoverable inline error shown when a sync fails.
- `transactions-empty-no-connections` — the empty state shown when no bank is connected.
- `transactions-empty-no-transactions` — the empty state shown when a bank is connected but nothing is synced yet.

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
