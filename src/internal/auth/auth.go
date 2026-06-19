// Package auth is the application's gate: the single local credential and the
// login flow that issues a session (ADR-0007). It is a supporting module, not
// part of the financial domain model. The reusable session machinery (token,
// cookie, guard middleware) lives in core; this module owns the credential
// store and login orchestration.
package auth

import "errors"

// userID is the fixed primary key of the single credential row. The system is
// single-user by design (ADR-0007), so the one row is addressed by a constant
// id, mirroring the budget config's single-row convention.
const userID = "default"

// defaultUsername labels the seeded credential. Login is password-only, so the
// username is identity/display only — never challenged.
const defaultUsername = "owner"

var (
	// ErrNoCredential is returned when no credential has been seeded yet — the
	// "no account configured" state the login page surfaces.
	ErrNoCredential = errors.New("no credential configured")
	// ErrInvalidCredentials is returned when the submitted password does not
	// match the stored hash.
	ErrInvalidCredentials = errors.New("invalid credentials")
)

// credential is the stored login: its id, the (display-only) username, and the
// bcrypt password hash. The hash never leaves this package.
type credential struct {
	id       string
	username string
	hash     string
}
