// Package accounts owns bank Connections and the Accounts they expose: it
// registers a new connection from a provider enrollment, refreshes balances and
// discovers new accounts on sync, tracks each connection's re-auth health, and
// derives the cash/credit overview. It reaches the bank only through the
// banking.BankProvider seam, so it is provider-agnostic and never imports a
// concrete provider client.
package accounts

import (
	"time"

	"github.com/alecdray/two-cents/src/internal/banking"
)

// ConnectionState is the lifecycle of a bank Connection.
type ConnectionState string

const (
	// ConnectionActive is a healthy connection whose accounts sync normally.
	ConnectionActive ConnectionState = "active"
	// ConnectionNeedsReconnect marks a connection whose provider reported the
	// login expired; its accounts and history are retained but sync is paused
	// until the user re-authenticates.
	ConnectionNeedsReconnect ConnectionState = "needs_reconnect"
	// ConnectionDisconnected is a terminal connection the user has removed.
	ConnectionDisconnected ConnectionState = "disconnected"
)

// AccountState is the display lifecycle of an Account.
type AccountState string

const (
	// AccountActive is a normal account included in the overview.
	AccountActive AccountState = "active"
	// AccountHidden is a user-hidden account, dropped from the overview but
	// otherwise intact.
	AccountHidden AccountState = "hidden"
	// AccountClosed is a terminal account whose connection was disconnected.
	AccountClosed AccountState = "closed"
)

// Connection is a linked bank login: one provider enrollment (Item) and the
// state of its sync health. The access token is held encrypted at rest and is
// never exposed on this entity.
type Connection struct {
	ID             string
	ProviderItemID string
	State          ConnectionState
}

// Account is one financial account under a Connection, with the seeded
// spending bucket, the counts-as-savings flag, the latest balance, and the
// override flags that protect a user's choices from being reseeded on sync.
type Account struct {
	ID                string
	ConnectionID      string
	ProviderAccountID string
	Name              string
	BankType          string
	Kind              banking.AccountKind
	KindOverridden    bool
	CountsAsSavings   bool
	SavingsOverridden bool
	Balance           banking.Balance
	State             AccountState
	LastSyncedAt      *time.Time
}

// AccountFacet is the small per-account read the transfer-subtype pairing pass
// consumes: an account's internal id, display name, spending bucket, and
// counts-as-savings flag. It carries only what pairing needs to learn a
// transfer's destination and derive its subtype, never the full Account.
type AccountFacet struct {
	ID              string
	Name            string
	Kind            banking.AccountKind
	CountsAsSavings bool
}

// Overview is the cash/credit position derived from the active, non-hidden,
// non-closed accounts: total spendable cash (savings included), total credit
// debt, and the net cash position (cash − debt). Accounts whose balance the
// provider has not reported are excluded entirely, never counted as zero, as
// are accounts in the other bucket (loans, investments, …).
type Overview struct {
	TotalCash float64
	TotalDebt float64
	NetCash   float64
	Currency  string
}
