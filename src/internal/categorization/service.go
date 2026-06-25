package categorization

import (
	"errors"
	"fmt"
	"strings"

	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/db"

	"github.com/google/uuid"
)

// ValidationError is a recoverable, user-facing input error (a blank or
// duplicate name, a blank substring, a spending Rule with no Category). Adapters
// surface its Message inline beside the offending control rather than treating it
// as a server failure.
type ValidationError struct {
	Message string
}

func (e ValidationError) Error() string { return e.Message }

// ReapplyCategorization re-runs auto-categorization over the (non-overridden)
// transactions whose cleaned merchant matches one of the given substrings, and
// returns how many were re-categorized. It is the dependency-inverted seam that
// lets a Rule mutation re-categorize matching transactions without categorization
// importing the transactions module: the composition root injects an
// implementation that drives Transactions.ApplyCategorization, so the module
// graph stays acyclic. A nil seam is treated as a no-op returning zero.
type ReapplyCategorization func(ctx contextx.ContextX, substrings []string) (int, error)

// Service owns the Category taxonomy and the user Rules. It writes only its own
// tables; its single cross-domain write (re-categorize matching transactions when
// a Rule changes) goes through the injected ReapplyCategorization seam, called
// after a Rule mutation commits. Category mutations never call the seam.
type Service struct {
	db      *db.DB
	reapply ReapplyCategorization
}

// NewService builds a categorization Service over the database and the injected
// re-categorization seam a Rule mutation triggers after committing its write.
func NewService(d *db.DB, reapply ReapplyCategorization) *Service {
	return &Service{db: d, reapply: reapply}
}

// repo binds a Repo to the global (non-transactional) query handle.
func (s *Service) repo() *Repo {
	return NewRepo(s.db.Queries())
}

// Resolve is the published consumption point the Transactions module calls to
// categorize one money movement. It loads the active Rules and the full taxonomy
// (so the engine can skip archived targets), cleans the merchant, and runs the
// pure ResolveCategorization engine — the module owns the decision, Transactions
// owns the write. It never marks a row overridden; callers pre-skip overridden
// rows. The amount follows the seam's sign convention (outflow positive).
func (s *Service) Resolve(ctx contextx.ContextX, bankCategory banking.Category, merchant, counterparty string, amount banking.Money) (Decision, error) {
	rules, err := s.repo().ListRules(ctx)
	if err != nil {
		return Decision{}, fmt.Errorf("failed to load rules: %w", err)
	}
	categories, err := s.repo().ListCategories(ctx)
	if err != nil {
		return Decision{}, fmt.Errorf("failed to load categories: %w", err)
	}
	return ResolveCategorization(ResolveInput{
		CleanMerchant: CleanMerchantName(merchant, counterparty),
		Rules:         rules,
		Categories:    categories,
		BankCategory:  bankCategory,
		Amount:        amount.Amount,
	}), nil
}

// MatchingRule is one Rule whose substring matches a transaction's cleaned
// merchant, the display name of its target Category (empty for an income/transfer
// Rule or when the target is unresolved), and whether it is the Rule the
// precedence engine actually applies. RulesMatching marks exactly one winner per
// non-empty result.
type MatchingRule struct {
	Rule         Rule
	CategoryName string
	IsWinner     bool
}

// RulesMatching reports the active Rules whose substring matches a transaction's
// cleaned merchant, in precedence order with the governing Rule first and marked
// the winner. It cleans the merchant and applies the archived-target skip exactly
// as Resolve / ResolveCategorization do, reusing the engine's own matching and
// precedence, so the surfaced match set and winner can never drift from how the
// transaction actually categorizes. A merchant no Rule matches yields an empty
// slice. Pure read — it writes nothing.
func (s *Service) RulesMatching(ctx contextx.ContextX, merchant, counterparty string) ([]MatchingRule, error) {
	rules, err := s.repo().ListRules(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load rules: %w", err)
	}
	categories, err := s.repo().ListCategories(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load categories: %w", err)
	}

	matches := matchingRulesBestFirst(CleanMerchantName(merchant, counterparty), rules, archivedCategoryIDs(categories))
	if len(matches) == 0 {
		return nil, nil
	}

	names := categoryNamesByID(categories)
	out := make([]MatchingRule, len(matches))
	for i, r := range matches {
		out[i] = MatchingRule{
			Rule:         r,
			CategoryName: spendingCategoryName(r, names),
			IsWinner:     i == 0,
		}
	}
	return out, nil
}

// categoryNamesByID indexes the taxonomy by id so a matching spending Rule's
// target Category can be named without a second lookup per Rule.
func categoryNamesByID(categories []Category) map[string]string {
	names := make(map[string]string, len(categories))
	for _, c := range categories {
		names[c.ID] = c.Name
	}
	return names
}

// spendingCategoryName resolves a Rule's target Category display name: empty for
// an income/transfer Rule (which carries no Category) or when the id resolves to
// no Category in the taxonomy.
func spendingCategoryName(r Rule, names map[string]string) string {
	if r.Classification != Spending || r.CategoryID == nil {
		return ""
	}
	return names[*r.CategoryID]
}

// --- Categories ---

// ListCategories returns the taxonomy ordered by name. With includeArchived
// false the archived categories are dropped — the picker / auto-assignment view;
// with it true every category is returned (the management page).
func (s *Service) ListCategories(ctx contextx.ContextX, includeArchived bool) ([]Category, error) {
	all, err := s.repo().ListCategories(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list categories: %w", err)
	}
	if includeArchived {
		return all, nil
	}
	active := make([]Category, 0, len(all))
	for _, c := range all {
		if !c.Archived {
			active = append(active, c)
		}
	}
	return active, nil
}

// CreateCategory adds a new custom (non-built-in) spending Category. The name
// must be non-blank and unique (case-insensitive) across the taxonomy; otherwise
// a ValidationError is returned for inline display.
func (s *Service) CreateCategory(ctx contextx.ContextX, name string) (Category, error) {
	name = strings.TrimSpace(name)
	if err := s.validateCategoryName(ctx, name, ""); err != nil {
		return Category{}, err
	}
	created, err := s.repo().CreateCategory(ctx, Category{
		ID:      uuid.NewString(),
		Name:    name,
		Builtin: false,
	})
	if err != nil {
		return Category{}, fmt.Errorf("failed to create category: %w", err)
	}
	return created, nil
}

// RenameCategory renames a Category (built-in or custom), keeping its id stable.
// The new name must be non-blank and unique (case-insensitive) across the other
// categories; otherwise a ValidationError is returned.
func (s *Service) RenameCategory(ctx contextx.ContextX, id, name string) (Category, error) {
	name = strings.TrimSpace(name)
	if err := s.validateCategoryName(ctx, name, id); err != nil {
		return Category{}, err
	}
	current, err := s.repo().GetCategory(ctx, id)
	if err != nil {
		return Category{}, fmt.Errorf("failed to load category: %w", err)
	}
	current.Name = name
	updated, err := s.repo().UpdateCategory(ctx, current)
	if err != nil {
		return Category{}, fmt.Errorf("failed to rename category: %w", err)
	}
	return updated, nil
}

// ArchiveCategory archives a Category: it drops out of pickers and future
// auto-assignment, but existing transactions assigned to it keep that assignment.
// No transactions are re-categorized — archiving never calls the seam.
func (s *Service) ArchiveCategory(ctx contextx.ContextX, id string) (Category, error) {
	return s.setCategoryArchived(ctx, id, true)
}

// UnarchiveCategory restores an archived Category to pickers and auto-assignment.
func (s *Service) UnarchiveCategory(ctx contextx.ContextX, id string) (Category, error) {
	return s.setCategoryArchived(ctx, id, false)
}

func (s *Service) setCategoryArchived(ctx contextx.ContextX, id string, archived bool) (Category, error) {
	current, err := s.repo().GetCategory(ctx, id)
	if err != nil {
		return Category{}, fmt.Errorf("failed to load category: %w", err)
	}
	current.Archived = archived
	updated, err := s.repo().UpdateCategory(ctx, current)
	if err != nil {
		return Category{}, fmt.Errorf("failed to update category: %w", err)
	}
	return updated, nil
}

// validateCategoryName enforces a non-blank, case-insensitively-unique name. On
// rename, excludeID is the category being renamed so its own name does not count
// as a collision; on create it is empty.
func (s *Service) validateCategoryName(ctx contextx.ContextX, name, excludeID string) error {
	if name == "" {
		return ValidationError{Message: "Category name can't be blank."}
	}
	existing, err := s.repo().ListCategories(ctx)
	if err != nil {
		return fmt.Errorf("failed to load categories: %w", err)
	}
	for _, c := range existing {
		if c.ID == excludeID {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(c.Name), name) {
			return ValidationError{Message: "A category with that name already exists."}
		}
	}
	return nil
}

// --- Rules ---

// Rule returns one Rule by id for the editor to prefill on open. Pure read — it
// writes nothing.
func (s *Service) Rule(ctx contextx.ContextX, id string) (Rule, error) {
	rule, err := s.repo().GetRule(ctx, id)
	if err != nil {
		return Rule{}, fmt.Errorf("failed to load rule: %w", err)
	}
	return rule, nil
}

// ListRules returns the Rules, most-recently-edited first.
func (s *Service) ListRules(ctx contextx.ContextX) ([]Rule, error) {
	rules, err := s.repo().ListRules(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list rules: %w", err)
	}
	return rules, nil
}

// CreateRule adds a Rule and, after the write commits, re-categorizes the
// matching non-overridden transactions through the seam — returning how many were
// re-categorized. The substring must be non-blank; a spending Rule requires a
// Category; an income/transfer Rule carries none.
func (s *Service) CreateRule(ctx contextx.ContextX, substring string, classification Classification, categoryID *string) (Rule, int, error) {
	substring = strings.TrimSpace(substring)
	categoryID, err := s.validateRule(substring, classification, categoryID)
	if err != nil {
		return Rule{}, 0, err
	}

	created, err := s.repo().CreateRule(ctx, Rule{
		ID:                uuid.NewString(),
		MerchantSubstring: substring,
		Classification:    classification,
		CategoryID:        categoryID,
	})
	if err != nil {
		return Rule{}, 0, fmt.Errorf("failed to create rule: %w", err)
	}

	count, err := s.reapplyFor(ctx, created.MerchantSubstring)
	return created, count, err
}

// EditRule updates a Rule and re-categorizes the transactions matching either the
// old or the new substring (their union), so rows the Rule no longer matches are
// re-resolved too. Same validation as CreateRule.
func (s *Service) EditRule(ctx contextx.ContextX, id, substring string, classification Classification, categoryID *string) (Rule, int, error) {
	substring = strings.TrimSpace(substring)
	categoryID, err := s.validateRule(substring, classification, categoryID)
	if err != nil {
		return Rule{}, 0, err
	}

	existing, err := s.repo().GetRule(ctx, id)
	if err != nil {
		return Rule{}, 0, fmt.Errorf("failed to load rule: %w", err)
	}
	oldSubstring := existing.MerchantSubstring

	existing.MerchantSubstring = substring
	existing.Classification = classification
	existing.CategoryID = categoryID
	updated, err := s.repo().UpdateRule(ctx, existing)
	if err != nil {
		return Rule{}, 0, fmt.Errorf("failed to edit rule: %w", err)
	}

	// Re-categorize the union of the old and new substrings, so rows the Rule no
	// longer matches are re-resolved too.
	count, err := s.reapplyFor(ctx, oldSubstring, updated.MerchantSubstring)
	return updated, count, err
}

// DeleteRule removes a Rule and re-categorizes the transactions that matched its
// substring, so rows it had categorized fall back to the remaining rules / bank
// category / direction. Returns how many were re-categorized.
func (s *Service) DeleteRule(ctx contextx.ContextX, id string) (int, error) {
	existing, err := s.repo().GetRule(ctx, id)
	if err != nil {
		return 0, fmt.Errorf("failed to load rule: %w", err)
	}
	if err := s.repo().DeleteRule(ctx, id); err != nil {
		return 0, fmt.Errorf("failed to delete rule: %w", err)
	}
	return s.reapplyFor(ctx, existing.MerchantSubstring)
}

// validateRule enforces the Rule invariants and normalizes the Category by
// outcome: a spending Rule must carry a Category; an income/transfer Rule carries
// none, so any supplied Category is cleared.
func (s *Service) validateRule(substring string, classification Classification, categoryID *string) (*string, error) {
	if substring == "" {
		return nil, ValidationError{Message: "Merchant text can't be blank."}
	}
	switch classification {
	case Spending:
		if categoryID == nil || strings.TrimSpace(*categoryID) == "" {
			return nil, ValidationError{Message: "Choose a category for a spending rule."}
		}
		return categoryID, nil
	case Income, Transfer:
		// Income/transfer rules carry no category; drop any supplied value.
		return nil, nil
	default:
		return nil, ValidationError{Message: "Choose a valid outcome for the rule."}
	}
}

// reapplyFor runs the injected re-categorization seam over the de-duplicated set
// of substrings affected by a Rule mutation, returning the count it reports. A
// nil seam (none wired) is a no-op returning zero.
func (s *Service) reapplyFor(ctx contextx.ContextX, substrings ...string) (int, error) {
	if s.reapply == nil {
		return 0, nil
	}
	affected := dedupeNonEmpty(substrings)
	if len(affected) == 0 {
		return 0, nil
	}
	count, err := s.reapply(ctx, affected)
	if err != nil {
		return 0, fmt.Errorf("failed to re-categorize transactions: %w", err)
	}
	return count, nil
}

// dedupeNonEmpty returns the distinct non-empty substrings, preserving order, so
// an edit whose old and new substrings coincide passes a single value.
func dedupeNonEmpty(substrings []string) []string {
	seen := make(map[string]bool, len(substrings))
	out := make([]string, 0, len(substrings))
	for _, s := range substrings {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
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
