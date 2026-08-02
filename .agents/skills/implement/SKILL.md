---
name: implement
description: Runs phase 2 (Implement) — edits canonical docs first, then builds, then reconciles both doc layers before handing off to audit. Surfaces eligible build/execution skills. Keywords: implement, phase 2, build, code, develop, execute.
argument-hint: "[optional: name of the spec folder under docs/spec/ to implement]"
---

Phase 2 of the process in [`docs/process.md`](../../../docs/process.md).

## Steps

0. **Orient.** Read [`docs/product/vision.md`](../../../docs/product/vision.md) — product philosophy, deployment context, and scale assumptions that should inform implementation decisions.

1. **Edit canonical docs first.** Before touching any code, codify the design into the docs the change touches — ADRs, architecture docs, data model, module READMEs. This is the non-negotiable opening move of this phase.

2. **Surface eligible skills.** Look through your available skills and present the ones relevant to building and execution (TDD, subagent-driven development, plan execution, `/build`) as a short list with their one-line descriptions. Ask the user which, if any, they want to use — then proceed with the chosen flow, or freehand if none apply.

3. **Reconcile before leaving.** Once the build is done, update both doc layers to reflect what actually shipped:
   - Canonical docs (ADRs, READMEs, architecture) describe the real merged behaviour — keep them live.
   - The spec folder under `docs/spec/<timestamp>-<name>/` gets its final update to match reality — its last edit before freeze.

4. **Confirm the gate.** Verify `go build ./...`, `go test ./src/...`, and `task test/e2e` are green (see [`docs/testing.md`](../../../docs/testing.md)). Then hand off to phase 3 (`/audit`).
