# Two Cents

A personal finance app that pulls your own bank transactions and balances (via Plaid) and makes spending legible: aggregation, budgeting, and month tracking. Self-hosted, single-user. *Your two cents on your own spending.*

Stack and architecture mirror the sibling project `wax`: **Go + templ + htmx**, Tailwind v4 + DaisyUI + Bootstrap Icons (mobile-first), **SQLite** (mattn/go-sqlite3) with **goose** migrations + **sqlc**, **robfig/cron** scheduling, packaged as a Docker container.

## Docs

- [`docs/scope.md`](docs/scope.md) — direction and the bank-provider decision (Teller→Plaid)
- [`docs/prd.md`](docs/prd.md) — v1 features, modules, testing plan
- [`docs/domain/`](docs/domain/README.md) — domain model & glossary
- [`docs/architecture/`](docs/architecture/) · [`docs/design/`](docs/design/) · [`docs/adr/`](docs/adr/) — rules and decisions

## Develop

Requires Go 1.26+, Node, and the `templ`, `sqlc`, `goose`, `task` tools.

```sh
cp .env.template .env        # adjust if needed
task build                   # templ + tailwind + go build → ./bin/app
task run                     # or: ./bin/app  (serves http://127.0.0.1:4690)
task test/unit               # go test ./src/...
```

`task` with no arguments lists all targets. See [`.claude/CLAUDE.md`](.claude/CLAUDE.md) for conventions.

## Status

Skeleton scaffolded — a hello-world page validates the full pipeline (templ → Tailwind → httpx → goose → Docker). The `plaid` external-client and the `banking` provider seam (the `BankProvider` interface + domain types) are in place; the domain modules (`accounts`, `transactions`, `categorization`, `budget`, `tracker`, `reporting`) are not yet built.
