# Configurable safety margin

The sweep's safety margin — the flat cushion held back on top of the required
checking figure — is currently fixed at boot from the `FIXED_SAFETY_MARGIN`
env var and can only change on a redeploy. This work lets the margin be
overridden from the `/sweep` page itself, taking effect on the very next run
(scheduled or on-demand) with no restart. The env var becomes the factory
default; a stored override takes precedence over it, mirroring the
declared-override-vs-computed-default shape [ADR-0031](../../adr/0031-scheduled-items-bootstrap-and-resolve-from-matches.md)
already established for a scheduled item's amount. See
[ADR-0032](../../adr/0032-safety-margin-becomes-a-stored-override.md) for the
decision and its rationale.
