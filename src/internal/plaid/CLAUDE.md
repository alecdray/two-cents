# plaid — external client

Rules: ../../../docs/architecture/archetypes/external-client.md

Satisfies the `banking.BankProvider` seam (`src/internal/banking`); returns only `banking` domain types, never Plaid-native shapes.

Module-specific notes:
- Every request carries the app credentials (`client_id` + `secret`, on the `Client`) plus a per-Item `access_token` (the bank login) passed per call. `NewClient` fails fast on a blank `client_id`/`secret`.
- Plaid-native wire types and all conversions live in `entities.go`; nothing outside this module references a Plaid type.
- Transactions use the cursor model (`/transactions/sync`): `SyncTransactions` loops over `has_more`, accumulating `added`/`modified`/`removed` and returning the final `next_cursor`.
- Plaid's amount sign (outflow positive) already matches the domain convention, so amounts carry through unchanged.
- `merchant.go` holds the provider-local merchant normalization used until the categorization domain's `CleanMerchantName` policy lands; Plaid's `merchant_name` is preferred when present.
