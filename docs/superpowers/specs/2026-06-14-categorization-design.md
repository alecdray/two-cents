# Categorization slice — design spec

Date: 2026-06-14. Status: draft, pending subagent validation → `/build`.

The first vertical slice through the documented **Categorization** domain
([`docs/domain/README.md` §Categorization](../../domain/README.md)) plus the categorization
seam into **Transactions** (§Transactions, §Cross-domain write ledger). It builds the
**classification + bucketing subset**: a built-in Category taxonomy (+ custom categories),
user **Rules**, the pure `ResolveCategorization` engine, `CleanMerchantName`, and the two
cross-domain writes the ledger names — auto-categorize on sync and re-categorize on rule/category
change — surfaced through a re-categorize picker, a Categories page, and a Rules page.

It explicitly **defers transfer subtype pairing** (ADR-0003 **layer 2** / `ResolveTransferSubtype`,
destination + Savings-contribution) to its own later slice. Layer-1 transfer *classification* (from
the bank category's primary level, no pairing) **is** in scope — it needs no Accounts read.

## Goal (smallest genuinely-useful version)

Every synced Transaction lands with a Classification (Income / Spending / Transfer / needs-review)
and, when Spending, a Category — derived automatically by precedence (override > Rule > bank
category > direction). The user can: correct any Transaction's categorization (sticky, survives
re-sync); create/rename/archive custom Categories alongside the built-ins; and create/edit/delete
Rules (cleaned-merchant substring → outcome) that re-categorize matching non-overridden rows
immediately and all future ones. All browser-testable.

## What the domain doc already decides (not free choices)

- **Categorization is its own domain → new `categorization` module.** It **owns** the Category
  taxonomy (built-in + custom, archive-not-delete) and Rules. It **never writes Transaction rows.**
- **Invariant: Categorization decides, Transactions writes.** `ResolveCategorization` is a *pure*
  policy (returns a decision, writes nothing); Transactions is the only writer of
  `Transaction.Classification`/`Category`. The two cross-domain writes both live in **operations**
  (cross-domain write ledger): auto-categorize inside `Transactions.SyncTransactions`, and
  `Transactions.ApplyCategorization` *triggered by* Categorization rule/category changes.
- **Precedence (first match wins):** manual override > Rule (longest matching substring; tie →
  most-recently-edited) > bank `personal_finance_category` (transfer signal → Transfer; else taxonomy
  map → Classification+Category) > amount direction (outflow → Spending/uncategorized; inflow →
  needs-review, **never auto-Income**).
- **One sticky facet for this slice: `categorizationOverridden`** (Classification + Category). It
  beats auto-resolution and survives re-sync; auto-categorization and rules never revert it. (The
  independent `transferDestinationOverridden` facet arrives with the transfer-subtype slice.)
- **Rules match the cleaned merchant, never the raw `counterparty`.** Archived Categories are never
  auto-assigned (rules 2 & 4 skip archived targets) and existing assignments to them are untouched.
- **A spending Category sets Classification = Spending; choosing Income/Transfer clears Category.**
  A refund (inflow mapping to a spending Category) is **negative Spending**, never Income.

## Decisions made for this slice (the doc leaves these open; chosen, with rationale)

- **Built-in taxonomy = Plaid PFC primaries (1:1 seed).** The 16 `personal_finance_category`
  *primary* values map directly: `INCOME` → Classification **Income**;
  `TRANSFER_IN`/`TRANSFER_OUT`/`LOAN_PAYMENTS` → Classification **Transfer** (layer-1 signal,
  matches the doc's `TRANSFER_*, LOAN_PAYMENTS` example); the remaining **12 → built-in spending
  Categories**: Bank Fees, Entertainment, Food & Drink, General Merchandise, Home Improvement,
  Medical, Personal Care, General Services, Government & Non-Profit, Transportation, Travel, Rent &
  Utilities. Seeded with **stable string ids** (`food_and_drink`, …) so the PFC→Category map is a
  static in-code table, not data to migrate. Most accurate auto-categorization, least mapping
  guesswork. Custom categories sit alongside the built-ins.
- **`CleanMerchantName`.** Plaid already supplies a cleaned `merchant_name` (stored in the existing
  `transactions.merchant` column); the raw payee is `counterparty`. The cleaned merchant is
  therefore: `merchant` when non-empty, else a normalized `counterparty` (uppercase-fold, strip
  trailing store numbers / numeric ids, collapse whitespace). Rule matching is **case-insensitive
  substring** over this cleaned value. Pure function in `categorization`.
- **`needs-review` is a fourth Classification state**, used only by the direction fallback for an
  inflow nothing else classified (could be income or a refund — the user decides). It counts as
  neither income nor spending until resolved. Outflow fallback stays Spending + no Category
  (uncategorized).

## What's currently true (starting point)

`transactions` stores the bank's two-level category verbatim (`category_primary`/`category_detailed`)
and has **no** internal Classification/Category/override columns; `SyncTransactions` step 4
(auto-categorize) is currently a no-op stub. The `categorization` module does not exist. The
re-categorize picker, Categories page, and Rules page do not exist.

## 1. Module shape

New `src/internal/categorization/` domain module (standard archetype): `service.go` + `repo.go`
(only file touching `core/db/sqlc`) + `adapters/{http.go,routes.go}` + `adapters/views/`, plus
`categorization/CLAUDE.md` and `categorization/README.md` mirroring the `accounts`/`transactions`
docs. Topic file `categorization.go` for the entities (`Category`, `Rule`, `Classification`,
`Decision`) + the pure `CleanMerchantName`/PFC-map helpers.

**Dependency direction (new acyclic edges):**
- `transactions` **imports** `categorization` — to call `ResolveCategorization` (sync + apply) and
  to read the Category list for the picker. `Classification` and the resolution `Decision` type are
  **defined in `categorization`** (the decider) so this edge carries no reverse type dependency.
- `categorization` **never imports `transactions`.** Its only cross-domain write (re-categorize on
  rule/category change) goes through a **server-wired seam** — same dependency-inversion pattern as
  the existing `BackfillTransactions` func seam: a `ReapplyCategorization` func type owned by the
  `categorization` adapter, closure wired in `server` calling `transactions.Service.ApplyCategorization`.
- `categorization` imports `core/*` and `banking` (for `banking.Category`/`Money`); **not** `accounts`
  (only the deferred subtype layer needs Accounts), **never** `plaid`.

Extend `architecture/isolation_test.go` with `TestCategorizationDependencyDirection`: `transactions`
imports `categorization`; `categorization` (and everything under it) imports `transactions` never;
neither imports `plaid` (already covered, add `categorization` to the no-plaid sweep).

## 2. Data model

New goose migration(s) (`task db/create -- <name>`) + sqlc queries in
`db/queries/{categories,rules}.sql` and additions to `db/queries/transactions.sql`
(`task build/sqlc`, apply `task db/up`). Update `db/schema.sql`.

**`categories`** — owned/written only by `categorization`:

| column | notes |
|---|---|
| `id` TEXT PK | stable string id; built-ins fixed (`food_and_drink`…), customs a generated id |
| `name` TEXT | display name |
| `builtin` INTEGER (0/1) | seeded built-in vs user custom (both archivable; no hard delete) |
| `archived` INTEGER (0/1) DEFAULT 0 | archive-not-delete |
| `created_at`,`updated_at` TIMESTAMP | |

The 12 built-in spending Categories are seeded by INSERTs **in the migration** (deterministic, fixed
ids). Income/Transfer are Classifications, not Categories — not seeded as rows.

**`rules`** — owned/written only by `categorization`:

| column | notes |
|---|---|
| `id` TEXT PK | generated |
| `merchant_substring` TEXT | matched case-insensitively against the cleaned merchant |
| `classification` TEXT CHECK(`income`,`spending`,`transfer`) | the Rule's outcome axis |
| `category_id` TEXT NULL FK → categories(id) | set only when `classification = 'spending'` |
| `created_at`,`updated_at` TIMESTAMP | `updated_at` is the most-recently-edited tiebreak |

**`transactions` additions** (new migration; columns owned/written only by `transactions`):

| column | notes |
|---|---|
| `classification` TEXT NOT NULL DEFAULT `''` | `income`/`spending`/`transfer`/`needs_review`; `''` only transiently pre-resolve |
| `category_id` TEXT NULL FK → categories(id) | set only when `classification='spending'`; archived target kept on existing rows |
| `categorization_overridden` INTEGER (0/1) DEFAULT 0 | the sticky facet |

(The `transferDestinationOverridden` column and transfer destination/subtype columns are **not**
added this slice — they arrive with the transfer-subtype slice, no dead columns now.)

## 3. The engine — `ResolveCategorization` + `CleanMerchantName` (pure, in `categorization`)

`ResolveCategorization(in) → Decision{Classification, CategoryID *string}`, side-effect free.
Inputs: cleaned merchant, the active Rule set, the bank `Category{Primary,Detailed}`, and the
amount's direction (sign). Callers pre-skip overridden rows (rule 1 is defensive). Logic:

1. **Rule match** — Rules whose `merchant_substring` is a case-insensitive substring of the cleaned
   merchant; pick the **longest** substring, tie → **most-recently-edited** (`updated_at`). A rule
   whose `category_id` is archived is skipped. → the Rule's outcome.
2. **Bank category** — `Primary ∈ {TRANSFER_IN, TRANSFER_OUT, LOAN_PAYMENTS}` → **Transfer**
   (layer 1, no pairing); `Primary == INCOME` → **Income**; else PFC-map `Primary` → a built-in
   spending Category (skip if that Category is archived) → **Spending** + that Category. (An inflow
   here is negative Spending — a refund — handled by the stored sign, no special case.)
3. **Direction fallback** — outflow (amount > 0) → **Spending**, no Category (uncategorized);
   inflow (amount < 0) → **needs_review** (never auto-Income).

`CleanMerchantName(merchant, counterparty) → string` as in §Decisions. Both are unit-tested in
isolation (table-driven), the precedence ladder exhaustively.

## 4. Transactions wiring (the two ledger writes + the manual override)

- **Auto-categorize in `SyncTransactions` (step 4).** After upserting `added`/`modified` rows, for
  each **new or still-uncategorized, non-overridden** row call `categorization.ResolveCategorization`
  and write `classification`/`category_id`. `PendingReconcileMatch` already overwrites bank fields on
  pending→posted; it now also re-runs the auto path for the non-overridden facet. Overridden rows are
  skipped. (`categorization.Service` is injected into `transactions.Service` — constructor gains it;
  **construction order becomes accounts → categorization → transactions** in `server/services.go`,
  no cycle since `categorization` imports neither.)
- **`Transactions.ReCategorize(ctx, txnID, choice)`** — the manual override operation. Sets
  Classification and/or Category to the user's pick (a spending Category sets Classification=Spending;
  Income/Transfer clears `category_id`) and sets `categorization_overridden = 1`. Sticky.
- **`Transactions.ApplyCategorization(ctx, substrings)`** — re-runs `ResolveCategorization` over
  rows **without** `categorization_overridden` whose cleaned merchant matches a changed Rule's
  substring (union of old+new on edit). No provenance tracked — re-resolve from scratch (a remaining
  rule wins, else bank-category/direction). Invoked **only** via the server-wired seam from
  Categorization's **CreateRule/EditRule/DeleteRule** — Rule mutations are the *only* Categorization
  operation that writes Transactions (per the cross-domain write ledger). Skips overridden rows.

## 5. Categorization operations (own tables only) + cross-module seam

- `CreateCategory` / `RenameCategory` (id-stable) / `ArchiveCategory` / `UnarchiveCategory`;
  `ListCategories(includeArchived)`. **None of these write Transactions** (domain: archive keeps
  existing rows' assignments, rename is id-stable, create has no existing matches) — archive only
  removes the Category from the picker and from *future* auto-assignment (`ResolveCategorization`
  skips archived targets); existing rows pointing at an archived Category are untouched, and revive
  in the picker on un-archive.
- `CreateRule` / `EditRule` / `DeleteRule`; `ListRules`. Each rule mutation, **after** committing its
  own write, calls the injected `ReapplyCategorization` seam with the affected substring(s) → counts
  re-categorized. This is the **only** Categorization → Transactions write.
- **Seam wiring in `server`:** `categorization.Service` (or its adapter) holds a `ReapplyCategorization
  func(ctx, []string) (int, error)`; `server` injects a closure over `transactions.Service.ApplyCategorization`.
  Keeps `categorization` free of any `transactions` import (mirrors `BackfillTransactions`).

## 6. UI (HTMX-first, fragments over pages, inline errors, theme tokens — `docs/design/`)

- **Re-categorize picker on `/transactions`.** Each row shows its Classification + Category (chip),
  with a control opening one picker that sets both (spending Category ⇒ Spending; Income/Transfer
  clear Category). `POST /transactions/{id}/categorize` → `ReCategorize` → htmx-swap that row’s
  fragment (inline error, no redirect). Picker is populated from `categorization.ListCategories`
  (active only) — so the **transactions `HttpHandler` also takes `categorization.Service`** (wired in
  `server/start.go`). needs-review rows are visually flagged.
- **Categories page `GET /categories`** (categorization adapter): list active + archived (separated),
  create (name), rename inline, archive/unarchive. Built-ins are archivable but not renamable-away of
  their identity (rename allowed, id stable). htmx swaps; inline validation (no blank/dupe name).
- **Rules page `GET /rules`** (categorization adapter): list Rules (substring → outcome), create
  (substring + Classification, + Category when Spending), edit, delete. Each mutation surfaces the
  "N transactions re-categorized" count. htmx swaps; inline errors.
- **Navbar:** extend `core/templates/navbar.templ` with `Categories` and `Rules` links alongside
  Overview / Transactions.
- **Testids** (register in `docs/design/testids.md`): `categories-page`, `category-row`,
  `category-create`, `category-archive`, `rules-page`, `rule-row`, `rule-create`, `rule-edit`,
  `rule-delete`, `txn-categorize`, `txn-category-chip`, `txn-classification`, `txn-needs-review`,
  `nav-categories`, `nav-rules`, plus the picker control ids.

## 7. Testing (gate: `go build ./...`, `go test ./src/...`, `task test/e2e` all green)

- **Go unit (pure, no DB where possible):** `ResolveCategorization` precedence ladder exhaustively —
  override pre-skip, longest-substring + recency tiebreak, transfer/income/spending PFC mapping,
  archived-target skip, refund (spending inflow stays negative Spending), both direction fallbacks;
  `CleanMerchantName` table-driven; the PFC→Category map covers all 16 primaries.
- **Go integration (repo + service over a temp sqlite, mirror `accounts`/`transactions` tests):**
  auto-categorize on sync; `ReCategorize` stickiness across a re-sync (`PendingReconcileMatch`
  preserves the overridden facet, re-runs the rest); `ApplyCategorization` re-categorizes matching
  non-overridden rows and skips overridden; rule edit uses old∪new substring; archive reapply.
- **fakebank:** ensure its deterministic set spans categories that exercise the ladder — at least one
  clear spending PFC, one transfer PFC, one income PFC, and one row with no usable bank category +
  a merchant a seeded Rule can match (so the e2e rule flow has a target). Stays canned (no
  persistence), within the external-client archetype.
- **e2e (`BANK_PROVIDER=fake`)** — new `e2e/feat` + `e2e/spec`: connect (fake) → `/transactions`
  shows auto-categorized chips incl. a transfer + a needs-review row → re-categorize a row, assert
  the chip changes and **survives a Sync-now** (stickiness) → `/categories` create + archive a
  custom category → `/rules` create a rule matching the seeded merchant, assert the matching row
  re-categorizes and the "N re-categorized" count shows. Reuse `helpers/db.ts` seeding.
- **Architecture test:** `TestCategorizationDependencyDirection` (see §1) + extend the no-plaid sweep.

## Out of scope (named, so it isn't mistaken for missing)

Transfer **subtype/destination** pairing (ADR-0003 layer 2, `ResolveTransferSubtype`,
`MarkTransferDestination`, `transferDestinationOverridden`, Savings-contribution) — its own slice,
needs the Accounts read. Also: inflow→prior-outflow refund pairing (named post-v1 gap), category
**merge**, Budget (limits on Categories), filtering/search/pagination on transactions, per-account
drill-down, multi-currency category rollups. Each is a later slice with its own spec.
