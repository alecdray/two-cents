# Custom account names

A user can rename any Account to a name of their own from the accounts overview, overriding the bank-reported name everywhere the Account is shown.

The override is a **nullable `custom_name` column**, not the boolean-flag pattern the `kind` / `counts-as-savings` overrides use ([ADR-0008](0008-account-kind-and-savings-overrides.md)). The bank name keeps living in `name` and is still refreshed on every sync; `custom_name` being non-NULL *is* the override signal, so sync simply never touches it — no second column to keep honest. A few rules fall out:

- **One precedence point.** A single resolver returns `custom_name` when set, else the bank `name`. Every read renders that resolved display name; nothing else branches on whether a custom name exists.
- **Shown everywhere through the existing chokepoints.** The overview rows and the shared connected-account lookup that feeds the pickers both switch to the display name, so the transactions edit-modal account picker and the transfer-pairing pass inherit the custom name with no per-surface change. The bank `name` is no longer rendered anywhere; the `mask` (last-4) still disambiguates same-named accounts.
- **Empty clears.** The rename operation trims its input, caps it at 60 characters, and treats an empty result as "clear" — `custom_name` back to NULL, reverting to the bank name. Renaming is a pure accounts write that touches neither `kind` nor `counts-as-savings`, so it fires no transfer re-pair.

Rejected: a `name_overridden` boolean mirroring the kind/savings overrides (the synced name and the custom name live in *different* columns here, so nullability already carries the signal — a flag would be a redundant second source of truth); rendering the bank name as subtext beside the custom name (the mask already disambiguates, and the bank name is the synced source of truth, not display chrome).
