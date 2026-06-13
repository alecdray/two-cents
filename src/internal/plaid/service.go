package plaid

import (
	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
)

// maxSyncPages bounds the /transactions/sync pagination loop, a safety valve
// against a provider that never reports has_more=false.
const maxSyncPages = 1000

// Service is the internal-facing surface domain modules call. It wraps the raw
// Client and translates Plaid wire shapes into the app's banking types. It
// satisfies banking.BankProvider.
type Service struct {
	client *Client
}

// NewService builds a Service over the given Client.
func NewService(client *Client) *Service {
	return &Service{client: client}
}

// compile-time check that Service satisfies the provider seam.
var _ banking.BankProvider = (*Service)(nil)

// ListAccounts returns one domain account per Plaid account on the bank login,
// with id, name, kind, current balance, and the counts-as-savings default.
func (s *Service) ListAccounts(ctx contextx.ContextX, accessToken string) ([]banking.Account, error) {
	resp, err := s.client.getAccounts(ctx, accessToken)
	if err != nil {
		return nil, err
	}
	accounts := make([]banking.Account, 0, len(resp.Accounts))
	for _, a := range resp.Accounts {
		accounts = append(accounts, a.toAccount())
	}
	return accounts, nil
}

// GetBalances returns the current balance per account. An account whose
// balance Plaid does not report is surfaced as unknown, not zero.
func (s *Service) GetBalances(ctx contextx.ContextX, accessToken string) ([]banking.Balance, error) {
	resp, err := s.client.getBalances(ctx, accessToken)
	if err != nil {
		return nil, err
	}
	balances := make([]banking.Balance, 0, len(resp.Accounts))
	for _, a := range resp.Accounts {
		balances = append(balances, a.Balances.toBalance(a.AccountID))
	}
	return balances, nil
}

// SyncTransactions pulls the changes since cursor (empty = from the beginning),
// following Plaid's has_more pagination to completion and accumulating every
// page. It returns the added and modified transactions, the exact set of
// removed ids, and the final cursor to resume from next time.
func (s *Service) SyncTransactions(ctx contextx.ContextX, accessToken, cursor string) (banking.TransactionChanges, error) {
	changes := banking.TransactionChanges{Cursor: cursor}

	for page := 0; page < maxSyncPages; page++ {
		resp, err := s.client.syncTransactions(ctx, accessToken, changes.Cursor)
		if err != nil {
			return banking.TransactionChanges{}, err
		}

		for _, t := range resp.Added {
			changes.Added = append(changes.Added, t.toTransaction())
		}
		for _, t := range resp.Modified {
			changes.Modified = append(changes.Modified, t.toTransaction())
		}
		for _, r := range resp.Removed {
			changes.RemovedIDs = append(changes.RemovedIDs, r.TransactionID)
		}
		changes.Cursor = resp.NextCursor

		if !resp.HasMore {
			break
		}
	}

	return changes, nil
}
