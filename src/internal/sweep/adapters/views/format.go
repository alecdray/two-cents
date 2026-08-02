package views

import (
	"fmt"
	"math"
	"strings"
)

// formatUSD renders an amount as a USD currency string with thousands
// separators, e.g. 1234.5 -> "$1,234.50". Negative amounts get a leading minus
// before the symbol.
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

// headlineAmount formats the headline sweep dollar amount. It rounds to the
// nearest whole dollar (away from zero so a sub-dollar non-zero value never
// becomes $0). The returned value is always positive — direction is expressed
// in the surrounding action wording, not in the sign.
func headlineAmount(sweep float64) string {
	abs := math.Abs(sweep)
	rounded := math.Round(abs)
	// Round away from zero: a non-zero amount must never show as $0.
	if rounded == 0 && abs > 0 {
		rounded = 1
	}
	return formatUSD(rounded)
}
