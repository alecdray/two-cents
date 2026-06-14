# banking

The **provider seam**: the single boundary through which the rest of the app reaches a linked bank. It defines the provider-agnostic shapes every domain reads — accounts, balances, transactions, and their classifications — and the `BankProvider` interface that a concrete provider client (today `plaid`) satisfies by translating provider-native wire data into those shapes.

The decision behind it: [ADR-0002 — Bank access behind a `BankProvider` abstraction](../../../docs/adr/0002-bankprovider-abstraction.md).

## Why it exists

Putting a provider-agnostic seam between the app and the bank means the rest of the codebase never learns a provider's vocabulary. Swapping or adding a provider is a change to one external-client module plus its wiring at the composition root — no domain module moves. The types here are the lingua franca that makes that true.

## Boundary

`banking` is a **dependency-graph leaf**: it imports no domain module and no provider client (only `core/contextx` and the stdlib). Domain modules depend on it for the shared shapes and the seam; provider clients depend on it to satisfy the seam. Nothing here may reference a provider-native type or endpoint.

That isolation is not a convention left to vigilance — `architecture/isolation_test.go` reads the real import graph and fails the build if `banking` pulls in a provider (or any provider-named dependency), or if any package other than the provider client and the composition root imports `plaid` directly.

## What lives here

- **Provider-agnostic value types** — the account, balance, money, transaction, and category shapes domain modules consume. Bank-reported types, subtypes, and categories are carried as plain strings, never provider shapes. Field-level meaning lives in the godoc on each type.
- **`BankProvider`** — the interface a provider client implements: list a login's accounts, fetch current balances, and incrementally sync transactions by cursor. Persistence of cursors, accounts, and transactions belongs to the consuming domain modules, never the provider.
- **`ErrReauthRequired`** — the provider-agnostic sentinel for an expired bank login. A provider client maps its native login-required condition onto it; consumers react (e.g. flag a connection needs-reconnect) without depending on a provider-specific error.

The money sign convention (outflow positive, inflow negative; a credit balance is the amount owed) is shared with the wider domain — see [`docs/architecture/data-model.md`](../../../docs/architecture/data-model.md).
