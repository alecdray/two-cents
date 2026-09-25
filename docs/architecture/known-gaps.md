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

Occurrence matching raises the stakes ([ADR-0027](../adr/0027-occurrence-matching-reconciles-the-schedule.md)): a learned merchant is compared for exact equality against the same adapter-cleaned string, so the same one-way door would also stop every learned merchant matching — and there the failure is quieter still, since declining to match over-reserves rather than erroring.

**Closing it:** make the domain policy the only normalizer — have the adapter report the provider's payee fields verbatim (`merchant_name`, `name`) and let `CleanMerchantName` do all the stripping and casing, so one implementation defines the cleaned merchant for every provider. That is a behaviour change, not a refactor: it re-cases every stored merchant, so it needs a backfill and a decision about existing Rules matched against the current casing. Deferred for that reason — and because with a single provider that pre-cleans its own payees, the duplicate has no live consequence to force the issue.

## Fragments are declared inside the `_page.templ` file whose page wraps them

**Rule:** a `.templ`'s archetype is determined by **location and name** ([`design/README.md`](../design/README.md)) — a view file carries a `_page` or `_frag` suffix, and a fragment is a [fragment templ](../design/archetypes/fragment-templ.md). [`archetypes/page-templ.md`](../design/archetypes/page-templ.md) §"When the same content is reachable as both a page and a fragment" says to *"factor the inner content into a fragment templ; the page templ renders that fragment inside `PageLayoutComponent`."*

**Reality:** most page surfaces do factor the content out into a component and call it from the page — but they declare that component **in the `_page.templ` file itself** rather than in its own `_frag.templ`. Every page that has a fragment counterpart is built this way (`sweep`, `transactions`, `budget`, `categorization`'s two surfaces, and `home`'s three), and in each case a handler returns the co-located `*Frag` directly as a standalone fragment response. Three of `home`'s also carry a `Region` noun the [OOB naming rule](../design/oob-swaps.md) reserves for the private templs a `*OOBFrag` composes.

**Consequence:** none functionally, and the *substantive* half of the page-templ rule holds — there is one implementation, two call sites, never a duplicated render path. What breaks is the location convention: a reader who trusts the suffix to find a fragment's definition will not find these, and the archetype a file belongs to stops being inferable from its name. The convention's value is that it is mechanical, so partial adherence costs most of it.

**Closing it:** move each co-located fragment into its own `<region>_frag.templ`, leaving the page templ to call it inside `PageLayoutComponent`, and drop the `Region` infix from the three exported `home` fragments. Deferred because it is a pure move across eight files with generated `_templ.go` churn and registered testids riding along, and it predates the work that surfaced it — worth its own change rather than a rider. The alternative is to decide the co-location is house style and amend `page-templ.md` to say a page file may declare the fragment it wraps; that is a design call, not a cleanup.

## The budget form computes and formats its residual in the browser

**Rule:** [`design/principles.md`](../design/principles.md) — *"Server renders HTML; HTMX drives interaction."* JavaScript is reserved for genuinely client-only state; it is not the medium for fetching, validating, or **transforming domain data**.

**Reality:** `budget/adapters/views/budget_page.templ` carries an inline `budgetForm()` Alpine component that subtracts the shown category limits and savings from income to produce the residual, decides the balanced / over-allocated verdict from it, and renders both through a `formatMoney` helper that reimplements the server's USD formatting (sign, `$`, thousands separators, two decimals) in JavaScript. The server computes and formats the same figures for the initial render and for every swap; the script exists so the residual tracks keystrokes without a round-trip per character.

**Consequence:** two implementations of one money format, in two languages, with nothing that fails when they diverge — the drift this rule exists to prevent. It is confined to one form and is display-only (the submitted values, and every persisted figure, come from the server's own arithmetic), so a divergence misreads the residual while typing and corrects itself on save.

**Closing it:** recompute server-side and swap the `budget-residual` / `budget-balance-banner` regions over HTMX on a debounced `input` trigger, deleting `formatMoney`. That trades the per-keystroke feel for a round-trip, which is a **product decision** rather than a mechanical refactor — recorded here rather than made unilaterally. If the live-typing feel is worth keeping, the alternative is to narrow the divergence surface: have the server emit the formatted strings the script interpolates, so only the arithmetic lives in the browser.
