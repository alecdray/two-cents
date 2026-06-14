package server

import (
	"fmt"
	"log/slog"

	"github.com/alecdray/two-cents/src/internal/accounts"
	"github.com/alecdray/two-cents/src/internal/core/app"
	"github.com/alecdray/two-cents/src/internal/core/db"
	"github.com/alecdray/two-cents/src/internal/core/task"
	"github.com/alecdray/two-cents/src/internal/plaid"
)

type services struct {
	taskManager     *task.TaskManager
	accountsService *accounts.Service
}

func NewServices(application app.App, database *db.DB) (*services, error) {
	s := &services{}

	s.taskManager = task.NewTaskManager(database, slog.Default())

	cfg := application.Config()

	plaidClient, err := plaid.NewClient(
		cfg.Plaid.ClientID,
		cfg.Plaid.Secret,
		plaid.WithOrigin(plaidOrigin(cfg.Plaid.Env)),
		plaid.WithLinkConfig(plaid.LinkConfig{
			ClientName:   cfg.AppName,
			CountryCodes: cfg.Plaid.CountryCodes,
			Products:     cfg.Plaid.Products,
			ClientUserID: cfg.AppName,
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build plaid client: %w", err)
	}
	bankProvider := plaid.NewService(plaidClient)

	s.accountsService = accounts.NewService(database, bankProvider, cfg.EncryptionKey)

	// Cron tasks (e.g. the transactions sync) register via
	// s.taskManager.RegisterCronTask(...).

	return s, nil
}

// plaidOrigin maps the configured Plaid environment onto its API base URL. An
// unrecognised value falls back to the production host.
func plaidOrigin(env string) string {
	switch env {
	case "sandbox":
		return "https://sandbox.plaid.com"
	case "development":
		return "https://development.plaid.com"
	default:
		return "https://production.plaid.com"
	}
}
