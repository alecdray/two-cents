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

// sweepID is the fixed primary key of the single latest recommendation row.
const sweepID = "default"

// SaveLatest upserts the single latest recommendation, replacing any previous
// one. Calling it a second time never creates a duplicate row.
func (r *Repo) SaveLatest(ctx context.Context, rec Recommendation) error {
	params, err := toUpsertParams(rec)
	if err != nil {
		return fmt.Errorf("sweep repo: marshal recommendation: %w", err)
	}
	if err := r.q.UpsertSweepRecommendation(ctx, params); err != nil {
		return fmt.Errorf("sweep repo: upsert: %w", err)
	}
	return nil
}

// LoadLatest returns the most recently stored recommendation and found=true, or
// found=false when no recommendation has ever been saved. An absent row is
// distinct from a needs-attention result.
func (r *Repo) LoadLatest(ctx context.Context) (Recommendation, bool, error) {
	model, err := r.q.GetLatestSweepRecommendation(ctx, sweepID)
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

// --- conversion helpers ---

func toUpsertParams(rec Recommendation) (sqlc.UpsertSweepRecommendationParams, error) {
	reasonsJSON, err := json.Marshal(reasonStrings(rec.Reasons))
	if err != nil {
		return sqlc.UpsertSweepRecommendationParams{}, err
	}

	p := sqlc.UpsertSweepRecommendationParams{
		ID:                    sweepID,
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
		Kind:                  RecommendationKind(m.Kind),
		SavingsUnknown:        m.SavingsUnknown != 0,
		TotalSpendingBudget:   m.TotalSpendingBudget,
		MtdSpending:           m.MtdSpending,
		SavingsTarget:         m.SavingsTarget,
		MtdSavingsContributed: m.MtdSavingsContributed,
		Reserve:               m.Reserve,
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
