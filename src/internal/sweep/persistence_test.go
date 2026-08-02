package sweep

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/alecdray/two-cents/src/internal/core/db"

	"github.com/pressly/goose/v3"

	_ "github.com/mattn/go-sqlite3"
)

// newTestDB opens a temporary SQLite file, runs all migrations, and returns a
// *db.DB backed by it. The file and connection are cleaned up when t ends.
func newTestDB(t *testing.T) *db.DB {
	t.Helper()

	migrationsDir, err := filepath.Abs("../../../db/migrations")
	if err != nil {
		t.Fatalf("resolve migrations dir: %v", err)
	}

	dbPath := filepath.Join(t.TempDir(), "test.db")
	sqlDB, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("set dialect: %v", err)
	}
	if err := goose.Up(sqlDB, migrationsDir); err != nil {
		t.Fatalf("goose up: %v", err)
	}
	return db.WrapSqlDB(sqlDB)
}

func newTestRepo(t *testing.T) *Repo {
	t.Helper()
	return NewRepo(newTestDB(t).Queries())
}

// ptr is already declared in sweep_test.go; do not redeclare it here.

// --- Persistence: store and retrieve ---

// TestSaveAndLoadNumericReturnsEveryFigure saves a numeric recommendation and
// asserts that LoadLatest returns every component figure unchanged.
func TestSaveAndLoadNumericReturnsEveryFigure(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	rec := Recommendation{
		Kind:                  KindNumeric,
		CurrentChecking:       3000.50,
		CurrentSavings:        1500.25,
		SavingsUnknown:        false,
		TotalSpendingBudget:   2000.00,
		MtdSpending:           800.75,
		SavingsTarget:         200.00,
		MtdSavingsContributed: 50.00,
		Reserve:               1349.25,
		FixedSafetyMargin:     500.00,
		SuggestedSweep:        1151.25,
		Direction:             DirectionCheckingToSavings,
	}

	if err := repo.SaveLatest(ctx, rec); err != nil {
		t.Fatalf("SaveLatest: %v", err)
	}

	got, found, err := repo.LoadLatest(ctx)
	if err != nil {
		t.Fatalf("LoadLatest: %v", err)
	}
	if !found {
		t.Fatal("LoadLatest: found=false after save")
	}

	if got.Kind != KindNumeric {
		t.Errorf("Kind: want %s, got %s", KindNumeric, got.Kind)
	}
	if got.CurrentChecking != rec.CurrentChecking {
		t.Errorf("CurrentChecking: want %v, got %v", rec.CurrentChecking, got.CurrentChecking)
	}
	if got.CurrentSavings != rec.CurrentSavings {
		t.Errorf("CurrentSavings: want %v, got %v", rec.CurrentSavings, got.CurrentSavings)
	}
	if got.SavingsUnknown != rec.SavingsUnknown {
		t.Errorf("SavingsUnknown: want %v, got %v", rec.SavingsUnknown, got.SavingsUnknown)
	}
	if got.TotalSpendingBudget != rec.TotalSpendingBudget {
		t.Errorf("TotalSpendingBudget: want %v, got %v", rec.TotalSpendingBudget, got.TotalSpendingBudget)
	}
	if got.MtdSpending != rec.MtdSpending {
		t.Errorf("MtdSpending: want %v, got %v", rec.MtdSpending, got.MtdSpending)
	}
	if got.SavingsTarget != rec.SavingsTarget {
		t.Errorf("SavingsTarget: want %v, got %v", rec.SavingsTarget, got.SavingsTarget)
	}
	if got.MtdSavingsContributed != rec.MtdSavingsContributed {
		t.Errorf("MtdSavingsContributed: want %v, got %v", rec.MtdSavingsContributed, got.MtdSavingsContributed)
	}
	if got.Reserve != rec.Reserve {
		t.Errorf("Reserve: want %v, got %v", rec.Reserve, got.Reserve)
	}
	if got.FixedSafetyMargin != rec.FixedSafetyMargin {
		t.Errorf("FixedSafetyMargin: want %v, got %v", rec.FixedSafetyMargin, got.FixedSafetyMargin)
	}
	if got.SuggestedSweep != rec.SuggestedSweep {
		t.Errorf("SuggestedSweep: want %v, got %v", rec.SuggestedSweep, got.SuggestedSweep)
	}
	if got.Direction != rec.Direction {
		t.Errorf("Direction: want %s, got %s", rec.Direction, got.Direction)
	}
}

// TestSaveAndLoadNeedsAttentionReturnsReasons saves a needs-attention
// recommendation and asserts that LoadLatest returns the reasons list intact.
func TestSaveAndLoadNeedsAttentionReturnsReasons(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	rec := Recommendation{
		Kind:    KindNeedsAttention,
		Reasons: []NeedsAttentionReason{ReasonCheckingUndetermined, ReasonSavingsUndetermined},
	}

	if err := repo.SaveLatest(ctx, rec); err != nil {
		t.Fatalf("SaveLatest: %v", err)
	}

	got, found, err := repo.LoadLatest(ctx)
	if err != nil {
		t.Fatalf("LoadLatest: %v", err)
	}
	if !found {
		t.Fatal("LoadLatest: found=false after save")
	}
	if got.Kind != KindNeedsAttention {
		t.Fatalf("Kind: want %s, got %s", KindNeedsAttention, got.Kind)
	}
	if len(got.Reasons) != 2 {
		t.Fatalf("Reasons: want 2, got %d: %v", len(got.Reasons), got.Reasons)
	}
	hasChecking, hasSavings := false, false
	for _, r := range got.Reasons {
		switch r {
		case ReasonCheckingUndetermined:
			hasChecking = true
		case ReasonSavingsUndetermined:
			hasSavings = true
		}
	}
	if !hasChecking {
		t.Error("Reasons: missing checking_undetermined")
	}
	if !hasSavings {
		t.Error("Reasons: missing savings_undetermined")
	}
}

// TestSaveNumericWithUnknownSavingsBalance saves a numeric recommendation where
// the savings balance is unknown and asserts the round-trip preserves that state.
func TestSaveNumericWithUnknownSavingsBalance(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	rec := Recommendation{
		Kind:                KindNumeric,
		CurrentChecking:     3000,
		SavingsUnknown:      true,
		CurrentSavings:      0, // zero because unknown
		TotalSpendingBudget: 2000,
		FixedSafetyMargin:   500,
		SuggestedSweep:      1100,
		Direction:           DirectionCheckingToSavings,
	}

	if err := repo.SaveLatest(ctx, rec); err != nil {
		t.Fatalf("SaveLatest: %v", err)
	}

	got, found, err := repo.LoadLatest(ctx)
	if err != nil {
		t.Fatalf("LoadLatest: %v", err)
	}
	if !found {
		t.Fatal("found=false after save")
	}
	if !got.SavingsUnknown {
		t.Error("SavingsUnknown: want true, got false")
	}
	if got.CurrentSavings != 0 {
		t.Errorf("CurrentSavings: want 0 when unknown, got %v", got.CurrentSavings)
	}
}

// --- Persistence: idempotent replace ---

// TestSecondSaveReplacesFirst asserts that saving a second recommendation
// replaces the first: LoadLatest always returns the most recent.
func TestSecondSaveReplacesFirst(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	first := Recommendation{
		Kind:            KindNumeric,
		CurrentChecking: 1000,
		SuggestedSweep:  100,
		Direction:       DirectionCheckingToSavings,
		FixedSafetyMargin: 500,
	}
	if err := repo.SaveLatest(ctx, first); err != nil {
		t.Fatalf("first SaveLatest: %v", err)
	}

	second := Recommendation{
		Kind:            KindNumeric,
		CurrentChecking: 5000,
		SuggestedSweep:  2000,
		Direction:       DirectionCheckingToSavings,
		FixedSafetyMargin: 500,
	}
	if err := repo.SaveLatest(ctx, second); err != nil {
		t.Fatalf("second SaveLatest: %v", err)
	}

	got, found, err := repo.LoadLatest(ctx)
	if err != nil {
		t.Fatalf("LoadLatest: %v", err)
	}
	if !found {
		t.Fatal("found=false after two saves")
	}
	if got.CurrentChecking != 5000 {
		t.Errorf("CurrentChecking: want 5000 (second save), got %v", got.CurrentChecking)
	}
	if got.SuggestedSweep != 2000 {
		t.Errorf("SuggestedSweep: want 2000 (second save), got %v", got.SuggestedSweep)
	}
}

// TestSaveNumericThenNeedsAttentionReplacesKind asserts that saving a
// needs-attention recommendation after a numeric one replaces it completely —
// LoadLatest returns the needs-attention result, not the stale numeric.
func TestSaveNumericThenNeedsAttentionReplacesKind(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	numeric := Recommendation{
		Kind:            KindNumeric,
		CurrentChecking: 3000,
		SuggestedSweep:  1000,
		Direction:       DirectionCheckingToSavings,
	}
	if err := repo.SaveLatest(ctx, numeric); err != nil {
		t.Fatalf("SaveLatest numeric: %v", err)
	}

	na := Recommendation{
		Kind:    KindNeedsAttention,
		Reasons: []NeedsAttentionReason{ReasonCheckingUndetermined},
	}
	if err := repo.SaveLatest(ctx, na); err != nil {
		t.Fatalf("SaveLatest needs-attention: %v", err)
	}

	got, found, err := repo.LoadLatest(ctx)
	if err != nil {
		t.Fatalf("LoadLatest: %v", err)
	}
	if !found {
		t.Fatal("found=false after save")
	}
	if got.Kind != KindNeedsAttention {
		t.Errorf("Kind: want needs_attention after replace, got %s", got.Kind)
	}
	if len(got.Reasons) != 1 || got.Reasons[0] != ReasonCheckingUndetermined {
		t.Errorf("Reasons: want [checking_undetermined], got %v", got.Reasons)
	}
}

// --- Persistence: none stored ---

// TestLoadLatestWhenNoneStoredReturnsFalse asserts that LoadLatest on a fresh
// database returns found=false — distinct from any stored result.
func TestLoadLatestWhenNoneStoredReturnsFalse(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	_, found, err := repo.LoadLatest(ctx)
	if err != nil {
		t.Fatalf("LoadLatest: %v", err)
	}
	if found {
		t.Error("found=true on fresh database, want false")
	}
}
