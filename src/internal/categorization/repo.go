package categorization

import (
	"context"
	"database/sql"

	"github.com/alecdray/two-cents/src/internal/core/db/sqlc"
)

// Repo is the categorization module's data access layer. It is the only file in
// package categorization that imports core/db/sqlc; its methods take and return
// this package's domain types (Category, Rule) — never sqlc.* shapes.
type Repo struct {
	q *sqlc.Queries
}

// NewRepo binds a Repo to the given Queries. Callers bind to db.Queries() for the
// global handle or to tx.Queries() inside a db.WithTx callback for transactional
// work.
func NewRepo(q *sqlc.Queries) *Repo {
	return &Repo{q: q}
}

// --- conversion helpers (private — only repo.go touches sqlc types) ---

func categoryFromModel(m sqlc.Category) Category {
	return Category{
		ID:        m.ID,
		Name:      m.Name,
		Builtin:   m.Builtin != 0,
		Archived:  m.Archived != 0,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}

func ruleFromModel(m sqlc.Rule) Rule {
	r := Rule{
		ID:                m.ID,
		MerchantSubstring: m.MerchantSubstring,
		Classification:    Classification(m.Classification),
		CreatedAt:         m.CreatedAt,
		UpdatedAt:         m.UpdatedAt,
	}
	if m.CategoryID.Valid {
		id := m.CategoryID.String
		r.CategoryID = &id
	}
	return r
}

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func nullStringFromPtr(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}

// --- Category queries ---

// CreateCategory persists a new category and returns it as a domain entity.
func (r *Repo) CreateCategory(ctx context.Context, c Category) (Category, error) {
	model, err := r.q.CreateCategory(ctx, sqlc.CreateCategoryParams{
		ID:       c.ID,
		Name:     c.Name,
		Builtin:  boolToInt(c.Builtin),
		Archived: boolToInt(c.Archived),
	})
	if err != nil {
		return Category{}, err
	}
	return categoryFromModel(model), nil
}

// GetCategory returns a single category as a domain entity.
func (r *Repo) GetCategory(ctx context.Context, id string) (Category, error) {
	model, err := r.q.GetCategory(ctx, id)
	if err != nil {
		return Category{}, err
	}
	return categoryFromModel(model), nil
}

// ListCategories returns every category, active and archived, ordered by name.
func (r *Repo) ListCategories(ctx context.Context) ([]Category, error) {
	models, err := r.q.ListCategories(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Category, len(models))
	for i, m := range models {
		out[i] = categoryFromModel(m)
	}
	return out, nil
}

// UpdateCategory writes a category's name and archived flag back, keeping its id
// (and builtin flag) stable.
func (r *Repo) UpdateCategory(ctx context.Context, c Category) (Category, error) {
	model, err := r.q.UpdateCategory(ctx, sqlc.UpdateCategoryParams{
		ID:       c.ID,
		Name:     c.Name,
		Archived: boolToInt(c.Archived),
	})
	if err != nil {
		return Category{}, err
	}
	return categoryFromModel(model), nil
}

// --- Rule queries ---

// CreateRule persists a new rule and returns it as a domain entity.
func (r *Repo) CreateRule(ctx context.Context, rule Rule) (Rule, error) {
	model, err := r.q.CreateRule(ctx, sqlc.CreateRuleParams{
		ID:                rule.ID,
		MerchantSubstring: rule.MerchantSubstring,
		Classification:    string(rule.Classification),
		CategoryID:        nullStringFromPtr(rule.CategoryID),
	})
	if err != nil {
		return Rule{}, err
	}
	return ruleFromModel(model), nil
}

// GetRule returns a single rule as a domain entity.
func (r *Repo) GetRule(ctx context.Context, id string) (Rule, error) {
	model, err := r.q.GetRule(ctx, id)
	if err != nil {
		return Rule{}, err
	}
	return ruleFromModel(model), nil
}

// ListRules returns every rule, most-recently-edited first.
func (r *Repo) ListRules(ctx context.Context) ([]Rule, error) {
	models, err := r.q.ListRules(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Rule, len(models))
	for i, m := range models {
		out[i] = ruleFromModel(m)
	}
	return out, nil
}

// UpdateRule writes a rule's mutable fields back, keeping its id stable and
// advancing updated_at (the most-recently-edited tiebreak).
func (r *Repo) UpdateRule(ctx context.Context, rule Rule) (Rule, error) {
	model, err := r.q.UpdateRule(ctx, sqlc.UpdateRuleParams{
		ID:                rule.ID,
		MerchantSubstring: rule.MerchantSubstring,
		Classification:    string(rule.Classification),
		CategoryID:        nullStringFromPtr(rule.CategoryID),
	})
	if err != nil {
		return Rule{}, err
	}
	return ruleFromModel(model), nil
}

// DeleteRule removes a rule by id.
func (r *Repo) DeleteRule(ctx context.Context, id string) error {
	return r.q.DeleteRule(ctx, id)
}
