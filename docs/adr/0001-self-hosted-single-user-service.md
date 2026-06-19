# Self-hosted single-user service, mirroring wax

Two Cents runs as a single-user, self-hosted backend — one Go binary backed by SQLite, deployed as a single Docker container the user runs on infrastructure they control — and adopts the stack and archetype system of the sibling [`wax`](../../../wax) project wholesale rather than reinventing them.

This shape is forced by two locked requirements: interval sync (every ~6h) needs an always-on process a desktop app opened occasionally can't provide; and the bank secrets — the Plaid app credentials and a per-Item `access_token` per connection — must stay on infrastructure the user owns, not a third-party cloud. The DB file and those secrets live on a mounted Docker volume so they survive restarts and rebuilds, and that volume is the encryption + backup target. The one deviation from wax is auth: single-user means a single local login, not wax's third-party OAuth (see [ADR-0007](0007-single-local-login.md)).

Rejected: a hosted cloud app (trusts a third party with bank credentials); a local desktop app (degrades "every 6h" to "while open"); diverging from wax's stack for no benefit at single-user scale.
