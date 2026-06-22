# Design docs

These docs describe the **conceptual** UI/UX rules: templ archetypes, cross-cutting design principles, design-system vocabulary. They are loaded by agents working anywhere in the project, so they must stay durable.

## Rule: no exhaustive lists

Do **not** enumerate every existing `.templ` file, every primitive in `core/templates/`, every page in a module's `adapters/`, every theme token, or any other set whose membership changes when UI is added.

### The test

Before listing concrete names, ask: *if a new instance is added next month, will this list be wrong?* If yes, drop it.

### What's OK

- **Single illustrative examples** that ground a rule (e.g. *"the shared layout primitive in `core/templates/layout.templ`"*).
- **Counts that match the architecture itself** (e.g. *"three archetypes"*) — these only change when the structure itself changes, in which case this doc is the obvious edit.
- **Conceptual descriptions** of what an archetype owns, allows, or forbids — phrased without naming current instances.

### What's not OK

- *"Primitives like `layout`, `navbar`, `modal`, `tooltip`, ..."* — a roster of current instances.
- *"Pages: overview, transactions, budget, wrap, ..."* — a list of current module pages.
- *"Theme tokens: `--color-primary`, `--color-accent`, `--color-base-100`, ..."* — a list that rots as tokens are added.

For these, write conceptually (*"any primitive in `core/templates/`"*, *"any page templ in a module's `adapters/`"*, *"the theme tokens defined in `static/src/main.css`"*) and let the live source be the registry.

## Rule: conceptual over implementation

Design docs describe the **why** and the **what**: what an archetype is, what it owns, what its boundaries are. Specific Tailwind utility strings, exact HTML structures, verbatim DaisyUI class names, and one-off signatures are implementation details — they belong in the templ files themselves or in module-specific notes, not as the load-bearing content of an archetype doc.

A small amount of concrete grounding (one example file path, one sample shape) is fine where it makes a rule unambiguous. The line: don't make agents reach for these docs to look up *names*, make them reach to understand *intent*.

## Rule: archetype docs stay neutral about current divergence

If a templ file is currently out of compliance (e.g. a page that hasn't yet adopted the shared layout, a "primitive" that still imports a domain type), don't name it in the archetype doc. That becomes false the moment it's fixed. Module-specific compliance gaps belong either in commit history or in a brief transitional note in the relevant `CLAUDE.md` — the archetype doc describes the target, not the current population.

## Rule: principles and design-system reflect what exists

`principles.md` codifies rules the codebase already follows or has consciously chosen — not aspirations. `design-system.md` describes the tokens, fonts, and patterns currently in `static/src/main.css` — not invented vocabulary. New principles or tokens land in the doc when they land in the code, not before.

## Rule: register a new convention doc in the index

A new convention doc under `docs/design/` goes in the **Cross-cutting rules** list of [`README.md`](README.md) with a terse, accurate one-line scope — that list is the index `audit` reads to find the conventions it must check. This is the design-doc instance of the project-wide discoverability rule in [`.claude/CLAUDE.md`](../../.claude/CLAUDE.md) §Documentation practices.
