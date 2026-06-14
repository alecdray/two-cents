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
	"errors"
	"time"

	"github.com/alecdray/two-cents/src/internal/core/contextx"
)

// ErrReauthRequired is the provider-agnostic signal that a bank login has
// expired and the user must re-authenticate before the provider will serve the
// connection's data again. A provider client maps its native login-required
// condition onto this sentinel so consumers can react (e.g. flag a connection
// needs-reconnect) without depending on a provider-specific error.
var ErrReauthRequired = errors.New("bank login requires re-authentication")

// AccountKind is the spending-focused bucket that drives the overview. Seeded
// from the bank's reported account type and later user-overridable.
type AccountKind string

const (
	// KindCash covers depository accounts (checking, savings, cash management);
	// their balances are spendable assets.
	KindCash AccountKind = "cash"
	// KindCredit covers credit accounts (credit cards); their balances are
	// amounts owed.
	KindCredit AccountKind = "credit"
	// KindOther covers everything that is neither spendable cash nor a credit
	// balance (loans, investments, brokerage, …); it sits outside the cash/debt
	// overview.
	KindOther AccountKind = "other"
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
	// Kind is the spending bucket, defaulted from the bank's account type.
	Kind AccountKind
	// Type is the bank's reported account type as a plain string (e.g.
	// "depository", "loan"); provider-agnostic, never a provider-native type.
	Type string
	// Subtype is the bank's reported account subtype as a plain string (e.g.
	// "checking", "mortgage", "401k"); provider-agnostic.
	Subtype string
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

// LinkToken is a short-lived token that authorizes the provider's hosted
// connect flow for a single attempt. The app hands it to the front end, which
// opens the provider UI; on success the flow returns a public token to
// exchange for a durable bank login.
type LinkToken struct {
	// Token is the opaque value the front end passes to the provider's connect
	// flow.
	Token string
	// Mode records which provider produced the token — "real" for a live
	// provider, "fake" for the in-memory stand-in used in development — so the
	// front end can choose between the real connect UI and a simulated one.
	Mode string
}

// Item is a durable bank login established once the connect flow completes. It
// pairs the access token used on every subsequent data call with the provider's
// own identifier for the connection.
type Item struct {
	// AccessToken is the per-login credential every data call carries; the
	// consuming domain persists it (encrypted), not the provider.
	AccessToken string
	// ProviderItemID is the provider's stable identifier for the connection.
	ProviderItemID string
}

// LinkOptions tunes a link-token request. An empty value requests a token for a
// brand-new connection; setting AccessToken requests an update-mode token that
// reconnects an existing login whose credentials have expired.
type LinkOptions struct {
	// AccessToken, when set, names the existing login to reconnect; empty means
	// connect a new bank.
	AccessToken string
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
	// CreateLinkToken mints a token that authorizes the provider's connect flow.
	// With empty LinkOptions it requests a new connection; with an access token
	// it requests an update-mode token to reconnect an expired login.
	CreateLinkToken(ctx contextx.ContextX, opts LinkOptions) (LinkToken, error)
	// ExchangePublicToken trades the public token the completed connect flow
	// returns for a durable Item (access token plus provider connection id).
	ExchangePublicToken(ctx contextx.ContextX, publicToken string) (Item, error)
	// RemoveItem severs a bank login at the provider, invalidating its access
	// token.
	RemoveItem(ctx contextx.ContextX, accessToken string) error
}
