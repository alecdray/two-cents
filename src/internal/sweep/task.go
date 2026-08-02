package sweep

import (
	"fmt"
	"time"

	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/task"
)

// sweeper is the narrow slice of *Service the monthly task depends on.
// Defined as an unexported interface so the task can be unit-tested with a
// lightweight stand-in without wiring up the full service dependency graph.
type sweeper interface {
	Compute(ctx contextx.ContextX) (Recommendation, error)
	SaveLatest(ctx contextx.ContextX, rec Recommendation) error
}

// monthlyTask is the once-a-month sweep computation task. On each tick it
// computes the current recommendation from live data and persists it as the
// latest, replacing any previous result.
type monthlyTask struct {
	svc      sweeper
	schedule task.CronExpression
}

// NewMonthlyTask builds the 7th-of-month sweep task anchored to loc. The
// schedule fires at 00:00 on the 7th of every month in that zone via a
// CRON_TZ= prefix that robfig/cron v3 interprets before parsing the 5-field
// spec — so the tick follows the app timezone even when the server runs in a
// different zone. Pass cfg.AppTimezone as loc. Register the returned task with
// the task manager at the composition root.
func NewMonthlyTask(svc *Service, loc *time.Location) task.Task {
	return &monthlyTask{
		svc:      svc,
		schedule: buildMonthlySchedule(loc),
	}
}

// buildMonthlySchedule constructs a CRON_TZ-prefixed expression that fires at
// 00:00 on the 7th of every month in the given location.
func buildMonthlySchedule(loc *time.Location) task.CronExpression {
	return task.CronExpression(fmt.Sprintf("CRON_TZ=%s 0 0 7 * *", loc.String()))
}

// Run computes the sweep recommendation and stores it as the latest, replacing
// any prior result. The underlying store is idempotent: re-firing produces one
// row, never a duplicate.
func (t *monthlyTask) Run(ctx contextx.ContextX) error {
	rec, err := t.svc.Compute(ctx)
	if err != nil {
		return fmt.Errorf("sweep monthly: compute: %w", err)
	}
	if err := t.svc.SaveLatest(ctx, rec); err != nil {
		return fmt.Errorf("sweep monthly: save: %w", err)
	}
	return nil
}

// Schedule returns the cron cadence anchored to the configured app timezone.
func (t *monthlyTask) Schedule() *task.CronExpression {
	s := t.schedule
	return &s
}

// Name identifies the task in logs.
func (t *monthlyTask) Name() string {
	return "sweep-monthly"
}
