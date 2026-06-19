package adapters

import (
	"errors"
	"net/http"

	"github.com/alecdray/two-cents/src/internal/auth"
	"github.com/alecdray/two-cents/src/internal/auth/adapters/views"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/httpx"
)

// HttpHandler serves the login screen and the login/logout actions. It reads and
// writes the session through the App (the token + cookie machinery) and checks
// credentials through the auth Service; it never reaches storage directly.
type HttpHandler struct {
	auth *auth.Service
}

// NewHttpHandler builds the handler over the auth Service.
func NewHttpHandler(authSvc *auth.Service) *HttpHandler {
	return &HttpHandler{auth: authSvc}
}

// GetLoginPage renders the login screen. An already-authenticated request is
// redirected home; otherwise the form renders, or a "no account configured"
// prompt when no credential has been seeded.
func (h *HttpHandler) GetLoginPage(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())
	a, err := ctx.App()
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}

	if claims, err := a.SessionFromRequest(r); err == nil && claims != nil && claims.UserID != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	configured, err := h.auth.HasCredential(ctx)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}
	views.LoginPage(views.LoginProps{Configured: configured}).Render(ctx, w)
}

// PostLogin verifies the submitted password. On success it issues the session
// cookie and redirects home; on a bad (or absent) credential it re-renders the
// login screen with one inline message — deliberately not distinguishing "no
// account" from "wrong password".
func (h *HttpHandler) PostLogin(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())
	a, err := ctx.App()
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}

	userID, err := h.auth.Authenticate(ctx, r.FormValue("password"))
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) || errors.Is(err, auth.ErrNoCredential) {
			configured := !errors.Is(err, auth.ErrNoCredential)
			views.LoginPage(views.LoginProps{Configured: configured, Error: "Incorrect password."}).Render(ctx, w)
			return
		}
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}

	if err := a.IssueSession(w, userID); err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// Logout clears the session cookie and returns to the login screen.
func (h *HttpHandler) Logout(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())
	a, err := ctx.App()
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}
	a.ClearSession(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
