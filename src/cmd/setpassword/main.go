// Command setpassword sets (or rotates) the single local login password
// (ADR-0007). It reads the new password from the AUTH_PASSWORD environment
// variable — never an argument, which would leak into shell history — and
// upserts the credential into the same database the app reads. Run it via
// `task auth/set-password`.
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/alecdray/two-cents/src/internal/auth"
	"github.com/alecdray/two-cents/src/internal/core/app"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/db"
)

func main() {
	password := os.Getenv("AUTH_PASSWORD")
	if password == "" {
		slog.Error("AUTH_PASSWORD must be set to the new login password")
		os.Exit(1)
	}

	cfg := app.LoadConfig()

	database, err := db.NewDB(cfg.DbPath)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	ctx := contextx.NewContextX(context.Background())
	if err := auth.NewService(database).SetPassword(ctx, password); err != nil {
		slog.Error("failed to set login password", "error", err)
		os.Exit(1)
	}
	slog.Info("login password set")
}
