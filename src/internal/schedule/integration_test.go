package schedule

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/db"

	"github.com/pressly/goose/v3"

	_ "github.com/mattn/go-sqlite3"
)

// These tests exercise the assembled schedule module end-to-end: the real
// Service over a real (migrated, temp-file) SQLite DB through the real repo,
// mirroring the budget integration tests.

func newTestDB(t *testing.T) *db.DB {
	t.Helper()

	migrationsDir, err := filepath.Abs("../../../db/migrations")
	if err != nil {
		t.Fatalf("resolve migrations dir: %v", err)
	}

	dbPath := filepath.Join(t.TempDir(), "test.db")
	sqlDB, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("set dialect: %v", err)
	}
	if err := goose.Up(sqlDB, migrationsDir); err != nil {
		t.Fatalf("goose up: %v", err)
	}
	return db.WrapSqlDB(sqlDB)
}

func newService(t *testing.T) (*Service, contextx.ContextX) {
	t.Helper()
	return NewService(newTestDB(t)), contextx.NewContextX(context.Background())
}

func rent() Item {
	return Item{
		Name:       "Rent",
		Direction:  DirectionOut,
		Amount:     2400,
		Cadence:    CadenceMonthly,
		DayOfMonth: 1,
		Active:     true,
	}
}

func paycheck() Item {
	return Item{
		Name:       "Paycheck",
		Direction:  DirectionIn,
		Amount:     3100,
		Cadence:    CadenceBiweekly,
		AnchorDate: time.Date(2026, time.September, 4, 0, 0, 0, 0, time.UTC),
		Active:     true,
	}
}

func TestServiceCreate(t *testing.T) {
	t.Run("a created item comes back with every field it was declared with", func(t *testing.T) {
		svc, ctx := newService(t)

		created, err := svc.Create(ctx, rent())
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if created.ID == "" {
			t.Error("Create assigned no ID")
		}

		items, err := svc.List(ctx)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(items) != 1 {
			t.Fatalf("List returned %d items, want 1", len(items))
		}
		got := items[0]
		if got.Name != "Rent" || got.Direction != DirectionOut || got.Amount != 2400 {
			t.Errorf("stored item = %+v, want the declared Rent", got)
		}
		if got.Cadence != CadenceMonthly || got.DayOfMonth != 1 {
			t.Errorf("stored cadence = %s day %d, want monthly day 1", got.Cadence, got.DayOfMonth)
		}
		if !got.Active {
			t.Error("a created item should be active")
		}
	})

	t.Run("a biweekly item round-trips its anchor date", func(t *testing.T) {
		svc, ctx := newService(t)

		if _, err := svc.Create(ctx, paycheck()); err != nil {
			t.Fatalf("Create: %v", err)
		}

		items, _ := svc.List(ctx)
		if len(items) != 1 {
			t.Fatalf("List returned %d items, want 1", len(items))
		}
		if got := items[0].AnchorDate.Format("2006-01-02"); got != "2026-09-04" {
			t.Errorf("anchor date = %s, want 2026-09-04", got)
		}
	})
}

func TestServiceUpdate(t *testing.T) {
	t.Run("an edited item keeps its id and reads back changed", func(t *testing.T) {
		svc, ctx := newService(t)
		created, err := svc.Create(ctx, rent())
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		edited := created
		edited.Amount = 2600
		edited.DayOfMonth = 3
		if err := svc.Update(ctx, edited); err != nil {
			t.Fatalf("Update: %v", err)
		}

		items, _ := svc.List(ctx)
		if len(items) != 1 {
			t.Fatalf("List returned %d items, want 1", len(items))
		}
		if items[0].ID != created.ID {
			t.Errorf("id = %s, want the original %s", items[0].ID, created.ID)
		}
		if items[0].Amount != 2600 || items[0].DayOfMonth != 3 {
			t.Errorf("stored item = %+v, want amount 2600 on day 3", items[0])
		}
	})

	t.Run("deactivating keeps the item but drops it from the active set", func(t *testing.T) {
		svc, ctx := newService(t)
		created, _ := svc.Create(ctx, rent())
		if _, err := svc.Create(ctx, paycheck()); err != nil {
			t.Fatalf("Create: %v", err)
		}

		deactivated := created
		deactivated.Active = false
		if err := svc.Update(ctx, deactivated); err != nil {
			t.Fatalf("Update: %v", err)
		}

		all, _ := svc.List(ctx)
		if len(all) != 2 {
			t.Errorf("List returned %d items, want both kept", len(all))
		}

		active, err := svc.ActiveItems(ctx)
		if err != nil {
			t.Fatalf("ActiveItems: %v", err)
		}
		if len(active) != 1 || active[0].Name != "Paycheck" {
			t.Errorf("ActiveItems = %+v, want only the Paycheck", active)
		}
	})
}

func TestServiceDelete(t *testing.T) {
	t.Run("a deleted item is gone from the schedule", func(t *testing.T) {
		svc, ctx := newService(t)
		created, _ := svc.Create(ctx, rent())

		if err := svc.Delete(ctx, created.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		items, _ := svc.List(ctx)
		if len(items) != 0 {
			t.Errorf("List returned %d items, want none", len(items))
		}
	})
}

func TestServiceValidation(t *testing.T) {
	tests := []struct {
		name string
		item Item
	}{
		{
			name: "a nameless item is rejected",
			item: Item{Name: "  ", Direction: DirectionOut, Amount: 10, Cadence: CadenceMonthly, DayOfMonth: 1},
		},
		{
			name: "a zero amount is rejected",
			item: Item{Name: "Rent", Direction: DirectionOut, Amount: 0, Cadence: CadenceMonthly, DayOfMonth: 1},
		},
		{
			name: "a negative amount is rejected — direction carries the sign",
			item: Item{Name: "Rent", Direction: DirectionOut, Amount: -10, Cadence: CadenceMonthly, DayOfMonth: 1},
		},
		{
			name: "an unknown direction is rejected",
			item: Item{Name: "Rent", Direction: "sideways", Amount: 10, Cadence: CadenceMonthly, DayOfMonth: 1},
		},
		{
			name: "an unknown cadence is rejected",
			item: Item{Name: "Rent", Direction: DirectionOut, Amount: 10, Cadence: "weekly", DayOfMonth: 1},
		},
		{
			name: "a monthly item with no day of month is rejected",
			item: Item{Name: "Rent", Direction: DirectionOut, Amount: 10, Cadence: CadenceMonthly},
		},
		{
			name: "a monthly item beyond day 31 is rejected",
			item: Item{Name: "Rent", Direction: DirectionOut, Amount: 10, Cadence: CadenceMonthly, DayOfMonth: 32},
		},
		{
			name: "a biweekly item with no anchor date is rejected",
			item: Item{Name: "Paycheck", Direction: DirectionIn, Amount: 10, Cadence: CadenceBiweekly},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, ctx := newService(t)

			_, err := svc.Create(ctx, tc.item)
			if err == nil {
				t.Fatal("Create accepted an invalid item")
			}
			if _, ok := IsValidationError(err); !ok {
				t.Errorf("error = %v, want a ValidationError the adapter can render inline", err)
			}

			items, _ := svc.List(ctx)
			if len(items) != 0 {
				t.Errorf("a rejected item was stored anyway: %+v", items)
			}
		})
	}
}
