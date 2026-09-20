package schedule

import (
	"context"
	"database/sql"
	"time"

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

// UpsertManualMatch records the user's own decision about an occurrence,
// overwriting whatever stood before — including an automatic match it corrects.
func (r *Repo) UpsertManualMatch(ctx context.Context, m Match) error {
	return r.q.UpsertManualScheduleOccurrenceMatch(ctx, sqlc.UpsertManualScheduleOccurrenceMatchParams{
		ItemID:         m.ItemID,
		OccurrenceDate: occurrenceKey(m.Occurrence),
		TransactionID:  transactionIDParam(m),
	})
}

// UpsertAutoMatch records resolution's decision, leaving a manual one standing.
//
// Two statements rather than one conditional upsert: the insert claims an
// occurrence nothing has been decided about, and the update supersedes an
// earlier automatic decision. Both carry the precedence themselves, so no caller
// can write past a manual decision by forgetting to check for one first.
func (r *Repo) UpsertAutoMatch(ctx context.Context, m Match) error {
	if err := r.q.InsertAutoScheduleOccurrenceMatch(ctx, sqlc.InsertAutoScheduleOccurrenceMatchParams{
		ItemID:         m.ItemID,
		OccurrenceDate: occurrenceKey(m.Occurrence),
		TransactionID:  transactionIDParam(m),
	}); err != nil {
		return err
	}
	return r.q.UpdateAutoScheduleOccurrenceMatch(ctx, sqlc.UpdateAutoScheduleOccurrenceMatchParams{
		TransactionID:  transactionIDParam(m),
		ItemID:         m.ItemID,
		OccurrenceDate: occurrenceKey(m.Occurrence),
	})
}

// DeleteMatchesForItem removes every decision recorded against an item.
func (r *Repo) DeleteMatchesForItem(ctx context.Context, itemID string) error {
	return r.q.DeleteScheduleOccurrenceMatchesForItem(ctx, itemID)
}

// DeleteMatch removes the decision recorded against one occurrence, whatever
// its source.
func (r *Repo) DeleteMatch(ctx context.Context, itemID string, occurrence time.Time) error {
	return r.q.DeleteScheduleOccurrenceMatch(ctx, sqlc.DeleteScheduleOccurrenceMatchParams{
		ItemID:         itemID,
		OccurrenceDate: occurrenceKey(occurrence),
	})
}

// LatestManualMatches returns, for each item that has one, the most recent
// occurrence the user settled by hand.
func (r *Repo) LatestManualMatches(ctx context.Context) ([]Match, error) {
	rows, err := r.q.ListLatestManualScheduleOccurrenceMatches(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Match, 0, len(rows))
	for _, row := range rows {
		out = append(out, Match{
			ItemID:        row.ItemID,
			TransactionID: row.TransactionID.String,
			Source:        MatchManual,
		})
	}
	return out, nil
}

// SettledTransactionIDs returns every transaction already bound to an
// occurrence, whatever the decision's source or date.
func (r *Repo) SettledTransactionIDs(ctx context.Context) ([]string, error) {
	rows, err := r.q.ListSettledScheduleOccurrenceTransactionIDs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.Valid {
			out = append(out, row.String)
		}
	}
	return out, nil
}

// MatchesInRange returns every decision recorded for an occurrence falling in
// [from, to], inclusive.
func (r *Repo) MatchesInRange(ctx context.Context, from, to time.Time) ([]Match, error) {
	rows, err := r.q.ListScheduleOccurrenceMatchesInRange(ctx, sqlc.ListScheduleOccurrenceMatchesInRangeParams{
		FromOccurrenceDate: occurrenceKey(from),
		ToOccurrenceDate:   occurrenceKey(to),
	})
	if err != nil {
		return nil, err
	}
	out := make([]Match, 0, len(rows))
	for _, row := range rows {
		occurrence, err := time.Parse("2006-01-02", row.OccurrenceDate)
		if err != nil {
			return nil, err
		}
		out = append(out, Match{
			ItemID:        row.ItemID,
			Occurrence:    occurrence,
			TransactionID: row.TransactionID.String,
			Source:        MatchSource(row.Source),
		})
	}
	return out, nil
}

// transactionIDParam writes NULL for a decision that nothing satisfied the
// occurrence, which the partial unique index depends on: any number of
// occurrences may be recorded as deliberately unmatched, but a transaction may
// settle only one.
func transactionIDParam(m Match) sql.NullString {
	if m.TransactionID == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: m.TransactionID, Valid: true}
}
