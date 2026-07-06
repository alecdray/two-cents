package tracker

import "testing"

func strptr(s string) *string { return &s }

// approxEq reports whether two ratios are within a small epsilon; used because a
// UsedRatio like 12000/30000 is not bit-identical to the literal 0.4 in float64.
func approxEq(a, b float64) bool {
	const eps = 1e-9
	d := a - b
	return d < eps && d > -eps
}

// findCategory returns the rendered row for a Category id, or fails.
func findCategory(t *testing.T, v TrackerView, id string) CategoryRemaining {
	t.Helper()
	for _, c := range v.Categories {
		if c.CategoryID == id {
			return c
		}
	}
	t.Fatalf("category %q not found in view (%d rows)", id, len(v.Categories))
	return CategoryRemaining{}
}

// TestBuildTrackerRemaining covers per-Category remaining, including a refund
// (negative net spend) increasing remaining, and the total across budgeted
// Categories.
func TestBuildTrackerRemaining(t *testing.T) {
	in := TrackerInput{
		Budget: &BudgetView{
			IncomeTargetCents:  500000,
			SavingsTargetCents: 100000,
			Limits: []CategoryLimitView{
				{CategoryID: "food", LimitCents: 30000},
				{CategoryID: "travel", LimitCents: 20000},
			},
		},
		Spend: []MonthSpend{
			{CategoryID: strptr("food"), NetCents: 12000},
			// travel has a net refund: a -5000 inflow exceeds nothing, so remaining
			// rises above the limit.
			{CategoryID: strptr("travel"), NetCents: -5000},
		},
		DaysLeftInclusive: 10,
	}

	v := BuildTracker(in)
	if v.NeedsBudget {
		t.Fatal("budget present, but view reports NeedsBudget")
	}

	food := findCategory(t, v, "food")
	if food.RemainingCents != 30000-12000 {
		t.Errorf("food remaining: got %d, want %d", food.RemainingCents, 18000)
	}
	if food.OverBudget {
		t.Error("food is not over budget")
	}

	travel := findCategory(t, v, "travel")
	if travel.RemainingCents != 20000-(-5000) {
		t.Errorf("travel remaining (refund increases it): got %d, want %d", travel.RemainingCents, 25000)
	}

	// Total remaining folds in the everything-else residual, so it equals
	// income - savings - total net spend: 500000 - 100000 - (12000 - 5000).
	if want := int64(500000 - 100000 - 7000); v.TotalRemainingCents != want {
		t.Errorf("total remaining: got %d, want %d", v.TotalRemainingCents, want)
	}
	// Invariant: the total is exactly the sum of the rows shown above it
	// (every Category row plus the everything-else row).
	var sumRows int64
	for _, c := range v.Categories {
		sumRows += c.RemainingCents
	}
	sumRows += v.EverythingElseRemainingCents
	if v.TotalRemainingCents != sumRows {
		t.Errorf("total remaining must equal Σ category rows + everything-else: total=%d, rows=%d", v.TotalRemainingCents, sumRows)
	}

	// UsedRatio is net spend ÷ limit per Category (a net refund goes negative);
	// TotalUsedRatio is total net spend ÷ (income − savings).
	if !approxEq(food.UsedRatio, 12000.0/30000.0) {
		t.Errorf("food used ratio: got %v, want %v", food.UsedRatio, 0.4)
	}
	if !approxEq(travel.UsedRatio, -5000.0/20000.0) {
		t.Errorf("travel used ratio (refund goes negative): got %v, want %v", travel.UsedRatio, -0.25)
	}
	if want := 7000.0 / (500000.0 - 100000.0); !approxEq(v.TotalUsedRatio, want) {
		t.Errorf("total used ratio: got %v, want %v", v.TotalUsedRatio, want)
	}
}

// TestBuildTrackerEverythingElse covers the residual draw-down by both
// unbudgeted-Category spend and uncategorized spend.
func TestBuildTrackerEverythingElse(t *testing.T) {
	in := TrackerInput{
		Budget: &BudgetView{
			IncomeTargetCents:  500000,
			SavingsTargetCents: 100000,
			Limits: []CategoryLimitView{
				{CategoryID: "food", LimitCents: 30000},
			},
		},
		Spend: []MonthSpend{
			{CategoryID: strptr("food"), NetCents: 10000}, // budgeted — not in residual
			{CategoryID: strptr("misc"), NetCents: 7000},  // unbudgeted Category
			{CategoryID: nil, NetCents: 3000},             // uncategorized
		},
		DaysLeftInclusive: 5,
	}

	v := BuildTracker(in)

	// residual = income - Σlimits - savings = 500000 - 30000 - 100000 = 370000
	// draw     = unbudgeted(7000) + uncategorized(3000) = 10000
	wantResidual := int64(500000 - 30000 - 100000)
	wantSpent := int64(10000)
	wantRemaining := wantResidual - wantSpent
	if v.EverythingElseRemainingCents != wantRemaining {
		t.Errorf("everything-else remaining: got %d, want %d", v.EverythingElseRemainingCents, wantRemaining)
	}
	// Everything else is reported like a Category: its "budget" is the residual,
	// its "spend" the unbudgeted + uncategorized draw, and it is not over budget.
	if v.EverythingElseBudgetCents != wantResidual {
		t.Errorf("everything-else budget: got %d, want %d", v.EverythingElseBudgetCents, wantResidual)
	}
	if v.EverythingElseSpentCents != wantSpent {
		t.Errorf("everything-else spent: got %d, want %d", v.EverythingElseSpentCents, wantSpent)
	}
	if v.EverythingElseOverBudget {
		t.Error("everything-else should not be over budget when spend is under the residual")
	}
	if want := float64(wantSpent) / float64(wantResidual); !approxEq(v.EverythingElseUsedRatio, want) {
		t.Errorf("everything-else used ratio: got %v, want %v", v.EverythingElseUsedRatio, want)
	}
}

// TestBuildTrackerPace covers pace at mid-month, on the last day (daysLeft=1 =>
// daily equals remaining), and over budget (clamp to 0 + flag).
func TestBuildTrackerPace(t *testing.T) {
	t.Run("mid-month divides remaining by days left", func(t *testing.T) {
		v := BuildTracker(TrackerInput{
			Budget: &BudgetView{
				IncomeTargetCents: 1000000,
				Limits:            []CategoryLimitView{{CategoryID: "food", LimitCents: 70000}},
			},
			Spend:             []MonthSpend{{CategoryID: strptr("food"), NetCents: 0}},
			DaysLeftInclusive: 7,
		})
		food := findCategory(t, v, "food")
		if food.Pace.DailyCents != 10000 {
			t.Errorf("daily pace: got %d, want %d", food.Pace.DailyCents, 10000)
		}
		if food.Pace.WeeklyCents != 70000 {
			t.Errorf("weekly pace: got %d, want %d", food.Pace.WeeklyCents, 70000)
		}
	})

	t.Run("last day pace equals remaining", func(t *testing.T) {
		v := BuildTracker(TrackerInput{
			Budget: &BudgetView{
				IncomeTargetCents: 1000000,
				Limits:            []CategoryLimitView{{CategoryID: "food", LimitCents: 30000}},
			},
			Spend:             []MonthSpend{{CategoryID: strptr("food"), NetCents: 12000}},
			DaysLeftInclusive: 1,
		})
		food := findCategory(t, v, "food")
		if food.Pace.DailyCents != food.RemainingCents {
			t.Errorf("last-day daily pace: got %d, want remaining %d", food.Pace.DailyCents, food.RemainingCents)
		}
	})

	t.Run("over budget clamps pace to zero and flags", func(t *testing.T) {
		v := BuildTracker(TrackerInput{
			Budget: &BudgetView{
				IncomeTargetCents: 1000000,
				Limits:            []CategoryLimitView{{CategoryID: "food", LimitCents: 10000}},
			},
			Spend:             []MonthSpend{{CategoryID: strptr("food"), NetCents: 15000}},
			DaysLeftInclusive: 5,
		})
		food := findCategory(t, v, "food")
		if !food.OverBudget {
			t.Error("net spend 15000 > limit 10000 should be over budget")
		}
		if food.RemainingCents != -5000 {
			t.Errorf("remaining: got %d, want %d", food.RemainingCents, -5000)
		}
		if food.Pace.DailyCents != 0 || food.Pace.WeeklyCents != 0 {
			t.Errorf("over-budget pace must clamp to 0, got %+v", food.Pace)
		}
	})
}

// TestBuildTrackerProgress covers income and savings progress (so-far, target,
// ratio) — never a pace.
func TestBuildTrackerProgress(t *testing.T) {
	v := BuildTracker(TrackerInput{
		Budget: &BudgetView{
			IncomeTargetCents:  400000,
			SavingsTargetCents: 100000,
		},
		IncomeCents:       300000,
		SavingsCents:      50000,
		DaysLeftInclusive: 10,
	})

	if v.IncomeProgress.SoFarCents != 300000 || v.IncomeProgress.TargetCents != 400000 {
		t.Errorf("income progress figures: got %+v", v.IncomeProgress)
	}
	if v.IncomeProgress.Ratio != 0.75 {
		t.Errorf("income ratio: got %v, want 0.75", v.IncomeProgress.Ratio)
	}
	if v.SavingsProgress.Ratio != 0.5 {
		t.Errorf("savings ratio: got %v, want 0.5", v.SavingsProgress.Ratio)
	}

	t.Run("zero target yields zero ratio, not a divide", func(t *testing.T) {
		v := BuildTracker(TrackerInput{
			Budget:            &BudgetView{IncomeTargetCents: 0, SavingsTargetCents: 100000},
			IncomeCents:       5000,
			DaysLeftInclusive: 10,
		})
		if v.IncomeProgress.Ratio != 0 {
			t.Errorf("zero-target ratio: got %v, want 0", v.IncomeProgress.Ratio)
		}
	})
}

// TestBuildTrackerNoBudget covers no-budget mode: actuals plus the NeedsBudget
// flag, with budget-relative cards omitted. Both a nil budget and an all-zero
// config read as no-budget.
func TestBuildTrackerNoBudget(t *testing.T) {
	cases := []struct {
		name   string
		budget *BudgetView
	}{
		{"nil budget", nil},
		{"all-zero config", &BudgetView{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := BuildTracker(TrackerInput{
				Budget: tc.budget,
				Spend: []MonthSpend{
					{CategoryID: strptr("food"), NetCents: 12000},
					{CategoryID: nil, NetCents: 3000},
				},
				IncomeCents:       200000,
				SavingsCents:      40000,
				DaysLeftInclusive: 10,
			})
			if !v.NeedsBudget {
				t.Fatal("expected NeedsBudget in no-budget mode")
			}
			if v.TotalSpendCents != 15000 {
				t.Errorf("actuals total spend: got %d, want %d", v.TotalSpendCents, 15000)
			}
			if v.IncomeCents != 200000 || v.SavingsCents != 40000 {
				t.Errorf("actuals income/savings: got %d/%d", v.IncomeCents, v.SavingsCents)
			}
			if len(v.Categories) != 0 || v.TotalRemainingCents != 0 || v.EverythingElseRemainingCents != 0 {
				t.Error("budget-relative cards must be omitted in no-budget mode")
			}
		})
	}
}

// TestBuildTrackerActiveLimitsOnly confirms only the passed (active) limits are
// counted: a Category absent from Limits is treated as everything-else spend, not
// a budgeted row, so the caller dropping an archived limit removes it from the
// budgeted set and folds its spend into the residual draw.
func TestBuildTrackerActiveLimitsOnly(t *testing.T) {
	// "archived" has no limit passed; its spend must draw on everything-else, and
	// it must not appear as a budgeted row.
	in := TrackerInput{
		Budget: &BudgetView{
			IncomeTargetCents: 500000,
			Limits: []CategoryLimitView{
				{CategoryID: "food", LimitCents: 30000},
			},
		},
		Spend: []MonthSpend{
			{CategoryID: strptr("food"), NetCents: 10000},
			{CategoryID: strptr("archived"), NetCents: 8000},
		},
		DaysLeftInclusive: 5,
	}

	v := BuildTracker(in)

	if len(v.Categories) != 1 {
		t.Fatalf("only the active limit should render a budgeted row, got %d", len(v.Categories))
	}
	if v.Categories[0].CategoryID != "food" {
		t.Errorf("budgeted row: got %q, want food", v.Categories[0].CategoryID)
	}
	// residual = 500000 - 30000 - 0 = 470000; the archived spend draws it down.
	wantRemaining := int64(500000-30000) - 8000
	if v.EverythingElseRemainingCents != wantRemaining {
		t.Errorf("everything-else after archived draw: got %d, want %d", v.EverythingElseRemainingCents, wantRemaining)
	}
}
