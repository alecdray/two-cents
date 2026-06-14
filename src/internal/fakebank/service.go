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
		Balance: banking.Balance{
			AccountID: "fake-credit",
			Known:     true,
			Money:     banking.Money{Amount: 450.00, Currency: "USD"},
		},
		CountsAsSavings: false,
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

// SyncTransactions reports no transactions: the stand-in carries accounts and
// balances only. The cursor is echoed unchanged so a draining consumer settles
// immediately.
func (s *Service) SyncTransactions(_ contextx.ContextX, _, cursor string) (banking.TransactionChanges, error) {
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
