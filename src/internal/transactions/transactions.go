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
	// Description is the bank's full raw descriptor; MerchantEntityID, LogoURL, and
	// Website are the bank's merchant-identity detail; PaymentChannel is how the
	// payment was made; CategoryConfidence is the bank's confidence in its category.
	// All read-only bank display detail, refreshed by every sync (ADR-0013).
	Description        string
	MerchantEntityID   string
	LogoURL            string
	Website            string
	PaymentChannel     string
	CategoryConfidence string
	// AuthorizedDate, Datetime, and AuthorizedDatetime are the bank's authorized
	// date and the posted/authorized timestamps; nil when the bank omits them.
	AuthorizedDate     *time.Time
	Datetime           *time.Time
	AuthorizedDatetime *time.Time
	// Counterparties is the bank's structured, typed list of the parties on the
	// transaction (merchant plus any intermediaries); read-only display detail.
	Counterparties []banking.Counterparty
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
	// AccountMask is the account's masked number (typically the last four digits),
	// empty when the provider did not supply one. The editor appends it to the
	// account name to disambiguate same-named accounts.
	AccountMask string
	// Date is the transaction date.
	Date time.Time
	// Amount is the signed monetary value (outflow positive, inflow negative).
	Amount banking.Money
	// Merchant is the cleaned/normalized payee.
	Merchant string
	// Counterparty is the other party as the bank reported it, empty when none was
	// supplied. The editor shows it only when it adds information beyond the merchant.
	Counterparty string
	// CategoryPrimary / CategoryDetailed are the bank's two-level category strings —
	// the raw signal that drove auto-categorization. The editor surfaces them as
	// context for why the row landed where it did.
	CategoryPrimary  string
	CategoryDetailed string
	// Description is the bank's full raw descriptor (richer than Merchant), shown as
	// read-only bank display detail (ADR-0013).
	Description string
	// MerchantEntityID, LogoURL, and Website are the bank's merchant-identity detail;
	// empty when the provider did not recognize the merchant.
	MerchantEntityID string
	LogoURL          string
	Website          string
	// PaymentChannel is how the payment was made ("online", "in store", "other").
	PaymentChannel string
	// CategoryConfidence is the bank's confidence in its category (e.g. "VERY_HIGH"),
	// empty when not reported.
	CategoryConfidence string
	// AuthorizedDate is the bank's authorized date; Datetime and AuthorizedDatetime
	// are the posted/authorized timestamps. All nil when the bank omits them.
	AuthorizedDate     *time.Time
	Datetime           *time.Time
	AuthorizedDatetime *time.Time
	// Counterparties is the bank's structured, typed list of the parties on the
	// transaction (merchant plus any intermediaries); empty when none reported.
	Counterparties []banking.Counterparty
	// Pending is true while the transaction is authorized but not yet posted.
	Pending bool
	// CategorizationOverridden is true when the row's classification/Category is a
	// sticky manual choice rather than the auto-resolved guess.
	CategorizationOverridden bool
	// TransferDestinationOverridden is true when the transfer facet is a sticky manual
	// choice rather than the auto-paired guess.
	TransferDestinationOverridden bool
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

// Filter narrows the /transactions activity read. Both facets are optional and
// compose: a non-empty Merchant matches the cleaned-merchant substring
// (case-insensitive), and NeedsAttention restricts to the needs-attention set — an
// unresolved inflow (needs_review), uncategorized Spending, or an unknown-destination
// outflow Transfer (see docs/domain/README.md). A zero Filter is inactive, the
// signal to show the recent-capped default list instead of a full-history read.
type Filter struct {
	// Merchant is a cleaned-merchant substring to match; empty means no merchant
	// filter. The handler trims it before constructing the Filter.
	Merchant string
	// NeedsAttention restricts the read to the needs-attention set.
	NeedsAttention bool
}

// Active reports whether the filter narrows anything. An inactive filter means the
// page shows the recent-capped default list (RecentTransactions); an active one
// triggers the uncapped full-history FilteredTransactions read.
func (f Filter) Active() bool {
	return f.Merchant != "" || f.NeedsAttention
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
		ID:                 bt.ID,
		AccountID:          accountID,
		Date:               bt.Date,
		Amount:             bt.Amount,
		Merchant:           bt.Merchant,
		Counterparty:       bt.Counterparty,
		Category:           bt.Category,
		Status:             statusFromPending(bt.Pending),
		Description:        bt.Description,
		MerchantEntityID:   bt.MerchantEntityID,
		LogoURL:            bt.LogoURL,
		Website:            bt.Website,
		PaymentChannel:     bt.PaymentChannel,
		CategoryConfidence: bt.CategoryConfidence,
		AuthorizedDate:     bt.AuthorizedDate,
		Datetime:           bt.Datetime,
		AuthorizedDatetime: bt.AuthorizedDatetime,
		Counterparties:     bt.Counterparties,
	}
}
