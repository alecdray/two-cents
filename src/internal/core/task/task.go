package task

import (
	"log/slog"

	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/db"

	cron "github.com/robfig/cron/v3"
)

type CronExpression string

func (ce CronExpression) String() string {
	return string(ce)
}

type Task interface {
	Run(ctx contextx.ContextX) error
	Schedule() *CronExpression
	Name() string
}

type TaskManager struct {
	cronTasks     []Task
	cron          *cron.Cron
	db            *db.DB
	logger        *slog.Logger
	adHocTaskChan chan Task
}

func NewTaskManager(db *db.DB, logger *slog.Logger) *TaskManager {
	return &TaskManager{
		cronTasks:     []Task{},
		cron:          cron.New(),
		db:            db,
		logger:        logger,
		adHocTaskChan: make(chan Task),
	}
}

func (tm *TaskManager) RegisterCronTask(task Task) {
	tm.cronTasks = append(tm.cronTasks, task)
}

func (tm *TaskManager) RegisterAdHocTask(task Task) {
	tm.adHocTaskChan <- task
}

func (tm *TaskManager) runAdhocTask(ctx contextx.ContextX, task Task) {
	go func() {
		if err := task.Run(ctx); err != nil {
			tm.logger.Error("ad hoc task failed", "task", task.Name(), "err", err)
		}
	}()
}

func (tm *TaskManager) startAdhocTaskHandler(ctx contextx.ContextX) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case task, ok := <-tm.adHocTaskChan:
				if !ok {
					return
				}
				tm.logger.Debug("received ad hoc task", "task", task.Name())
				tm.runAdhocTask(ctx, task)
			}
		}
	}()
}

func (tm *TaskManager) Start(ctx contextx.ContextX) {
	tm.logger.Debug("task manager started")

	for _, task := range tm.cronTasks {
		if task.Schedule() == nil {
			continue
		}

		tm.cron.AddFunc(task.Schedule().String(), func() {
			tm.logger.Debug("cron task started", "task", task.Name())

			if err := task.Run(ctx); err != nil {
				tm.logger.Error("cron task failed", "task", task.Name(), "err", err)
			}
		})
	}
	tm.cron.Start()

	tm.startAdhocTaskHandler(ctx)
}

func (tm *TaskManager) Stop() {
	tm.cron.Stop()
	close(tm.adHocTaskChan)
}
