package sweep

import (
	"context"
	"testing"

	"github.com/alecdray/two-cents/src/internal/core/contextx"
)

// TestSafetyMarginRepoDefaultsAndRoundTrips drives the repo straight against a
// freshly migrated database: the migration seeds the one row at $500 (the env
// var's former default), and a set/get round-trips exactly ([ADR-0032]).
func TestSafetyMarginRepoDefaultsAndRoundTrips(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	got, err := repo.GetSafetyMargin(ctx)
	if err != nil {
		t.Fatalf("GetSafetyMargin: %v", err)
	}
	if got != 500 {
		t.Fatalf("GetSafetyMargin = %v, want the migration-seeded 500", got)
	}

	if err := repo.SetSafetyMargin(ctx, 750.5); err != nil {
		t.Fatalf("SetSafetyMargin: %v", err)
	}
	got, err = repo.GetSafetyMargin(ctx)
	if err != nil {
		t.Fatalf("GetSafetyMargin after set: %v", err)
	}
	if got != 750.5 {
		t.Fatalf("GetSafetyMargin = %v, want 750.5 after SetSafetyMargin", got)
	}
}

// TestServiceSafetyMarginValidation covers the Service-level gate:
// SetSafetyMargin accepts 0 (a deliberate no-cushion choice) and rejects
// negative with a ValidationError, never touching the stored value.
func TestServiceSafetyMarginValidation(t *testing.T) {
	ctx := contextx.NewContextX(context.Background())
	svc := NewService(nil, nil, newTestDB(t), nil)

	if err := svc.SetSafetyMargin(ctx, 0); err != nil {
		t.Fatalf("SetSafetyMargin(0): %v, want 0 accepted", err)
	}
	got, err := svc.SafetyMargin(ctx)
	if err != nil {
		t.Fatalf("SafetyMargin: %v", err)
	}
	if got != 0 {
		t.Fatalf("SafetyMargin = %v, want 0", got)
	}

	err = svc.SetSafetyMargin(ctx, -10)
	if _, ok := IsValidationError(err); !ok {
		t.Fatalf("SetSafetyMargin(-10) = %v, want a ValidationError", err)
	}
	got, err = svc.SafetyMargin(ctx)
	if err != nil {
		t.Fatalf("SafetyMargin: %v", err)
	}
	if got != 0 {
		t.Fatalf("SafetyMargin = %v after a rejected negative set, want it unchanged at 0", got)
	}
}
