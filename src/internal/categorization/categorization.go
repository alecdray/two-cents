// Package categorization owns the classification taxonomy and the user Rules
// that decide how a money movement is bucketed. It is the decider, never a
// writer of Transaction rows: the pure ResolveCategorization engine returns a
// Decision (Classification + optional Category) and writes nothing, so the
// Transactions module remains the only writer of a transaction's classification.
//
// The module owns two persisted concepts — the Category taxonomy (built-in +
// custom, archive-not-delete) and the Rules — plus the pure engine and the
// CleanMerchantName helper. Its only cross-domain write (re-categorize matching
// transactions when a Rule changes) goes through the injected ReapplyCategorization
// seam, so it imports neither transactions nor any provider client; it depends
// only on core/* and banking (for banking.Category).
package categorization

import (
	"strings"
	"time"
	"unicode"

	"github.com/alecdray/two-cents/src/internal/banking"
)

// Classification is the bucket a transaction lands in. Income and Spending are
// the two flows that drive the wrap; Transfer is money moving between the user's
// own accounts (layer-1 signal only this slice); needs-review is the holding
// state for an inflow nothing else could classify (the user resolves it).
type Classification string

const (
	// Income is money coming in that counts toward the period's earnings.
	Income Classification = "income"
	// Spending is money going out (an inflow mapped to a spending Category is a
	// refund — negative Spending — never Income).
	Spending Classification = "spending"
	// Transfer is money moving between the user's own accounts; it counts as
	// neither income nor spending.
	Transfer Classification = "transfer"
	// NeedsReview is the holding state for an inflow the engine could not
	// classify (no Rule, no income/transfer signal). The engine never guesses an
	// inflow into Income; the user resolves it.
	NeedsReview Classification = "needs_review"
)

// Built-in spending Category ids. These are stable strings seeded by the
// categories migration, so the PFC→Category map below is a static in-code table
// rather than data to look up. They must stay in lockstep with the seed.
const (
	CategoryFoodAndDrink           = "food_and_drink"
	CategoryGeneralMerchandise     = "general_merchandise"
	CategoryTransportation         = "transportation"
	CategoryTravel                 = "travel"
	CategoryRentAndUtilities       = "rent_and_utilities"
	CategoryMedical                = "medical"
	CategoryPersonalCare           = "personal_care"
	CategoryGeneralServices        = "general_services"
	CategoryEntertainment          = "entertainment"
	CategoryHomeImprovement        = "home_improvement"
	CategoryBankFees               = "bank_fees"
	CategoryGovernmentAndNonProfit = "government_and_non_profit"
)

// Bank category (Plaid personal_finance_category) primary values. INCOME maps to
// the Income Classification; the three transfer-signal primaries map to Transfer
// (layer 1, no pairing); the remaining twelve map to a built-in spending
// Category (see pfcSpendingCategory). Together they are the sixteen primaries.
const (
	pfcIncome        = "INCOME"
	pfcTransferIn    = "TRANSFER_IN"
	pfcTransferOut   = "TRANSFER_OUT"
	pfcLoanPayments  = "LOAN_PAYMENTS"
	pfcBankFees      = "BANK_FEES"
	pfcEntertainment = "ENTERTAINMENT"
	pfcFoodAndDrink  = "FOOD_AND_DRINK"
	pfcGeneralMerch  = "GENERAL_MERCHANDISE"
	pfcHomeImprove   = "HOME_IMPROVEMENT"
	pfcMedical       = "MEDICAL"
	pfcPersonalCare  = "PERSONAL_CARE"
	pfcGeneralServ   = "GENERAL_SERVICES"
	pfcGovNonProfit  = "GOVERNMENT_AND_NON_PROFIT"
	pfcTransport     = "TRANSPORTATION"
	pfcTravel        = "TRAVEL"
	pfcRentUtilities = "RENT_AND_UTILITIES"
)

// transferPrimaries are the bank-category primaries whose layer-1 signal is a
// Transfer (money between the user's own accounts, no destination pairing yet).
var transferPrimaries = map[string]bool{
	pfcTransferIn:   true,
	pfcTransferOut:  true,
	pfcLoanPayments: true,
}

// pfcSpendingCategory maps each spending bank-category primary onto the built-in
// spending Category it auto-assigns. The four non-spending primaries (INCOME and
// the three transfer signals) are deliberately absent — they resolve to a
// Classification with no Category.
var pfcSpendingCategory = map[string]string{
	pfcBankFees:      CategoryBankFees,
	pfcEntertainment: CategoryEntertainment,
	pfcFoodAndDrink:  CategoryFoodAndDrink,
	pfcGeneralMerch:  CategoryGeneralMerchandise,
	pfcHomeImprove:   CategoryHomeImprovement,
	pfcMedical:       CategoryMedical,
	pfcPersonalCare:  CategoryPersonalCare,
	pfcGeneralServ:   CategoryGeneralServices,
	pfcGovNonProfit:  CategoryGovernmentAndNonProfit,
	pfcTransport:     CategoryTransportation,
	pfcTravel:        CategoryTravel,
	pfcRentUtilities: CategoryRentAndUtilities,
}

// Category is one value in the classification taxonomy: a built-in spending
// bucket seeded by the migration or a user-created custom one. Categories are
// archived, never hard-deleted; an archived Category keeps existing assignments
// but is excluded from pickers and from future auto-assignment.
type Category struct {
	ID        string
	Name      string
	Builtin   bool
	Archived  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Rule is a user-defined mapping from a cleaned-merchant substring onto an
// outcome. A spending Rule carries the Category to assign; an income/transfer
// Rule carries none. UpdatedAt is the most-recently-edited tiebreak when two
// rules match a merchant with equal-length substrings.
type Rule struct {
	ID                string
	MerchantSubstring string
	Classification    Classification
	CategoryID        *string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// Decision is the engine's verdict: the Classification and, when it is Spending,
// the Category to assign. CategoryID is nil for income, transfer, needs-review,
// and uncategorized spending.
type Decision struct {
	Classification Classification
	CategoryID     *string
}

// ResolveInput carries everything ResolveCategorization needs to decide, with no
// access to storage — the engine is pure. Override is the defensive manual-override
// param (callers pre-skip overridden rows, but if it is set the engine returns it
// unchanged). Categories is the full taxonomy (active and archived) used to skip
// archived targets. Amount follows the seam's sign convention (outflow positive,
// inflow negative).
type ResolveInput struct {
	Override      *Decision
	CleanMerchant string
	Rules         []Rule
	Categories    []Category
	BankCategory  banking.Category
	Amount        float64
}

// ResolveCategorization is the pure decision engine. It reads its input and
// returns a Decision, mutating nothing. Precedence, first match wins:
//
//  1. Manual override present → use it (defensive; callers pre-skip overridden rows).
//  2. A Rule whose substring is a case-insensitive substring of the cleaned
//     merchant → its outcome; the longest matching substring wins, ties broken by
//     most-recently-edited. A spending Rule whose target Category is archived is
//     skipped.
//  3. Bank category: a transfer-signal primary → Transfer; INCOME → Income; else
//     the PFC→Category map → Spending + that built-in Category (skipped if that
//     Category is archived). An inflow here stays Spending — a refund — by the
//     stored negative sign, never re-routed to Income.
//  4. Direction fallback: outflow (amount > 0) → Spending with no Category;
//     inflow → needs-review (never auto-Income).
func ResolveCategorization(in ResolveInput) Decision {
	if in.Override != nil {
		return *in.Override
	}

	archived := archivedCategoryIDs(in.Categories)

	if d, ok := matchRule(in.CleanMerchant, in.Rules, archived); ok {
		return d
	}

	if d, ok := resolveBankCategory(in.BankCategory, archived); ok {
		return d
	}

	if in.Amount > 0 {
		return Decision{Classification: Spending}
	}
	return Decision{Classification: NeedsReview}
}

// matchRule finds the winning Rule for a cleaned merchant: a case-insensitive
// substring match, longest substring first, ties broken by most-recently-edited.
// A spending Rule pointing at an archived Category is skipped so an archived
// target never wins.
func matchRule(merchant string, rules []Rule, archived map[string]bool) (Decision, bool) {
	lowerMerchant := strings.ToLower(merchant)

	var best *Rule
	for i := range rules {
		r := rules[i]
		if r.MerchantSubstring == "" {
			continue
		}
		if !strings.Contains(lowerMerchant, strings.ToLower(r.MerchantSubstring)) {
			continue
		}
		if r.Classification == Spending && r.CategoryID != nil && archived[*r.CategoryID] {
			continue
		}
		if best == nil || rulePreferred(r, *best) {
			best = &rules[i]
		}
	}

	if best == nil {
		return Decision{}, false
	}
	return decisionFromRule(*best), true
}

// rulePreferred reports whether candidate should win over current: longer
// matching substring wins; on an equal-length tie the most-recently-edited
// (later UpdatedAt) wins.
func rulePreferred(candidate, current Rule) bool {
	if len(candidate.MerchantSubstring) != len(current.MerchantSubstring) {
		return len(candidate.MerchantSubstring) > len(current.MerchantSubstring)
	}
	return candidate.UpdatedAt.After(current.UpdatedAt)
}

// decisionFromRule turns a winning Rule into its Decision, carrying the Category
// only for a spending outcome.
func decisionFromRule(r Rule) Decision {
	d := Decision{Classification: r.Classification}
	if r.Classification == Spending && r.CategoryID != nil {
		id := *r.CategoryID
		d.CategoryID = &id
	}
	return d
}

// resolveBankCategory applies the bank-category step: a transfer-signal primary
// is a Transfer; INCOME is Income; a spending primary maps to its built-in
// Category (skipped, returning no decision, if that Category is archived). It
// returns false when the primary is unknown so the caller falls through to the
// direction fallback.
func resolveBankCategory(bankCategory banking.Category, archived map[string]bool) (Decision, bool) {
	primary := strings.ToUpper(strings.TrimSpace(bankCategory.Primary))

	if transferPrimaries[primary] {
		return Decision{Classification: Transfer}, true
	}
	if primary == pfcIncome {
		return Decision{Classification: Income}, true
	}
	if categoryID, ok := pfcSpendingCategory[primary]; ok {
		if archived[categoryID] {
			return Decision{}, false
		}
		id := categoryID
		return Decision{Classification: Spending, CategoryID: &id}, true
	}
	return Decision{}, false
}

// archivedCategoryIDs builds the set of archived Category ids from the taxonomy,
// so the engine can skip archived targets in O(1).
func archivedCategoryIDs(categories []Category) map[string]bool {
	archived := make(map[string]bool)
	for _, c := range categories {
		if c.Archived {
			archived[c.ID] = true
		}
	}
	return archived
}

// CleanMerchantName resolves the cleaned merchant a Rule matches against. The
// provider-cleaned merchant is preferred when present; otherwise the raw
// counterparty is normalized: uppercase-folded, trailing store numbers / numeric
// ids stripped, and whitespace collapsed.
func CleanMerchantName(merchant, counterparty string) string {
	if m := strings.TrimSpace(merchant); m != "" {
		return m
	}
	return normalizeCounterparty(counterparty)
}

// normalizeCounterparty uppercase-folds, collapses runs of whitespace, and strips
// trailing store-number / numeric-id tokens from a raw counterparty.
func normalizeCounterparty(counterparty string) string {
	fields := strings.Fields(strings.ToUpper(counterparty))
	for len(fields) > 0 && isStoreNumber(fields[len(fields)-1]) {
		fields = fields[:len(fields)-1]
	}
	return strings.Join(fields, " ")
}

// isStoreNumber reports whether a trailing token is a store number or numeric id:
// an optional leading '#' or '*' followed by digits only (e.g. "#1234", "0099",
// "*042"). A token with any letter is a name, not a number, and is kept.
func isStoreNumber(token string) bool {
	token = strings.TrimLeft(token, "#*")
	if token == "" {
		return false
	}
	for _, r := range token {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}
