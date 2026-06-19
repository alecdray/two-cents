# Development Process

How a feature ships. The four phases are **fixed**; *how* you carry out each one is at the
agent/user's discretion. Work happens on a branch off `main`.

**Core principle — the repo holds decisions, not exploration.** The repository only ever contains
canonical, codified docs. Scratch artifacts (specs, plans, build records, research notes) are
encouraged, but they live **outside** the repo (`~/workshop/builds/two-cents-*/`, `/tmp`, …) and are
**never committed** — see [Working artifacts](../.claude/CLAUDE.md). Whatever you decide in a working
doc must be reflected in the repo's canonical docs before the branch merges.

## 1. Spec

Capture the design **by creating or editing the affected canonical docs in place** on the branch —
domain model, ADRs, PRD/scope, architecture docs, module READMEs — **not** as a separate spec file.

Validate and sharpen the write-up with a grilling pass (`/grill-with-docs`), which tests it against the
existing domain language and recorded decisions and updates the docs inline as decisions crystallise.

Scratch exploration may use temp docs outside the repo; only the codified result lands in the repo.

## 2. Implement

Build the change with whatever flow fits — `/build`, `/tdd`, subagent-driven, or by hand. The choice is
yours; no working artifacts get committed either way.

Implementation always diverges from the plan. When it does, **reconcile the repo docs to what actually
shipped** before leaving this phase — the canonical docs must describe the real, merged behaviour.

Gate: `go build ./...`, `go test ./src/...`, and `task test/e2e` green (see [testing.md](./testing.md)).

## 3. Audit

Run `/audit` — the pre-merge gate covering both code and docs. Fix what it finds; repeat until clean.

## 4. Merge (PR → merge)

Push the branch and open a pull request (use `/gh-pr` for the canonical PR body). Once the audit is clean
and review passes, **squash-merge the PR** to `main`.
