# Two Cents

A personal finance app that pulls your own bank transactions and balances (via Plaid) and makes spending legible: aggregation, budgeting, and month tracking. Self-hosted, single-user. *Your two cents on your own spending.*

Stack and architecture mirror the sibling project `wax`: **Go + templ + htmx**, Tailwind v4 + DaisyUI + Bootstrap Icons (mobile-first), **SQLite** (mattn/go-sqlite3) with **goose** migrations + **sqlc**, **robfig/cron** scheduling, packaged as a Docker container.

## Docs

- [`docs/product/`](docs/product/README.md) — product vision & feature set (start with [`vision.md`](docs/product/vision.md))
- [`docs/backlog/`](docs/backlog/) — deferred features and known bugs
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

`task` with no arguments lists all targets. See [`AGENTS.md`](AGENTS.md) for conventions.

## Status

Skeleton scaffolded — a hello-world page validates the full pipeline (templ → Tailwind → httpx → goose → Docker). The `plaid` external-client and the `banking` provider seam (the `BankProvider` interface + domain types) are in place; the domain modules (`accounts`, `transactions`, `categorization`, `budget`, `tracker`, `reporting`) are not yet built.
