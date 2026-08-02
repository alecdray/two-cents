---
name: code-review
description: Adversarial code review across correctness, product correctness, readability (incl. inline comments), maintainability, efficiency, security, and testability. Use when asking for a code review, before opening a PR, wanting to stress-test an implementation, or when /audit (structural compliance gate) is not the right tool.
argument-hint: "[optional: 'diff' to scope to changed files vs main (default), a module name, or a file path]"
---

Run an adversarial multi-dimension code review by dispatching a **single subagent**.

## Step

Dispatch one `Agent` (subagent_type `Explore`) with this prompt:

> You are doing an adversarial code review for this project. Your job is to actively find problems — assume the author made mistakes, probe edge cases, and check the code against the product and domain specs. Read `.agents/skills/code-review/checklist.md` for the full per-dimension checklist, scope rules, and output format. Argument (scope): `<arg-or-diff>`.

Wait for the subagent to return, then relay its output verbatim.

## Output

---

## Code Review

**Verdict:** <one line — ✅ clean, or ⚠️ N findings, worst severity named>

Severity key: 🔴 bug (incorrect behaviour) · 🟡 suggestion (worth fixing) · ⚪ nit (optional polish)

### Correctness
For each finding: `path:line` 🔴/🟡/⚪ — what's wrong, why it matters, recommended fix.

### Product correctness
For each finding: `path:line` 🔴/🟡 — what the code does, what the product/domain doc says it should do, how to reconcile.

### Readability
For each finding: `path:line` 🟡/⚪ — what's hard to follow, recommended improvement.
Inline comments sub-section: flag comments that restate the code (delete); flag non-obvious invariants with no comment (add one).

### Maintainability
For each finding: `path:line` 🟡/⚪ — coupling, duplication, or abstraction issue, recommended fix.

### Efficiency
For each finding: `path:line` 🟡/⚪ — what's wasteful, context (hot path?), recommended fix.

### Security
For each finding: `path:line` 🔴/🟡 — vulnerability class, how it could be exploited, recommended fix.

### Testability
For each finding: `path:line` 🟡/⚪ — what makes this hard to test or signals missing coverage, recommended fix.

### Clean
Modules and files that passed without findings.

### Judgement calls
Anything ambiguous where you chose not to flag, or where the right call depends on intent the review can't infer.

---
