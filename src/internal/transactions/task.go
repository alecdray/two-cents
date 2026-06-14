package transactions

import (
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/task"
)

// syncSchedule runs the bank sync every six hours (standard 5-field cron). The
// cursor model means each pass only pulls the delta since the last one, so a
// modest cadence keeps balances and activity fresh without hammering the
// provider.
const syncSchedule = task.CronExpression("0 */6 * * *")

// syncTask is the recurring bank-sync task: on each tick it runs the module's
// full sync pass (Accounts first, then per-connection transaction pulls).
type syncTask struct {
	service *Service
}

// NewSyncTask builds the recurring transactions sync task over the service.
// Register it with the task manager at the composition root.
func NewSyncTask(service *Service) task.Task {
	return &syncTask{service: service}
}

// Run executes one full sync pass.
func (t *syncTask) Run(ctx contextx.ContextX) error {
	return t.service.SyncTransactions(ctx)
}

// Schedule returns the recurring cadence.
func (t *syncTask) Schedule() *task.CronExpression {
	s := syncSchedule
	return &s
}

// Name identifies the task in logs.
func (t *syncTask) Name() string {
	return "transactions-sync"
}
