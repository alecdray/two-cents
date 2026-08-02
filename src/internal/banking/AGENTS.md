# banking — provider seam

The seam between the app and a linked bank: the provider-agnostic types every domain reads, plus the `BankProvider` interface a concrete provider client satisfies. Not one of the three archetypes — it's the shared kernel the abstraction is built around. Decision: [ADR-0002](../../../docs/adr/0002-bankprovider-abstraction.md).

Rules:
- **Dependency-graph leaf.** Imports no domain module and no provider client (only `core/contextx` + stdlib). Domain modules depend on it for the shared shapes and the seam; provider clients depend on it to satisfy the seam.
- **No provider-native types, ever.** Every type here is provider-agnostic — bank-reported account/transaction type and category are plain strings, never a Plaid (or any provider) shape. A Plaid-named import anywhere in this package is a leak.
- Both rules above are enforced across the whole tree by `architecture/isolation_test.go` — it fails the build if `banking` reaches a provider or if any consumer outside the provider client and the composition root imports `plaid` directly.
- **Sign convention:** a spending outflow is positive, an inflow (refund, deposit) negative; a credit-account balance is the amount owed (positive). New monetary types follow it.
- `ErrReauthRequired` is the provider-agnostic signal that a bank login has expired; provider clients map their native login-required condition onto it so consumers react without depending on a provider error.
- Persistence of cursors, accounts, and transactions belongs to the consuming domain modules — the seam carries shapes, never state.
