# Code Review Checklist

Adversarial review across quality dimensions. Read-only — report findings, do not edit. Actively try to find problems in each dimension; don't just passively scan.

## Scope

Default (`diff`): files changed vs `main` (`git diff --name-only main...HEAD`), filtered to `.go`, `.templ`, and `static/src/main.css`.

When an argument is supplied:
- Module name (e.g. `transactions`) — limit to `src/internal/<module>/`.
- File path or directory — limit to that scope.
- `diff` — explicit default behaviour above.

## Steps

1. **Establish context.** Read [`docs/product/vision.md`](../../../docs/product/vision.md) first — its deployment context (self-hosted, single-user) calibrates security and efficiency findings. Then for each in-scope module, read its `AGENTS.md` and `README.md`. For files that implement product behaviour, identify and read the relevant `docs/product/` and `docs/domain/` docs.

2. **Review each dimension below** against the in-scope files.

3. **Assign severity** to every finding:
   - 🔴 **bug** — incorrect behaviour, data loss, security hole, or violation of a domain invariant. Must fix.
   - 🟡 **suggestion** — quality issue worth addressing before the code is relied on. Worth fixing.
   - ⚪ **nit** — optional polish; reasonable to defer.

---

## Dimension: Correctness

Probe for logic errors and missing safeguards. Ask: *what happens when this goes wrong?*

- Off-by-one errors, incorrect boundary conditions.
- Nil/zero-value dereferences or missing presence checks.
- Error returns silently ignored or swallowed.
- Incorrect error propagation (e.g. returning `nil` after a failed write).
- Race conditions or shared-state mutations without synchronisation.
- Incorrect assumptions about external API behaviour (HTTP codes, pagination, empty responses).
- SQL queries that could return unexpected row counts (missing `LIMIT`, missing `WHERE`, wrong join type).
- Transactions that do not roll back on partial failure.

## Dimension: Product correctness

Cross-reference the implementation against the product and domain specs. Ask: *does the code actually do what the spec says?*

- Read the relevant `docs/product/*.md` and `docs/domain/*.md` for each in-scope module.
- Flag implementation that contradicts a stated rule (e.g. a domain invariant, an eligibility rule, a state machine transition).
- Flag behaviour the spec requires that isn't implemented (missing guard, missing side-effect, missing state update).
- Flag UI copy or labels that don't match the product doc's terminology.

## Dimension: Readability

Ask: *could a new contributor follow this without help?*

- **Naming** — variables, functions, and types that are ambiguous, misleadingly short, or don't match the domain vocabulary in `docs/domain/README.md`.
- **Cognitive complexity** — functions that do too much or require tracking too much state to follow; suggest a split.
- **Inline comments:**
  - 🗑 **Delete** — comments that restate what the surrounding code already says clearly (e.g. `// increment i` above `i++`). Flag each one.
  - ✏️ **Add** — non-obvious invariants, subtle constraints, or workarounds that a future reader would likely get wrong without a note. Flag where a short comment would save real confusion.
- **Magic values** — unexplained literals that should be named constants.

## Dimension: Maintainability

Ask: *how hard will this be to change in six months?*

- Tight coupling between modules that should be independent (check the archetype rules in `docs/architecture/`).
- Duplication that could drift: the same logic in two places with no shared home.
- Abstraction leaks — implementation details exposed through a public interface.
- Functions or types with too many responsibilities.
- Hardcoded config that belongs in the environment or a constant.

## Dimension: Efficiency

Focus on real costs; don't micro-optimise. Ask: *is this wasteful in a way that will matter?*

- N+1 query patterns — a query inside a loop that could be batched.
- Unbounded queries — no `LIMIT` on a result set that could grow large.
- Unnecessary allocations in hot paths (repeated string concatenation, per-request slice growth).
- Redundant work — the same computation repeated where a single pass or a cache would do.

## Dimension: Security

Ask: *how could an attacker misuse this?*

- Unsanitised external input used in SQL, shell commands, file paths, or template rendering.
- Missing or bypassable auth/authorisation checks on handler routes.
- Sensitive data (tokens, secrets, PII) logged, returned in responses, or stored unencrypted.
- Insecure defaults (e.g. TLS skipped, CORS wildcard, unbounded timeouts).
- Cryptographic misuse (home-rolled crypto, weak algorithms, predictable nonces).

## Dimension: Testability

Ask: *how easy is it to verify this is correct?*

- Hard-coded dependencies (time, randomness, external services) not injected or interfaced — makes unit testing impossible without the real thing.
- Functions that mix I/O with logic — suggest separating the pure computation.
- Functions with no clear seam for a test to intercept.
- Side-effects in constructors or package `init()` that complicate test setup.
- Signal of missing test coverage: complex branching logic or error paths with no obvious corresponding test.
