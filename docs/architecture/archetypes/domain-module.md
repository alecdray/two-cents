# Domain Module

## Purpose

A domain module owns a slice of the application's domain end-to-end: business logic, persistence, and (optionally) HTTP entrypoints. Most modules under `src/internal/` are domain modules. Each one is the single home for the rules and data that belong to its slice — peer modules consume it only through its exported `Service` and DTO types.

## File layout

```
src/internal/<module>/
├── service.go          # Service struct + business logic — required
├── repo.go             # ONLY file allowed to import core/db/sqlc — required if the module owns persistence
├── <package>.go        # domain types, view models, pure helpers — optional, default name matches the package (e.g. transactions/transactions.go)
├── task.go             # background tasks (core/task.Task) — optional
├── *_test.go           # tests live next to the file under test
├── README.md           # required
├── CLAUDE.md           # required; declares archetype
└── adapters/           # required only if the module has HTTP entrypoints
    ├── http.go         # HttpHandler struct + handler methods
    ├── routes.go       # RegisterRoutes(mux *httpx.Mux, h *HttpHandler)
    └── views/          # rendering layer — its own Go package (`package views`)
        ├── *.templ     # templ source; see docs/design/ for archetype rules
        └── *_templ.go  # generated; do not edit by hand
```

Required: `service.go`, `README.md`, `CLAUDE.md`. `repo.go` is required if the module owns persistence (the common case); a module that delegates all persistence to peer services can omit it. Everything else is optional. `adapters/` exists if and only if the module exposes HTTP routes.

## Service layer

The service is the module's domain rule keeper and the contract surface — what peer modules and adapters consume. `*Repo` is internal; `*Service` is what crosses module boundaries.

- The `Service` struct lives in `service.go`. All business logic is a method on `*Service`.
- The constructor takes peer domain modules' `*Service` types and the module's own `*Repo`. Wiring happens in `server/`.
- Methods take `contextx.ContextX`, never `context.Context`. Extract the user ID with `ctx.UserId()`.
- Services hold concrete `*Repo`. If a service test needs to mock the repo, the service file declares a small interface locally next to the consumer (Go convention: accept interfaces, return structs).

A service method that is a thin 1:1 passthrough to its repo is fine — the service is still doing real work (import boundary, `ContextX → userID` translation, future home for authz/validation). When a service feels chatty across many small repo calls, the lever is fatter SQL (CTEs, `RETURNING`, multi-row writes), not fatter repos. Letting the repo hold policy collapses the encapsulation, because peers consume `*Service`, not `*Repo`.

## Repository layer

The repo is a domain ↔ SQL adapter: it speaks domain in both directions (takes domain IDs, returns DTOs) and contains zero policy. No validation, no decisions about what should happen, no invariants enforced — those belong in the service, which composes single-purpose repo calls.

- `repo.go` is the **only** file in the module allowed to import `core/db/sqlc`.
- Methods are named for domain operations (e.g. `GetTransactionsForMonth`, `InsertBudget`) and return DTOs / domain types — never `sqlc.*` types. SQLC types do not appear in any signature outside `repo.go`.
- `Repo` is a concrete struct. Its constructor takes a `*sqlc.Queries` so it can be bound either to the global handle or to a transaction (see *Transactions* below).

When a method feels borderline, ask: *could a different domain flow reuse this exact persistence step?* If yes, it's a repo method. If the step encodes a decision tied to one specific flow (a read-then-write conditional on business state, a cross-table write that only makes sense for one caller), it's the service composing repo calls.

## Transactions

- Multi-step persistence happens at the service layer via `core/db.WithTx`:

  ```go
  err := s.db.WithTx(func(tx *db.DB) error {
      // build a transactional repo bound to tx.Queries() and call it
  })
  ```

- Repo constructors take `*sqlc.Queries` (not `*db.DB`) so the same repo type works against the global handle or a transaction.

- A repo method does **one** persistence operation. If correctness depends on multiple writes landing atomically — including cross-domain side effects on other modules' tables — that orchestration belongs at the service layer, not bundled in a repo method. The service composes single-purpose repo calls inside `WithTx`; each repo method stays callable on its own.

- **Exception:** if a repo method genuinely must run inside a transaction, encode that in its signature with a type that callers can only obtain from a tx context (e.g. a `*db.Tx` argument). Invariants on repo methods are enforced by the type system, not by comments telling callers what to do.

## Domain types

- **Default: one topic file named after the package** (e.g. `transactions/transactions.go`, `budget/budget.go`). It holds all DTOs, value objects, view models, and pure helpers — methods on those types belong in the same file as the type.
- Do not create a `models.go` file.
- Types that cross module boundaries (e.g. consumed by another module's `Service`) must be exported.

### When to use multiple topic files

Default to one. Split into multiple topic files **only when all** of these hold:

- The two areas are **distinct concepts**, not two views of the same aggregate.
- They **share no types** — neither references the other's types in its own definitions.
- **No methods cross them** — there is no method that needs to reach across the split.

If any of those fail, it's one topic, one file.

Size is a *signal* that splitting might be worth investigating, not a *reason* to split. A 500-line topic file that meets the rules above stays one file.

Canonical example of a justified split: in `categorization`, `category.go` (the taxonomy — built-in and custom categories, archive state, ids) and `rule.go` (the substring-match rule engine — matching cleaned merchant names to outcomes). They are genuinely independent concepts: a Category is a value in a taxonomy, a Rule is a matcher; they share no types and no methods cross them.

Canonical example of a *wrong* split: separating `transactions` into `transaction.go` + `classification.go` + `view.go`. A Transaction's Classification and Category are fields of the Transaction, and the list views slice over the same aggregate — they share types (`TransactionDTO` carries its `Classification`; `TransactionDTOs` is a slice of `TransactionDTO`) and methods cross them (`TransactionDTOs.TotalSpending` reads each row's classification). One `transactions.go` file is correct.

## Where shared logic lives

Invariants and shared rules land in different places depending on what they touch:

- **Pure-value rules** (validation of a value, a state-machine transition, formatting a domain ID) — method on the domain type in `<package>.go`. Stateless, no DB, callable from anywhere in the module.
- **Service-level rules reused across methods of the same module** (a read+check several public methods perform before mutating) — private method on `*Service`.
- **Cross-module rules** (a constraint that depends on data owned by another module) — method on the *owning* module's `*Service`, called by peers via the injected service. The rule lives where the data lives. For example, the `budget` module reads Transactions through the `transactions` service rather than querying the transactions table directly; the `reporting` and `tracker` utilities consume the same way.

When several service methods all do the same read → guard → write dance, the right move is often *not* extracting a private helper — it's that there's a missing domain verb. Repeated guards are a signal that the callers should converge on one bigger operation rather than each composing the same primitives. Shared helpers fix duplication; a missing concept fixes the *reason* for the duplication.

## Allowed imports

- `core/*` sub-packages.
- Other domain modules' **exported** `Service` types and DTO / value-object types, injected via the constructor.
- Stdlib and small focused third-party libraries (e.g. `github.com/google/uuid`).

## Forbidden imports

- `core/db/sqlc` from any file other than `repo.go`.
- Other domain modules' `adapters/` packages (e.g. `transactions/adapters` may not be imported by `budget`).
- Other domain modules' unexported types or non-`Service` internal helpers.
- External-client modules from anywhere — and even where allowed (in the domain module that owns the bank integration), only via constructor injection of the client's `*Service` behind the `BankProvider` interface.

## Adapters

- HTTP handlers are methods on `HttpHandler` in `adapters/http.go`. The struct's fields are the peer `*Service` types it needs; a constructor `NewHttpHandler(...)` takes them.
- Templ components live in `adapters/views/*.templ` and are generated to `*_templ.go` by `task build/templ`. Never edit `*_templ.go` by hand. The `views/` sub-package is its own Go package (`package views`); handlers in `http.go` import it and call components by qualified name (e.g. `views.OverviewPage(...)`). Design-side rules for the templ files themselves live in [`docs/design/`](../../design/).
- URL patterns and route registration live in `adapters/routes.go`. Signature:

  ```go
  func RegisterRoutes(mux *httpx.Mux, h *HttpHandler) {
      mux.Handle("GET /transactions/{id}", httpx.HandlerFunc(h.GetTransaction))
      // ...
  }
  ```

  `server/` passes the appropriate mux (root mux for public routes, app sub-mux for authenticated routes). The module decides which paths and methods bind to which handler methods.
- Adapters import their own module's `*Service` and peer modules' `*Service` / DTO types **only**. They do not import `repo.go`, `sqlc`, or peer modules' `adapters/` (or `adapters/views/`) packages.
- Error responses go through `httpx.HandleErrorResponse`. Responses are HTML fragments for HTMX consumption.

## Background tasks

- Background tasks live in `task.go` at the module root.
- They implement the `core/task.Task` interface (`Run`, `Schedule`, `Name`).
- Constructors are named `NewXxxTask(service *Service, ...) task.Task`.
- Tasks call into the module's own `*Service`, never directly into the repo. The recurring ~6h bank sync, for instance, lives in the `transactions` module's `task.go` and drives the module's `*Service`.
- For working examples, look at existing `task.go` files in `src/internal/`.

## Where new code goes

| Change | File |
|---|---|
| New SQL query | `repo.go` (and add the `.sql` file under `db/queries/`, then `task build/sqlc`) |
| New business-logic method | `service.go` |
| New domain type / pure function | `<package>.go` at module root (or the existing topic file if there's a justified split) |
| New HTTP handler | `adapters/http.go` |
| New URL route | `adapters/routes.go` |
| New templ component | `adapters/views/<name>_page.templ` or `_frag.templ` per [`docs/design/`](../../design/), then `task build/templ` |
| New background task | `task.go` (implements `core/task.Task`) |

## Module sizing & splits

Line count is a trigger to look, not a reason to split. The actual question is whether the module owns one coherent responsibility.

*Indicators worth investigating* (any one prompts a closer look):
- `service.go` materially larger than peers.
- Frequent edits to the module touch only one of its sub-areas (high churn asymmetry).
- A new feature in this area routinely changes only a subset of the module.
- The module is imported from another module mostly for the needs of one specific sub-area.

*Justification to split* (must hold to actually recommend a split):
- The module owns multiple distinct domain concepts that don't share state.
- One sub-area would be reusable from a module that doesn't currently use this one.
- The sub-areas can be described independently without referring to each other.

*Don't split when*
- The two halves would constantly call each other — that is one aggregate, not two modules.
- One side is just data the other side owns — that is an internal type, not a module.
