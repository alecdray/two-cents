---
name: code-audit
description: Audits implementation code for violations of the project's architecture and design rules. Read-only — reports findings; does not edit. Use only when the user explicitly wants a code-only audit; for any pre-merge / before-push check, use the `audit` skill instead so docs are covered too. Keywords: code audit, audit code, architecture violations, design violations, archetype compliance, rule check.
context: fork
agent: Explore
argument-hint: "[optional: module name, file path, directory, or 'diff' to scope to changed files]"
---

Audit project code for compliance with the rules in `docs/architecture/` and `docs/design/`. Read-only — report findings, do not edit.

## Scope

Default: every directory under `src/internal/`, every `.templ` file, and `static/src/main.css`.

When an argument is supplied:

- Module name (e.g. `categorization`) — limit to `src/internal/<module>/`.
- File path or directory — limit to that scope.
- `diff` — limit to files changed vs `main` (`git diff --name-only main...HEAD`), filtered to code paths.

## Steps

1. **Read the rule docs** in `docs/architecture/` (README, archetypes, known-gaps) and `docs/design/` (README, archetypes, principles, plus the convention files). Also read each singleton's `CLAUDE.md` under `src/internal/`. These are the spec.
2. **Classify each module.** Read its `CLAUDE.md` to determine archetype (or singleton). A missing or undeclared archetype is itself a violation.
3. **Audit each module against its archetype's rules** — file layout, imports, service contracts, persistence isolation (only `repo.go` touches `sqlc`), the peer-adapter rule, provider isolation (no `plaid` import outside the external client). The archetype docs list the rules.
4. **Audit each `.templ` against its archetype** (determined by location + suffix) and against the cross-cutting design rules in `principles.md` (theme tokens, testids, cross-region updates — events vs OOB + single-sourcing, HTMX-first, inline errors).
5. **Reconcile with known gaps.** Cross-check every potential violation against `docs/architecture/known-gaps.md`. Matches are tracked, not new. Also report any known-gap entry that no longer matches reality — it should be removed.
6. **Report.** Group findings, sort by path, include the rule violated and a recommended fix.

## Output

---

## Code Audit Summary

### Archetype violations
For each finding: `path:line` *(archetype)* — what's wrong, which rule, recommended fix.

### Design rule violations
For each finding: `path:line` — what's wrong, which rule, recommended fix.

### Convention violations
For each finding: `path` — what's wrong, recommended fix.

### Known gaps confirmed
For each: gap title — still present, as documented.

### Known gaps no longer present
For each: gap title — no matching violation found; candidate for removal from `known-gaps.md`.

### Clean
Modules and templs that passed without findings.

### Judgement calls
Anything ambiguous where you chose not to flag, or where the right call depends on intent the audit can't infer.

---
