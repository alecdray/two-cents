# core/templates — UI primitives (singleton)

Home of UI **primitives** — reusable, domain-free visual building blocks used by 2+ modules. Full rules: [`docs/design/archetypes/primitive.md`](../../../../docs/design/archetypes/primitive.md).

## Rules

- Files here are primitives: domain-free, parameterized by plain values, consumed by any number of pages and fragments.
- Do **not** import any domain module. A primitive that needs a domain type isn't a primitive — it belongs in the owning module's `adapters/`.
- Do **not** import `core/db` query code.
- `RootComponent` / `PageLayoutComponent` are loaded by every page templ. Anything that should appear on every page — chrome, fonts, scripts — lives here and is pulled in through the layout, not duplicated in pages.

## The Icon primitive

`icons.templ` defines the single `Icon` primitive wrapping Bootstrap Icons. Pass a BI catalog name (without the `bi-` prefix) and an optional `IconStyle` (Outline | Fill). Sizing comes from the parent's `text-{size}`; color from the parent's text color. The CSS + font are vendored under `static/public/`; catalog at https://icons.getbootstrap.com/. Emit icons through this primitive, not raw `<i class="bi ...">`.

## The Modal primitive

A domain-free dialog **shell**: a `<dialog class="modal modal-bottom sm:modal-middle">` with a `modal-box` body slot and a close control, mounted into a fixed-id container the layout renders once per page. A view opens it by `hx-get`-ing fragment content whose root is this shell — the shell's container swaps into that mount point out-of-band, so the opening control never owns a target. It knows nothing of any module ([ADR-0011](../../../../docs/adr/0011-reusable-transaction-editing-modal.md) loads the transaction editor this way). The mobile-bottom / desktop-middle anchoring follows the design [principles](../../../../docs/design/principles.md). The shell is the primitive; any domain-specific modal *content* lives in the owning module's `adapters/`, never here.

## The AppNavbar primitive

`navbar.templ` is the app's primary navigation: a fixed bottom bar of the primary destinations plus a client-only "More" overflow sheet ([ADR-0014](../../../../docs/adr/0014-bottom-bar-navigation.md)). It stays domain-free — each authenticated page passes its `NavTab` (the navbar's own destination enum, not a domain type) so the bar marks the active slot, and overflow destinations highlight the More control. The sheet is a native `<dialog>` opened client-side via Alpine `x-ref`, not the HTMX Modal primitive — its links are static navigation with no server round-trip.

## The RequestProgressBar primitive

`request_progress.templ` is the app-wide pending indicator ([ADR-0015](../../../../docs/adr/0015-app-wide-request-feedback.md)): a thin, indeterminate bar pinned to the top of the viewport, shown while at least one HTMX request is in flight (and held briefly after the last settles, so even a sub-millisecond local response still reads as a sweep). It owns no state of its own — a small driver script in `root.templ`'s head maintains an in-flight request counter (incremented on `htmx:beforeSend`, decremented on each request's own XHR `loadend`, which survives the boosted-body morph) and toggles `data-request-pending` on the document while any request is outstanding (enforcing a brief minimum visible window); the bar's visibility (and its slide keyframe) is defined in `static/src/main.css`. Mounted once in the root layout, it covers every interaction — boosted navigation, form posts, fragment and modal loads — with no per-element wiring.

## After editing

Run `task build/templ` after modifying any `.templ` file.
