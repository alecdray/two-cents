package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/alecdray/two-cents/src/internal/core/app"
	"github.com/alecdray/two-cents/src/internal/server"
)

func main() {
	slog.Info("Starting app")
	ctx := context.Background()

	config := app.LoadConfig()

	if config.Env == app.EnvLocal {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})))
	}

	server.Start(ctx, app.NewApp(*config))
}
