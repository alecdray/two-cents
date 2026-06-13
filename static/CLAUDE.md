# static/ — frontend assets (singleton)

The frontend asset pipeline.

- `src/` — Tailwind sources. Today: `main.css` (the `twocents` theme + named utilities).
- `public/` — files served at `/static/*`: Tailwind's compiled `main.css`, vendored third-party assets (HTMX, Bootstrap Icons + font), and brand assets (favicon, manifest).

`static/src/main.css` is the **source of truth** for the theme and for all app-specific utilities. Don't add per-page/per-component styling here — templ files use Tailwind utilities directly. Conceptual token roles live in [`docs/design/design-system.md`](../docs/design/design-system.md).

## What's defined in main.css

- The `[data-theme="twocents"]` token block (base / primary / secondary / accent / neutral / info / success / warning / error, each with a `-content` pair) + corner radii.
- Element-state utilities: `.is-disabled`, `.hover-fade-out`, `.hover-fade-in`.
- Text-emphasis utilities: `text-default`, `text-muted`, `text-subtle`, `text-ghost`.
- `.font-brand` (Instrument Sans).

## Bootstrap Icons (vendored)

`bootstrap-icons.css` + `fonts/bootstrap-icons.woff2` are loaded by `core/templates/root.templ`. Emit icons as `<i class="bi bi-{name}"></i>`. Catalog: https://icons.getbootstrap.com/.

## After editing

Run `task build/tailwind` to regenerate `static/public/main.css`.
