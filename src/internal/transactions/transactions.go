// Package transactions owns the Transaction rows: it pulls each connection's
// incremental changes through the banking.BankProvider seam, persists them by
// stable provider id (insert-or-update in place, delete on removal), tracks a
// per-connection resume cursor, and serves the recent-activity read model. It
// hosts the recurring sync task and drives Accounts first on every pass, so the
// dependency direction is one-way: transactions imports accounts, never the
// reverse.
//
// It reaches the bank only through the banking seam, so it is provider-agnostic
// and never imports a concrete provider client. Categorization is out of scope
// here — the bank's category strings are stored as-is until the Categorization
// domain lands.
package transactions

import (
	"time"

	"github.com/alecdray/two-cents/src/internal/banking"
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
// transaction joined to its account's display name, ordered most-recent first
// by the caller. It carries only what the activity view renders.
type RecentTransaction struct {
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
