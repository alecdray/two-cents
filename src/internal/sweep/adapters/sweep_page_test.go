package adapters_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/db"
	"github.com/alecdray/two-cents/src/internal/sweep"
	"github.com/alecdray/two-cents/src/internal/sweep/adapters"

	"github.com/pressly/goose/v3"

	_ "github.com/mattn/go-sqlite3"
)

func newTestDB(t *testing.T) *db.DB {
	t.Helper()

	migrationsDir, err := filepath.Abs("../../../../db/migrations")
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

func newTestService(t *testing.T) *sweep.Service {
	t.Helper()
	database := newTestDB(t)
	return sweep.NewService(nil, nil, nil, database, nil, 500)
}

// getSweepPage drives a GET /sweep through the handler and returns status + body.
func getSweepPage(t *testing.T, svc *sweep.Service) (int, string) {
	t.Helper()
	handler := adapters.NewHttpHandler(svc)
	req := httptest.NewRequest(http.MethodGet, "/sweep", nil)
	rec := httptest.NewRecorder()
	handler.GetPage(rec, req)
	return rec.Code, rec.Body.String()
}

// TestNumericCheckingToSavingsPageActionAndFigures saves a numeric
// recommendation with direction checking->savings and asserts the page leads
// with the correct action line, shows all supporting figures, and that the
// savings balance is rendered (not "unknown").
func TestNumericCheckingToSavingsPageActionAndFigures(t *testing.T) {
	svc := newTestService(t)
	ctx := contextx.NewContextX(context.Background())

	rec := sweep.Recommendation{
		Kind:                  sweep.KindNumeric,
		CurrentChecking:       3500.00,
		CurrentSavings:        1200.50,
		SavingsUnknown:        false,
		TotalSpendingBudget:   2000.00,
		MtdSpending:           800.00,
		SavingsTarget:         300.00,
		MtdSavingsContributed: 100.00,
		Reserve:               1400.00,
		FixedSafetyMargin:     500.00,
		SuggestedSweep:        1600.00,
		Direction:             sweep.DirectionCheckingToSavings,
	}
	if err := svc.SaveLatest(ctx, rec); err != nil {
		t.Fatalf("SaveLatest: %v", err)
	}

	status, body := getSweepPage(t, svc)

	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	// The page must lead with the numeric section.
	if !strings.Contains(body, `data-testid="sweep-numeric"`) {
		t.Error("body missing sweep-numeric section")
	}

	// Criterion 1: action line leads with the correct direction wording.
	if !strings.Contains(body, "Move") {
		t.Error("action line missing 'Move'")
	}
	if !strings.Contains(body, "from checking to savings") {
		t.Error("action line missing 'from checking to savings'")
	}

	// Criterion 2: all supporting figures are present.
	mustContain := map[string]string{
		"current checking testid":        `data-testid="sweep-checking"`,
		"current savings testid":         `data-testid="sweep-savings"`,
		"total spending budget testid":   `data-testid="sweep-spending-budget"`,
		"mtd spending testid":            `data-testid="sweep-mtd-spending"`,
		"savings target testid":          `data-testid="sweep-savings-target"`,
		"mtd savings contributed testid": `data-testid="sweep-mtd-savings"`,
		"reserve testid":                 `data-testid="sweep-reserve"`,
		"safety margin testid":           `data-testid="sweep-safety-margin"`,
		"checking value":                 "$3,500.00",
		"savings value":                  "$1,200.50",
		"spending budget value":          "$2,000.00",
		"mtd spending value":             "$800.00",
		"savings target value":           "$300.00",
		"mtd savings value":              "$100.00",
		"reserve value":                  "$1,400.00",
		"safety margin value":            "$500.00",
	}
	for label, want := range mustContain {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %s (%q)", label, want)
		}
	}

	// The savings balance is known: "unknown" must not appear.
	if strings.Contains(body, ">unknown<") {
		t.Error("savings shown as 'unknown' but balance is known")
	}
}

// TestNumericSavingsToCheckingPageActionLine saves a recommendation with
// direction savings->checking and asserts the page uses the pull wording.
func TestNumericSavingsToCheckingPageActionLine(t *testing.T) {
	svc := newTestService(t)
	ctx := contextx.NewContextX(context.Background())

	rec := sweep.Recommendation{
		Kind:                  sweep.KindNumeric,
		CurrentChecking:       400.00,
		CurrentSavings:        2000.00,
		SavingsUnknown:        false,
		TotalSpendingBudget:   2000.00,
		MtdSpending:           500.00,
		SavingsTarget:         300.00,
		MtdSavingsContributed: 0.00,
		Reserve:               1800.00,
		FixedSafetyMargin:     500.00,
		SuggestedSweep:        -1900.00,
		Direction:             sweep.DirectionSavingsToChecking,
	}
	if err := svc.SaveLatest(ctx, rec); err != nil {
		t.Fatalf("SaveLatest: %v", err)
	}

	status, body := getSweepPage(t, svc)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	// The action line must describe a pull from savings to checking.
	if !strings.Contains(body, "from savings to checking") {
		t.Errorf("pull-direction action line missing 'from savings to checking'; body excerpt: %q",
			extractActionLine(body))
	}
}

// TestNumericUnknownSavingsShowsUnknown saves a numeric recommendation where
// the savings balance is unknown and asserts the page renders "unknown" for the
// savings figure.
func TestNumericUnknownSavingsShowsUnknown(t *testing.T) {
	svc := newTestService(t)
	ctx := contextx.NewContextX(context.Background())

	rec := sweep.Recommendation{
		Kind:                  sweep.KindNumeric,
		CurrentChecking:       3000.00,
		SavingsUnknown:        true,
		CurrentSavings:        0,
		TotalSpendingBudget:   2000.00,
		MtdSpending:           800.00,
		SavingsTarget:         300.00,
		MtdSavingsContributed: 100.00,
		Reserve:               1400.00,
		FixedSafetyMargin:     500.00,
		SuggestedSweep:        1100.00,
		Direction:             sweep.DirectionCheckingToSavings,
	}
	if err := svc.SaveLatest(ctx, rec); err != nil {
		t.Fatalf("SaveLatest: %v", err)
	}

	status, body := getSweepPage(t, svc)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	// Savings balance is unknown: "unknown" must appear in the breakdown.
	if !strings.Contains(body, "unknown") {
		t.Error("body missing 'unknown' for savings balance")
	}
	// The savings row must still be present.
	if !strings.Contains(body, `data-testid="sweep-savings"`) {
		t.Error("savings row missing when savings is unknown")
	}
}

// TestNumericSubDollarAmountNeverShowsZero saves a recommendation with a
// sub-dollar non-zero SuggestedSweep and asserts the headline amount is not
// "$0" — any rounding must be away from zero.
func TestNumericSubDollarAmountNeverShowsZero(t *testing.T) {
	svc := newTestService(t)
	ctx := contextx.NewContextX(context.Background())

	rec := sweep.Recommendation{
		Kind:                  sweep.KindNumeric,
		CurrentChecking:       1000.30,
		CurrentSavings:        500.00,
		SavingsUnknown:        false,
		TotalSpendingBudget:   500.00,
		MtdSpending:           0.00,
		SavingsTarget:         0.00,
		MtdSavingsContributed: 0.00,
		Reserve:               500.00,
		FixedSafetyMargin:     500.00,
		SuggestedSweep:        0.30,
		Direction:             sweep.DirectionCheckingToSavings,
	}
	if err := svc.SaveLatest(ctx, rec); err != nil {
		t.Fatalf("SaveLatest: %v", err)
	}

	status, body := getSweepPage(t, svc)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	// The action line must be present.
	if !strings.Contains(body, "from checking to savings") {
		t.Error("action line missing for sub-dollar positive recommendation")
	}

	// The headline amount must not be "$0.00" or "$0" for a non-zero sweep.
	actionLine := extractActionLine(body)
	if strings.Contains(actionLine, "$0.00") || strings.Contains(actionLine, " $0 ") {
		t.Errorf("headline shows $0 for a non-zero recommendation; action line: %q", actionLine)
	}
}

// TestSweepPageEmptyState asserts the page renders the first-run empty state
// when no recommendation has been stored, shows the sweep-empty testid,
// mentions the next run is on the 7th, and does not render numeric or
// needs-attention sections.
//
// The service is intentionally built with nil accounts/transactions/budget
// dependencies: if the handler called Compute rather than LoadLatest it would
// panic. The test passing proves no computation is triggered.
func TestSweepPageEmptyState(t *testing.T) {
	svc := newTestService(t)

	status, body := getSweepPage(t, svc)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if !strings.Contains(body, `data-testid="sweep-empty"`) {
		t.Error("body missing sweep-empty state testid")
	}
	if !strings.Contains(body, "7th") {
		t.Error("empty state must mention the next run is on the 7th")
	}
	// The empty state must not render numeric or needs-attention sections.
	if strings.Contains(body, `data-testid="sweep-numeric"`) {
		t.Error("empty state must not render the numeric section")
	}
	if strings.Contains(body, `data-testid="sweep-needs-attention"`) {
		t.Error("empty state must not render the needs-attention section")
	}
}

// TestNeedsAttentionBothReasonsListed saves a needs-attention recommendation
// carrying both reasons and asserts the page shows both human-readable reason
// strings — never a number, never the empty state.
func TestNeedsAttentionBothReasonsListed(t *testing.T) {
	svc := newTestService(t)
	ctx := contextx.NewContextX(context.Background())

	rec := sweep.Recommendation{
		Kind: sweep.KindNeedsAttention,
		Reasons: []sweep.NeedsAttentionReason{
			sweep.ReasonCheckingUndetermined,
			sweep.ReasonSavingsUndetermined,
		},
	}
	if err := svc.SaveLatest(ctx, rec); err != nil {
		t.Fatalf("SaveLatest: %v", err)
	}

	status, body := getSweepPage(t, svc)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	// Must render the needs-attention section.
	if !strings.Contains(body, `data-testid="sweep-needs-attention"`) {
		t.Error("body missing sweep-needs-attention section")
	}
	// Must not render numeric or empty sections.
	if strings.Contains(body, `data-testid="sweep-numeric"`) {
		t.Error("needs-attention must not render numeric section")
	}
	if strings.Contains(body, `data-testid="sweep-empty"`) {
		t.Error("needs-attention must not render empty state")
	}
	// Both reason strings must appear.
	if !strings.Contains(body, "Checking account cannot be uniquely identified") {
		t.Error("body missing checking-undetermined reason text")
	}
	if !strings.Contains(body, "Savings account cannot be uniquely identified") {
		t.Error("body missing savings-undetermined reason text")
	}
}

// TestNeedsAttentionSingleReasonListed saves a needs-attention recommendation
// with only the checking reason and asserts only that reason is rendered — the
// savings reason must be absent.
func TestNeedsAttentionSingleReasonListed(t *testing.T) {
	svc := newTestService(t)
	ctx := contextx.NewContextX(context.Background())

	rec := sweep.Recommendation{
		Kind:    sweep.KindNeedsAttention,
		Reasons: []sweep.NeedsAttentionReason{sweep.ReasonCheckingUndetermined},
	}
	if err := svc.SaveLatest(ctx, rec); err != nil {
		t.Fatalf("SaveLatest: %v", err)
	}

	status, body := getSweepPage(t, svc)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	if !strings.Contains(body, "Checking account cannot be uniquely identified") {
		t.Error("body missing the stored reason")
	}
	// Savings reason must not appear when only checking was stored.
	if strings.Contains(body, "Savings account cannot be uniquely identified") {
		t.Error("body must not show savings reason when only checking reason was stored")
	}
}

// TestNeedsAttentionNoComputeTriggered saves a needs-attention result and
// calls the page handler. The service has nil accounts/transactions/budget
// dependencies; if the handler called Compute it would panic with a nil
// dereference. The test passing proves the handler reads only the stored result.
func TestNeedsAttentionNoComputeTriggered(t *testing.T) {
	svc := newTestService(t)
	ctx := contextx.NewContextX(context.Background())

	rec := sweep.Recommendation{
		Kind:    sweep.KindNeedsAttention,
		Reasons: []sweep.NeedsAttentionReason{sweep.ReasonSavingsUndetermined},
	}
	if err := svc.SaveLatest(ctx, rec); err != nil {
		t.Fatalf("SaveLatest: %v", err)
	}

	// If GetPage called Compute the handler would panic — nil accounts service.
	status, _ := getSweepPage(t, svc)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
}

// extractActionLine pulls the text content of the sweep-action-line element
// from the rendered HTML for targeted failure messages.
func extractActionLine(body string) string {
	const start = `data-testid="sweep-action-line"`
	idx := strings.Index(body, start)
	if idx < 0 {
		return "(action line element not found)"
	}
	after := body[idx:]
	open := strings.Index(after, ">")
	if open < 0 {
		return "(could not find opening >)"
	}
	close := strings.Index(after[open:], "<")
	if close < 0 {
		return "(could not find closing <)"
	}
	return strings.TrimSpace(after[open+1 : open+close])
}
