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
	"math"
	"sort"
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
	matches := matchingRulesBestFirst(merchant, rules, archived)
	if len(matches) == 0 {
		return Decision{}, false
	}
	return decisionFromRule(matches[0]), true
}

// matchingRulesBestFirst returns every Rule whose substring is a case-insensitive
// substring of the cleaned merchant, in precedence order: longest matching
// substring first, ties broken by most-recently-edited (later UpdatedAt). A
// spending Rule pointing at an archived Category is skipped so an archived target
// never matches. The winning Rule (the one ResolveCategorization would apply) is
// the first element; both the engine and RulesMatching read this single source so
// the surfaced match set and winner cannot drift from how a transaction resolves.
func matchingRulesBestFirst(merchant string, rules []Rule, archived map[string]bool) []Rule {
	lowerMerchant := strings.ToLower(merchant)

	var matches []Rule
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
		matches = append(matches, r)
	}

	sort.SliceStable(matches, func(i, j int) bool {
		return rulePreferred(matches[i], matches[j])
	})
	return matches
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

// TransferSubtype is the second facet of a Transfer: once layer-1 classification
// has decided money is moving between the user's own accounts, the subtype records
// where it went. It lives only on the outflow (source) leg so an aggregation
// counts a contribution once; the paired inflow leg stays a plain Transfer.
type TransferSubtype string

const (
	// SubtypeNone is the unset value: a non-transfer leg, or a Transfer whose
	// subtype has not been resolved (e.g. an inflow mirror leg).
	SubtypeNone TransferSubtype = ""
	// SubtypeSavingsContribution is an outflow Transfer paired to a destination
	// account that counts as savings — the contribution the wrap counts.
	SubtypeSavingsContribution TransferSubtype = "savings_contribution"
	// SubtypePlain is an outflow Transfer that is not a savings contribution:
	// either paired to a non-savings destination (incl. a credit-card payment) or
	// left unresolved because the destination is unknown or ambiguous.
	SubtypePlain TransferSubtype = "plain"
)

// TransferLeg is a candidate inflow leg considered for pairing, annotated with
// its account's counts-as-savings flag. The caller assembles these from stored
// inflow Transfer legs on the user's other connected accounts; the engine itself
// reads no storage and no account state beyond what each leg carries.
type TransferLeg struct {
	TransactionID string
	AccountID     string
	// AmountCents is the inflow amount as round(|amount|*100) — integer cents so
	// the exact-amount match avoids float wobble on banking.Money's float Amount.
	AmountCents int64
	Date        time.Time
	// CountsAsSavings is the destination account's savings flag and the only
	// discriminator between a savings contribution and a plain Transfer. A credit
	// destination simply has this false, so it falls out as plain with no separate
	// kind check.
	CountsAsSavings bool
}

// TransferSubtypeInput carries everything ResolveTransferSubtype needs to pair an
// outflow Transfer leg, with no access to storage — the engine is pure. The
// caller passes only outflow legs (amount > 0) classified Transfer and not
// overridden, with Candidates drawn from inflow Transfer legs on the user's other
// connected accounts.
type TransferSubtypeInput struct {
	SourceAccountID string
	// AmountCents is the outflow amount as round(|amount|*100).
	AmountCents int64
	Date        time.Time
	Candidates  []TransferLeg
	// WindowDays is the inclusive ±N calendar-day window the matching inflow may
	// fall within (3 in this slice).
	WindowDays int
}

// TransferSubtypeDecision is the engine's verdict for an outflow Transfer leg:
// the paired destination account (nil = unknown) and the subtype to record.
type TransferSubtypeDecision struct {
	// DestinationAccountID is the paired destination account, or nil when the
	// destination is unknown (no match) or ambiguous (more than one match).
	DestinationAccountID *string
	// Subtype is SubtypeSavingsContribution or SubtypePlain — never SubtypeNone
	// for a resolved outflow leg; an unknown/ambiguous pairing stays Plain.
	Subtype TransferSubtype
}

// ResolveTransferSubtype is the pure pairing engine. It reads its input and
// returns a decision, mutating nothing. It matches the outflow leg to an inflow
// leg on another connected account — exact amount in cents, within the inclusive
// ±WindowDays calendar-day window — and learns the destination from that pairing:
//
//  1. A candidate matches when it is on a different account, its AmountCents
//     equals the outflow's, and the two dates are within WindowDays of each other
//     by calendar day (the day component only, so a real timestamp with a time
//     part still compares correctly).
//  2. Exactly one match → destination known: the subtype is a savings
//     contribution when that account counts as savings, else a plain Transfer.
//  3. Zero or more than one match → destination unknown, subtype plain. Pairing
//     is conservative: a missing or ambiguous pair is never guessed into a
//     contribution, since a false pair silently hides real spending.
func ResolveTransferSubtype(in TransferSubtypeInput) TransferSubtypeDecision {
	var match *TransferLeg
	matches := 0
	for i := range in.Candidates {
		c := in.Candidates[i]
		if c.AccountID == in.SourceAccountID {
			continue
		}
		if c.AmountCents != in.AmountCents {
			continue
		}
		if calendarDayDiff(in.Date, c.Date) > in.WindowDays {
			continue
		}
		matches++
		match = &in.Candidates[i]
	}

	if matches != 1 {
		return TransferSubtypeDecision{Subtype: SubtypePlain}
	}

	subtype := SubtypePlain
	if match.CountsAsSavings {
		subtype = SubtypeSavingsContribution
	}
	dest := match.AccountID
	return TransferSubtypeDecision{DestinationAccountID: &dest, Subtype: subtype}
}

// calendarDayDiff is the absolute difference in whole calendar days between two
// instants, comparing the day component only (not a raw 24h subtraction) so the
// ±window stays exact even when an instant carries a time-of-day part. Both
// instants are reduced to midnight UTC of their own calendar date, sidestepping
// daylight-saving offsets in the subtraction.
func calendarDayDiff(a, b time.Time) int {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	dayA := time.Date(ay, am, ad, 0, 0, 0, 0, time.UTC)
	dayB := time.Date(by, bm, bd, 0, 0, 0, 0, time.UTC)
	diff := int(dayA.Sub(dayB).Hours() / 24)
	if diff < 0 {
		diff = -diff
	}
	return diff
}

// AmountCents converts a signed monetary amount (the banking.Money sign
// convention, outflow positive / inflow negative) to its magnitude in integer
// cents as round(|amount|*100). Callers building TransferLeg / TransferSubtypeInput
// use it so the engine's exact-amount match never sees float wobble.
func AmountCents(amount float64) int64 {
	if amount < 0 {
		amount = -amount
	}
	return int64(math.Round(amount * 100))
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
