package plaid

import (
	"time"

	"github.com/alecdray/two-cents/src/internal/banking"
)

// Plaid account types and subtypes we map from. Plaid exposes more types
// (investment, loan) but v1 covers only depository and credit accounts.
const (
	accountTypeDepository = "depository"
	accountTypeCredit     = "credit"

	accountSubtypeSavings = "savings"
)

// dateLayout is the format Plaid uses for transaction dates (calendar date,
// no time component).
const dateLayout = "2006-01-02"

// account mirrors a Plaid account object as returned by /accounts/get and
// /accounts/balance/get.
type account struct {
	AccountID    string  `json:"account_id"`
	Name         string  `json:"name"`
	OfficialName string  `json:"official_name"`
	Type         string  `json:"type"`
	Subtype      string  `json:"subtype"`
	Balances     balance `json:"balances"`
}

// balance mirrors a Plaid balances object. Available and Current are nullable;
// a nil Current means Plaid reported no current balance for the account.
type balance struct {
	Available       *float64 `json:"available"`
	Current         *float64 `json:"current"`
	Limit           *float64 `json:"limit"`
	ISOCurrencyCode *string  `json:"iso_currency_code"`
}

// accountsResponse is the shared response shape of /accounts/get and
// /accounts/balance/get.
type accountsResponse struct {
	Accounts []account `json:"accounts"`
}

// personalFinanceCategory mirrors Plaid's two-level transaction category.
type personalFinanceCategory struct {
	Primary  string `json:"primary"`
	Detailed string `json:"detailed"`
}

// transaction mirrors a Plaid transaction object from /transactions/sync.
type transaction struct {
	TransactionID           string                   `json:"transaction_id"`
	AccountID               string                   `json:"account_id"`
	Amount                  float64                  `json:"amount"`
	ISOCurrencyCode         *string                  `json:"iso_currency_code"`
	Date                    string                   `json:"date"`
	AuthorizedDate          string                   `json:"authorized_date"`
	Name                    string                   `json:"name"`
	MerchantName            string                   `json:"merchant_name"`
	Pending                 bool                     `json:"pending"`
	PersonalFinanceCategory *personalFinanceCategory `json:"personal_finance_category"`
}

// removedTransaction mirrors a Plaid removed-transaction entry; only the id is
// guaranteed.
type removedTransaction struct {
	TransactionID string `json:"transaction_id"`
}

// transactionsSyncRequest is the endpoint-specific body of /transactions/sync.
// The auth credentials are merged in by the client.
type transactionsSyncRequest struct {
	Cursor string `json:"cursor,omitempty"`
}

// transactionsSyncResponse mirrors the /transactions/sync response.
type transactionsSyncResponse struct {
	Added      []transaction        `json:"added"`
	Modified   []transaction        `json:"modified"`
	Removed    []removedTransaction `json:"removed"`
	NextCursor string               `json:"next_cursor"`
	HasMore    bool                 `json:"has_more"`
}

// toAccount converts a Plaid account into the app's banking.Account, seeding
// kind and the counts-as-savings default from Plaid's type/subtype.
func (a account) toAccount() banking.Account {
	return banking.Account{
		ID:              a.AccountID,
		Name:            a.displayName(),
		Kind:            accountKind(a.Type),
		Balance:         a.Balances.toBalance(a.AccountID),
		CountsAsSavings: a.Subtype == accountSubtypeSavings,
	}
}

// displayName prefers Plaid's official name, falling back to the short name.
func (a account) displayName() string {
	if a.OfficialName != "" {
		return a.OfficialName
	}
	return a.Name
}

// accountKind maps a Plaid account type onto the cash/credit axis: credit
// accounts are credit, everything else (depository, …) is cash.
func accountKind(plaidType string) banking.AccountKind {
	if plaidType == accountTypeCredit {
		return banking.KindCredit
	}
	return banking.KindCash
}

// toBalance converts a Plaid balances object into a banking.Balance. A nil
// current balance is surfaced as unknown rather than zero.
func (b balance) toBalance(accountID string) banking.Balance {
	if b.Current == nil {
		return banking.Balance{AccountID: accountID, Known: false}
	}
	return banking.Balance{
		AccountID: accountID,
		Known:     true,
		Money: banking.Money{
			Amount:   *b.Current,
			Currency: currency(b.ISOCurrencyCode),
		},
	}
}

// toTransaction converts a Plaid transaction into the app's banking.Transaction.
// Plaid's amount sign (outflow positive, inflow negative) already matches the
// domain convention, so the value carries through unchanged.
func (t transaction) toTransaction() banking.Transaction {
	return banking.Transaction{
		ID:        t.TransactionID,
		AccountID: t.AccountID,
		Date:      parseDate(t.Date),
		Amount: banking.Money{
			Amount:   t.Amount,
			Currency: currency(t.ISOCurrencyCode),
		},
		Merchant:     cleanMerchant(t.MerchantName, t.Name),
		Counterparty: rawCounterparty(t.MerchantName, t.Name),
		Category:     t.category(),
		Pending:      t.Pending,
	}
}

// category carries both levels of Plaid's personal_finance_category through to
// the domain. A missing category yields empty levels.
func (t transaction) category() banking.Category {
	if t.PersonalFinanceCategory == nil {
		return banking.Category{}
	}
	return banking.Category{
		Primary:  t.PersonalFinanceCategory.Primary,
		Detailed: t.PersonalFinanceCategory.Detailed,
	}
}

// rawCounterparty is the unmodified bank-reported payee: Plaid's merchant_name
// when present, otherwise the raw transaction name.
func rawCounterparty(merchantName, name string) string {
	if merchantName != "" {
		return merchantName
	}
	return name
}

// currency defaults an absent ISO currency code to USD (the only supported
// currency in v1).
func currency(code *string) string {
	if code == nil || *code == "" {
		return "USD"
	}
	return *code
}

// parseDate parses a Plaid calendar date; an unparseable or empty value yields
// the zero time.
func parseDate(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
