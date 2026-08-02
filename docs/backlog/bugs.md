# Bugs

Known bugs, regressions, and tech debt. Architectural violations are tracked separately in [`../architecture/known-gaps.md`](../architecture/known-gaps.md).

- **Flaky e2e** `transaction-categorization.spec.ts:67` ("manual re-categorization survives a later sync") — an htmx `selectOption → waitForResponse` race; predates the budget slices. Suite is green with `--retries=2`. Candidate for `/diagnose`.
- **Manual categorization / transfer overrides can be lost on pending → posted.** Sync assumes a transaction moves pending → posted *in place* (same provider id), so override facets survive (they're excluded from the upsert). But when an institution reissues a **new** provider id for the posted transaction (Plaid links the two via `pending_transaction_id`), the pending row is deleted via the `removed` set and the posted row arrives as a fresh `added` row — so a manual re-categorization (and any transfer-destination override) is silently dropped and the posted row re-categorizes from scratch. Fix candidate: when applying an `added` row carrying a `pending_transaction_id`, carry the superseded pending row's override facets forward.
- **`PartialFlag` under-flags** later-added connections' backfill-edge months (correct for the common single-connection case; a precise history window is a [feature candidate](features.md)).
- **Disconnect hard-deletes accounts** instead of the domain's terminal `closed` state (a dangling transfer-destination FK) — tracked in [`../architecture/known-gaps.md`](../architecture/known-gaps.md).
