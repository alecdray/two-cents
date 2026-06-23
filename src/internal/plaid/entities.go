package plaid

import (
	"time"

	"github.com/alecdray/two-cents/src/internal/banking"
)

// Plaid account types and subtypes we map from. Depository seeds cash and
// credit seeds credit; every other Plaid type (investment, loan, brokerage, …)
// falls into the catch-all other bucket.
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
	Mask         string  `json:"mask"`
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

// personalFinanceCategory mirrors Plaid's two-level transaction category and
// the provider's confidence in it.
type personalFinanceCategory struct {
	Primary         string `json:"primary"`
	Detailed        string `json:"detailed"`
	ConfidenceLevel string `json:"confidence_level"`
}

// transaction mirrors a Plaid transaction object from /transactions/sync.
type transaction struct {
	TransactionID           string                   `json:"transaction_id"`
	AccountID               string                   `json:"account_id"`
	Amount                  float64                  `json:"amount"`
	ISOCurrencyCode         *string                  `json:"iso_currency_code"`
	Date                    string                   `json:"date"`
	AuthorizedDate          string                   `json:"authorized_date"`
	Datetime                string                   `json:"datetime"`
	AuthorizedDatetime      string                   `json:"authorized_datetime"`
	Name                    string                   `json:"name"`
	MerchantName            string                   `json:"merchant_name"`
	MerchantEntityID        string                   `json:"merchant_entity_id"`
	LogoURL                 string                   `json:"logo_url"`
	Website                 string                   `json:"website"`
	PaymentChannel          string                   `json:"payment_channel"`
	Pending                 bool                     `json:"pending"`
	PersonalFinanceCategory *personalFinanceCategory `json:"personal_finance_category"`
	Counterparties          []counterparty           `json:"counterparties"`
}

// counterparty mirrors a Plaid counterparties[] entry: the typed parties on a
// transaction — the merchant plus any intermediaries (marketplace, payment_app,
// payment_terminal, financial_institution). Read-only display detail.
type counterparty struct {
	Name            string `json:"name"`
	Type            string `json:"type"`
	LogoURL         string `json:"logo_url"`
	Website         string `json:"website"`
	EntityID        string `json:"entity_id"`
	ConfidenceLevel string `json:"confidence_level"`
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

// linkUser identifies the end user a link token is minted for. A single-user
// app has no per-user auth, so the id is a fixed app-level value.
type linkUser struct {
	ClientUserID string `json:"client_user_id"`
}

// linkTokenCreateRequest is the endpoint-specific body of /link/token/create.
// The auth credentials are merged in by the client. Products is sent only for a
// new connection; update mode (reconnecting an existing login) carries the
// login's access_token instead and must omit products, so the field is
// omitempty and left unset in that case.
type linkTokenCreateRequest struct {
	ClientName   string   `json:"client_name"`
	Language     string   `json:"language"`
	CountryCodes []string `json:"country_codes"`
	Products     []string `json:"products,omitempty"`
	User         linkUser `json:"user"`
}

// linkTokenCreateResponse mirrors the /link/token/create response; only the
// token the front end needs is decoded.
type linkTokenCreateResponse struct {
	LinkToken string `json:"link_token"`
}

// toLinkToken converts a Plaid link-token response into the app's
// banking.LinkToken, tagging it as produced by the real provider.
func (r linkTokenCreateResponse) toLinkToken() banking.LinkToken {
	return banking.LinkToken{Token: r.LinkToken, Mode: linkModeReal}
}

// publicTokenExchangeRequest is the endpoint-specific body of
// /item/public_token/exchange. The auth credentials are merged in by the
// client; this call carries no access token.
type publicTokenExchangeRequest struct {
	PublicToken string `json:"public_token"`
}

// publicTokenExchangeResponse mirrors the /item/public_token/exchange response:
// the durable access token and the provider's item identifier.
type publicTokenExchangeResponse struct {
	AccessToken string `json:"access_token"`
	ItemID      string `json:"item_id"`
}

// toItem converts a Plaid exchange response into the app's banking.Item.
func (r publicTokenExchangeResponse) toItem() banking.Item {
	return banking.Item{AccessToken: r.AccessToken, ProviderItemID: r.ItemID}
}

// itemRemoveResponse mirrors the /item/remove response. The call's only outcome
// the app cares about is success vs. error, so no fields are read.
type itemRemoveResponse struct{}

// toAccount converts a Plaid account into the app's banking.Account, seeding
// kind and the counts-as-savings default from Plaid's type/subtype and carrying
// the bank's type/subtype/mask through as provider-agnostic label strings.
func (a account) toAccount() banking.Account {
	return banking.Account{
		ID:              a.AccountID,
		Name:            a.displayName(),
		Kind:            accountKind(a.Type),
		Type:            a.Type,
		Subtype:         a.Subtype,
		Mask:            a.Mask,
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

// accountKind maps a Plaid account type onto the spending bucket: credit
// accounts are credit, depository accounts are cash, and everything else
// (loan, investment, brokerage, …) falls into other. This is the single source
// of the bucketing rule.
func accountKind(plaidType string) banking.AccountKind {
	switch plaidType {
	case accountTypeCredit:
		return banking.KindCredit
	case accountTypeDepository:
		return banking.KindCash
	default:
		return banking.KindOther
	}
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
		Merchant:           cleanMerchant(t.MerchantName, t.Name),
		Counterparty:       rawCounterparty(t.MerchantName, t.Name),
		Description:        t.Name,
		MerchantEntityID:   t.MerchantEntityID,
		LogoURL:            t.LogoURL,
		Website:            t.Website,
		PaymentChannel:     t.PaymentChannel,
		CategoryConfidence: t.categoryConfidence(),
		AuthorizedDate:     parseDatePtr(t.AuthorizedDate),
		Datetime:           parseDatetimePtr(t.Datetime),
		AuthorizedDatetime: parseDatetimePtr(t.AuthorizedDatetime),
		Category:           t.category(),
		Counterparties:     t.counterparties(),
		Pending:            t.Pending,
	}
}

// counterparties converts Plaid's counterparties[] into the domain's typed
// list, preserving order; nil when the bank reported none.
func (t transaction) counterparties() []banking.Counterparty {
	if len(t.Counterparties) == 0 {
		return nil
	}
	out := make([]banking.Counterparty, 0, len(t.Counterparties))
	for _, c := range t.Counterparties {
		out = append(out, banking.Counterparty{
			Name:       c.Name,
			Type:       c.Type,
			LogoURL:    c.LogoURL,
			Website:    c.Website,
			EntityID:   c.EntityID,
			Confidence: c.ConfidenceLevel,
		})
	}
	return out
}

// categoryConfidence carries Plaid's confidence in its category through to the
// domain; a missing category yields an empty confidence.
func (t transaction) categoryConfidence() string {
	if t.PersonalFinanceCategory == nil {
		return ""
	}
	return t.PersonalFinanceCategory.ConfidenceLevel
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

// parseDatePtr parses a Plaid calendar date into a pointer; an empty or
// unparseable value yields nil (the bank did not report it).
func parseDatePtr(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return nil
	}
	return &t
}

// parseDatetimePtr parses a Plaid RFC3339 timestamp into a pointer; an empty or
// unparseable value yields nil (timestamps are frequently absent).
func parseDatetimePtr(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}
	return &t
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
