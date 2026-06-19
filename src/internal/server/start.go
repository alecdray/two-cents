package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/alecdray/two-cents/src/internal/core/app"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/db"
	"github.com/alecdray/two-cents/src/internal/core/httpx"
	"github.com/alecdray/two-cents/src/internal/core/templates"

	accountsAdapters "github.com/alecdray/two-cents/src/internal/accounts/adapters"
	authAdapters "github.com/alecdray/two-cents/src/internal/auth/adapters"
	budgetAdapters "github.com/alecdray/two-cents/src/internal/budget/adapters"
	categorizationAdapters "github.com/alecdray/two-cents/src/internal/categorization/adapters"
	homeAdapters "github.com/alecdray/two-cents/src/internal/home/adapters"
	transactionsAdapters "github.com/alecdray/two-cents/src/internal/transactions/adapters"
)

func Start(ctx context.Context, app app.App) {
	database, err := db.NewDB(app.Config().DbPath)
	if err != nil {
		slog.Error("Failed to create database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	services, err := NewServices(app, database)
	if err != nil {
		slog.Error("Failed to construct services", "error", err)
		os.Exit(1)
	}
	services.taskManager.Start(contextx.NewContextX(ctx).WithApp(app))
	defer services.taskManager.Stop()

	templates.InitCSSVersion("static/public/main.css")

	// Two mux layers (ADR-0007). The root mux carries only logging and the public
	// surface — static assets and the login/logout actions, which are how a
	// request becomes authenticated. Everything else mounts on the protected
	// sub-mux behind JwtMiddleware: the catch-all "/" prefix routes every
	// non-public path through the gate, while the more-specific public patterns
	// above win for their own paths.
	rootMux := httpx.NewMux(app, httpx.RequestLoggingMiddleware)
	rootMux.Handle("/static/", httpx.WrapHandler(http.StripPrefix("/static/", http.FileServer(http.Dir("static/public")))))

	authHandler := authAdapters.NewHttpHandler(services.authService)
	authAdapters.RegisterRoutes(rootMux, authHandler)

	appMux := httpx.NewMux(app, httpx.JwtMiddleware)

	// The connect/reconnect handlers trigger the initial transaction backfill
	// through this injected seam — the only place both services are in scope —
	// so accounts never imports transactions and the module graph stays acyclic.
	backfillTransactions := func(ctx contextx.ContextX) error {
		return services.transactionsService.SyncTransactions(ctx)
	}
	// The kind/savings override handlers re-pair stored transfers through this
	// second seam (same acyclic reasoning), so a counts-as-savings change reflects
	// in the Tracker immediately rather than at the next sync.
	repairTransfers := func(ctx contextx.ContextX) error {
		return services.transactionsService.RepairTransferSubtypes(ctx)
	}
	accountsHandler := accountsAdapters.NewHttpHandler(services.accountsService, services.bankMode, backfillTransactions, repairTransfers)
	accountsAdapters.RegisterRoutes(appMux, accountsHandler)

	transactionsHandler := transactionsAdapters.NewHttpHandler(services.transactionsService, services.accountsService, services.categorizationService)
	transactionsAdapters.RegisterRoutes(appMux, transactionsHandler)

	categorizationHandler := categorizationAdapters.NewHttpHandler(services.categorizationService)
	categorizationAdapters.RegisterRoutes(appMux, categorizationHandler)

	budgetHandler := budgetAdapters.NewHttpHandler(services.budgetService, services.categorizationService)
	budgetAdapters.RegisterRoutes(appMux, budgetHandler)

	homeHandler := homeAdapters.NewHttpHandler(services.homeService)
	homeAdapters.RegisterRoutes(appMux, homeHandler)

	rootMux.Use("/", appMux)

	addr := fmt.Sprintf(":%s", app.Config().Port)
	slog.Info("Starting server", "addr", addr)
	if err := http.ListenAndServe(addr, rootMux); err != nil {
		slog.Error("Failed to start server", "error", err)
		os.Exit(1)
	}
}
