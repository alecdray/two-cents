# Configurable safety margin

The sweep's safety margin — the flat cushion held back on top of the required
checking figure — is currently fixed at boot from the `FIXED_SAFETY_MARGIN`
env var and can only change on a redeploy. This work moves it into the
database as an ordinary editable value, read fresh on every run and edited
from the `/sweep` page itself; the env var is removed entirely, a migration
seeding the stored value at the $500 it used to default to. See
[ADR-0032](../../adr/0032-safety-margin-is-a-stored-value.md) for the
decision and its rationale.
