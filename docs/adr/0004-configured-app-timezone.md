# Single configured timezone for time reckoning

"Now" — the current calendar month, the days left in it (the pace target divides by this), and therefore what the tracker shows — is reckoned in a single configured app timezone (a persisted setting, default EST), not a per-request browser zone.

Background jobs (the recurring sync, the orphaned-pending reaper) run with no HTTP request in scope, so a request-supplied zone can't reach them — a server-side zone is needed regardless, and two sources would invite disagreement. Bank transaction dates are zoneless calendar dates assigned to a month as-is, so reckoning "today" in that same zone keeps the tracker's clock and the month buckets aligned with no boundary off-by-one; a browser zone, shifting with the user's location, could otherwise move a transaction into a different month and flip "did I stay under budget?". Changing the configured zone shifts "current month" and "days left" going forward; historical buckets, being calendar-date based, are unaffected.

Rejected: a per-request browser timezone — needs client detection plus a custom header on every request, still needs a server-side fallback for scheduled work, and makes month boundaries depend on where the user physically is.
