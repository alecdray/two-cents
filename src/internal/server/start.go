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
	categorizationAdapters "github.com/alecdray/two-cents/src/internal/categorization/adapters"
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

	rootMux := httpx.NewMux(app, httpx.RequestLoggingMiddleware)

	rootMux.Handle("/static/", httpx.WrapHandler(http.StripPrefix("/static/", http.FileServer(http.Dir("static/public")))))

	// The connect/reconnect handlers trigger the initial transaction backfill
	// through this injected seam — the only place both services are in scope —
	// so accounts never imports transactions and the module graph stays acyclic.
	backfillTransactions := func(ctx contextx.ContextX) error {
		return services.transactionsService.SyncTransactions(ctx)
	}
	accountsHandler := accountsAdapters.NewHttpHandler(services.accountsService, services.bankMode, backfillTransactions)
	accountsAdapters.RegisterRoutes(rootMux, accountsHandler)

	transactionsHandler := transactionsAdapters.NewHttpHandler(services.transactionsService, services.accountsService, services.categorizationService)
	transactionsAdapters.RegisterRoutes(rootMux, transactionsHandler)

	categorizationHandler := categorizationAdapters.NewHttpHandler(services.categorizationService)
	categorizationAdapters.RegisterRoutes(rootMux, categorizationHandler)

	addr := fmt.Sprintf(":%s", app.Config().Port)
	slog.Info("Starting server", "addr", addr)
	if err := http.ListenAndServe(addr, rootMux); err != nil {
		slog.Error("Failed to start server", "error", err)
		os.Exit(1)
	}
}
