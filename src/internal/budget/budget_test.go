package budget

import "testing"

// TestComputeResidual covers the leftover "everything else" residual and the
// total spending budget, including that an archived limit is excluded simply by
// the caller passing only the active limits.
func TestComputeResidual(t *testing.T) {
	t.Run("residual is income minus limits minus savings", func(t *testing.T) {
		limits := []CategoryLimit{
			{CategoryID: "food_and_drink", Limit: 400},
			{CategoryID: "transportation", Limit: 150},
		}
		residual, total := ComputeResidual(3000, 500, limits)
		if residual != 3000-550-500 {
			t.Errorf("residual = %v, want %v", residual, 3000.0-550-500)
		}
		if total != 3000-500 {
			t.Errorf("totalSpendingBudget = %v, want %v", total, 3000.0-500)
		}
	})

	t.Run("total spending budget is income minus savings", func(t *testing.T) {
		_, total := ComputeResidual(2000, 800, nil)
		if total != 1200 {
			t.Errorf("totalSpendingBudget = %v, want 1200", total)
		}
	})

	t.Run("archived limit excluded by caller passing only active limits", func(t *testing.T) {
		active := []CategoryLimit{{CategoryID: "food_and_drink", Limit: 400}}
		// The archived "travel" limit of 600 is simply not in the slice, so it
		// does not draw down the residual.
		residual, _ := ComputeResidual(3000, 500, active)
		if residual != 3000-400-500 {
			t.Errorf("residual = %v, want %v (archived limit must not be counted)", residual, 3000.0-400-500)
		}
	})
}

// TestBalanceCheck covers the balanced vs over-allocated verdict, where
// over-allocated means limits + savings exceed income.
func TestBalanceCheck(t *testing.T) {
	t.Run("balanced when limits plus savings fit income", func(t *testing.T) {
		limits := []CategoryLimit{{CategoryID: "food_and_drink", Limit: 400}}
		if got := BalanceCheck(3000, 500, limits); got != Balanced {
			t.Errorf("BalanceCheck = %v, want %v", got, Balanced)
		}
	})

	t.Run("balanced when limits plus savings exactly equal income", func(t *testing.T) {
		limits := []CategoryLimit{{CategoryID: "food_and_drink", Limit: 500}}
		if got := BalanceCheck(1000, 500, limits); got != Balanced {
			t.Errorf("BalanceCheck (equal) = %v, want %v", got, Balanced)
		}
	})

	t.Run("over-allocated when limits plus savings exceed income", func(t *testing.T) {
		limits := []CategoryLimit{
			{CategoryID: "food_and_drink", Limit: 2000},
			{CategoryID: "transportation", Limit: 800},
		}
		if got := BalanceCheck(3000, 500, limits); got != OverAllocated {
			t.Errorf("BalanceCheck = %v, want %v", got, OverAllocated)
		}
	})
}

// TestIsNoBudget covers the live "no budget set" predicate.
func TestIsNoBudget(t *testing.T) {
	t.Run("all-zero with no limits is no-budget", func(t *testing.T) {
		if !IsNoBudget(Budget{}, nil) {
			t.Errorf("zero config should read as no-budget")
		}
	})
	t.Run("a limit means a budget exists", func(t *testing.T) {
		if IsNoBudget(Budget{}, []CategoryLimit{{CategoryID: "food_and_drink", Limit: 100}}) {
			t.Errorf("a limit should not read as no-budget")
		}
	})
	t.Run("a nonzero target means a budget exists", func(t *testing.T) {
		if IsNoBudget(Budget{IncomeTarget: 3000}, nil) {
			t.Errorf("a nonzero income target should not read as no-budget")
		}
	})
}
