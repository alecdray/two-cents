# Richer bank transaction detail

A Transaction carries **read-only bank display detail** beyond the cleaned merchant and bank category — the raw descriptor, merchant identity (logo, website, entity id), payment channel, the bank's categorization confidence, the authorized/posted timestamps, and a structured **counterparties** list. This detail is sourced from the bank on every sync and is kept distinct from the fields that *drive* categorization.

The editor surfaced only the cleaned merchant and the raw bank category, while the provider returns much more on every `/transactions/sync` — detail we decoded into ten fields and discarded the rest of. The gap was visible to the user: an order shows as the cleaned `Twobootsp` when the bank descriptor (`DD *DOORDASH TWOBOOTSP`) and a structured counterparties list both name the real merchant *and* the intermediary (DoorDash, PayPal, Toast). The field set was chosen against real production data, not assumptions — only fields a real institution actually populates were kept.

A few consequences shape the decision:

- **Display detail is not a categorization input.** Categorization still resolves on the cleaned merchant derived from `counterparty` ([CleanMerchantName](../domain/README.md)), and on `personal_finance_category`. The new descriptor and counterparties are shown, never matched — the "Categorization decides on the cleaned merchant, never the raw payee" invariant is unchanged. Using `counterparties.type` to *improve* categorization (a marketplace like DoorDash signalling a sub-merchant) is a separate future decision, deliberately out of scope here.

- **Bank-sourced, so it lives in the sync upsert.** Unlike the categorization and transfer-destination **override facets** — deliberately excluded from `UpsertTransaction` so a user's sticky edit survives re-sync ([transactions/CLAUDE.md](../../src/internal/transactions/CLAUDE.md)) — these fields carry no user state. They are refreshed from the bank every sync, so they belong *in* the upsert. The two column groups are governed by opposite rules; the distinction is the load-bearing one.

- **Counterparties is a denormalized JSON column, not a child table.** It is read-only display data — never queried, joined, or aggregated — so a JSON blob on the Transaction row matches its use and keeps the schema flat. A normalized `transaction_counterparties` table would buy query power the feature never exercises.

- **Existing rows backfill by re-pull.** Rows stored before the columns existed hold their defaults until refetched, and the cursor sync does not revisit unchanged transactions. Clearing the per-connection cursor (`transaction_sync_state`) makes the next sync re-pull and upsert the full set — idempotent by `DedupeKey`, and the same self-healing stance the categorization sweep already takes.

Rejected: Plaid's `original_description` as the raw-descriptor source — empty on real production data, so the raw `name` field carries the descriptor instead; a normalized counterparties table (unjustified for display-only data); `personal_finance_category_icon_url` (we surface the categorization *confidence*, not a category icon).

Which fields appear in the editor and how they lay out are presentation details, not part of this decision; they live with the owning module ([`src/internal/transactions/README.md`](../../src/internal/transactions/README.md)) and the templ.
