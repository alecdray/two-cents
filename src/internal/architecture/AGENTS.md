# architecture — structural tests

Not a module — a home for tests that assert structural properties of the codebase as a whole, rather than the behaviour of any one package. They read the import graph (via `go list -json`) and fail when a boundary the design depends on is crossed.

Rules:
- Tests here own **cross-cutting invariants only** — properties no single package can assert about itself because they constrain the relationships *between* packages. Behaviour of a module belongs in that module's own tests.
- Add a new test here when a design decision depends on a structural boundary holding tree-wide (an allowed/forbidden import direction, a package that must stay a leaf, a dependency that must not spread). One-off, package-local rules don't belong here.
- Guard against vacuous passes: a graph sweep that names no package can pass because it checked nothing. Assert the anchor packages are present before asserting properties about them, so dropping one fails loudly instead of silently shrinking the sweep.
- Runs in the normal suite (`go test ./...` / `task test/unit`) — no build tags, no special setup beyond a working Go toolchain.

The current invariant is the **provider seam** ([ADR-0002](../../../docs/adr/0002-bankprovider-abstraction.md)): no package outside the provider client and the composition root imports `plaid`, and `banking` imports no provider or provider-named dependency.
