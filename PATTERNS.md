# Patterns

A reading guide to the organizing system encoded in this repo. Each pattern has a one-line summary and a pointer to where it lives.

| Pattern | Summary | Where it's encoded |
|---|---|---|
| Four-phase process | All work follows Spec → Implement → Audit → Merge; spec (a committed spec folder + codified canonical docs) is always phase 1 | `docs/process.md` |
| Two doc layers: canonical + spec | Canonical docs (ADRs, architecture, data model, module READMEs) are the live source of current-state truth; per-chunk spec docs under `docs/spec/<timestamp>-<name>/` are committed, reconciled to what shipped, then frozen at merge as a point-in-time record | `docs/process.md`; `docs/spec/README.md`; root `AGENTS.md` (Working docs and durable outputs section) |
| Layered, auto-loading agent context | Each directory has its own `AGENTS.md`; agents auto-load context by walking up from the working directory | Per-directory `AGENTS.md` files; `docs/architecture/README.md` |
| Tool-agnostic canonical + Claude symlink | `AGENTS.md` and `.agents/` are canonical; `CLAUDE.md` and `.claude/` are gitignored symlinks created by hooks | `.githooks/link-agents.sh`; `.gitignore` |
| Code archetypes | Every `src/internal/` module is one of: domain-module (`accounts`, `transactions`, `budget`…), external-client (`plaid`), utility (`tracker`, `reporting`), or singleton (`core`, `server`); archetype determines file layout | `docs/architecture/`; each module's `AGENTS.md` |
| Design archetypes | Every `.templ` file is one of: page, fragment, or primitive; archetype is determined by location in the tree | `docs/design/`; `docs/design/archetypes/` |
| Bank access behind an interface | All bank data flows through a `BankProvider` abstraction returning our own domain types; the concrete provider is selected by config, with a deterministic in-process fake for tests and end-to-end wiring | `docs/adr/0002-bankprovider-abstraction.md`; `docs/adr/0006-bank-provider-selected-by-config.md` |
| Cross-region updates: events over OOB | An action that must refresh regions beyond its target emits an HTMX event and each region re-fetches itself; OOB swaps are reserved for tightly-coupled siblings | `docs/adr/0010-event-driven-cross-region-refresh.md`; `docs/design/oob-swaps.md` |
| Doc discipline | No exhaustive lists; link-don't-restate; current-state-only; register every reference doc in its directory index; synchronized content tracked explicitly | `docs/architecture/AGENTS.md`; `docs/design/AGENTS.md`; root `AGENTS.md` (Documentation practices section) |
| ADR discipline | Architectural decisions are recorded in short ADR files; the decision log is the discovery surface | `docs/adr/README.md` |
| Agent-driven testing | Unit tests + Playwright e2e; BDD style; real backend, seed the DB for e2e; gate enforced at audit | `docs/testing.md`; `e2e/` |
| Pre-merge audit gate | Before any merge, run `/audit` — it dispatches the code and docs audits (code audit pinned to Opus) in parallel | `.agents/skills/audit/` |
