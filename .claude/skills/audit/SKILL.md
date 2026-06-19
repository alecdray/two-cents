---
name: audit
description: Pre-merge audit gate — runs code-audit and docs-audit together as parallel subagents and merges their reports. ALWAYS prompt the user to run this before merging, pushing a PR, or marking a branch as done. Keywords: audit, pre-merge, before merge, before push, before merging, ready to merge, ready to ship, final check, merge check, pre-PR.
argument-hint: "[optional: 'diff' to scope to changed files vs main, or a path]"
---

Run the project's full pre-merge audit. This is the canonical gate before any merge or PR — it covers both code and docs in one pass.

Do **not** run `code-audit` or `docs-audit` standalone in a pre-merge context; run this instead so both reports land together.

## Steps

1. **Dispatch both audits in parallel.** Send a single message with two `Agent` tool calls (subagent_type `Explore`), one per child skill. Forward the user's argument (if any) to both.

   - Code agent prompt: "Execute the two-cents `code-audit` skill at `.claude/skills/code-audit/SKILL.md` against this repo. Argument: `<arg-or-none>`. Follow the skill's Steps and Output sections exactly. Return only the 'Code Audit Summary' block."
   - Docs agent prompt: "Execute the two-cents `docs-audit` skill at `.claude/skills/docs-audit/SKILL.md` against this repo. Argument: `<arg-or-none>`. Follow the skill's Steps and Output sections exactly. Return only the 'Docs Audit Summary' block."

2. **Wait for both** to return. Do not summarise or rewrite their content — relay the two reports verbatim under a combined header (see Output).

3. **Add a top-line verdict.** One line: ✅ clean, or ⚠️ N findings across code/docs, with the worst category named.

## Output

---

## Pre-Merge Audit

**Verdict:** <one line — clean or N findings, worst category>

<verbatim Code Audit Summary block from the code subagent>

<verbatim Docs Audit Summary block from the docs subagent>

---
