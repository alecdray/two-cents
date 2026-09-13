# banking — provider seam

The seam between the app and a linked bank: the provider-agnostic types every domain reads, plus the `BankProvider` interface a concrete provider client satisfies. Not one of the three archetypes — it's the shared kernel the abstraction is built around. Decision: [ADR-0002](../../../docs/adr/0002-bankprovider-abstraction.md).

Rules:
- **Dependency-graph leaf.** Imports no domain module and no provider client (only `core/contextx` + stdlib). Domain modules depend on it for the shared shapes and the seam; provider clients depend on it to satisfy the seam.
- **No provider-native types, ever.** Every type here is provider-agnostic — bank-reported account/transaction type and category are plain strings, never a Plaid (or any provider) shape. A Plaid-named import anywhere in this package is a leak.
- Both rules above are enforced across the whole tree by `architecture/isolation_test.go` — it fails the build if `banking` reaches a provider or if any consumer outside the provider client and the composition root imports `plaid` directly.
- **Sign convention:** a spending outflow is positive, an inflow (refund, deposit) negative; a credit-account balance is the amount owed (positive). New monetary types follow it.
- `ErrReauthRequired` is the provider-agnostic signal that a bank login has expired; provider clients map their native login-required condition onto it so consumers react without depending on a provider error.
- `ErrStatementsUnavailable` is the signal that a login will not serve billing-cycle detail. **Distinct from `ErrReauthRequired` on purpose**: that one means nothing flows, this one means everything flows except one product, so it must never flag the connection needs-reconnect ([ADR-0026](../../../docs/adr/0026-statement-detail-is-an-enhancement.md)). A new sentinel earns its place only when consumers must react *differently*, which is the test this pair illustrates.
- **Seam types are named for what is taken, not for the provider product that carries it.** `CardStatement` is billing-cycle facts for a credit card; that a provider serves them from a "liabilities" product is the client's business, not the seam's. A type named after a product is the same leak as a provider-native field.
- **An unreported fact is `Known: false`, never a zero value.** `Balance` set the precedent and `CardStatement` follows it — the distinction is load-bearing downstream, where an unknown degrades to the worst case rather than reading as "nothing is due".
- **Loan and APR detail stay out of the shapes.** The liabilities non-goal narrowed to exactly those ([ADR-0024](../../../docs/adr/0024-cash-flow-timeline-sweep.md)); a field carrying them here would reopen it.
- Persistence of cursors, accounts, transactions, and statement detail belongs to the consuming domain modules — the seam carries shapes, never state.
