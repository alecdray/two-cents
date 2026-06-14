package adapters

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/alecdray/two-cents/src/internal/accounts"
	"github.com/alecdray/two-cents/src/internal/accounts/adapters/views"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/httpx"
)

// BackfillTransactions triggers a transaction backfill for the banks now in
// scope, right after a successful connect or reconnect. It is the
// dependency-inverted seam that lets the accounts adapter kick off a sync
// without importing the transactions module: the composition root injects an
// implementation that drives transactions.SyncTransactions, so the module graph
// stays acyclic (transactions imports accounts, never the reverse). A nil hook
// means no backfill is wired and the post-action is skipped.
type BackfillTransactions func(ctx contextx.ContextX) error

// HttpHandler serves the accounts module's pages. It holds the accounts Service
// and reads through it; it never reaches the bank provider directly. The bank
// mode tells the connect control whether to open the live provider UI or post to
// the deterministic stand-in. The backfill hook is the injected seam it calls
// after a successful connect/reconnect to pull the bank's transactions.
type HttpHandler struct {
	accountsService *accounts.Service
	bankMode        string
	backfill        BackfillTransactions
}

// The connect-control modes the composition root threads in, re-exported from
// the views package so the server names the mode through the module's adapter
// surface rather than reaching into its views.
const (
	BankModeReal = views.BankModeReal
	BankModeFake = views.BankModeFake
)

// NewHttpHandler builds the handler over the accounts Service, the bank mode the
// connect control renders against ("real" or "fake"), and the injected backfill
// hook it runs after a successful connect/reconnect to pull the bank's
// transactions. A nil backfill skips that post-action.
func NewHttpHandler(accountsService *accounts.Service, bankMode string, backfill BackfillTransactions) *HttpHandler {
	return &HttpHandler{accountsService: accountsService, bankMode: bankMode, backfill: backfill}
}

// GetOverviewPage renders the root accounts overview: the net cash position and
// the linked accounts grouped into cash, credit, and other.
func (h *HttpHandler) GetOverviewPage(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())

	dashboard, err := h.accountsService.Dashboard(ctx)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{
			Status: http.StatusInternalServerError,
			Err:    err,
		})
		return
	}

	views.AccountsOverviewPage(dashboard, h.bankMode).Render(ctx, w)
}

// GetConnectLinkToken mints a link token for the real-mode connect interceptor.
// The front end hands the token to the provider's connect flow; the response
// carries the token and the provider mode.
func (h *HttpHandler) GetConnectLinkToken(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())

	token, err := h.accountsService.BeginConnect(ctx)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{
			Status: http.StatusInternalServerError,
			Err:    err,
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"token": token.Token, "mode": token.Mode})
}

// PostConnection completes an enrollment: it exchanges the posted public token
// for a durable bank login, registers the connection, and swaps the now-updated
// overview region back in. A failed exchange renders the same region with a
// recoverable inline error in the connect control — no redirect, no full-page
// replacement — leaving the existing accounts in view.
func (h *HttpHandler) PostConnection(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())

	publicToken := r.FormValue("public_token")

	if _, err := h.accountsService.CompleteConnect(ctx, publicToken); err != nil {
		slog.ErrorContext(ctx, "failed to connect bank", "error", err)
		dashboard, derr := h.accountsService.Dashboard(ctx)
		if derr != nil {
			httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{
				Status: http.StatusInternalServerError,
				Err:    derr,
			})
			return
		}
		views.OverviewContentFrag(dashboard, h.bankMode, "We couldn't link your bank. Please try again.", "", "").Render(ctx, w)
		return
	}

	// The bank is linked; pull its transactions so they're available immediately
	// without a manual sync. A backfill failure is non-fatal — the connection
	// already succeeded and the recurring sync will catch it up — so it is logged
	// and the successful connect render proceeds.
	h.backfillTransactions(ctx)

	dashboard, err := h.accountsService.Dashboard(ctx)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{
			Status: http.StatusInternalServerError,
			Err:    err,
		})
		return
	}

	views.OverviewContentFrag(dashboard, h.bankMode, "", "", "").Render(ctx, w)
}

// backfillTransactions runs the injected backfill hook after a successful
// connect/reconnect, pulling the now-available transactions. It swallows the
// hook's error (logging it) so a backfill failure never breaks the response to a
// connect/reconnect that already succeeded; the recurring sync will catch up. A
// nil hook (none wired) is a no-op.
func (h *HttpHandler) backfillTransactions(ctx contextx.ContextX) {
	if h.backfill == nil {
		return
	}
	if err := h.backfill(ctx); err != nil {
		slog.ErrorContext(ctx, "failed to backfill transactions after connect", "error", err)
	}
}

// DeleteConnection disconnects a bank: it removes the connection's login at the
// provider and deletes its accounts and the connection, then swaps the
// now-updated overview region back in. Removing the only linked bank empties the
// overview, so the region returns to the empty state with its connect control.
func (h *HttpHandler) DeleteConnection(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())

	if err := h.accountsService.Disconnect(ctx, r.PathValue("id")); err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{
			Status: http.StatusInternalServerError,
			Err:    err,
		})
		return
	}

	dashboard, err := h.accountsService.Dashboard(ctx)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{
			Status: http.StatusInternalServerError,
			Err:    err,
		})
		return
	}

	views.OverviewContentFrag(dashboard, h.bankMode, "", "", "").Render(ctx, w)
}

// GetReconnectLinkToken mints an update-mode link token for the real-mode
// reconnect interceptor. The front end hands the token to the provider's update
// flow; the response carries the token and the provider mode.
func (h *HttpHandler) GetReconnectLinkToken(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())

	token, err := h.accountsService.BeginReconnect(ctx, r.PathValue("id"))
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{
			Status: http.StatusInternalServerError,
			Err:    err,
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"token": token.Token, "mode": token.Mode})
}

// PostReconnect completes a reconnect: it confirms the refreshed login works and
// clears the connection's needs-reconnect flag, then swaps the overview region
// back in (the badge gone). A still-rejected login renders the same region with
// a recoverable inline error beside that connection's row — no redirect, no
// full-page replacement — with the connection still flagged.
func (h *HttpHandler) PostReconnect(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())

	connectionID := r.PathValue("id")

	if err := h.accountsService.CompleteReconnect(ctx, connectionID); err != nil {
		slog.ErrorContext(ctx, "failed to reconnect bank", "error", err)
		dashboard, derr := h.accountsService.Dashboard(ctx)
		if derr != nil {
			httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{
				Status: http.StatusInternalServerError,
				Err:    derr,
			})
			return
		}
		views.OverviewContentFrag(dashboard, h.bankMode, "", connectionID, "We couldn't reconnect your bank. Please try again.").Render(ctx, w)
		return
	}

	// The login is restored; pull the transactions that were unavailable while
	// the connection was flagged. Same non-fatal treatment as connect: a backfill
	// failure is logged and the successful reconnect render proceeds.
	h.backfillTransactions(ctx)

	dashboard, err := h.accountsService.Dashboard(ctx)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{
			Status: http.StatusInternalServerError,
			Err:    err,
		})
		return
	}

	views.OverviewContentFrag(dashboard, h.bankMode, "", "", "").Render(ctx, w)
}
