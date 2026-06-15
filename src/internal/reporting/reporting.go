// Package reporting holds the retrospective month-wrap read-model: given a
// month's transaction rows it derives net income, savings contributed, spend by
// Category, the wrap's settling/final state, and a passed-through partial flag.
// Wraps are ACTUALS ONLY — this package never computes or exposes any comparison
// against a Budget.
//
// It is a pure utility leaf: it persists nothing and imports no domain package.
// Every input is a local struct with raw Category ids (*string; nil =
// uncategorized) and money as signed integer cents; the composing layer fetches
// the rows, fills these structs, and joins Category names afterward.
//
// Money sign convention (inherited from banking): outflow positive, inflow
// negative. Spending is summed SIGNED, so a refund (negative inflow) reduces the
// month's spend. Income legs arrive as negative inflows and are negated to a
// positive total; savings-contribution source legs are positive outflows.
package reporting

// Classification mirrors the categorization classification strings so a caller
// can pass string(decision.Classification) directly without importing
// categorization. Transfers are excluded from the wrap's income and spending
// sums; only Income and Spending rows contribute.
const (
	ClassificationIncome   = "income"
	ClassificationSpending = "spending"
	ClassificationTransfer = "transfer"
)

// SubtypeSavingsContribution mirrors the categorization transfer subtype string
// marking the source leg of a savings contribution. Only rows carrying this
// subtype count toward SavingsContributed; the paired mirror inflow leg carries a
// different ("plain") subtype and is never counted.
const SubtypeSavingsContribution = "savings_contribution"

// WrapTxn is one transaction assigned to the month, reduced to the fields the
// wrap needs. AmountCents is signed (outflow positive, inflow negative).
// Classification is one of the Classification* strings; TransferSubtype is empty
// unless the row is a transfer with a resolved subtype.
type WrapTxn struct {
	CategoryID      *string
	Classification  string
	AmountCents     int64
	TransferSubtype string
	Pending         bool
}

// WrapInput is the month's rows plus the partial flag the composer computed from
// the connect/backfill edge (this package does not derive it).
type WrapInput struct {
	Txns    []WrapTxn
	Partial bool
}

// CategorySpend is the signed net spend grouped under one Category id (nil =
// uncategorized) for the month.
type CategorySpend struct {
	CategoryID *string
	NetCents   int64
}

// WrapState is the derived completeness of the wrap: settling while any row is
// still pending, final once every row has posted. It is recomputed each read, not
// stored.
type WrapState string

const (
	// WrapSettling means at least one transaction in the month is still pending.
	WrapSettling WrapState = "settling"
	// WrapFinal means every transaction in the month has posted.
	WrapFinal WrapState = "final"
)

// WrapView is the rendered month-wrap read-model — actuals only, never compared
// against a budget.
type WrapView struct {
	NetIncomeCents          int64
	SavingsContributedCents int64
	SpendByCategory         []CategorySpend
	State                   WrapState
	Partial                 bool
}

// BuildWrap derives the month-wrap view from the input rows. Net income is total
// income minus signed total spending (transfers excluded both sides); savings
// contributed sums the savings-contribution source legs; spend-by-Category groups
// signed net spend; the state is settling if any row is pending else final; the
// partial flag is passed through.
func BuildWrap(in WrapInput) WrapView {
	var totalIncome, totalSpending, savings int64
	state := WrapFinal

	spendByCat := make([]CategorySpend, 0)
	indexOf := make(map[string]int)
	nilIndex := -1

	for _, t := range in.Txns {
		if t.Pending {
			state = WrapSettling
		}

		if t.TransferSubtype == SubtypeSavingsContribution {
			// The source leg is a positive outflow; count it once as the month's
			// contributed savings. The mirror inflow leg never reaches here.
			savings += t.AmountCents
		}

		switch t.Classification {
		case ClassificationIncome:
			// Income legs are inflows (negative); negate to a positive total.
			totalIncome += -t.AmountCents
		case ClassificationSpending:
			// Signed sum: a refund is a negative inflow that reduces spend.
			totalSpending += t.AmountCents

			if t.CategoryID == nil {
				if nilIndex == -1 {
					nilIndex = len(spendByCat)
					spendByCat = append(spendByCat, CategorySpend{})
				}
				spendByCat[nilIndex].NetCents += t.AmountCents
				continue
			}
			id := *t.CategoryID
			i, ok := indexOf[id]
			if !ok {
				i = len(spendByCat)
				indexOf[id] = i
				cid := id
				spendByCat = append(spendByCat, CategorySpend{CategoryID: &cid})
			}
			spendByCat[i].NetCents += t.AmountCents
		}
		// Transfers (and any other classification) contribute to neither income nor
		// spending; savings movement is already captured by subtype above.
	}

	return WrapView{
		NetIncomeCents:          totalIncome - totalSpending,
		SavingsContributedCents: savings,
		SpendByCategory:         spendByCat,
		State:                   state,
		Partial:                 in.Partial,
	}
}
