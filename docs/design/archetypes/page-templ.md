# Page Templ

## Purpose

A page templ renders a complete HTML page in response to a top-level GET — the kind of response a browser address bar produces, or an `hx-boost`-navigated link delivers. The page templ defines a single URL's content and wraps that content in the shared page layout so every page shares chrome, fonts, scripts, and the `<head>`.

## Where it lives

`src/internal/<module>/adapters/views/<surface>_page.templ`. The `_page` suffix names the archetype; the file's stem describes the surface (e.g. `transaction_detail_page.templ`), not what the handler does to produce it. The `views/` sub-package is the dedicated home for `.templ` files within a module's adapters.

## Shape

A page templ exports one top-level templ component whose name matches the file's stem with a `Page` suffix, and which wraps its body in the shared layout primitive:

```templ
templ TransactionDetailPage(props TransactionDetailProps) {
  @templates.PageLayoutComponent(templates.PageLayoutProps{Title: "..."}) {
    // page content
  }
}
```

That wrapper supplies the root chrome — `<!DOCTYPE>`, `<head>`, fonts, HTMX, modal container — and a slot for the page's navbar. The page passes its own navbar component into that slot (or omits it, for pages with no chrome — e.g. login). If a page needs to add to the `<head>`, that capability extends through the layout primitive, not by reaching around it. The page is responsible for the shape of its content container inside the wrapper; widths, scroll behaviour, and layout direction differ per page and belong with the page, not the primitive.

## Import rules

The architecture doc covers adapter imports at a structural level — see [`docs/architecture/archetypes/domain-module.md`](../../architecture/archetypes/domain-module.md). The design-specific addition is that page templs consume `core/templates` primitives (always) and peer-module DTO types (via the `HttpHandler`'s injected `*Service` types, never imported directly inside the templ).

## When the same content is reachable as both a page and a fragment

The fragment is the reusable piece. Factor the inner content into a fragment templ; the page templ renders that fragment inside `PageLayoutComponent`. Same component, two call sites — never two implementations.

## What goes where

| Change | File |
|---|---|
| New page surface | new `<surface>_page.templ` in `adapters/views/` |
| New URL route for the page | `routes.go` |
| The handler that returns the page | `http.go` |
| Layout-level chrome shared by every page | the shared layout primitive in `core/templates/` |
