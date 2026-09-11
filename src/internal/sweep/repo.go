package sweep

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/alecdray/two-cents/src/internal/core/db/sqlc"
)

// Repo is the sweep module's data access layer. It is the only file in package
// sweep that imports core/db/sqlc; its methods take and return this package's
// domain types — never sqlc.* shapes.
type Repo struct {
	q *sqlc.Queries
}

// NewRepo binds a Repo to the given Queries.
func NewRepo(q *sqlc.Queries) *Repo {
	return &Repo{q: q}
}

// Save appends rec to the history as a new snapshot. It never replaces a
// previous one — including when the figures are identical to the last snapshot,
// because the record being kept is that a run happened at that instant and what
// it advised.
//
// rec carries its own ID, assigned by the service before saving (the convention
// the other domain modules follow), so the run that produced a snapshot knows its
// address without reading it back.
func (r *Repo) Save(ctx context.Context, rec Recommendation) error {
	params, err := toInsertParams(rec)
	if err != nil {
		return fmt.Errorf("sweep repo: marshal recommendation: %w", err)
	}
	if err := r.q.InsertSweepRecommendation(ctx, params); err != nil {
		return fmt.Errorf("sweep repo: insert: %w", err)
	}
	return nil
}

// LoadLatest returns the newest snapshot by computed instant and found=true, or
// found=false when nothing has ever been saved. An empty history is distinct
// from a needs-attention snapshot.
func (r *Repo) LoadLatest(ctx context.Context) (Recommendation, bool, error) {
	model, err := r.q.GetLatestSweepRecommendation(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return Recommendation{}, false, nil
	}
	if err != nil {
		return Recommendation{}, false, fmt.Errorf("sweep repo: load latest: %w", err)
	}
	rec, err := fromModel(model)
	if err != nil {
		return Recommendation{}, false, fmt.Errorf("sweep repo: decode recommendation: %w", err)
	}
	return rec, true, nil
}

// LoadByID returns one snapshot by its id, backing the page's deep link.
// found=false when no snapshot carries that id.
func (r *Repo) LoadByID(ctx context.Context, id string) (Recommendation, bool, error) {
	model, err := r.q.GetSweepRecommendationByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Recommendation{}, false, nil
	}
	if err != nil {
		return Recommendation{}, false, fmt.Errorf("sweep repo: load by id: %w", err)
	}
	rec, err := fromModel(model)
	if err != nil {
		return Recommendation{}, false, fmt.Errorf("sweep repo: decode recommendation: %w", err)
	}
	return rec, true, nil
}

// List returns every snapshot, newest first. The whole history is returned
// unpaged: a single-user app produces a handful of snapshots a month and all of
// them are retained, so there is nothing here worth paginating.
func (r *Repo) List(ctx context.Context) ([]Recommendation, error) {
	models, err := r.q.ListSweepRecommendations(ctx)
	if err != nil {
		return nil, fmt.Errorf("sweep repo: list: %w", err)
	}
	out := make([]Recommendation, 0, len(models))
	for _, m := range models {
		rec, err := fromModel(m)
		if err != nil {
			return nil, fmt.Errorf("sweep repo: decode recommendation: %w", err)
		}
		out = append(out, rec)
	}
	return out, nil
}

// --- conversion helpers ---

func toInsertParams(rec Recommendation) (sqlc.InsertSweepRecommendationParams, error) {
	reasonsJSON, err := json.Marshal(reasonStrings(rec.Reasons))
	if err != nil {
		return sqlc.InsertSweepRecommendationParams{}, err
	}

	p := sqlc.InsertSweepRecommendationParams{
		ID:                    rec.ID,
		ComputedAt:            rec.ComputedAt,
		CardBalance:           rec.CardBalance,
		Kind:                  string(rec.Kind),
		SavingsUnknown:        boolToInt(rec.SavingsUnknown),
		TotalSpendingBudget:   rec.TotalSpendingBudget,
		MtdSpending:           rec.MtdSpending,
		SavingsTarget:         rec.SavingsTarget,
		MtdSavingsContributed: rec.MtdSavingsContributed,
		Reserve:               rec.Reserve,
		FixedSafetyMargin:     rec.FixedSafetyMargin,
		SuggestedSweep:        rec.SuggestedSweep,
		Direction:             string(rec.Direction),
		Reasons:               string(reasonsJSON),
	}

	// current_checking is NULL for needs-attention (checking may be unknown);
	// for numeric results it carries the live checking balance.
	if rec.Kind == KindNumeric {
		p.CurrentChecking = sql.NullFloat64{Float64: rec.CurrentChecking, Valid: true}
		// current_savings is NULL when savings balance is unknown (SavingsUnknown=true);
		// it carries the known balance otherwise.
		if !rec.SavingsUnknown {
			p.CurrentSavings = sql.NullFloat64{Float64: rec.CurrentSavings, Valid: true}
		}
	}

	return p, nil
}

func fromModel(m sqlc.SweepRecommendation) (Recommendation, error) {
	var rawReasons []string
	if err := json.Unmarshal([]byte(m.Reasons), &rawReasons); err != nil {
		return Recommendation{}, fmt.Errorf("decode reasons: %w", err)
	}
	reasons := make([]NeedsAttentionReason, len(rawReasons))
	for i, s := range rawReasons {
		reasons[i] = NeedsAttentionReason(s)
	}

	rec := Recommendation{
		ID:                    m.ID,
		Kind:                  RecommendationKind(m.Kind),
		SavingsUnknown:        m.SavingsUnknown != 0,
		TotalSpendingBudget:   m.TotalSpendingBudget,
		MtdSpending:           m.MtdSpending,
		SavingsTarget:         m.SavingsTarget,
		MtdSavingsContributed: m.MtdSavingsContributed,
		Reserve:               m.Reserve,
		CardBalance:           m.CardBalance,
		FixedSafetyMargin:     m.FixedSafetyMargin,
		SuggestedSweep:        m.SuggestedSweep,
		Direction:             SweepDirection(m.Direction),
		Reasons:               reasons,
	}

	if m.CurrentChecking.Valid {
		rec.CurrentChecking = m.CurrentChecking.Float64
	}
	if m.CurrentSavings.Valid {
		rec.CurrentSavings = m.CurrentSavings.Float64
	}
	rec.ComputedAt = m.ComputedAt

	return rec, nil
}

func reasonStrings(reasons []NeedsAttentionReason) []string {
	if reasons == nil {
		return []string{}
	}
	out := make([]string, len(reasons))
	for i, r := range reasons {
		out[i] = string(r)
	}
	return out
}

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
