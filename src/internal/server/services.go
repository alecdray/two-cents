package server

import (
	"log/slog"

	"github.com/alecdray/two-cents/src/internal/core/app"
	"github.com/alecdray/two-cents/src/internal/core/db"
	"github.com/alecdray/two-cents/src/internal/core/task"
)

type services struct {
	taskManager *task.TaskManager
}

func NewServices(app app.App, db *db.DB) *services {
	s := &services{}

	s.taskManager = task.NewTaskManager(db, slog.Default())

	// Domain module services are constructed and wired here as they land.
	// Cron tasks (e.g. the transactions sync) register via
	// s.taskManager.RegisterCronTask(...).

	return s
}
