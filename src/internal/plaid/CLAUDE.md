# plaid — external client

Rules: ../../../docs/architecture/archetypes/external-client.md

Satisfies the `banking.BankProvider` seam (`src/internal/banking`); returns only `banking` domain types, never Plaid-native shapes.

Module-specific notes:
- Every request carries the app credentials (`client_id` + `secret`, on the `Client`) plus a per-Item `access_token` (the bank login) passed per call. `NewClient` fails fast on a blank `client_id`/`secret`.
- Plaid-native wire types and all conversions live in `entities.go`; nothing outside this module references a Plaid type.
- Transactions use the cursor model (`/transactions/sync`): `SyncTransactions` loops over `has_more`, accumulating `added`/`modified`/`removed` and returning the final `next_cursor`.
- Plaid's amount sign (outflow positive) already matches the domain convention, so amounts carry through unchanged.
- `merchant.go` holds the provider-local merchant normalization used until the categorization domain's `CleanMerchantName` policy lands; Plaid's `merchant_name` is preferred when present.
- The wire `transaction` decodes the **read-only display detail** ([ADR-0013](../../../docs/adr/0013-richer-bank-transaction-detail.md)) alongside the categorization inputs: the raw `name` descriptor, `merchant_entity_id` / `logo_url` / `website`, `payment_channel`, `personal_finance_category.confidence_level`, the `authorized_date` / `datetime` / `authorized_datetime` timestamps, and the structured `counterparties` array (each entry typed: merchant vs marketplace/payment_app/etc.). These map onto `banking.Transaction` and are shown, never matched. Deliberately **not** decoded: `original_description` (empty on real production data — the raw `name` carries the descriptor) and `personal_finance_category_icon_url` (we surface confidence, not the icon).
