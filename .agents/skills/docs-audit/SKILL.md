---
name: docs-audit
description: Audits project documentation for rot-prone content, drift against the codebase, and structural violations. Read-only — reports findings; does not edit. Use only when the user explicitly wants a docs-only audit; for any pre-merge / before-push check, use the `audit` skill instead so code is covered too. Keywords: docs audit, audit docs, doc rot, doc drift, documentation audit.
context: fork
agent: Explore
argument-hint: "[optional: file path, directory, or 'diff' to scope to changed files]"
---

Audit project documentation for compliance with the rules in `AGENTS.md`, `docs/architecture/AGENTS.md`, and `docs/design/AGENTS.md`. Read-only — report findings, do not edit.

## Scope

Default: the root `README.md`, `AGENTS.md`, everything under `docs/` (excluding gitignored paths), and every `README.md` / `AGENTS.md` under `src/internal/<module>/`.

When an argument is supplied:

- File path or directory — limit to that scope.
- `diff` — limit to files changed vs `main` (`git diff --name-only main...HEAD`), filtered to docs.

## Steps

1. **Read the rule docs.** Those listed above are the spec — especially the **Documentation practices**, **Synchronized content**, and **Working docs and durable outputs** sections of `AGENTS.md`. Re-read the relevant rule before flagging anything ambiguous.
2. **Establish sources of truth.** The rule docs name what's canonical for each kind of claim: domain language → `docs/domain/README.md`, decisions → `docs/adr/`, schema → `db/migrations/`, module membership → the `src/internal/` listing. Gather what you need.
3. **Audit each in-scope doc** for rot, drift, and misplacement (see Output categories). A `AGENTS.md` asserting current state must not carry historical, forward-looking ("later slice"), or comparative content.
4. **Check duplication against the registry.** Any fact stated in 2+ docs must be registered under **Synchronized content** in `AGENTS.md`. Unregistered duplication is a violation — recommend cutting from one location and linking, or registering it. Prose that restates a canonical definition (instead of linking) is the common offender.
5. **Check the spec record.** Working docs for a chunk of work live committed under `docs/spec/<timestamp>-<name>/` and are **frozen** after that work merges; genuinely throwaway scratch stays out of the repo. Flag a spec folder edited after its work merged, and any durable learning still stranded in a spec folder that should be folded into a permanent home per the rule table.
6. **Report.** Group findings, sort by path, include the rule violated and a recommended action.

## Output

---

## Docs Audit Summary

### Rot-prone content
For each finding: `path:line` — what's wrong, which rule, recommended cut/soften.

### Drift against the codebase
For each finding: `path:line` — what the doc says, what the code does, how to reconcile.

### Unregistered duplication
For each finding: the fact, the 2+ `path:line` locations, recommended single home + link (or register).

### Working-artifact violations
For each finding: `path` — a committed scratch artifact, or a stranded durable learning + its permanent home.

### Structural violations
For each finding: `path` — what's wrong, which rule, recommended fix.

### Clean
Documents that passed without findings.

### Judgement calls
Anything ambiguous where you chose not to flag, or where the right call depends on intent the audit can't infer.

---
