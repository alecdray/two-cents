# The safety margin becomes a stored override, the env var its factory default

The sweep's safety margin can now be overridden from the UI, persisted, and
read fresh on every run; the env var that previously fixed it at boot becomes
the value used only while no override is stored.

A margin fixed at boot meant changing it needed a redeploy, which doesn't fit
a figure that is tuned by feel rather than decided once. Storing the override
nullable rather than replacing the env var outright keeps a fresh deploy
zero-config, and mirrors the declared-override-vs-computed-default shape
[ADR-0031](0031-scheduled-items-bootstrap-and-resolve-from-matches.md) already
established: absence is itself meaningful (use the factory default), not a
failure to supply a value. The override is read on the same per-run clock
read as everything else the sweep derives from, so a change between two runs
never retroactively describes an earlier one — a past snapshot keeps the
margin it actually ran with.

Rejected: a general-purpose settings store. The margin is the sweep's own
concept and its only consumer, so it stores the override itself rather than
standing up a cross-cutting settings module for one value.
