# Architecture docs

These docs describe the **conceptual** architecture: archetypes, foundational singletons, the rules each archetype follows. They are loaded by agents working anywhere in the project, so they must stay durable.

## Rule: no exhaustive lists

Do **not** enumerate every existing module, every external client, every utility, every `core/*` sub-package, or any other set whose membership changes when code is added or renamed. Lists like that rot the same way a hand-maintained wiki does — they go out of date silently as soon as the codebase moves.

### The test

Before listing concrete names, ask: *if a new instance is added next month, will this list be wrong?* If yes, it's an exhaustive list — drop it.

### What's OK

- **Single illustrative examples** that ground a rule (e.g. *"a topic file named after the package, like `transactions/transactions.go`"*). One concrete anchor per rule, not a roster.
- **Counts that match the architecture itself** (e.g. *"three archetypes"*, *"two singletons"*) — these only change when the architecture changes, in which case this doc is the obvious edit.
- **Conceptual descriptions** of what an archetype owns, allows, or forbids — phrased without naming current instances.

### What's not OK

- *"Domain modules (`accounts`, `transactions`, `categorization`, `budget`, ...)"* — a list of all current instances.
- *"Allowed: `core/contextx`, `core/httpx`, `core/db`, `core/task`, ..."* — a list of all current sub-packages.
- *"Vendor SDKs like `github.com/robfig/cron/v3`, `github.com/google/uuid`"* — a list of currently-used libraries.
- *"Canonical examples: `transactions/task.go`, `accounts/task.go`"* — a partial enumeration that pretends to be a single example.

For these, write conceptually (*"any domain module"*, *"any `core/*` sub-package"*, *"vendor SDKs for the wrapped API"*, *"existing `task.go` files in `src/internal/`"*) and let the live codebase be the source of truth for the membership. The README's `grep -h "^# " src/internal/*/CLAUDE.md` one-liner gives the at-a-glance view.

## Rule: conceptual over implementation

Architecture docs describe the **why** and the **what**: what an archetype is, what it owns, what its boundaries are. Specific file names, function signatures, helper functions, build commands, and import paths are implementation details — concrete enough that they belong in per-package `CLAUDE.md` files or in code, not as the load-bearing content of an archetype doc.

That said, a small amount of concrete grounding (single example file names, sample function signatures) is fine where it makes a rule unambiguous. The line is: don't make agents reach for these docs to look up *names*, make them reach to understand *intent*.

## Rule: archetype docs stay neutral about current divergence

If a module is currently out of compliance (a transitional gap), don't name it in the archetype doc (e.g. *"`teller` diverges because of X"*). That becomes false the moment that module is fixed. Module-specific compliance gaps belong either in the module's own `CLAUDE.md` (briefly, while transitional) or in commit history — the archetype doc describes the target, not the current population.
