# Fragment Templ

## Purpose

A fragment templ renders a piece of HTML that swaps into an already-loaded page via HTMX. It has no `<head>`, no layout wrapper, and no surrounding chrome — only the content that replaces (or augments) the targeted DOM region.

## Where it lives

`src/internal/<module>/adapters/views/<region>_frag.templ`, alongside page templs in the same `views/` sub-package. The `_frag` suffix names the archetype; the file's stem describes the swappable region (e.g. `transaction_classification_frag.templ`), not the HTMX verb that triggers it.

## Shape

A fragment templ exports one or more top-level templ components whose names match the file's stem with a `Frag` suffix, and which emit HTML without a layout wrapper:

```templ
templ TransactionClassificationFrag(txnID string, classification transactions.Classification) {
  <div id={ transactionClassificationID(txnID) }>
    // ...
  </div>
}
```

Helper templs that are only used internally by the exported fragment (sub-pieces composed inside it) do not need the suffix — they aren't archetype-level entrypoints.

The fragment is responsible for any wrapper element that HTMX targets (the `id` HTMX looks up), and for any data attributes that the swapped-in element needs to function (`x-data`, `hx-trigger` on child elements, etc.).

## Self-containment

After the swap, the browser has no chance to re-run page-init code that lived in the original document. Any client-side hook the fragment relies on (Alpine `x-data`, HTMX attributes, event listeners) must be inside the fragment itself, on the element being swapped in or its descendants.

## Out-of-band swaps

When a single HTMX response needs to update multiple DOM regions (e.g. a re-categorized transaction row plus the spend-by-category total it feeds), the fragment templ emits sibling elements marked with `hx-swap-oob`. Keep OOB siblings in the same templ as their primary swap so the relationship is visible in one place. See [`../oob-swaps.md`](../oob-swaps.md).

## DOM ids belong to the templ that owns the region

If a fragment defines a region that HTMX targets by id, the id-generating helper lives in the same templ file (or its `.go` sibling). Callers obtain the id by calling that helper, not by hard-coding the string — the templ stays the single source of truth for what its swap target is named.

## Import rules

Same as page templ — see [`page-templ.md`](page-templ.md). The structural rules for adapter imports live in [`docs/architecture/archetypes/domain-module.md`](../../architecture/archetypes/domain-module.md).

## When a fragment becomes large enough to be a page

If a fragment grows to the point where it's reasonable to navigate to it directly (its own URL, its own `<title>`), wrap it in a page templ. The fragment stays the inner content; the page templ adds the layout. The fragment does not absorb layout responsibilities.
