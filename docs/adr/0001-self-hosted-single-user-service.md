# Self-hosted single-user service, mirroring wax

Two Cents runs as a **single-user, self-hosted backend** — one Go binary backed by **SQLite**, deployed as one Docker container the user runs on infrastructure they control (home server or small VPS).

This shape is forced by two locked requirements: **interval sync (every ~6h)** needs an always-on process, which a desktop app opened occasionally can't provide; and the bank **secrets** — the Plaid app credentials (`client_id` + `secret`) and a per-Item `access_token` per connection — are secrets we don't want sitting in a third-party cloud. (Keeping these secrets on owned infra was originally a reason Teller was chosen over a hosted aggregator; the privacy posture is unchanged now that the provider is Plaid — see [ADR-0002](0002-bankprovider-abstraction.md).) The DB file and these secrets live on a **mounted Docker volume** (not baked into the image) so they survive restarts/rebuilds; that volume is the encryption + backup target.

**The stack and architecture mirror the sibling project [`wax`](../../../wax)** (`projects/wax`), which is a mature Go + templ + htmx + SQLite app with the same single-binary/self-hosted shape. We adopt its conventions wholesale rather than reinvent them:

- **Go** + **templ** (server-rendered HTML, compiled) + **htmx** (interaction over HTTP fragments) + **Alpine.js** for ephemeral client state. No web framework — a custom `httpx.Mux` like wax's.
- **CSS:** Tailwind v4 + DaisyUI (custom theme) + Bootstrap Icons, mobile-first. Tokens-not-raw-colors.
- **DB:** SQLite via **mattn/go-sqlite3** (cgo — matches wax; the multi-stage Docker build carries the toolchain), **goose** migrations (`db/migrations`), **sqlc** type-safe queries (`db/queries` → generated package; no sqlx), text UUID ids via `google/uuid`.
- **Scheduling:** **robfig/cron/v3** behind a `core/task.Task` archetype, registered in the `server/` composition root — this is what runs the 6h sync.
- **Config:** `.env` via `godotenv`. **Build/dev:** **Task** (`taskfile.yml`) targets (`build/*`, `dev`, `db/up`, `test/unit`, `test/e2e`, `docker/*`) — never invoke tools directly.
- **Docs + code layout** mirror wax's archetype system (see ADR-0002 and the PRD architecture section).

One deviation from wax: wax authenticates via Spotify OAuth + JWT; Two Cents is single-user, so auth is a **single local login (JWT)**, no third-party OAuth.

Rejected: a hosted cloud app — trusts a third party with bank credentials; a local desktop app — degrades "every 6h" to "while open." Postgres / modernc driver — diverge from wax for no benefit at single-user scale.
