package server_test

// Assembled cross-module tests for scheduled items bootstrapping from a
// transaction, attaching to one, and staying current everywhere once matched
// ([ADR-0031]). They drive the real sweep and transactions HTTP handlers
// together over one migrated DB and one schedule.Service — the composition
// root's own wiring shape — so they belong here rather than inside either
// module's own adapter test package: a domain module's adapters may not
// import a peer's, but the composition root legitimately reaches into every
// module's adapters (see AGENTS.md).
//
// schedule_ledger.go's own scheduleLedger is unexported, so it can't be named
// from this external test package; scheduleTestLedger below is this file's own
// copy of the same translation, built from the same two exported services.

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/alecdray/two-cents/src/internal/accounts"
	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/categorization"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/schedule"
	"github.com/alecdray/two-cents/src/internal/sweep"
	sweepAdapters "github.com/alecdray/two-cents/src/internal/sweep/adapters"
	"github.com/alecdray/two-cents/src/internal/transactions"
	txnAdapters "github.com/alecdray/two-cents/src/internal/transactions/adapters"
)

// scheduleTestLedger is this test file's own stand-in for the composition
// root's scheduleLedger, over the same two exported services.
type scheduleTestLedger struct {
	accounts     *accounts.Service
	transactions *transactions.Service
}

func (l scheduleTestLedger) CheckingCandidates(ctx contextx.ContextX, from, to time.Time) ([]schedule.Candidate, error) {
	checkingID, determined, err := l.accounts.CheckingAccountID(ctx)
	if err != nil || !determined {
		return nil, err
	}
	rows, err := l.transactions.MonthTransactions(ctx, from, to.AddDate(0, 0, 1))
	if err != nil {
		return nil, err
	}
	out := make([]schedule.Candidate, 0, len(rows))
	for _, row := range rows {
		if row.AccountID != checkingID {
			continue
		}
		out = append(out, scheduleTestCandidate(row))
	}
	return out, nil
}

func (l scheduleTestLedger) TransactionsByID(ctx contextx.ContextX, ids []string) ([]schedule.Candidate, error) {
	rows, err := l.transactions.TransactionsByID(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]schedule.Candidate, 0, len(rows))
	for _, row := range rows {
		out = append(out, scheduleTestCandidate(row))
	}
	return out, nil
}

func scheduleTestCandidate(row transactions.RecentTransaction) schedule.Candidate {
	return schedule.Candidate{
		TransactionID: row.ID,
		Date:          row.Date,
		Amount:        row.Amount.Amount,
		Merchant:      row.Merchant,
	}
}

// scheduleAssembledStack wires real accounts, categorization, transactions,
// schedule and sweep Services over one migrated DB — the same composition the
// root builds — seeded with one checking account and two dated checking
// transactions via the recording provider, then hands back the two HTTP
// handlers a real request actually reaches: the transaction editor and the
// sweep schedule surface (its per-item modal, opened from either page).
func scheduleAssembledStack(t *testing.T) (*txnAdapters.HttpHandler, *sweepAdapters.HttpHandler, *schedule.Service) {
	t.Helper()
	database := newTestDB(t)
	ctx := testCtx()

	provider := &recordingProvider{
		accounts: []banking.Account{cashProviderAccount("p-check", "Everyday Checking")},
		transactions: []banking.Transaction{
			{
				ID: "t1", AccountID: "p-check",
				Date:     time.Date(2026, time.August, 10, 9, 0, 0, 0, time.UTC),
				Amount:   banking.Money{Amount: 42, Currency: "USD"},
				Merchant: "Corner Market",
			},
			{
				ID: "t2", AccountID: "p-check",
				Date:     time.Date(2026, time.August, 11, 9, 0, 0, 0, time.UTC),
				Amount:   banking.Money{Amount: -1500, Currency: "USD"},
				Merchant: "Employer Inc",
			},
		},
	}

	accountsSvc := accounts.NewService(database, provider, testKey)
	categorizationSvc := categorization.NewService(database, nil)
	txnSvc := transactions.NewService(database, provider, accountsSvc, categorizationSvc, nil, nil)

	if _, err := accountsSvc.RegisterConnection(ctx, "access-token", "item-id"); err != nil {
		t.Fatalf("RegisterConnection: %v", err)
	}
	if err := txnSvc.SyncTransactions(ctx); err != nil {
		t.Fatalf("SyncTransactions: %v", err)
	}

	scheduleSvc := schedule.NewService(database, scheduleTestLedger{accounts: accountsSvc, transactions: txnSvc}, time.UTC)
	sweepSvc := sweep.NewService(accountsSvc, scheduleSvc, database, time.UTC, 500)

	txnHandler := txnAdapters.NewHttpHandler(txnSvc, accountsSvc, categorizationSvc, scheduleSvc)
	sweepHandler := sweepAdapters.NewHttpHandler(sweepSvc, scheduleSvc)
	return txnHandler, sweepHandler, scheduleSvc
}

// getTransactionEditor drives the real GET /transactions/{id}/edit handler and
// returns its body.
func getTransactionEditor(h *txnAdapters.HttpHandler, id string) string {
	req := httptest.NewRequest(http.MethodGet, "/transactions/"+id+"/edit", nil)
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	h.GetEditModal(rec, req)
	return rec.Body.String()
}

// scheduleItemIDNamed reads the id the DB assigned a declared item by its
// name — an auxiliary lookup for building the next request's URL, standing in
// for a page reading the id off a link it just rendered. No assertion in
// these tests is made against this call's return value; every assertion below
// reads a real handler's HTTP response.
func scheduleItemIDNamed(t *testing.T, svc *schedule.Service, name string) string {
	t.Helper()
	items, err := svc.List(testCtx())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, item := range items {
		if item.Name == name {
			return item.ID
		}
	}
	t.Fatalf("no declared item named %q; declared: %+v", name, items)
	return ""
}

// TestTransactionMatchIsMutuallyExclusiveAcrossRealHandlers drives two
// different ways a transaction reaches a match — create-from-transaction and
// attach-to-existing — through the real sweep and transactions HTTP handlers,
// and confirms a transaction already matched can never also be matched a
// second way, and that the transaction editor never shows both an
// already-matched state and an attach/create control for the same
// transaction.
func TestTransactionMatchIsMutuallyExclusiveAcrossRealHandlers(t *testing.T) {
	txnHandler, sweepHandler, scheduleSvc := scheduleAssembledStack(t)

	// Before anything is declared, t1's editor offers the attach/create
	// controls and shows no matched-item link.
	before := getTransactionEditor(txnHandler, "t1")
	if !strings.Contains(before, `data-testid="transaction-editor-schedule-attach"`) ||
		!strings.Contains(before, `data-testid="transaction-editor-schedule-create"`) {
		t.Fatalf("unmatched editor = %s, want the attach/create controls", before)
	}
	if strings.Contains(before, `data-testid="transaction-editor-schedule-item"`) {
		t.Fatalf("unmatched editor = %s, want no matched-item link", before)
	}

	// Create a scheduled item straight from t1 — the create-from-transaction
	// path, opened exactly as the transaction editor's own control would open
	// it, return_to carrying the same transaction's edit endpoint.
	createForm := url.Values{
		"transaction_id": {"t1"},
		"name":           {"Corner Market"},
		"cadence":        {"monthly"},
		"day_of_month":   {"10"},
		"return_to":      {"/transactions/t1/edit"},
	}
	req := httptest.NewRequest(http.MethodPost, "/sweep/schedule", strings.NewReader(createForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	sweepHandler.PostScheduleItem(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create-from-transaction status = %d body = %s", rec.Code, rec.Body.String())
	}
	if trigger := rec.Header().Get("HX-Trigger"); !strings.Contains(trigger, "schedule-changed") {
		t.Errorf("HX-Trigger = %q, want it to announce schedule-changed", trigger)
	}
	if !strings.Contains(rec.Body.String(), `hx-get="/transactions/t1/edit"`) {
		t.Errorf("save response = %s, want the return loader pointed at /transactions/t1/edit", rec.Body.String())
	}

	// Returned to via return_to, t1's editor now shows the matched-item link
	// and neither attach nor create — never both states at once.
	after := getTransactionEditor(txnHandler, "t1")
	if !strings.Contains(after, `data-testid="transaction-editor-schedule-item"`) || !strings.Contains(after, "Corner Market") {
		t.Fatalf("matched editor = %s, want the matched-item link naming Corner Market", after)
	}
	if strings.Contains(after, `data-testid="transaction-editor-schedule-attach"`) || strings.Contains(after, `data-testid="transaction-editor-schedule-create"`) {
		t.Fatalf("matched editor = %s, want no attach/create control alongside the matched link", after)
	}

	// Declare a second, independent item that t1 could in principle be
	// attached to, then attempt exactly that — a real transaction can never
	// belong to two items, however it reaches the second one.
	rentForm := url.Values{
		"name":         {"Rent"},
		"direction":    {"out"},
		"cadence":      {"monthly"},
		"day_of_month": {"1"},
	}
	req = httptest.NewRequest(http.MethodPost, "/sweep/schedule", strings.NewReader(rentForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	sweepHandler.PostScheduleItem(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("declare Rent status = %d body = %s", rec.Code, rec.Body.String())
	}
	rentID := scheduleItemIDNamed(t, scheduleSvc, "Rent")

	attachForm := url.Values{
		"item_id":        {rentID},
		"transaction_id": {"t1"},
		"occurrence":     {"2026-09-01"},
	}
	req = httptest.NewRequest(http.MethodPost, "/sweep/schedule/attach", strings.NewReader(attachForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	sweepHandler.PostAttachScheduleItem(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("second-match attempt status = %d, want 200 (a recoverable inline failure, not a server error) body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "already settles another occurrence") {
		t.Errorf("second-match attempt body = %s, want the recoverable already-settled message", rec.Body.String())
	}

	// The rejected second match must have left the first one exactly as it
	// was: t1's editor still shows it matched to Corner Market, never Rent,
	// and still offers no attach/create control.
	still := getTransactionEditor(txnHandler, "t1")
	if !strings.Contains(still, "Corner Market") {
		t.Errorf("editor after the rejected attach = %s, want it still naming Corner Market", still)
	}
	if strings.Contains(still, `data-testid="transaction-editor-schedule-attach"`) || strings.Contains(still, `data-testid="transaction-editor-schedule-create"`) {
		t.Errorf("editor after the rejected attach = %s, want no attach/create control — the row is still matched", still)
	}
}

// TestScheduleChangeStaysCurrentAcrossTransactionsAndSweepSurfaces drives a
// schedule mutation opened from a transaction's editor (return_to set) and a
// second one opened directly from /sweep (no return_to), and confirms every
// other surface a real request can reach — the origin transaction's editor
// once returned to, the transactions list's per-row badge, and the /sweep
// compact list — reads the current match state on its next fetch, regardless
// of which of the two ways the mutation was made ([ADR-0010], [ADR-0031]).
//
// This proves the data half of the self-refresh: that a fresh fetch of each
// surface is correct after either kind of mutation. The client-side half —
// that the browser actually re-fetches these surfaces without a manual
// reload — is what the schedule-changed event's listener markup and its
// e2e-level drive cover; sweep and transactions may not be imported together
// in either module's own test package ([architecture]'s
// TestSweepReadsNeitherBudgetNorLedger-style boundary), so this composition-root
// test is where the two real handlers can be driven together at all.
func TestScheduleChangeStaysCurrentAcrossTransactionsAndSweepSurfaces(t *testing.T) {
	txnHandler, sweepHandler, _ := scheduleAssembledStack(t)

	// Baseline: nothing declared yet, so the list carries no match badge.
	baseline := getTransactionsList(txnHandler)
	if strings.Contains(baseline, `data-testid="txn-schedule-match"`) {
		t.Fatalf("baseline list = %s, want no schedule-match badge before anything is declared", baseline)
	}

	// Mutation 1: create-from-transaction, opened from t1's editor
	// (return_to set) — the origin-editor path.
	createForm := url.Values{
		"transaction_id": {"t1"},
		"name":           {"Corner Market"},
		"cadence":        {"monthly"},
		"day_of_month":   {"10"},
		"return_to":      {"/transactions/t1/edit"},
	}
	req := httptest.NewRequest(http.MethodPost, "/sweep/schedule", strings.NewReader(createForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	sweepHandler.PostScheduleItem(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create-from-transaction status = %d body = %s", rec.Code, rec.Body.String())
	}
	if trigger := rec.Header().Get("HX-Trigger"); !strings.Contains(trigger, "schedule-changed") {
		t.Errorf("HX-Trigger = %q, want schedule-changed announced on the return_to path too", trigger)
	}

	// The origin editor, returned to via return_to, shows the match.
	editor := getTransactionEditor(txnHandler, "t1")
	if !strings.Contains(editor, `data-testid="transaction-editor-schedule-item"`) || !strings.Contains(editor, "Corner Market") {
		t.Fatalf("origin editor after return_to = %s, want it showing matched to Corner Market", editor)
	}

	// The transactions list's own row badge reflects it too.
	list := getTransactionsList(txnHandler)
	if !strings.Contains(list, `data-testid="txn-schedule-match"`) || !strings.Contains(list, "Corner Market") {
		t.Fatalf("transactions list = %s, want the row's schedule-match badge naming Corner Market", list)
	}

	// The /sweep compact list, fetched via the same endpoint its own hidden
	// listener re-gets on schedule-changed, reflects it too — reached here
	// from a mutation whose origin was a transaction's editor, not /sweep.
	region := getScheduleRegion(sweepHandler)
	if !strings.Contains(region, `data-testid="schedule-row-name"`) || !strings.Contains(region, "Corner Market") {
		t.Fatalf("/sweep schedule region = %s, want the new row naming Corner Market", region)
	}

	// Mutation 2: an edit made directly from /sweep (no return_to) —
	// renaming the very item just created.
	itemID := regexpFirstSubmatch(t, region, `/sweep/schedule/([\w-]+)/edit`)
	editForm := url.Values{
		"name":         {"Corner Market Rent"},
		"direction":    {"out"},
		"cadence":      {"monthly"},
		"day_of_month": {"10"},
	}
	req = httptest.NewRequest(http.MethodPost, "/sweep/schedule/"+itemID, strings.NewReader(editForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", itemID)
	rec = httptest.NewRecorder()
	sweepHandler.PutScheduleItem(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("rename status = %d body = %s", rec.Code, rec.Body.String())
	}
	if trigger := rec.Header().Get("HX-Trigger"); !strings.Contains(trigger, "schedule-changed") {
		t.Errorf("HX-Trigger = %q, want schedule-changed announced on the direct-from-/sweep path too", trigger)
	}
	if !strings.Contains(rec.Body.String(), "Corner Market Rent") {
		t.Errorf("rename response = %s, want the refreshed region naming the item's new name", rec.Body.String())
	}

	// The transaction editor — opened from an entirely different origin than
	// this mutation — reads the rename on its next fetch.
	editorAfterRename := getTransactionEditor(txnHandler, "t1")
	if !strings.Contains(editorAfterRename, "Corner Market Rent") {
		t.Errorf("editor after a /sweep-direct rename = %s, want it reading the new name", editorAfterRename)
	}

	// So does the transactions list's row badge.
	listAfterRename := getTransactionsList(txnHandler)
	if !strings.Contains(listAfterRename, "Corner Market Rent") {
		t.Errorf("transactions list after a /sweep-direct rename = %s, want the badge reading the new name", listAfterRename)
	}
}

// getTransactionsList drives the real GET /transactions handler and returns
// its body.
func getTransactionsList(h *txnAdapters.HttpHandler) string {
	req := httptest.NewRequest(http.MethodGet, "/transactions", nil)
	rec := httptest.NewRecorder()
	h.GetTransactionsPage(rec, req)
	return rec.Body.String()
}

// regexpFirstSubmatch extracts the item id straight out of the rendered
// region's own edit link — the same id a real page would follow, rather than
// one read back through schedule.Service.
func regexpFirstSubmatch(t *testing.T, body, pattern string) string {
	t.Helper()
	m := regexp.MustCompile(pattern).FindStringSubmatch(body)
	if len(m) < 2 {
		t.Fatalf("pattern %q found no match in body: %s", pattern, body)
	}
	return m[1]
}

// getScheduleRegion drives the real GET /sweep/schedule handler — the
// endpoint the schedule region's own hidden listener re-gets on
// schedule-changed — and returns its body.
func getScheduleRegion(h *sweepAdapters.HttpHandler) string {
	req := httptest.NewRequest(http.MethodGet, "/sweep/schedule", nil)
	rec := httptest.NewRecorder()
	h.GetScheduleRegion(rec, req)
	return rec.Body.String()
}
