// Package transactions owns the Transaction rows: it pulls each connection's
// incremental changes through the banking.BankProvider seam, persists them by
// stable provider id (insert-or-update in place, delete on removal), tracks a
// per-connection resume cursor, and serves the recent-activity read model. It
// hosts the recurring sync task and drives Accounts first on every pass, so the
// dependency direction is one-way: transactions imports accounts, never the
// reverse.
//
// It reaches the bank only through the banking seam, so it is provider-agnostic
// and never imports a concrete provider client. It is the only writer of a
// transaction's categorization facet: on sync it asks the categorization module
// to decide and then writes the result, and it re-categorizes matching rows when
// a Rule changes (through the server-wired seam). The categorization module
// decides; transactions writes.
package transactions

import (
	"time"

	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/categorization"
)

// Status is the lifecycle position of a stored Transaction. A row is created
// pending or posted and a later pull may move it pending → posted in place; a
// provider removal deletes the row outright (see the domain's Transaction state
// machine), so there is no stored removed status.
type Status string

const (
	// StatusPending marks a transaction the bank has authorized but not yet
	// posted.
	StatusPending Status = "pending"
	// StatusPosted marks a transaction the bank has posted.
	StatusPosted Status = "posted"
)

// Transaction is a single money movement on one Account, persisted by its
// stable provider id. Amount follows the seam's sign convention (outflow
// positive, inflow negative) and is stored as-is. The bank's two-level category
// is recorded verbatim; mapping it onto the app's Classification/Category
// taxonomy is the Categorization domain's job, not stored here.
type Transaction struct {
	// ID is the provider's stable transaction id and the row's primary key.
	ID string
	// AccountID is the owning account's internal id (accounts.id), resolved from
	// the provider account id the seam reports.
	AccountID string
	// Date is the transaction date (not the posted date); its calendar month is
	// the period the transaction belongs to.
	Date time.Time
	// Amount is the signed monetary value: outflow positive, inflow negative.
	Amount banking.Money
	// Merchant is the cleaned/normalized payee used for display and rules.
	Merchant string
	// Counterparty is the raw bank-reported payee, before cleaning.
	Counterparty string
	// Category is the bank's two-level classification, stored as-is.
	Category banking.Category
	// Status is the pending/posted lifecycle position.
	Status Status
}

// RecentTransaction is the read model for the recent-activity list: a stored
// transaction joined to its account's display name (and its assigned Category's
// name), ordered most-recent first by the caller. It carries what the activity
// view renders, including the categorization facet the re-categorize picker
// reflects and mutates.
type RecentTransaction struct {
	// ID is the transaction's stable provider id, the target of the per-row
	// re-categorize control.
	ID string
	// AccountName is the display name of the account the transaction belongs to.
	AccountName string
	// Date is the transaction date.
	Date time.Time
	// Amount is the signed monetary value (outflow positive, inflow negative).
	Amount banking.Money
	// Merchant is the cleaned/normalized payee.
	Merchant string
	// Pending is true while the transaction is authorized but not yet posted.
	Pending bool
	// Classification is the row's resolved bucket (income/spending/transfer/
	// needs_review), or empty before it has been categorized.
	Classification categorization.Classification
	// CategoryID is the assigned spending Category id, nil unless the row is a
	// categorized Spending.
	CategoryID *string
	// CategoryName is the assigned Category's display name, empty unless the row
	// carries a Category.
	CategoryName string
	// TransferSubtype is the resolved subtype of an outflow Transfer leg (a
	// savings contribution or a plain transfer); empty on non-transfer and inflow
	// mirror legs. The transfer chip reads it to render the resolved state.
	TransferSubtype categorization.TransferSubtype
	// TransferDestinationName is the display name of the paired/marked destination
	// account, empty when the destination is unknown (or a past destination's
	// account row has since been removed).
	TransferDestinationName string
	// TransferDestinationUnknown flags an outflow Transfer leg whose destination is
	// still unresolved and unmarked — the state the UI prompts the user to mark. It
	// keys on the destination column (a NULL destination that has not been
	// overridden), never on the subtype, since the subtype cannot tell a
	// resolved-non-savings leg from an unknown one.
	TransferDestinationUnknown bool
}

// ActivityRow is the minimal read model the month-scoped projections (budget
// tracker + month wrap) aggregate over: one transaction's date, signed amount,
// resolved categorization facet, transfer subtype, and pending flag. It carries
// no account state — every row in the range counts regardless of its account's
// hidden/closed state — and no display joins; names are joined later by the
// composing module. Amount keeps the seam's sign convention (outflow positive,
// inflow negative) so the projections can sum it signed.
type ActivityRow struct {
	// ID is the transaction's stable provider id.
	ID string
	// Date is the transaction date (its calendar month is the period it belongs
	// to).
	Date time.Time
	// Amount is the signed monetary value: outflow positive, inflow negative.
	Amount banking.Money
	// Classification is the resolved bucket (income/spending/transfer/needs_review),
	// or empty before the row has been categorized.
	Classification categorization.Classification
	// CategoryID is the assigned spending Category id, nil unless the row is a
	// categorized Spending.
	CategoryID *string
	// TransferSubtype is the resolved subtype of an outflow Transfer leg (a
	// savings contribution or a plain transfer); empty on non-transfer and inflow
	// mirror legs.
	TransferSubtype categorization.TransferSubtype
	// Pending is true while the transaction is authorized but not yet posted.
	Pending bool
}

// categorizationRow carries the inputs the categorization engine needs to
// (re-)resolve one stored transaction, plus its current facet so callers can
// skip overridden / already-categorized rows. It never leaves the module.
type categorizationRow struct {
	ID             string
	Merchant       string
	Counterparty   string
	Category       banking.Category
	Amount         banking.Money
	Classification categorization.Classification
	CategoryID     *string
	Overridden     bool
}

// transferLeg is one stored Transfer leg the auto-pairing pass considers: an
// outflow source leg to resolve, or an inflow candidate to pair against. Amount
// follows the seam's sign convention (outflow positive, inflow negative); the
// pass converts it to integer cents for the exact-amount match. Overridden is the
// sticky transfer facet so the pass can skip a manually-marked leg. It never
// leaves the module.
type transferLeg struct {
	ID         string
	AccountID  string
	Amount     float64
	Date       time.Time
	Overridden bool
}

// transferDestination is the stored transfer facet of one transaction, read back
// for tests (the public read-model fields land with the UI slice): the resolved
// destination account (nil = unknown), the recorded subtype, and whether the
// facet was manually overridden. It never leaves the module.
type transferDestination struct {
	DestinationAccountID *string
	Subtype              categorization.TransferSubtype
	Overridden           bool
}

// statusFromPending maps the seam's pending flag onto the stored status.
func statusFromPending(pending bool) Status {
	if pending {
		return StatusPending
	}
	return StatusPosted
}

// transactionFromBanking builds a stored Transaction from a seam transaction and
// the resolved internal account id, recording the bank fields verbatim.
func transactionFromBanking(bt banking.Transaction, accountID string) Transaction {
	return Transaction{
		ID:           bt.ID,
		AccountID:    accountID,
		Date:         bt.Date,
		Amount:       bt.Amount,
		Merchant:     bt.Merchant,
		Counterparty: bt.Counterparty,
		Category:     bt.Category,
		Status:       statusFromPending(bt.Pending),
	}
}
