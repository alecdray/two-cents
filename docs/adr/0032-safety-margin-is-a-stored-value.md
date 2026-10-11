# The safety margin becomes a stored value, replacing the env var

The sweep's safety margin is now a value stored in the database, editable from
the UI and read fresh on every run. `FIXED_SAFETY_MARGIN` is removed; a
migration seeds the stored value at the $500 the env var itself defaulted to.

A margin fixed at boot meant changing it needed a redeploy, which doesn't fit
a figure that is tuned by feel rather than decided once. Reading it fresh per
run, on the same per-run clock read as everything else the sweep derives
from, means a change between two runs never retroactively describes an
earlier one — a past snapshot keeps the margin it actually ran with.

Removing the env var outright, rather than keeping it as a fallback behind a
nullable override (the shape [ADR-0031](0031-scheduled-items-bootstrap-and-resolve-from-matches.md)
uses for a scheduled item's amount), was a deliberate choice: two inputs for
one figure invites exactly the drift that invites — a deployment's env var
quietly re-asserting itself under a value the UI shows as current, or a
change behind both a redeploy and an on-screen save making "what will the
*next* run actually use" unanswerable without reading both. One stored value
has one place to look. The migration cannot read the process environment (a
goose migration is pure SQL), so a deployment that had customized the env var
away from its default is reset to $500 on upgrade — accepted as a one-time,
one-user cost, since the UI the same change adds is exactly where that value
is now set and seen.

Rejected: a general-purpose settings store. The margin is the sweep's own
concept and its only consumer, so it stores the value itself rather than
standing up a cross-cutting settings module for one field.
