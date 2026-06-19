package adapters

import (
	"github.com/alecdray/two-cents/src/internal/core/httpx"
)

// RegisterRoutes mounts the public login screen and the login/logout actions.
// These sit outside the auth middleware (they are how a request becomes
// authenticated); every other route is registered on the guarded mux.
func RegisterRoutes(mux *httpx.Mux, h *HttpHandler) {
	mux.HandleFunc("GET /login", httpx.HandlerFunc(h.GetLoginPage))
	mux.HandleFunc("POST /login", httpx.HandlerFunc(h.PostLogin))
	mux.HandleFunc("GET /logout", httpx.HandlerFunc(h.Logout))
}
