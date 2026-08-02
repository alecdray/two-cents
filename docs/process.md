# Development Process

How a feature ships. The four phases are **fixed**; *how* you carry out each one is at the
agent/user's discretion. Work happens on a branch off `main`.

**Two doc layers.** Canonical docs (ADRs, architecture, data model, module READMEs) are the live
source of current-state truth. Alongside them, each chunk of work keeps a **committed** spec record
under [`docs/spec/<timestamp>-<name>/`](spec/) — the scope, goals, and spec for that work, reconciled
to what shipped and **frozen at merge** (see [`docs/spec/README.md`](spec/README.md)). Genuinely
throwaway scratch (raw exploration, dead ends) still lives outside the repo (`/tmp`, …) and is never
committed. Whatever a spec folder decides must also be reflected in the canonical docs before the
branch merges.

## 1. Spec

**Run this phase on Opus** regardless of the session default — it is pure design judgement (domain
model, ADRs, tradeoffs), and a wrong decision here cascades into every downstream build. Set the
session model to Opus before starting, or run the Spec work on Opus explicitly.

Create the spec folder `docs/spec/<timestamp>-<name>/` with a `scope.md` (plus any goals/spec/support
docs the work needs), **and** capture the design by creating or editing the affected canonical docs in
place on the branch — domain model, ADRs, architecture docs, module READMEs. The spec folder records
the *narrative* of the work; the canonical docs hold the *durable* decisions.

Validate and sharpen the write-up with a grilling pass (`/grill-with-docs`), which tests it against the
existing domain language and recorded decisions and updates the docs inline as decisions crystallise.

## 2. Implement

Build the change with whatever flow fits — `/build`, `/tdd`, subagent-driven, or by hand.

Implementation always diverges from the plan. Before leaving this phase, **reconcile both doc layers to
what actually shipped**: the canonical docs must describe the real, merged behaviour, and the spec
folder gets its final update to match reality — its last edit before freeze.

Gate: `go build ./...`, `go test ./src/...`, and `task test/e2e` green (see [testing.md](./testing.md)).

## 3. Audit

Run `/audit` — the pre-merge gate covering both code and docs. Fix what it finds; repeat until clean.

## 4. Merge (PR → merge)

Push the branch and open a pull request (use `/gh-pr` for the canonical PR body). Once the audit is clean
and review passes, **squash-merge the PR** to `main`. The spec folder is now **frozen** — an immutable
record of that chunk of work; revisiting the same area later means a new `docs/spec/<timestamp>-<name>/`,
never an edit to the old one.
