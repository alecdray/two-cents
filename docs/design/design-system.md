# Design System

The visual vocabulary used across pages, fragments, and primitives. This doc is the **conceptual** layer over `static/src/main.css` — it describes the categories, conventions, and intent. `main.css` is the source of truth for exact values; this doc explains how they're used.

The visual direction is calm financial clarity: dark, low-glare surfaces that are easy on the eyes for frequent check-ins, a restrained accent reserved for the one number that matters in a given view, light text on dark surfaces by default, and motion only where it carries meaning. The tokens and utilities below are how that direction is enacted. `main.css` is the authority for the actual palette.

## Foundation

The styling stack is **Tailwind CSS + DaisyUI**, with a custom DaisyUI theme named `twocents` declared in `static/src/main.css`. The theme defines semantic color tokens (base / primary / secondary / accent / neutral / info / success / warning / error, each paired with a `*-content` variant for legible text on that background), corner radii (box / button / badge), and `color-scheme: dark`.

**Dark-mode-first** (see [principles.md](principles.md)): `twocents` is the single, dark, default theme today — every surface and `-content` pairing is tuned for light-on-dark. A light theme, if added later, is a second DaisyUI theme toggled at the root; dark remains default and markup keeps using semantic tokens so it survives the swap unchanged.

Use semantic token names in markup — never hex literals, never `--color-*` variables directly. Tokens are referenced through Tailwind utility classes (`bg-base-100`, `text-primary-content`, `border-accent`) and DaisyUI component classes (`btn`, `card`, `badge`).

## Icons

Two Cents uses **Bootstrap Icons** (MIT, ~2000 icons) as its single icon source. The vendored CSS and font live under `static/`; the layout primitive loads them. All icons in the app are emitted by the single `Icon` primitive in `core/templates/icons.templ` — call sites pass the BI catalog name (without the `bi-` prefix) and an optional `IconStyle` (Outline or Fill). Sizing comes from the parent's `text-{size}`; color comes from the parent's text color (BI inherits `currentColor`).

**Outline / fill convention.** Outline is the default presentation; Fill marks the current page or selected state. Used wherever a UI surface has a paired notion of "this one vs the others" (today: the nav header). Most icons are decorative or single-meaning — for those, leave `Style` at its default.

**Single-variant icons.** Some BI icons exist in only one style (`bank`, `arrow-repeat`, `piggy-bank`, etc.). Check BI's catalog before passing `Fill` for an icon name; if no `-fill` variant exists, omit the prop.

See `core/templates/icons.templ` for the primitive's signature.

## Colors

The twocents theme defines several groups of color tokens, each with a distinct role.

### Surfaces (backgrounds, borders, dividers)

Stratified by elevation:
- `bg-base-100` — page background. The default canvas.
- `bg-base-200` — raised surface (cards, panels, dropdowns, modal bodies).
- `bg-base-300` — highest elevation (hovered rows, pressed states, the topmost layer); also the default border color (`border-base-300`).

### Brand tones (emphasis, identity, decorative chrome)

Each token has a defined role; reach for the role, not the color that "looks right" in isolation.

- **`primary`** — the brand color. Appears on the wordmark and as the one CTA per context (filled, soft, or selected-state). Reserved beyond that; scarcity is the point.
- **`accent`** — decorative highlights and emphasis moments — animated chrome, the headline figure in a summary. Used sparingly.
- **`secondary`** — an alternate brand tone, used when `primary` is taken or carries the wrong weight. Currently has no in-app uses; reserved in the theme.
- **`neutral`** — non-surface, non-brand chrome. Currently has no in-app uses; reserved in the theme.

Each tone has a paired `-content` token for legible text **on** that color. Always use the pair; never put `text-base-content` on a brand background.

### Semantic (status only)

- `info` — informational accents: status indicators that aren't success/warning/error, and in-content links.
- `success` — completed actions, positive validation, under-budget and on-track states.
- `warning` — recoverable problems or degraded states (a Connection that needs reconnect, a settling or partial month wrap, approaching a budget limit).
- `error` — failed actions, validation errors, over-budget flags, **and destructive actions** (delete buttons, archive-category, irreversible CTAs).

### Categorical palette

A distinct token group from the status tokens above, registered in `main.css` and usable as `text-category-*` utilities. These are **identity hues, not status signals**: a set of interchangeable, evenly spaced colors that stay legible on the dark base, plus a neutral for the no-identity case. Their job is to tell one category apart from another at a glance — the transaction-row avatar tints its glyph with one. A category has no fixed slot in the palette; the in-code mapping (a built-in's assigned hue, a custom category's id-hashed hue) owns which hue a given category wears, so the palette stays a plain set of swatches with no per-category meaning baked into the token. Never repurpose a status token (success/error/…) as an arbitrary category hue, and never reach for a categorical hue to signal status.

Three tokens serve the row classification buckets rather than spending categories — each maps to a fixed role:
- `text-category-income` — money in (Income rows).
- `text-category-transfer` — account-to-account movement (plain Transfer rows).
- `text-category-savings` — savings contribution (Transfer rows paired to a savings account).

### Text emphasis scale

Four named utilities express the text hierarchy. Use the named utility, not raw `text-base-content/NN`.

| Utility | Role |
|---|---|
| `text-default` | Body copy, headings, primary values — the voice of the page. |
| `text-muted` | Section labels, captions, supporting meta-context. |
| `text-subtle` | Timestamps, helper text, low-priority metadata. |
| `text-ghost` | Placeholders, empty-state hints, dimmed icons. |

Brand-colored text (`text-primary`, `text-error`, etc.) is a separate mechanism — emphasis by color, not by hierarchy. Brand colors don't get the four-stop scale.

### Element opacity

Three narrow roles, each wrapped as a utility class. Raw `opacity-NN` on a whole element should not appear in templ markup outside these.

- `.is-disabled` — disabled state (whole element non-interactive). Pairs the visual dim with a not-allowed cursor and suppressed pointer events so they can't drift apart.
- `.is-deemphasized` — permanent at-rest dim of a whole block that stays interactive: a group set apart from the page's primary content (e.g. an account group excluded from the totals). No cursor or pointer-event change — unlike `.is-disabled`, the contents still work.
- `.hover-fade-out` — hover affordance on a whole-element block (cards, link-wrapped rows). Visible → subtly dimmed on hover. Don't layer onto buttons or controls where DaisyUI handles the hover.
- `.hover-fade-in` — reveal-on-hover for secondary affordances (small buttons, ✕ controls on chips, row-scoped actions). Dim → fully visible on hover. Use when an action should be present but de-emphasized at rest.

See `static/CLAUDE.md` for the verbatim CSS definitions.

## Button hierarchy

Buttons decompose into three orthogonal axes: a **variant** (how much visual weight the button carries), a **color** (what intent it signals), and a **size** (matched to how much breathing room the surrounding surface has). Pair one from each based on what the button does and where it sits.

### Variant (visual weight)

| Class | Visual | Use |
|---|---|---|
| (none) | Solid filled | The single CTA in a context |
| `btn-soft` | Filled, subtle | Peer action with real visual weight (cancel, alternate flow) |
| `btn-outline` | Bordered, transparent | Selectable chip-bar option at rest |
| `btn-ghost` | Transparent | Low-emphasis chrome — icon affordances, dense row controls, quiet navigation |

### Color (intent)

| Class | Tone | Use |
|---|---|---|
| (none) | Neutral | Cancel, generic peer action, chrome |
| `btn-primary` | Brand | Primary CTAs, active/selected state |
| `btn-error` | Red | Destructive actions |

### Size (surface density)

| Class | Use |
|---|---|
| (none) | Buttons on a page-level open area |
| `btn-sm` | Buttons in compact contexts — modals, section editors, chip bars, navigation chrome |
| `btn-xs` | Dense row-level controls — transaction rows, icon clusters, in-content micro-actions |

Size is decided by **where the button sits**, not by what role it plays. A secondary action on a page surface is still page-sized; "soft" or "ghost" carries the not-the-main-action signal, not a smaller size.

### Canonical pairings

- **Primary CTA** (Save, Apply, Connect bank) — `btn-primary`
- **Peer action / cancel** — `btn-soft`
- **Active filter chip** — `btn-soft btn-primary`
- **Inactive filter chip** — `btn-outline`
- **Labeled destructive action** (archive category, remove rule) — `btn-soft btn-error`
- **Icon-only chrome** (✕, edit pencil, nav icons) — `btn-ghost`
- **Destructive icon in a dense row** — `btn-ghost btn-error`

External links (anything that opens off-site, e.g. the bank's reconnect flow) carry a trailing `box-arrow-up-right` icon and pick their variant by role, not by being external. An external link that's the page's main CTA uses the same `btn-primary` as any other main CTA; an external link that's a peer action uses `btn-soft`. The icon marks destination; the variant marks hierarchy.

## Typography

Body and UI text use DaisyUI's default stack. The brand mark uses a custom `.font-brand` utility defined in `main.css`. Add a font utility only when a new typographic role recurs across multiple surfaces; one-off styling stays inline at the call site.

## Animations

Custom keyframes live in `main.css` and are applied via Tailwind `animate-*` utilities or inline `style`. Animations exist for purposeful motion (transitions, progress reveals); the codebase does not animate for its own sake.

## When to add to `main.css`

`main.css` is small on purpose. Add to it when:

- **A new theme token is needed.** Extend the `[data-theme="twocents"]` block. Use a semantic name (`--color-success-muted`), not a literal one (`--color-green-light`).
- **A reusable utility is needed.** Define it at file scope, like `.font-brand`. Do not add utilities that wrap a single Tailwind class — use the Tailwind class directly.
- **A new keyframe is needed.** Define it at file scope; consume it via Tailwind's `animate-*` or inline.
- **A named-role wrapper around theme-semantic atoms is needed.** E.g. text emphasis utilities, state utilities like `.is-disabled`. Bare single-class wraps (`text-brand` for `text-primary`) are still not added.

Do **not** add per-page or per-component styling to `main.css`. Templ files use Tailwind utilities directly.

## Client-side libraries

The root primitive loads three external libraries: **HTMX** for interaction, **idiomorph** for smart DOM swaps, and **Alpine.js** for ephemeral client state. Use them in that order of preference — HTMX first, idiomorph as a swap strategy when needed, Alpine only for state that genuinely lives on the client (open/closed UI, focus rings, hover affordances).

A fourth library is added only when none of the three can do the job. Adding one means updating the root primitive and documenting the addition here.

The layout also loads **Bootstrap Icons** as a third-party stylesheet for icon rendering — see the Icons section above.

## Request feedback

In-flight feedback rides the HTMX request lifecycle rather than being wired per call. A global request-progress indicator in the root primitive reflects whether any request is outstanding ([ADR-0015](../adr/0015-app-wide-request-feedback.md)); a control that wants its own in-progress affordance keys off HTMX's request state rather than bespoke script. A completed action that changes nothing visible still acknowledges itself — briefly and inline, then auto-clearing — since the global indicator signals *pending*, not *result*.
