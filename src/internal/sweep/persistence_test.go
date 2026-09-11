package sweep

import (
	"context"
	"fmt"
	"time"
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
		ID:   "rec",
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

	if err := repo.Save(ctx, rec); err != nil {
		t.Fatalf("Save: %v", err)
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
		ID:   "rec",
		Kind:    KindNeedsAttention,
		Reasons: []NeedsAttentionReason{ReasonCheckingUndetermined, ReasonSavingsUndetermined},
	}

	if err := repo.Save(ctx, rec); err != nil {
		t.Fatalf("Save: %v", err)
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
		ID:   "rec",
		Kind:                KindNumeric,
		CurrentChecking:     3000,
		SavingsUnknown:      true,
		CurrentSavings:      0, // zero because unknown
		TotalSpendingBudget: 2000,
		FixedSafetyMargin:   500,
		SuggestedSweep:      1100,
		Direction:           DirectionCheckingToSavings,
	}

	if err := repo.Save(ctx, rec); err != nil {
		t.Fatalf("Save: %v", err)
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
// A second run must not destroy the first. This is the whole point of the
// append-only model (ADR-0022): a snapshot is a record of what was advised at an
// instant, and overwriting it throws away the history the page navigates.
func TestSecondSaveAppendsRatherThanReplacing(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	earlier := Recommendation{
		ID:                "earlier",
		Kind:              KindNumeric,
		CurrentChecking:   1000,
		SuggestedSweep:    100,
		Direction:         DirectionCheckingToSavings,
		FixedSafetyMargin: 500,
		ComputedAt:        time.Date(2026, time.September, 7, 0, 0, 0, 0, time.UTC),
	}
	if err := repo.Save(ctx, earlier); err != nil {
		t.Fatalf("first Save: %v", err)
	}

	later := Recommendation{
		ID:                "later",
		Kind:              KindNumeric,
		CurrentChecking:   5000,
		SuggestedSweep:    2000,
		Direction:         DirectionCheckingToSavings,
		FixedSafetyMargin: 500,
		ComputedAt:        time.Date(2026, time.September, 8, 9, 30, 0, 0, time.UTC),
	}
	if err := repo.Save(ctx, later); err != nil {
		t.Fatalf("second Save: %v", err)
	}

	got, found, err := repo.LoadLatest(ctx)
	if err != nil {
		t.Fatalf("LoadLatest: %v", err)
	}
	if !found {
		t.Fatal("found=false after two saves")
	}
	if got.CurrentChecking != 5000 {
		t.Errorf("LoadLatest CurrentChecking: want 5000 (the newer snapshot), got %v", got.CurrentChecking)
	}

	all, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("want 2 retained snapshots, got %d", len(all))
	}
	if all[0].CurrentChecking != 5000 || all[1].CurrentChecking != 1000 {
		t.Errorf("want newest-first [5000 1000], got [%v %v]", all[0].CurrentChecking, all[1].CurrentChecking)
	}
}

// Identical figures still append: the record is that the user asked at that
// instant and what the answer was, which collapsing duplicates would destroy.
func TestRepeatedIdenticalRunsEachAppend(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	rec := Recommendation{
		ID:   "rec",
		Kind:              KindNumeric,
		CurrentChecking:   3000,
		SuggestedSweep:    500,
		Direction:         DirectionCheckingToSavings,
		FixedSafetyMargin: 500,
	}
	for i := 0; i < 3; i++ {
		rec.ID = fmt.Sprintf("run-%d", i)
		rec.ComputedAt = time.Date(2026, time.September, 8, 10, i, 0, 0, time.UTC)
		if err := repo.Save(ctx, rec); err != nil {
			t.Fatalf("Save %d: %v", i, err)
		}
	}

	all, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("want 3 snapshots from 3 identical runs, got %d", len(all))
	}
}

// A snapshot is addressable on its own so the page can deep-link one.
func TestLoadByIDReturnsThatSnapshot(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	older := Recommendation{
		ID:                "older",
		Kind:              KindNumeric,
		CurrentChecking:   1000,
		FixedSafetyMargin: 500,
		ComputedAt:        time.Date(2026, time.September, 7, 0, 0, 0, 0, time.UTC),
	}
	if err := repo.Save(ctx, older); err != nil {
		t.Fatalf("Save older: %v", err)
	}
	newer := older
	newer.ID = "newer"
	newer.CurrentChecking = 5000
	newer.ComputedAt = time.Date(2026, time.September, 8, 0, 0, 0, 0, time.UTC)
	if err := repo.Save(ctx, newer); err != nil {
		t.Fatalf("Save newer: %v", err)
	}

	all, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	oldest := all[len(all)-1]
	if oldest.ID == "" {
		t.Fatal("a stored snapshot must carry an id to be addressable")
	}

	got, found, err := repo.LoadByID(ctx, oldest.ID)
	if err != nil {
		t.Fatalf("LoadByID: %v", err)
	}
	if !found {
		t.Fatalf("LoadByID(%q): found=false", oldest.ID)
	}
	if got.CurrentChecking != 1000 {
		t.Errorf("LoadByID returned the wrong snapshot: CurrentChecking = %v, want 1000", got.CurrentChecking)
	}
}

func TestLoadByIDUnknownReturnsFalse(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	_, found, err := repo.LoadByID(ctx, "no-such-snapshot")
	if err != nil {
		t.Fatalf("LoadByID: %v", err)
	}
	if found {
		t.Error("found=true for an id that was never stored")
	}
}

// A needs-attention run after a numeric one is simply the newer snapshot:
// LoadLatest returns it, and the numeric one it followed stays in the history.
func TestLoadLatestReturnsTheNewestKind(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	numeric := Recommendation{
		ID:              "numeric",
		Kind:            KindNumeric,
		CurrentChecking: 3000,
		SuggestedSweep:  1000,
		Direction:       DirectionCheckingToSavings,
		ComputedAt:      time.Date(2026, time.September, 7, 0, 0, 0, 0, time.UTC),
	}
	if err := repo.Save(ctx, numeric); err != nil {
		t.Fatalf("Save numeric: %v", err)
	}

	na := Recommendation{
		ID:         "needs-attention",
		Kind:       KindNeedsAttention,
		Reasons:    []NeedsAttentionReason{ReasonCheckingUndetermined},
		ComputedAt: time.Date(2026, time.September, 8, 0, 0, 0, 0, time.UTC),
	}
	if err := repo.Save(ctx, na); err != nil {
		t.Fatalf("Save needs-attention: %v", err)
	}

	got, found, err := repo.LoadLatest(ctx)
	if err != nil {
		t.Fatalf("LoadLatest: %v", err)
	}
	if !found {
		t.Fatal("found=false after save")
	}
	if got.Kind != KindNeedsAttention {
		t.Errorf("Kind: want the newest snapshot's needs_attention, got %s", got.Kind)
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

// The id is assigned by the caller before saving (the same convention the other
// domain services follow), so the run that produced a snapshot knows its address
// and can send the user straight to it.
func TestSaveStoresTheCallerAssignedID(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	rec := Recommendation{
		ID:                "snapshot-of-record",
		Kind:              KindNumeric,
		CurrentChecking:   3000,
		FixedSafetyMargin: 500,
		ComputedAt:        time.Date(2026, time.September, 8, 10, 0, 0, 0, time.UTC),
	}
	if err := repo.Save(ctx, rec); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, found, err := repo.LoadByID(ctx, "snapshot-of-record")
	if err != nil {
		t.Fatalf("LoadByID: %v", err)
	}
	if !found {
		t.Fatal("LoadByID: found=false for the id the caller assigned")
	}
	if got.CurrentChecking != 3000 {
		t.Errorf("CurrentChecking: want 3000, got %v", got.CurrentChecking)
	}
}

// The card balance is one of the figures that produced the number, so a stored
// snapshot must carry it — the breakdown has to reconstruct from the snapshot
// alone, not from whatever the cards say today.
func TestSaveAndLoadCarriesTheCardBalance(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	rec := Recommendation{
		ID:                  "with-cards",
		Kind:                KindNumeric,
		CurrentChecking:     10000,
		TotalSpendingBudget: 5000,
		CardBalance:         6000,
		Reserve:             6000,
		FixedSafetyMargin:   500,
		SuggestedSweep:      3500,
		Direction:           DirectionCheckingToSavings,
		ComputedAt:          time.Date(2026, time.September, 10, 9, 0, 0, 0, time.UTC),
	}
	if err := repo.Save(ctx, rec); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, found, err := repo.LoadLatest(ctx)
	if err != nil {
		t.Fatalf("LoadLatest: %v", err)
	}
	if !found {
		t.Fatal("found=false after save")
	}
	if got.CardBalance != 6000 {
		t.Errorf("CardBalance = %v, want 6000", got.CardBalance)
	}
}
