package categorization

import (
	"testing"
)

// TestCreateEditDeleteRulePersist asserts the Rule lifecycle round-trips through
// storage and the list reflects each mutation.
func TestCreateEditDeleteRulePersist(t *testing.T) {
	stub := &reapplyStub{count: 0}
	svc := NewService(newTestDB(t), stub.fn())
	ctx := testCtx()

	created, _, err := svc.CreateRule(ctx, "STARBUCKS", Spending, strptr(CategoryFoodAndDrink))
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}
	if created.ID == "" || created.CategoryID == nil || *created.CategoryID != CategoryFoodAndDrink {
		t.Fatalf("created rule not as expected: %+v", created)
	}

	rules, err := svc.ListRules(ctx)
	if err != nil {
		t.Fatalf("ListRules: %v", err)
	}
	if len(rules) != 1 || rules[0].ID != created.ID {
		t.Fatalf("created rule not listed: %+v", rules)
	}

	edited, _, err := svc.EditRule(ctx, created.ID, "STARBUCKS RESERVE", Transfer, nil)
	if err != nil {
		t.Fatalf("EditRule: %v", err)
	}
	if edited.ID != created.ID {
		t.Errorf("edit changed the id: %q -> %q", created.ID, edited.ID)
	}
	if edited.Classification != Transfer || edited.CategoryID != nil {
		t.Errorf("edit to transfer should clear the category: %+v", edited)
	}

	if _, err := svc.DeleteRule(ctx, created.ID); err != nil {
		t.Fatalf("DeleteRule: %v", err)
	}
	remaining, err := svc.ListRules(ctx)
	if err != nil {
		t.Fatalf("ListRules after delete: %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("rule not deleted: %+v", remaining)
	}
}

// TestRuleValidation asserts a blank substring is rejected and a spending Rule
// requires a Category, both as ValidationErrors.
func TestRuleValidation(t *testing.T) {
	svc := NewService(newTestDB(t), (&reapplyStub{}).fn())
	ctx := testCtx()

	if _, _, err := svc.CreateRule(ctx, "   ", Spending, strptr(CategoryFoodAndDrink)); err == nil {
		t.Errorf("blank substring was accepted")
	} else if _, ok := IsValidationError(err); !ok {
		t.Errorf("blank substring error is not a ValidationError: %v", err)
	}

	if _, _, err := svc.CreateRule(ctx, "WALMART", Spending, nil); err == nil {
		t.Errorf("spending rule with no category was accepted")
	} else if _, ok := IsValidationError(err); !ok {
		t.Errorf("missing-category error is not a ValidationError: %v", err)
	}
}

// TestRuleMutationsInvokeSeam asserts each Rule mutation invokes the seam with
// the right substrings — create passes the new substring, edit passes the union
// of old and new, delete passes the removed substring — and the count is
// surfaced.
func TestRuleMutationsInvokeSeam(t *testing.T) {
	stub := &reapplyStub{count: 3}
	svc := NewService(newTestDB(t), stub.fn())
	ctx := testCtx()

	created, count, err := svc.CreateRule(ctx, "STARBUCKS", Spending, strptr(CategoryFoodAndDrink))
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}
	if count != 3 {
		t.Errorf("create returned count %d, want 3 (the seam's report)", count)
	}
	if len(stub.calls) != 1 || !sameSet(stub.calls[0], []string{"STARBUCKS"}) {
		t.Fatalf("create seam call = %v, want [STARBUCKS]", stub.calls)
	}

	_, count, err = svc.EditRule(ctx, created.ID, "STARBUCKS RESERVE", Spending, strptr(CategoryFoodAndDrink))
	if err != nil {
		t.Fatalf("EditRule: %v", err)
	}
	if count != 3 {
		t.Errorf("edit returned count %d, want 3", count)
	}
	if len(stub.calls) != 2 || !sameSet(stub.calls[1], []string{"STARBUCKS", "STARBUCKS RESERVE"}) {
		t.Fatalf("edit seam call = %v, want union [STARBUCKS, STARBUCKS RESERVE]", stub.calls[1])
	}

	count, err = svc.DeleteRule(ctx, created.ID)
	if err != nil {
		t.Fatalf("DeleteRule: %v", err)
	}
	if count != 3 {
		t.Errorf("delete returned count %d, want 3", count)
	}
	if len(stub.calls) != 3 || !sameSet(stub.calls[2], []string{"STARBUCKS RESERVE"}) {
		t.Fatalf("delete seam call = %v, want [STARBUCKS RESERVE]", stub.calls[2])
	}
}

// TestEditWithUnchangedSubstringPassesOne asserts an edit that leaves the
// substring unchanged passes it just once (deduped), not twice.
func TestEditWithUnchangedSubstringPassesOne(t *testing.T) {
	stub := &reapplyStub{count: 1}
	svc := NewService(newTestDB(t), stub.fn())
	ctx := testCtx()

	created, _, err := svc.CreateRule(ctx, "WALMART", Spending, strptr(CategoryGeneralMerchandise))
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}
	if _, _, err := svc.EditRule(ctx, created.ID, "WALMART", Transfer, nil); err != nil {
		t.Fatalf("EditRule: %v", err)
	}
	if got := stub.calls[1]; len(got) != 1 || got[0] != "WALMART" {
		t.Errorf("edit with unchanged substring passed %v, want a single [WALMART]", got)
	}
}

func sameSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	seen := map[string]bool{}
	for _, g := range got {
		seen[g] = true
	}
	for _, w := range want {
		if !seen[w] {
			return false
		}
	}
	return true
}
