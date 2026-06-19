package httpx

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/alecdray/two-cents/src/internal/core/contextx"
)

type Middleware func(HandlerFunc) HandlerFunc

func ApplyMiddleware(handler HandlerFunc, middlewares ...Middleware) HandlerFunc {
	for _, middleware := range middlewares {
		handler = middleware(handler)
	}
	return handler
}

// loginPath is where an unauthenticated request is sent. Public (login, static)
// routes are registered outside this middleware; everything else it guards.
const loginPath = "/login"

// JwtMiddleware guards a route group behind the single local login (ADR-0007).
// It validates the session cookie, puts the user id into the request context,
// and re-issues the cookie to slide the expiry. Authorization is binary: a
// valid session passes through, anything else is redirected to the login page
// (the cookie cleared on the way out). HTMX requests get an HX-Redirect so the
// client navigates rather than swapping the login page into a fragment.
func JwtMiddleware(next HandlerFunc) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := contextx.NewContextX(r.Context())
		a, err := ctx.App()
		if err != nil {
			HandleErrorResponse(ctx, w, HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
			return
		}

		claims, err := a.SessionFromRequest(r)
		if err != nil || claims == nil || claims.UserID == nil {
			a.ClearSession(w)
			redirectToLogin(w, r)
			return
		}

		if err := a.RefreshSession(w, claims); err != nil {
			HandleErrorResponse(ctx, w, HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
			return
		}

		ctx = ctx.WithUserId(*claims.UserID)
		next(w, r.WithContext(ctx))
	}
}

// redirectToLogin sends the caller to the login page: an HX-Redirect header for
// HTMX requests (so the whole page navigates), a 303 otherwise.
func redirectToLogin(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", loginPath)
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, loginPath, http.StatusSeeOther)
}

type RequestLoggingMiddlewareResponseWriter struct {
	http.ResponseWriter
	statusCode int
	startTime  time.Time
}

func (w *RequestLoggingMiddlewareResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *RequestLoggingMiddlewareResponseWriter) Duration() time.Duration {
	return time.Since(w.startTime)
}

func RequestLoggingMiddleware(next HandlerFunc) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ww := &RequestLoggingMiddlewareResponseWriter{ResponseWriter: w, statusCode: 200, startTime: time.Now()}
		next(ww, r)
		slog.InfoContext(r.Context(), "Request", "status", ww.statusCode, "method", r.Method, "path", r.URL.Path, "url", r.URL.String(), "duration", ww.Duration())
	}
}
