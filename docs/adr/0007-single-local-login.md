# Single local login

The whole single-user app sits behind one **password-only** local login. The credential is a hashed password in its own DB table (not config), the session is a signed token in a sliding `HttpOnly` cookie, and no domain data is partitioned by user — so authorization is binary: a valid session reaches everything, an invalid one only the login screen.

Every page was otherwise open to anyone who could reach the port, and a username challenge is pure ceremony for a single user. The credential lives in the DB — beside the encrypted bank tokens, on the same mounted volume — rather than an env var so it can be rotated by a CLI command (the same operational model as migrations), with no in-app account screen. The bank `access_token`s never enter the session token, so the cookie's blast radius is "view my finances," not "move my money" — which is why a generous sliding lifetime is an acceptable trade. Deliberately absent for v1: login rate-limiting/lockout, CSRF defence beyond the cookie's same-site restriction, and any second user or role.

Rejected: a hashed password in an env var (no in-app rotation path); a username + password challenge (ceremony for one user); an in-app first-run wizard to set the password (an unauthenticated write endpoint); disabling the gate in test builds (leaves it untested).
