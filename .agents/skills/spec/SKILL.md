---
name: spec
description: Starts phase 1 (Spec) — creates the committed spec folder with a scope doc, codifies the design into canonical docs, then surfaces eligible planning skills. Keywords: spec, phase 1, new feature, start work, plan, brainstorm, scoping.
argument-hint: "<name of the work — kebab-case slug; the skill prefixes the current Unix timestamp>"
---

Phase 1 of the process in [`docs/process.md`](../../../docs/process.md). Full convention: [`docs/spec/README.md`](../../../docs/spec/README.md).

**Run this phase on Opus** — Spec is pure design judgement and a wrong decision here cascades into every downstream build (see `docs/process.md`).

## Steps

0. **Orient.** Read [`docs/product/vision.md`](../../../docs/product/vision.md) — it defines the product philosophy, deployment context, and scale assumptions that should inform scoping decisions.

1. **Create the spec folder.** Get the current Unix timestamp, then make `docs/spec/<timestamp>-<name>/` with a single `scope.md` — a heading and one short paragraph describing the work at a high level. The slug comes from the user's argument; if not supplied, ask for one before proceeding.

2. **Capture the design in canonical docs.** Phase 1 also codifies the design into the docs it touches — ADRs, architecture, data model, module READMEs. The spec folder holds the *narrative*; the canonical docs hold the *durable* decisions.

3. **Surface eligible skills.** Look through your available skills and present the ones relevant to planning and design work (e.g. `/grill-with-docs`) as a short list with their one-line descriptions. Ask the user which, if any, they want to use next.
