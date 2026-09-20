package accounts

import "github.com/alecdray/two-cents/src/internal/core/contextx"

// DeriveChecking identifies the checking account among a set of active cash
// Accounts: the single one not marked counts-as-savings. It reports false when
// there is no such account, or more than one.
//
// The rule lives here because counts-as-savings is this module's flag
// ([ADR-0008]). Its consumers - the sweep's derivation, and the adapter that
// scopes occurrence matching to checking - ask rather than re-encoding it, so
// there is one definition rather than two that can drift ([ADR-0027]).
//
// It answers *which account* and nothing more. Whether that account's balance is
// known, or fresh, is deliberately not decided here: those are separate failures
// with separate fixes, and the sweep names them apart for its user.
func DeriveChecking(cashAccounts []Account) (Account, bool) {
	var found Account
	var count int
	for _, a := range cashAccounts {
		if a.CountsAsSavings {
			continue
		}
		found = a
		count++
	}
	if count != 1 {
		return Account{}, false
	}
	return found, true
}

// CheckingAccountID resolves the checking account's id for a caller that holds
// this service rather than the account list. Reports false when checking cannot
// be determined, which is a state the caller must handle rather than an error.
func (s *Service) CheckingAccountID(ctx contextx.ContextX) (string, bool, error) {
	cashAccounts, err := s.ActiveCashAccounts(ctx)
	if err != nil {
		return "", false, err
	}
	checking, ok := DeriveChecking(cashAccounts)
	if !ok {
		return "", false, nil
	}
	return checking.ID, true, nil
}
