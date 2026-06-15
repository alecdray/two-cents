package budget

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/alecdray/two-cents/src/internal/categorization"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/db"
)

// ValidationError is a recoverable, user-facing input error (a limit on a
// Category that does not exist). Adapters surface its Message inline rather than
// treating it as a server failure.
type ValidationError struct {
	Message string
}

func (e ValidationError) Error() string { return e.Message }

// Service owns the single rolling Budget config and its per-Category limits. It
// writes only its own tables and reads categorization (the Category list) to
// validate limit targets on save and to drop archived targets at read time. It
// imports neither transactions nor accounts.
type Service struct {
	db             *db.DB
	categorization *categorization.Service
}

// NewService builds a budget Service over the database and the categorization
// Service it consults for Category existence and archived state.
func NewService(d *db.DB, categorization *categorization.Service) *Service {
	return &Service{db: d, categorization: categorization}
}

// repo binds a Repo to the global (non-transactional) query handle.
func (s *Service) repo() *Repo {
	return NewRepo(s.db.Queries())
}

// GetBudget returns the stored config and its active-Category limits. An unset
// config reads as a zero Budget with no limits. A limit whose Category is
// archived is dropped from the returned set (inert-while-archived) but kept in
// storage, so it reappears when the Category is un-archived.
func (s *Service) GetBudget(ctx contextx.ContextX) (Budget, []CategoryLimit, error) {
	b, err := s.repo().GetBudget(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			b = Budget{}
		} else {
			return Budget{}, nil, fmt.Errorf("failed to load budget: %w", err)
		}
	}

	stored, err := s.repo().ListCategoryLimits(ctx)
	if err != nil {
		return Budget{}, nil, fmt.Errorf("failed to load category limits: %w", err)
	}

	active, err := s.activeCategoryIDs(ctx)
	if err != nil {
		return Budget{}, nil, err
	}

	limits := make([]CategoryLimit, 0, len(stored))
	for _, l := range stored {
		if active[l.CategoryID] {
			limits = append(limits, l)
		}
	}
	return b, limits, nil
}

// SetBudget upserts the single config row and replaces the per-Category limit
// set in one transaction, then returns the BalanceCheck verdict. Every limit's
// Category must exist (built-in or custom, archived or not); an unknown target
// is a ValidationError and nothing is saved. The verdict is non-blocking: an
// over-allocated plan is still persisted and returns OverAllocated.
func (s *Service) SetBudget(ctx contextx.ContextX, income, savings float64, limits []CategoryLimit) (BalanceStatus, error) {
	known, err := s.knownCategoryIDs(ctx)
	if err != nil {
		return "", err
	}
	for _, l := range limits {
		if !known[l.CategoryID] {
			return "", ValidationError{Message: "A budget limit targets a category that doesn't exist."}
		}
	}

	err = s.db.WithTx(func(tx *db.DB) error {
		repo := NewRepo(tx.Queries())
		if _, err := repo.UpsertBudget(ctx, Budget{IncomeTarget: income, SavingsTarget: savings}); err != nil {
			return fmt.Errorf("failed to save budget: %w", err)
		}
		if err := repo.ReplaceCategoryLimits(ctx, limits); err != nil {
			return fmt.Errorf("failed to save category limits: %w", err)
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	return BalanceCheck(income, savings, limits), nil
}

// activeCategoryIDs returns the set of non-archived Category ids, used to drop
// archived limits from a read.
func (s *Service) activeCategoryIDs(ctx contextx.ContextX) (map[string]bool, error) {
	active, err := s.categorization.ListCategories(ctx, false)
	if err != nil {
		return nil, fmt.Errorf("failed to load categories: %w", err)
	}
	ids := make(map[string]bool, len(active))
	for _, c := range active {
		ids[c.ID] = true
	}
	return ids, nil
}

// knownCategoryIDs returns the set of every Category id (archived included),
// used to validate that a submitted limit targets a real Category.
func (s *Service) knownCategoryIDs(ctx contextx.ContextX) (map[string]bool, error) {
	all, err := s.categorization.ListCategories(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("failed to load categories: %w", err)
	}
	ids := make(map[string]bool, len(all))
	for _, c := range all {
		ids[c.ID] = true
	}
	return ids, nil
}

// IsValidationError reports whether err is (or wraps) a ValidationError, so
// adapters can render its message inline instead of returning a server error.
func IsValidationError(err error) (ValidationError, bool) {
	var ve ValidationError
	if errors.As(err, &ve) {
		return ve, true
	}
	return ValidationError{}, false
}
