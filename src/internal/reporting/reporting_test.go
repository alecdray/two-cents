package reporting

import "testing"

func strptr(s string) *string { return &s }

func findSpend(t *testing.T, v WrapView, id string) CategorySpend {
	t.Helper()
	for _, c := range v.SpendByCategory {
		if c.CategoryID != nil && *c.CategoryID == id {
			return c
		}
	}
	t.Fatalf("category %q not found in spend-by-category", id)
	return CategorySpend{}
}

func uncategorizedSpend(t *testing.T, v WrapView) CategorySpend {
	t.Helper()
	for _, c := range v.SpendByCategory {
		if c.CategoryID == nil {
			return c
		}
	}
	t.Fatalf("uncategorized spend not found")
	return CategorySpend{}
}

// TestBuildWrapNetIncome covers net income with a refund (negative spending) and
// transfers excluded from both sides.
func TestBuildWrapNetIncome(t *testing.T) {
	in := WrapInput{Txns: []WrapTxn{
		// Income legs are inflows (negative); two paychecks.
		{Classification: ClassificationIncome, AmountCents: -300000},
		{Classification: ClassificationIncome, AmountCents: -100000},
		// Spending: a purchase (outflow positive) and a refund (inflow negative).
		{Classification: ClassificationSpending, CategoryID: strptr("food"), AmountCents: 5000},
		{Classification: ClassificationSpending, CategoryID: strptr("food"), AmountCents: -2000},
		// Transfers excluded both sides regardless of sign.
		{Classification: ClassificationTransfer, AmountCents: 50000},
		{Classification: ClassificationTransfer, AmountCents: -50000},
	}}

	v := BuildWrap(in)

	// totalIncome = 400000; totalSpending = 5000 - 2000 = 3000; net = 397000.
	if v.NetIncomeCents != 397000 {
		t.Errorf("net income: got %d, want %d", v.NetIncomeCents, 397000)
	}
	// Gross income is the income legs alone (before subtracting spending) — the
	// drillable figure.
	if v.GrossIncomeCents != 400000 {
		t.Errorf("gross income: got %d, want %d", v.GrossIncomeCents, 400000)
	}
}

// TestBuildWrapSavingsContributed confirms only the savings-contribution source
// leg counts — the paired mirror inflow leg (a plain transfer) is never counted.
func TestBuildWrapSavingsContributed(t *testing.T) {
	in := WrapInput{Txns: []WrapTxn{
		// Source leg: outflow positive, marked as a savings contribution.
		{Classification: ClassificationTransfer, AmountCents: 50000, TransferSubtype: SubtypeSavingsContribution},
		// Mirror inflow leg: a plain transfer (different subtype) — must not count.
		{Classification: ClassificationTransfer, AmountCents: -50000, TransferSubtype: "plain"},
	}}

	v := BuildWrap(in)

	if v.SavingsContributedCents != 50000 {
		t.Errorf("savings contributed (source leg only): got %d, want %d", v.SavingsContributedCents, 50000)
	}
}

// TestBuildWrapSurplus confirms surplus is net income minus savings contributed
// (i.e. income − spending − savings), including a case with savings, and that a
// deficit stays negative (never clamped).
func TestBuildWrapSurplus(t *testing.T) {
	t.Run("net income minus savings contributed", func(t *testing.T) {
		in := WrapInput{Txns: []WrapTxn{
			{Classification: ClassificationIncome, AmountCents: -300000},
			{Classification: ClassificationSpending, CategoryID: strptr("food"), AmountCents: 50000},
			// A $1000 savings contribution (source leg); the mirror inflow never counts.
			{Classification: ClassificationTransfer, AmountCents: 100000, TransferSubtype: SubtypeSavingsContribution},
			{Classification: ClassificationTransfer, AmountCents: -100000, TransferSubtype: "plain"},
		}}

		v := BuildWrap(in)

		// net income = 300000 − 50000 = 250000; savings = 100000; surplus = 150000.
		if v.NetIncomeCents != 250000 {
			t.Fatalf("net income precondition: got %d, want %d", v.NetIncomeCents, 250000)
		}
		if v.SavingsContributedCents != 100000 {
			t.Fatalf("savings precondition: got %d, want %d", v.SavingsContributedCents, 100000)
		}
		if v.SurplusCents != v.NetIncomeCents-v.SavingsContributedCents {
			t.Errorf("surplus must equal net income − savings: got %d, want %d", v.SurplusCents, v.NetIncomeCents-v.SavingsContributedCents)
		}
		if v.SurplusCents != 150000 {
			t.Errorf("surplus: got %d, want %d", v.SurplusCents, 150000)
		}
	})

	t.Run("deficit stays negative", func(t *testing.T) {
		v := BuildWrap(WrapInput{Txns: []WrapTxn{
			{Classification: ClassificationIncome, AmountCents: -100000},
			{Classification: ClassificationSpending, AmountCents: 120000},
			{Classification: ClassificationTransfer, AmountCents: 50000, TransferSubtype: SubtypeSavingsContribution},
		}})
		// net income = 100000 − 120000 = -20000; surplus = -20000 − 50000 = -70000.
		if v.SurplusCents != -70000 {
			t.Errorf("deficit surplus must stay negative: got %d, want %d", v.SurplusCents, -70000)
		}
	})
}

// TestBuildWrapSpendByCategory covers grouping signed net spend by Category id,
// including uncategorized (nil), with refunds netted in.
func TestBuildWrapSpendByCategory(t *testing.T) {
	in := WrapInput{Txns: []WrapTxn{
		{Classification: ClassificationSpending, CategoryID: strptr("food"), AmountCents: 8000},
		{Classification: ClassificationSpending, CategoryID: strptr("food"), AmountCents: -1000}, // refund nets down
		{Classification: ClassificationSpending, CategoryID: strptr("travel"), AmountCents: 20000},
		{Classification: ClassificationSpending, CategoryID: nil, AmountCents: 3000}, // uncategorized
		// Income and transfers never appear in spend-by-category.
		{Classification: ClassificationIncome, AmountCents: -100000},
		{Classification: ClassificationTransfer, AmountCents: 50000},
	}}

	v := BuildWrap(in)

	if got := findSpend(t, v, "food").NetCents; got != 7000 {
		t.Errorf("food net spend: got %d, want %d", got, 7000)
	}
	if got := findSpend(t, v, "travel").NetCents; got != 20000 {
		t.Errorf("travel net spend: got %d, want %d", got, 20000)
	}
	if got := uncategorizedSpend(t, v).NetCents; got != 3000 {
		t.Errorf("uncategorized net spend: got %d, want %d", got, 3000)
	}
	if len(v.SpendByCategory) != 3 {
		t.Errorf("spend-by-category groups: got %d, want 3 (food, travel, uncategorized)", len(v.SpendByCategory))
	}
}

// TestBuildWrapState covers settling (any pending) vs final (all posted).
func TestBuildWrapState(t *testing.T) {
	t.Run("any pending row is settling", func(t *testing.T) {
		v := BuildWrap(WrapInput{Txns: []WrapTxn{
			{Classification: ClassificationSpending, AmountCents: 1000, Pending: false},
			{Classification: ClassificationSpending, AmountCents: 2000, Pending: true},
		}})
		if v.State != WrapSettling {
			t.Errorf("state: got %q, want %q", v.State, WrapSettling)
		}
	})

	t.Run("all posted is final", func(t *testing.T) {
		v := BuildWrap(WrapInput{Txns: []WrapTxn{
			{Classification: ClassificationSpending, AmountCents: 1000},
		}})
		if v.State != WrapFinal {
			t.Errorf("state: got %q, want %q", v.State, WrapFinal)
		}
	})

	t.Run("empty month is final", func(t *testing.T) {
		v := BuildWrap(WrapInput{})
		if v.State != WrapFinal {
			t.Errorf("empty-month state: got %q, want %q", v.State, WrapFinal)
		}
	})
}

// TestBuildWrapPartialPassthrough confirms the partial flag is reflected exactly
// as passed (this package never derives it).
func TestBuildWrapPartialPassthrough(t *testing.T) {
	for _, partial := range []bool{true, false} {
		v := BuildWrap(WrapInput{Partial: partial})
		if v.Partial != partial {
			t.Errorf("partial passthrough: got %v, want %v", v.Partial, partial)
		}
	}
}
