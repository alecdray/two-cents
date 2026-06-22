package views

import (
	"fmt"
	"strings"

	"github.com/alecdray/two-cents/src/internal/categorization"
)

// classificationLabel renders a Classification for the wrap list's row chip; an
// uncategorized row (empty classification) reads "Uncategorized".
func classificationLabel(c categorization.Classification) string {
	switch c {
	case categorization.Income:
		return "Income"
	case categorization.Spending:
		return "Spending"
	case categorization.Transfer:
		return "Transfer"
	case categorization.NeedsReview:
		return "Needs review"
	default:
		return "Uncategorized"
	}
}

// formatUSD renders an amount as a USD currency string with thousands
// separators, e.g. 1234.5 -> "$1,234.50". Negative amounts get a leading minus
// before the symbol. Mirrors the budget page's money formatter.
func formatUSD(amount float64) string {
	sign := ""
	if amount < 0 {
		sign = "-"
		amount = -amount
	}
	whole := int64(amount)
	cents := int64((amount-float64(whole))*100 + 0.5)
	if cents == 100 {
		whole++
		cents = 0
	}

	digits := fmt.Sprintf("%d", whole)
	var grouped strings.Builder
	for i, d := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			grouped.WriteByte(',')
		}
		grouped.WriteRune(d)
	}
	return fmt.Sprintf("%s$%s.%02d", sign, grouped.String(), cents)
}

// barWidth renders a progress percent as an inline CSS width for a bar fill.
func barWidth(percent int) string {
	return fmt.Sprintf("width:%d%%", percent)
}
