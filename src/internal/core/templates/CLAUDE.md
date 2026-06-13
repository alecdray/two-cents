# core/templates — UI primitives (singleton)

Home of UI **primitives** — reusable, domain-free visual building blocks used by 2+ modules. Full rules: [`docs/design/archetypes/primitive.md`](../../../../docs/design/archetypes/primitive.md).

## Rules

- Files here are primitives: domain-free, parameterized by plain values, consumed by any number of pages and fragments.
- Do **not** import any domain module. A primitive that needs a domain type isn't a primitive — it belongs in the owning module's `adapters/`.
- Do **not** import `core/db` query code.
- `RootComponent` / `PageLayoutComponent` are loaded by every page templ. Anything that should appear on every page — chrome, fonts, scripts — lives here and is pulled in through the layout, not duplicated in pages.

## The Icon primitive

`icons.templ` defines the single `Icon` primitive wrapping Bootstrap Icons. Pass a BI catalog name (without the `bi-` prefix) and an optional `IconStyle` (Outline | Fill). Sizing comes from the parent's `text-{size}`; color from the parent's text color. The CSS + font are vendored under `static/public/`; catalog at https://icons.getbootstrap.com/. Emit icons through this primitive, not raw `<i class="bi ...">`.

## After editing

Run `task build/templ` after modifying any `.templ` file.
