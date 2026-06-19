package app

import "net/http"

// App is the application handle threaded through requests and tasks. It holds
// the loaded Config and the session-token helpers for the single local login
// (ADR-0007 — single-user, no third-party OAuth).
type App struct {
	config Config
}

func NewApp(config Config) App {
	return App{config: config}
}

func (app App) Config() Config {
	return app.config
}

// IssueSession signs a fresh session for the given user and writes the cookie,
// sliding the expiry window. Called on login and re-issued by the auth
// middleware on each authenticated request.
func (app App) IssueSession(w http.ResponseWriter, userID string) error {
	return NewClaims(userID).save(app.config, w)
}

// RefreshSession re-signs an already-validated claim set, sliding the expiry.
func (app App) RefreshSession(w http.ResponseWriter, claims *Claims) error {
	return claims.save(app.config, w)
}

// ClearSession removes the session cookie (logout, or an invalid token).
func (app App) ClearSession(w http.ResponseWriter) {
	deleteCookie(app.config, w)
}

// SessionFromRequest validates the request's session cookie and returns its
// claims, or an error when absent/invalid.
func (app App) SessionFromRequest(r *http.Request) (*Claims, error) {
	return validateClaimsFromRequest(r, app.config.JwtSecret)
}
