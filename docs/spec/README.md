# Spec docs

The per-chunk-of-work record. Each folder here is the **scope, goals, spec, and supporting
documents** for one piece of work — the working docs that used to be thrown away, now kept in the
repo as history. Written in phase 1, reconciled to what shipped in phase 2, and **frozen at merge**
(see [`../process.md`](../process.md)).

These are **not** canonical docs. Canonical docs (ADRs, architecture, data model, module READMEs)
describe **current state** and stay live; a spec folder is a **point-in-time record** and is never
edited after its work merges. When the two disagree, the canonical docs win — the spec folder
reflects what was true when the work landed.

## One folder per chunk of work

```
docs/spec/<timestamp>-<name>/
├── scope.md        # what's in, what's out, and why — the boundary of this work
└── ...             # any supporting documents the work needs
```

`<timestamp>` is the Unix timestamp at the time the folder is created — a stable, sortable ordering
without gaps or renumbering. `<name>` is a short kebab-case slug for the work. Together they produce
folders that sort chronologically: `1753401600-refund-pairing/`.

`scope.md` is the only required file — a heading and short paragraph describing the work at a high
level. Add further files as the work needs them; a small change may need nothing else, a larger one
may add research notes, diagrams, or a decision scratchpad.

## Lifecycle

1. **Created in phase 1 (Spec)** — the folder and its scope/goals/spec are the deliverable of the
   Spec phase.
2. **Reconciled in phase 2 (Implement)** — updated to reflect how the work actually landed, as its
   last edit before freeze.
3. **Frozen at merge** — immutable thereafter. Revisiting the same area later means a **new**
   `docs/spec/<timestamp>-<name>/`, not an edit to the old one (the same rule ADRs follow).

## Relationship to canonical docs

A spec folder records the *narrative* of a chunk of work; durable outputs still land in their
canonical homes during Implement — a decision's rationale in an [ADR](../adr/), a reusable rule in
[`docs/architecture/`](../architecture/) or [`docs/design/`](../design/), user-facing behaviour in
the owning module's `README.md`. The spec folder is preserved **beside** those, not instead of them.
