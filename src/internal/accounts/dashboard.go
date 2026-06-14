package accounts

import (
	"fmt"

	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
)

// Dashboard is the read model behind the accounts overview page: the derived
// cash/credit Overview alongside the active accounts grouped into the spending
// buckets the page renders separately. The cash and credit groups feed the net
// cash position; the other group (loans, investments, …) is shown but excluded
// from that position. Each row carries its owning connection's needs-reconnect
// state so the page can flag accounts whose data may be stale.
type Dashboard struct {
	Overview Overview
	Cash     []AccountRow
	Credit   []AccountRow
	Other    []AccountRow
}

// HasAccounts reports whether any group holds an account, so the page can choose
// between the populated view and the empty state.
func (d Dashboard) HasAccounts() bool {
	return len(d.Cash)+len(d.Credit)+len(d.Other) > 0
}

// AccountRow is one account as the overview page displays it: its name, the
// bank's subtype label, its spending bucket, its latest balance (with the Known
// flag so an unreported balance renders as unknown rather than zero), and
// whether its owning connection needs the user to re-authenticate.
type AccountRow struct {
	Name           string
	BankType       string
	Kind           banking.AccountKind
	Balance        banking.Balance
	NeedsReconnect bool
}

// Dashboard assembles the overview page's read model. It reuses computeOverview
// for the totals so the displayed position can never diverge from Overview, then
// buckets the active accounts and joins each to its connection's state to mark
// the ones that need reconnecting.
func (s *Service) Dashboard(ctx contextx.ContextX) (Dashboard, error) {
	accounts, err := s.repo().ListAccounts(ctx)
	if err != nil {
		return Dashboard{}, fmt.Errorf("failed to list accounts: %w", err)
	}
	connections, err := s.repo().ListConnections(ctx)
	if err != nil {
		return Dashboard{}, fmt.Errorf("failed to list connections: %w", err)
	}

	needsReconnect := make(map[string]bool, len(connections))
	for _, c := range connections {
		needsReconnect[c.ID] = c.State == ConnectionNeedsReconnect
	}

	dashboard := Dashboard{Overview: computeOverview(accounts)}
	for _, a := range accounts {
		if a.State != AccountActive {
			continue
		}
		row := AccountRow{
			Name:           a.Name,
			BankType:       a.BankType,
			Kind:           a.Kind,
			Balance:        a.Balance,
			NeedsReconnect: needsReconnect[a.ConnectionID],
		}
		switch a.Kind {
		case banking.KindCash:
			dashboard.Cash = append(dashboard.Cash, row)
		case banking.KindCredit:
			dashboard.Credit = append(dashboard.Credit, row)
		default:
			dashboard.Other = append(dashboard.Other, row)
		}
	}
	return dashboard, nil
}
