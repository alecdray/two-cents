package transactions

import (
	"testing"

	"github.com/alecdray/two-cents/src/internal/accounts"
	"github.com/alecdray/two-cents/src/internal/banking"

	cron "github.com/robfig/cron/v3"
)

// --- E. Background sync task ---

// TestSyncTaskRunsSync proves the recurring task drives a real sync pass: after
// Run, the connection's pulled transactions are persisted — the same effect as
// calling Service.SyncTransactions directly.
func TestSyncTaskRunsSync(t *testing.T) {
	database := newTestDB(t)

	const token = "tok-a"
	provider := newStub()
	provider.accountsByToken[token] = []banking.Account{cashAccount("p-check", "Checking")}
	provider.syncByToken[token] = func(cursor string) (banking.TransactionChanges, error) {
		if cursor == "" {
			return banking.TransactionChanges{
				Added:  []banking.Transaction{bankTxn("t1", "p-check", 1, 10, false)},
				Cursor: "c1",
			}, nil
		}
		return banking.TransactionChanges{Cursor: cursor}, nil
	}

	accountsSvc := accounts.NewService(database, provider, testKey)
	registerConnection(t, accountsSvc, token, "item-a")
	svc := NewService(database, provider, accountsSvc, newCategorization(database), nil)

	task := NewSyncTask(svc)
	if err := task.Run(testCtx()); err != nil {
		t.Fatalf("task.Run: %v", err)
	}

	if got := countTransactions(t, database); got != 1 {
		t.Errorf("after the task ran, stored %d transactions, want 1", got)
	}
}

// TestSyncTaskScheduleIsValidCron proves the task advertises a name and a
// schedule the cron parser accepts, so the task manager can register it.
func TestSyncTaskScheduleIsValidCron(t *testing.T) {
	task := NewSyncTask(nil)

	if task.Name() == "" {
		t.Error("task name is empty")
	}

	schedule := task.Schedule()
	if schedule == nil {
		t.Fatal("task schedule is nil; a recurring task must advertise a cadence")
	}
	if _, err := cron.ParseStandard(schedule.String()); err != nil {
		t.Errorf("schedule %q is not a valid standard cron expression: %v", schedule.String(), err)
	}
}
