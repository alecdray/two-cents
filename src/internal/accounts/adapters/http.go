package adapters

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/alecdray/two-cents/src/internal/accounts"
	"github.com/alecdray/two-cents/src/internal/accounts/adapters/views"
	"github.com/alecdray/two-cents/src/internal/banking"
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

// RepairTransfers re-resolves stored transfers after a kind/savings override that
// changed an account's effective counts-as-savings flag, so the change applies
// immediately instead of waiting for the next sync. Like BackfillTransactions it
// is the dependency-inverted seam that lets accounts trigger transactions work
// without importing it: the composition root injects an implementation driving
// transactions.RepairTransferSubtypes. A nil hook means no re-pair is wired and
// the post-action is skipped.
type RepairTransfers func(ctx contextx.ContextX) error

// HttpHandler serves the accounts module's pages. It holds the accounts Service
// and reads through it; it never reaches the bank provider directly. The bank
// mode tells the connect control whether to open the live provider UI or post to
// the deterministic stand-in. The backfill hook is the injected seam it calls
// after a successful connect/reconnect to pull the bank's transactions.
type HttpHandler struct {
	accountsService *accounts.Service
	bankMode        string
	backfill        BackfillTransactions
	repair          RepairTransfers
}

// The connect-control modes the composition root threads in, re-exported from
// the views package so the server names the mode through the module's adapter
// surface rather than reaching into its views.
const (
	BankModeReal = views.BankModeReal
	BankModeFake = views.BankModeFake
)

// NewHttpHandler builds the handler over the accounts Service, the bank mode the
// connect control renders against ("real" or "fake"), the injected backfill hook
// it runs after a successful connect/reconnect to pull the bank's transactions,
// and the injected re-pair hook it runs after a kind/savings override that
// changed counts-as-savings. A nil hook skips that post-action.
func NewHttpHandler(accountsService *accounts.Service, bankMode string, backfill BackfillTransactions, repair RepairTransfers) *HttpHandler {
	return &HttpHandler{accountsService: accountsService, bankMode: bankMode, backfill: backfill, repair: repair}
}

// GetOverviewPage renders the accounts overview at /accounts: the net cash
// position and the linked accounts grouped into cash, credit, and other.
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

// PostAccountKind overrides an account's spending bucket to the posted value and
// swaps the now-updated overview region back in — net cash recomputes, the row
// re-buckets, and its own controls re-render (the savings toggle appears or
// vanishes as the kind crosses the credit boundary). An override to credit that
// clears counts-as-savings re-pairs existing transfers through the injected seam.
// An unknown kind value is a 400.
func (h *HttpHandler) PostAccountKind(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())

	kind := banking.AccountKind(r.FormValue("kind"))
	savingsChanged, err := h.accountsService.SetAccountKind(ctx, r.PathValue("id"), kind)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, accounts.ErrInvalidKind) {
			status = http.StatusBadRequest
		}
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: status, Err: err})
		return
	}

	if savingsChanged {
		h.repairTransfers(ctx)
	}
	h.renderOverview(ctx, w)
}

// PostCountsAsSavings flips an account's counts-as-savings flag and swaps the
// overview region back in. The flag has no visible effect on the overview itself,
// but it changes downstream transfer pairing, so the flip re-pairs existing
// transfers through the injected seam — the Tracker reflects it at once. Toggling
// a credit account (withheld in the UI) is a 400.
func (h *HttpHandler) PostCountsAsSavings(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())

	savingsChanged, err := h.accountsService.ToggleCountsAsSavings(ctx, r.PathValue("id"))
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, accounts.ErrSavingsNotApplicable) {
			status = http.StatusBadRequest
		}
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: status, Err: err})
		return
	}

	if savingsChanged {
		h.repairTransfers(ctx)
	}
	h.renderOverview(ctx, w)
}

// renderOverview loads the dashboard and swaps the clean overview region back in
// (no inline errors), the shared tail of the kind/savings override handlers.
func (h *HttpHandler) renderOverview(ctx contextx.ContextX, w http.ResponseWriter) {
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

// repairTransfers runs the injected re-pair seam after a kind/savings override
// that changed the effective counts-as-savings flag, re-resolving stored
// transfers so the Tracker reflects the change at once. It mirrors
// backfillTransactions: a nil hook is a no-op, and a hook error is non-fatal
// (logged) — the override already committed and the next sync re-pairs anyway.
func (h *HttpHandler) repairTransfers(ctx contextx.ContextX) {
	if h.repair == nil {
		return
	}
	if err := h.repair(ctx); err != nil {
		slog.ErrorContext(ctx, "failed to re-pair transfers after account override", "error", err)
	}
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
