# Single configured timezone for time reckoning

Several domain values are derived from "now": which calendar month is current, how many days are left in it (the pace target divides by this), and therefore what the tracker shows. These are all reckoned in a **single configured app timezone** — a persisted setting, default **EST** — rather than from a per-request browser timezone.

The tempting alternative is to read the viewer's zone from each request, so the app tracks wherever the user physically is. We rejected it for three reasons:

- **Background jobs have no request.** The recurring sync and the orphaned-pending reaper run on a schedule with no HTTP request in scope, so a request-supplied zone can't reach them. A server-side zone is needed regardless — and having two sources (header for pages, fallback for cron) invites them to disagree.
- **Month boundaries must be stable.** A browser zone shifts with the user's device and location; opening the app from another timezone could move a boundary transaction into a different month and flip "did I stay under budget this month?" Personal-finance months should track the user's home financial life, not their physical location at read time.
- **It keeps two clocks aligned.** Bank transaction dates are zoneless calendar dates, and a transaction is assigned to a month by that date as-is. Reckoning "today" in the same single zone means the tracker's clock and the month buckets are both calendar-date-based in one zone, so no off-by-one opens at the boundary.

Consequences: there is one source of truth for "now" that both rendering and scheduled work share. Changing the configured zone shifts what "the current month" and "days left" mean going forward; historical month buckets, being calendar-date based, are unaffected. The browser's detected zone may *suggest* a default at setup, but it never drives a live request.

Rejected: per-request browser timezone (requires client-side detection plus a custom header or cookie on every request, still needs a server-side fallback for scheduled work, and makes month boundaries depend on where the user happens to be).
