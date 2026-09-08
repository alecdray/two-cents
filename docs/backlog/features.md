# Features

Candidate features and improvements, deferred out of the current build. Sourced from the PRD's out-of-scope list, the domain model's deferred notes, and shipped slices' known gaps.

## Near-term candidates (usability)

- **Home needs-attention alert.** When the current month has uncategorized / incomplete transactions, surface an alert on the Tracker deep-linking to the (already-shipped) needs-attention worklist (`/transactions?view=needs-attention`). Open finding from the real-Plaid validation run.
- **Sync result count / last-synced time.** The in-flight signal shipped ([ADR-0015](../adr/0015-app-wide-request-feedback.md)) and the *stale* case now surfaces on the accounts overview ([ADR-0022](../adr/0022-fault-isolating-sync-pass.md)); still deferred is a concrete "Synced (n updated)" count (needs a sync summary the service doesn't return today) and an always-visible last-synced time on every account, not only the aged ones.
- **Rules matching richer transaction detail** ([ADR-0013](../adr/0013-richer-bank-transaction-detail.md) deferred note). Today rules match only the cleaned merchant, so the ingested platform/intermediary and raw descriptor are shown but not matchable (a "DoorDash → Dining" rule can't catch `DD *DOORDASH …`). Feed structured `counterparties` and/or raw `description` into the rule engine, with an explicit platform-vs-sub-merchant precedence decision. Its own slice.
- **Transactions pagination** (the unfiltered default list is capped at the recent 100; search + needs-attention query full history) and **per-account drill-down**.
- **Transaction groupings / spending events.** Tag transactions into an ad-hoc named group cutting across categories and months (e.g. an "Italy trip") and see the group's total. A new capability (lightweight many-to-many tag + a group view), not polish.
- **Sweep multi-account aggregation.** The cash-sweep derivation requires exactly one checking + one savings account; aggregate across multiple of each so multi-account users get a recommendation without manual setup. Contained to `sweep/service.go` + the MTD SQL queries. Deferred note in [ADR-0020](../adr/0020-monthly-cash-sweep-recommendation.md).
- **Refund → prior-outflow pairing** — a refund inflow matched to its original purchase.
- **External-account entity** for transfers to *unconnected* accounts (today you can mark a subtype, not a real destination).
- **Precise provider history window** for the wrap's `partial` flag (today an earliest-transaction heuristic; under-flags later connections — see [bugs.md](bugs.md)).
- **Sync reconciliation safety net.** Sync trusts Plaid's `/transactions/sync` cursor as the sole completeness mechanism (idempotent upserts, atomic cursor advance, `removed`-set deletes) but nothing independently re-checks for drift. Candidates: a "force full reconcile" that diffs a fresh full pull and prunes orphans, and/or a periodic drift check. Robustness hardening, not a current bug — the cursor is contractually complete.

## Explicitly deferred (PRD out-of-scope)

- Budget rollover / envelope carry-over (v1 is monthly, no rollover).
- Historical per-week/day actual breakdown (the week/day view is forward pace only).
- Goal-based named savings (beyond a single monthly savings target).
- Bill / subscription detection & reminders.
- Category merge (archive/rename only in v1).
- Fuzzy / pattern merchant matching in rules (v1 is substring on the cleaned merchant).
- Investments / holdings & liabilities detail (loan APR, interest breakdown).
- Payments / money movement initiated from the app.
- Multi-user / managing others' accounts.
- Mobile app.
- Non-USD / non-US banks.
