# Known Architectural Gaps

This doc tracks current architectural violations in the codebase — places where the rules in [`archetypes/`](archetypes/) (and module `AGENTS.md` files) don't yet match reality. Each entry describes the gap, why it exists, and what closing it would require.

Gaps are tracked here (and not enumerated inside the archetype docs themselves) so the rule docs stay durable and conceptual, while the concrete list of divergences lives in one searchable place.

When a module ends up out of compliance with its archetype (a peer reaching into another module's `adapters/`, a non-`repo.go` file importing `sqlc`, an external client growing persistence), record it here rather than weakening the archetype doc. Each entry names the rule violated, where, why it exists, and what closing it would require.

## Transactions read model joins other modules' tables in raw SQL for display

**Rule:** [`data-model.md`](data-model.md) — *"cross-module reads flow through the owning module's `*Service`, never raw SQL."* The rule lives where the data lives ([`archetypes/domain-module.md`](archetypes/domain-module.md)).

**Reality:** the `transactions` activity read queries (`db/queries/transactions.sql`) still `JOIN accounts a` for `a.mask` and `LEFT JOIN categories c` for `c.name`, reading columns owned by `accounts` and `categorization` directly. `ListTransferLegs` also joins `accounts` for an `a.state != 'closed'` filter. The account **display name** was the one such read moved onto the owning service (`accounts.DisplayNames`, [ADR-0017](../adr/0017-custom-account-names.md)) — because its precedence (`custom_name` else `name`) is domain policy that must not be re-encoded across the boundary — but the mask, category name, and the closed-state filter still cross in SQL.

**Consequence:** none functionally — these are stable columns, and the join is an efficient read-model denormalization. The cost is architectural: two modules' schemas leak into the transactions queries, so a column rename there silently breaks this module.

**Closing it:** resolve mask + category name through the owning services the way the display name now is (an `accounts` mask/facet lookup and a `categorization` name lookup keyed by id), and move the transfer-leg state filter behind an `accounts` predicate. Deferred because, unlike the display name, none of these encodes cross-boundary *policy* — they are plain value reads whose only sin is being raw SQL.

## Account disconnect hard-deletes instead of transitioning to `closed`

**Rule:** the domain [`Account` state machine](../domain/README.md) has a terminal **`closed`** state; an Account should be retired by transitioning to `closed`, not removed.

**Reality:** `accounts.Disconnect` **hard-deletes** the connection's Account rows (it does not set them `closed`) while leaving the Transactions that reference them in place. sqlite foreign keys are declarative-only here — `core/db` opens the database without `_foreign_keys=on` — so nothing rejects the resulting dangle.

**Consequence:** a saved `transfer_destination_account_id` that points at a now-deleted Account dangles. The destination-name JOIN returns empty, so a past **Savings contribution** to that account renders with a **blank destination name**. Display-only — the contribution is still summed correctly.

**Closing it:** transition disconnected Accounts to `closed` (preserve the rows) instead of deleting them, so transfer-destination references stay resolvable. Do **not** paper over it with an FK cascade — that would delete the historical transfers too.

## `home` renders the transactions module's row templ across module boundaries

**Rule:** a module's `adapters/` is private to it — peers compose behaviour through the owning module's `*Service`, not by importing its `adapters/views`. The established norm is [ADR-0016](../adr/0016-rule-editor-modal-and-cross-modal-return.md): `transactions` opens `categorization`'s editor **by URL, with no view import**.

**Reality:** `home/adapters/views` imports `transactions/adapters/views` (aliased `txnviews`) to render the canonical `TransactionRowFrag` for every transaction-row surface it owns — the wrap month list, the Tracker list (both via `AllTransactionsFrag`), and the spend drill-down. This unifies rows across the app: the `/transactions` tab, wrap, Tracker, and drill share one row component, so a row change propagates everywhere and cannot drift.

**Consequence:** none functionally; the edge is one-way and acyclic (`home` is the read-side composition root — nothing imports it but `server`, and the isolation test enforces that). The cost is architectural: `home`'s view layer now couples to `transactions`' view layer, so a rename or signature change to `TransactionRowFrag` breaks `home`. The tradeoff is deliberate — a private copy per surface was the alternative, and it drifts (which is what prompted the unification).

**Closing it:** promote `TransactionRowFrag` to a shared, module-neutral home both can import (it takes a `transactions.RecentTransaction`, so it is not a `core/templates` primitive as those forbid domain types) — or accept it as a sanctioned composition-root exception and lift the "no peer `adapters/` import" rule for `home` specifically. Deferred: the reuse is worth more than the coupling while `home` is the only importer.

## `accounts` splits one aggregate across two topic files

**Rule:** the [domain-module archetype](archetypes/domain-module.md) allows splitting into multiple topic files only when the parts share no types and no methods cross them.

**Reality:** `accounts` carries both `accounts.go` and `dashboard.go`, and they fail that test. `dashboard.go` declares `Dashboard` with an embedded `Overview` (declared in `accounts.go`), builds `AccountRow` from `Account`/`AccountState` (both declared in `accounts.go`), and hosts the `(*Service).Dashboard` method. These are two views of one aggregate, not two concepts — the read model and the entity it reads.

**Consequence:** none functionally. The cost is that the module's shape misreports itself: a reader expecting two independent topics finds one aggregate whose types are split across files by no discernible rule, so new read-model code lands in whichever file the author happened to open. [ADR-0021](../adr/0021-fault-isolating-sync-pass.md) deepened it (the staleness threshold and two `AccountRow` fields landed in `dashboard.go`).

**Closing it:** fold `dashboard.go`'s types into `accounts.go` and its `Dashboard` method into `service.go`. Deferred because the split predates the work that surfaced it and untangling it is a pure move touching every read-model call site — worth its own change, not a rider on a production fix.

## Merchant normalization is implemented twice, and the domain policy is the copy that rarely runs

**Rule:** the [`CleanMerchantName` policy card](../domain/README.md) declares merchant normalization a **Categorization** policy — *"normalize (strip store numbers, trailing ids, casing) → a stable cleaned merchant. Rules and display use this cleaned merchant."* One domain owns it; adapters convert, they do not decide policy ([`archetypes/external-client.md`](archetypes/external-client.md)).

**Reality:** it is implemented twice, in both places named in that sentence. `plaid.cleanMerchant` (`src/internal/plaid/merchant.go`) strips trailing store numbers, collapses whitespace and **title-cases** at ingest, filling `banking.Transaction.Merchant`. `categorization.CleanMerchantName` strips store-number tokens and **uppercase-folds** — but it prefers a non-empty `Merchant` verbatim, and `cleanMerchant` returns empty only when the provider sends neither a `merchant_name` nor a `name`. So for effectively every Plaid-sourced row the domain policy is a pass-through, and the adapter's normalization is what Rules match against and what the UI shows.

The two disagree on identical input:

| raw descriptor | `plaid.cleanMerchant` (what Rules see) | `CleanMerchantName` fallback |
|---|---|---|
| `PURCHASE WM SUPERCENTER #1700` | `Purchase Wm Supercenter` | `PURCHASE WM SUPERCENTER` |
| `SQ *COFFEE BAR 991` | `Sq *coffee Bar` | `SQ *COFFEE BAR` |

The policy card's **Inputs** line compounds it: it names `Transaction.counterparty` as the input, but the counterparty is only read on the branch that does not normally run.

**Consequence:** none today — the preference order keeps exactly one of them live per row, so nothing double-normalizes and no Rule mismatches. The gap is latent and it is a one-way door: a second provider that does not pre-clean its payees, or any change that lets `Merchant` arrive empty, would start routing rows down the uppercase branch. Rules stored against `Sq *coffee Bar` would silently stop matching the same merchant arriving as `SQ *COFFEE BAR`, and the failure surfaces as transactions quietly landing in needs-review rather than as an error.

**Closing it:** make the domain policy the only normalizer — have the adapter report the provider's payee fields verbatim (`merchant_name`, `name`) and let `CleanMerchantName` do all the stripping and casing, so one implementation defines the cleaned merchant for every provider. That is a behaviour change, not a refactor: it re-cases every stored merchant, so it needs a backfill and a decision about existing Rules matched against the current casing. Deferred for that reason — and because with a single provider that pre-cleans its own payees, the duplicate has no live consequence to force the issue.
