# Known Architectural Gaps

This doc tracks current architectural violations in the codebase — places where the rules in [`archetypes/`](archetypes/) (and module `CLAUDE.md` files) don't yet match reality. Each entry describes the gap, why it exists, and what closing it would require.

Gaps are tracked here (and not enumerated inside the archetype docs themselves) so the rule docs stay durable and conceptual, while the concrete list of divergences lives in one searchable place.

When a module ends up out of compliance with its archetype (a peer reaching into another module's `adapters/`, a non-`repo.go` file importing `sqlc`, an external client growing persistence), record it here rather than weakening the archetype doc. Each entry names the rule violated, where, why it exists, and what closing it would require.

## Account disconnect hard-deletes instead of transitioning to `closed`

**Rule:** the domain [`Account` state machine](../domain/README.md) has a terminal **`closed`** state; an Account should be retired by transitioning to `closed`, not removed.

**Reality:** `accounts.Disconnect` **hard-deletes** the connection's Account rows (it does not set them `closed`) while leaving the Transactions that reference them in place. sqlite foreign keys are declarative-only here — `core/db` opens the database without `_foreign_keys=on` — so nothing rejects the resulting dangle.

**Consequence:** a saved `transfer_destination_account_id` that points at a now-deleted Account dangles. The destination-name JOIN returns empty, so a past **Savings contribution** to that account renders with a **blank destination name**. Display-only — the contribution is still summed correctly.

**Closing it:** transition disconnected Accounts to `closed` (preserve the rows) instead of deleting them, so transfer-destination references stay resolvable. Do **not** paper over it with an FK cascade — that would delete the historical transfers too.

## Overview fragment defined in the page templ, not its own `*_frag.templ`

**Rule:** per [fragment-templ.md](../design/archetypes/fragment-templ.md), content reachable as both a page and an HTMX swap target lives in a `<region>_frag.templ` file alongside the page, not inside the page templ.

**Reality:** `OverviewContentFrag` (the `/accounts` overview swap region) and its child templs live in `accounts/adapters/views/accounts_overview_page.templ`. It is swapped in by six handlers (connect, disconnect, reconnect, kind, counts-as-savings, visibility) yet shares the page file with `AccountsOverviewPage`.

**Consequence:** none functionally — the fragment renders and swaps correctly. It is a file-layout divergence that makes the page file carry both archetypes.

**Closing it:** move `OverviewContentFrag` and its helper templs into a new `accounts_overview_frag.templ`, leaving the page templ to call it. Mechanical (re-run `task build/templ`, re-run the gate); deferred to keep the feature slice that surfaced it focused.
