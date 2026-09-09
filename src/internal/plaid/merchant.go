package plaid

import (
	"regexp"
	"strings"
)

// storeNumberRe matches trailing store/location numbers and reference ids that
// Plaid's raw transaction name often carries (e.g. "WM SUPERCENTER #1700",
// "STARBUCKS 00042").
var storeNumberRe = regexp.MustCompile(`(?i)[#*]?\s*\b\d{3,}\b\s*$`)

// whitespaceRe collapses runs of whitespace left behind after stripping.
var whitespaceRe = regexp.MustCompile(`\s+`)

// cleanMerchant produces the normalized payee name for display and rule
// matching. It prefers Plaid's already-normalized merchant_name; only when
// that is absent does it normalize the raw transaction name by stripping
// trailing store numbers, collapsing whitespace, and title-casing. Plaid's
// merchant_name is usually present, so the fallback path is the exception.
//
// This is provider-local and stays that way: it resolves one provider's raw
// name into the Merchant the seam reports. The matching-time authority is
// categorization.CleanMerchantName, which prefers this value verbatim when it
// is non-empty — so what this returns is what Rules see.
func cleanMerchant(merchantName, name string) string {
	if merchantName != "" {
		return merchantName
	}
	cleaned := storeNumberRe.ReplaceAllString(name, "")
	cleaned = whitespaceRe.ReplaceAllString(cleaned, " ")
	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" {
		return strings.TrimSpace(name)
	}
	return titleCase(cleaned)
}

// titleCase upper-cases the first letter of each space-separated word and
// lower-cases the rest, e.g. "WM SUPERCENTER" -> "Wm Supercenter".
func titleCase(s string) string {
	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		r := []rune(w)
		r[0] = []rune(strings.ToUpper(string(r[0])))[0]
		words[i] = string(r)
	}
	return strings.Join(words, " ")
}
