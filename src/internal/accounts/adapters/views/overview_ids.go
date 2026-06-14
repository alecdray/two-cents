package views

// overviewRegionID is the DOM id of the overview's shared swap region — the
// <main> element AccountsOverviewPage renders the content into and the connect
// form targets. The page owns the id here so the form and the page agree on the
// region's name without hard-coding the string at the call site.
const overviewRegionID = "accounts-overview"

// OverviewRegionID returns the overview swap region's DOM id for callers (e.g.
// the connect form's hx-target) that need to name the region.
func OverviewRegionID() string { return overviewRegionID }

// The bank modes the connect control renders against, mirroring
// banking.LinkToken.Mode: "real" opens the live provider's connect UI behind an
// Alpine interceptor; "fake" posts the form directly to the deterministic
// stand-in. The composition root derives the mode from configuration and threads
// it through the handler.
const (
	bankModeReal = "real"
	bankModeFake = "fake"
)

// BankModeReal and BankModeFake expose the connect-control mode strings so the
// composition root can thread the configured mode in without restating the
// literals.
const (
	BankModeReal = bankModeReal
	BankModeFake = bankModeFake
)

// reconnectFailure carries an inline reconnect error scoped to one connection:
// the id of the connection whose reconnect just failed and the message to show
// beside its row(s). The zero value (empty message) means no failure, so the
// overview threads it everywhere and only the matching row renders the error.
type reconnectFailure struct {
	connectionID string
	message      string
}

// matches reports whether the failure belongs to the given connection — true
// only when there is a message and the connection ids agree.
func (f reconnectFailure) matches(connectionID string) bool {
	return f.message != "" && f.connectionID == connectionID
}
