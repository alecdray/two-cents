// Package banking holds the provider-agnostic types and the seam through
// which the rest of the app reaches a linked bank: the Account and
// Transaction shapes every domain reads, and the BankProvider interface a
// concrete provider client satisfies.
//
// It is a dependency-graph leaf: it imports no domain module and no provider
// client. Domain modules depend on it for the shared shapes and the seam;
// provider clients depend on it to satisfy the seam. Nothing here references a
// provider-native type or endpoint — that isolation is the whole point of the
// abstraction (see ADR-0002).
package banking

import (
	"time"

	"github.com/alecdray/two-cents/src/internal/core/contextx"
)

// AccountKind is the cash/credit axis that drives the overview. Seeded from
// the bank's reported account type and later user-overridable.
type AccountKind string

const (
	// KindCash covers depository accounts (checking, savings, cash management);
	// their balances are spendable assets.
	KindCash AccountKind = "cash"
	// KindCredit covers credit accounts (credit cards); their balances are
	// amounts owed.
	KindCredit AccountKind = "credit"
)

// Money is a monetary amount in a single currency. Following the domain's
// convention, a Spending outflow is positive and an inflow (refund, deposit)
// is negative; a credit-account balance is the amount owed (positive).
type Money struct {
	// Amount is the value in major currency units (e.g. dollars).
	Amount float64
	// Currency is the ISO 4217 code (e.g. "USD").
	Currency string
}

// Balance is an account's current balance. Known is false when the provider
// reports no balance for the account, so callers can surface it as unknown
// rather than mistaking an absent balance for zero.
type Balance struct {
	// AccountID is the provider identifier of the account this balance is for.
	AccountID string
	// Known is false when the provider reports no balance for the account.
	Known bool
	// Money is the current balance; meaningful only when Known is true.
	Money Money
}

// Account is one financial account exposed through a bank connection. The
// provider supplies the source data; the owning domain seeds kind and the
// counts-as-savings flag from these defaults and lets the user override them.
type Account struct {
	// ID is the provider's stable account identifier.
	ID string
	// Name is the account's display name.
	Name string
	// Kind is the cash/credit axis, defaulted from the bank's account type.
	Kind AccountKind
	// Balance is the account's current balance (or unknown if unreported).
	Balance Balance
	// CountsAsSavings defaults true for savings-type accounts and false
	// otherwise; it marks a transfer's destination as a savings contribution.
	CountsAsSavings bool
}

// Category carries the provider's two-level classification of a transaction.
// It is recorded as-is; mapping it onto the app's Classification/Category
// taxonomy is a separate concern owned elsewhere.
type Category struct {
	// Primary is the broad classification (e.g. "GENERAL_MERCHANDISE").
	Primary string
	// Detailed is the finer-grained classification (e.g.
	// "GENERAL_MERCHANDISE_SUPERSTORES").
	Detailed string
}

// Transaction is a single money movement on one account, as reported by the
// bank. Amount follows the domain sign convention (outflow positive).
type Transaction struct {
	// ID is the provider's stable transaction identifier.
	ID string
	// AccountID is the owning account's provider identifier.
	AccountID string
	// Date is the transaction date (not the posted date); the calendar month
	// of this date is the period the transaction belongs to.
	Date time.Time
	// Amount is the signed monetary value: outflow positive, inflow negative.
	Amount Money
	// Merchant is the cleaned/normalized payee name used for display and rules.
	Merchant string
	// Counterparty is the raw bank-reported payee name, before cleaning.
	Counterparty string
	// Category is the provider's two-level classification.
	Category Category
	// Pending is true while the transaction is authorized but not yet posted.
	Pending bool
}

// TransactionChanges is the result of an incremental transaction sync: the
// transactions added and modified since the prior cursor, the ids removed, and
// the cursor to resume from on the next sync. Cursor advances even when no
// changes are present.
type TransactionChanges struct {
	// Added are transactions newly observed since the prior cursor.
	Added []Transaction
	// Modified are previously seen transactions that changed.
	Modified []Transaction
	// RemovedIDs are the ids of transactions the provider has removed.
	RemovedIDs []string
	// Cursor is the position to resume from on the next sync.
	Cursor string
}

// BankProvider is the seam between the app and a linked bank. A provider's
// external-client service satisfies it, translating provider-native wire
// shapes into the types above. Persistence of cursors, accounts, and
// transactions belongs to the consuming domain modules, never the provider.
type BankProvider interface {
	// ListAccounts returns the accounts exposed through the given bank login.
	ListAccounts(ctx contextx.ContextX, accessToken string) ([]Account, error)
	// GetBalances returns the current balance per account; an account whose
	// balance the provider does not report is surfaced as unknown.
	GetBalances(ctx contextx.ContextX, accessToken string) ([]Balance, error)
	// SyncTransactions pulls the changes since cursor (empty = from the
	// beginning), following pagination to completion and accumulating every
	// page into a single result.
	SyncTransactions(ctx contextx.ContextX, accessToken, cursor string) (TransactionChanges, error)
}
