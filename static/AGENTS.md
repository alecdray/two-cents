# static/ — frontend assets (singleton)

The frontend asset pipeline.

- `src/` — Tailwind sources. Today: `main.css` (the `twocents` theme + named utilities).
- `public/` — files served at `/static/*`: Tailwind's compiled `main.css`, vendored third-party assets (HTMX, Bootstrap Icons + font), and brand assets (favicon, manifest).

`static/src/main.css` is the **source of truth** for the theme and for all app-specific utilities. Don't add per-page/per-component styling here — templ files use Tailwind utilities directly. Conceptual token roles live in [`docs/design/design-system.md`](../docs/design/design-system.md).

## What's defined in main.css

- The `[data-theme="twocents"]` token block (base / primary / secondary / accent / neutral / info / success / warning / error, each with a `-content` pair) + corner radii.
- The categorical palette (`@theme` block) — identity hues for category avatars, distinct from the status tokens; roles in [`docs/design/design-system.md`](../docs/design/design-system.md).
- The element-state and text-emphasis utilities whose roles are catalogued in [`docs/design/design-system.md`](../docs/design/design-system.md) — `main.css` is their definition (truth).
- `.font-brand` (Instrument Sans).

## Brand icon

`favicon.svg` is the single source mark — a shaded ¢ "coin" (radial-gradient sphere + drop shadow + edge sheen, with the glyph raised via a bevel highlight and its own shadow) on a transparent background. Its `<text>` `x`/`y` are tuned to the glyph's ink bounding-box center, not the font baseline/metrics (which render the ¢ low and right) — re-measure and retune if the glyph, font, or size changes. It is served directly as the favicon; the home-screen PNGs (`apple-touch-icon.png` for iOS, `icon-{192,512}.png` for the PWA manifest) are rendered from it by `task build/icons`, which composites the dark square iOS masks. Edit `favicon.svg`, rerun the task, and commit the PNGs — the Docker build copies `public/` as-is and has no `rsvg-convert`.

## Bootstrap Icons (vendored)

`bootstrap-icons.css` + `fonts/bootstrap-icons.woff2` are loaded by `core/templates/root.templ`. Emit icons as `<i class="bi bi-{name}"></i>`. Catalog: https://icons.getbootstrap.com/.

## After editing

Run `task build/tailwind` to regenerate `static/public/main.css`.
