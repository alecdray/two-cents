package schedule

import (
	"context"
	"database/sql"

	"github.com/alecdray/two-cents/src/internal/core/db/sqlc"
)

// Repo is the schedule module's data access layer. It is the only file in
// package schedule that imports core/db/sqlc; its methods take and return this
// package's Item — never sqlc.* shapes.
type Repo struct {
	q *sqlc.Queries
}

// NewRepo binds a Repo to the given Queries.
func NewRepo(q *sqlc.Queries) *Repo {
	return &Repo{q: q}
}

// List returns every stored Item, inactive ones included — the Service filters
// for the sweep's active-only read.
func (r *Repo) List(ctx context.Context) ([]Item, error) {
	models, err := r.q.ListScheduleItems(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Item, len(models))
	for i, m := range models {
		out[i] = itemFromModel(m)
	}
	return out, nil
}

// Insert stores a new Item under the id it already carries.
func (r *Repo) Insert(ctx context.Context, item Item) error {
	return r.q.InsertScheduleItem(ctx, sqlc.InsertScheduleItemParams{
		ID:         item.ID,
		Name:       item.Name,
		Direction:  string(item.Direction),
		Amount:     item.Amount,
		Cadence:    string(item.Cadence),
		DayOfMonth: dayOfMonthParam(item),
		AnchorDate: anchorDateParam(item),
		Active:     boolToInt(item.Active),
	})
}

// Update overwrites every declared field of the Item with the given id.
func (r *Repo) Update(ctx context.Context, item Item) error {
	return r.q.UpdateScheduleItem(ctx, sqlc.UpdateScheduleItemParams{
		ID:         item.ID,
		Name:       item.Name,
		Direction:  string(item.Direction),
		Amount:     item.Amount,
		Cadence:    string(item.Cadence),
		DayOfMonth: dayOfMonthParam(item),
		AnchorDate: anchorDateParam(item),
		Active:     boolToInt(item.Active),
	})
}

// Delete removes the Item with the given id.
func (r *Repo) Delete(ctx context.Context, id string) error {
	return r.q.DeleteScheduleItem(ctx, id)
}

// --- conversion helpers (private — only repo.go touches sqlc types) ---

// dayOfMonthParam and anchorDateParam write only the date field the cadence
// uses, leaving the other NULL: a monthly item has no anchor and a biweekly one
// has no day of month, and the table's CHECK enforces exactly that pairing.
func dayOfMonthParam(item Item) sql.NullInt64 {
	if item.Cadence != CadenceMonthly {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(item.DayOfMonth), Valid: true}
}

func anchorDateParam(item Item) sql.NullTime {
	if item.Cadence != CadenceBiweekly {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: item.AnchorDate, Valid: true}
}

func itemFromModel(m sqlc.ScheduleItem) Item {
	item := Item{
		ID:        m.ID,
		Name:      m.Name,
		Direction: Direction(m.Direction),
		Amount:    m.Amount,
		Cadence:   Cadence(m.Cadence),
		Active:    m.Active != 0,
	}
	if m.DayOfMonth.Valid {
		item.DayOfMonth = int(m.DayOfMonth.Int64)
	}
	if m.AnchorDate.Valid {
		item.AnchorDate = m.AnchorDate.Time
	}
	return item
}

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
