// Package fakebank is a deterministic, in-memory stand-in for a real bank
// provider. It satisfies the banking.BankProvider seam with fixed, canned data
// and no network, so the connection flows can be exercised end to end against
// the real server (SQLite, HTTP, templ all running) with the provider as the
// only swapped part. The composition root selects it by configuration; see
// ADR-0006.
//
// It is a leaf: it imports only the banking seam, core/contextx, and stdlib —
// no persistence, no domain modules, no provider SDK. The data below is stable
// on purpose: callers (including end-to-end tests) may assert on these exact
// accounts, balances, and tokens.
package fakebank

import (
	"time"

	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
)

// linkModeFake tags the link tokens this stand-in mints, telling the front end
// to open the simulated connect flow rather than a live provider's UI.
const linkModeFake = "fake"

// The canned link-token and item values this stand-in always returns. Fixed so
// callers can rely on them across runs.
const (
	fakeLinkToken      = "fake-link-token"
	fakeAccessToken    = "fake-access-token"
	fakeProviderItemID = "fake-item-id"
)

// fixedAccounts is the deterministic set of accounts every bank login through
// this stand-in exposes: a checking and a savings depository account (both
// spendable cash) and a credit card (debt). The balances are echoed by
// GetBalances so the two calls always agree.
var fixedAccounts = []banking.Account{
	{
		ID:      "fake-checking",
		Name:    "Everyday Checking",
		Kind:    banking.KindCash,
		Type:    "depository",
		Subtype: "checking",
		Mask:    "1234",
		Balance: banking.Balance{
			AccountID: "fake-checking",
			Known:     true,
			Money:     banking.Money{Amount: 1200.00, Currency: "USD"},
		},
		CountsAsSavings: false,
	},
	{
		ID:      "fake-savings",
		Name:    "High-Yield Savings",
		Kind:    banking.KindCash,
		Type:    "depository",
		Subtype: "savings",
		Mask:    "5678",
		Balance: banking.Balance{
			AccountID: "fake-savings",
			Known:     true,
			Money:     banking.Money{Amount: 3400.00, Currency: "USD"},
		},
		CountsAsSavings: true,
	},
	{
		ID:      "fake-credit",
		Name:    "Travel Rewards Card",
		Kind:    banking.KindCredit,
		Type:    "credit",
		Subtype: "credit card",
		Mask:    "9012",
		Balance: banking.Balance{
			AccountID: "fake-credit",
			Known:     true,
			Money:     banking.Money{Amount: 450.00, Currency: "USD"},
		},
		CountsAsSavings: false,
	},
}

// fakeSyncCursor is the resume cursor SyncTransactions returns after the initial
// backfill. Presenting it on a later call yields no further changes, so a
// draining consumer settles after exactly one batch.
const fakeSyncCursor = "fake-cursor-v1"

// fakeTxnDate builds a fixed transaction date so the canned set never depends on
// the wall clock — callers (including tests) may assert on these exact dates.
func fakeTxnDate(day int) time.Time {
	return time.Date(2026, time.June, day, 0, 0, 0, 0, time.UTC)
}

// fixedTransactions is the deterministic set the stand-in reports on the initial
// (empty-cursor) sync, spanning the fixed accounts above. It deliberately spans
// the shapes the sync and the categorization ladder must handle:
//
//   - a clearly-spending OUTFLOW (positive amount): groceries on checking,
//     posted, with a spending bank category → Spending + that Category;
//   - an INFLOW (negative amount) with the income signal: a paycheck deposit on
//     checking, posted → Income;
//   - a PENDING outflow with a spending bank category: a coffee charge on the
//     credit card, not yet posted → Spending + that Category;
//   - a TRANSFER-signal outflow: a $500 move out of checking, posted → Transfer;
//   - its matching TRANSFER-signal inflow on savings (−$500, same day): the mirror
//     leg that lets the outflow pair into the counts-as-savings savings account,
//     so the source leg resolves to a Savings contribution and the inflow mirror
//     stays a plain, unlabeled Transfer;
//   - an INFLOW whose bank category is unusable: a side-gig payment whose primary
//     is blank → needs-review, until a Rule matching its merchant re-categorizes
//     it (the target the e2e rule flow relies on).
//
// Amounts follow the seam's sign convention (outflow positive, inflow negative).
// The set is fixed on purpose; change it only with the dependent tests.
var fixedTransactions = []banking.Transaction{
	{
		ID:           "fake-txn-groceries",
		AccountID:    "fake-checking",
		Date:         fakeTxnDate(1),
		Amount:       banking.Money{Amount: 84.32, Currency: "USD"},
		Merchant:     "Whole Foods",
		Counterparty: "WHOLEFDS #4821",
		Category:     banking.Category{Primary: "GENERAL_MERCHANDISE", Detailed: "GENERAL_MERCHANDISE_SUPERSTORES"},
		Pending:      false,
	},
	{
		ID:           "fake-txn-paycheck",
		AccountID:    "fake-checking",
		Date:         fakeTxnDate(2),
		Amount:       banking.Money{Amount: -2400.00, Currency: "USD"},
		Merchant:     "Acme Payroll",
		Counterparty: "ACME CORP DIRECT DEP",
		Category:     banking.Category{Primary: "INCOME", Detailed: "INCOME_WAGES"},
		Pending:      false,
	},
	{
		ID:           "fake-txn-coffee",
		AccountID:    "fake-credit",
		Date:         fakeTxnDate(3),
		Amount:       banking.Money{Amount: 5.75, Currency: "USD"},
		Merchant:     "Blue Bottle Coffee",
		Counterparty: "BLUE BOTTLE 0091",
		Category:     banking.Category{Primary: "FOOD_AND_DRINK", Detailed: "FOOD_AND_DRINK_COFFEE"},
		Pending:      true,
	},
	{
		ID:           "fake-txn-transfer",
		AccountID:    "fake-checking",
		Date:         fakeTxnDate(4),
		Amount:       banking.Money{Amount: 500.00, Currency: "USD"},
		Merchant:     "Rainy Day Savings",
		Counterparty: "TRANSFER TO SAVINGS",
		Category:     banking.Category{Primary: "TRANSFER_OUT", Detailed: "TRANSFER_OUT_SAVINGS"},
		Pending:      false,
	},
	{
		ID:           "fake-txn-transfer-in",
		AccountID:    "fake-savings",
		Date:         fakeTxnDate(4),
		Amount:       banking.Money{Amount: -500.00, Currency: "USD"},
		Merchant:     "Transfer from Checking",
		Counterparty: "TRANSFER FROM CHECKING",
		Category:     banking.Category{Primary: "TRANSFER_IN", Detailed: "TRANSFER_IN_ACCOUNT_TRANSFER"},
		Pending:      false,
	},
	{
		ID:           "fake-txn-sidegig",
		AccountID:    "fake-checking",
		Date:         fakeTxnDate(5),
		Amount:       banking.Money{Amount: -150.00, Currency: "USD"},
		Merchant:     "Side Hustle Co",
		Counterparty: "SIDE HUSTLE CO",
		Category:     banking.Category{Primary: "", Detailed: ""},
		Pending:      false,
	},
}

// Service is the deterministic bank-provider stand-in. It holds no state; every
// method returns the same canned data on every call.
type Service struct{}

// NewService builds the stand-in provider.
func NewService() *Service {
	return &Service{}
}

// compile-time check that Service satisfies the provider seam.
var _ banking.BankProvider = (*Service)(nil)

// ListAccounts returns the fixed set of accounts, independent of the access
// token.
func (s *Service) ListAccounts(_ contextx.ContextX, _ string) ([]banking.Account, error) {
	accounts := make([]banking.Account, len(fixedAccounts))
	copy(accounts, fixedAccounts)
	return accounts, nil
}

// GetBalances returns the current balance of each fixed account, matching the
// balances ListAccounts reports.
func (s *Service) GetBalances(_ contextx.ContextX, _ string) ([]banking.Balance, error) {
	balances := make([]banking.Balance, 0, len(fixedAccounts))
	for _, a := range fixedAccounts {
		balances = append(balances, a.Balance)
	}
	return balances, nil
}

// SyncTransactions reports the fixed backfill on the first pull (empty cursor),
// returning every fixedTransactions row as added and a non-empty resume cursor.
// Presented that cursor on a later pull it reports no changes and echoes the
// cursor, so a re-sync over unchanged data is a no-op and a draining consumer
// settles after one batch.
func (s *Service) SyncTransactions(_ contextx.ContextX, _, cursor string) (banking.TransactionChanges, error) {
	if cursor == "" {
		added := make([]banking.Transaction, len(fixedTransactions))
		copy(added, fixedTransactions)
		return banking.TransactionChanges{Added: added, Cursor: fakeSyncCursor}, nil
	}
	return banking.TransactionChanges{Cursor: cursor}, nil
}

// CreateLinkToken mints the canned link token, tagged "fake" so the front end
// opens the simulated connect flow. New-connect and update mode return the same
// token.
func (s *Service) CreateLinkToken(_ contextx.ContextX, _ banking.LinkOptions) (banking.LinkToken, error) {
	return banking.LinkToken{Token: fakeLinkToken, Mode: linkModeFake}, nil
}

// ExchangePublicToken returns the canned durable bank login, ignoring the
// public token.
func (s *Service) ExchangePublicToken(_ contextx.ContextX, _ string) (banking.Item, error) {
	return banking.Item{AccessToken: fakeAccessToken, ProviderItemID: fakeProviderItemID}, nil
}

// RemoveItem is a no-op: there is no remote login to sever.
func (s *Service) RemoveItem(_ contextx.ContextX, _ string) error {
	return nil
}
