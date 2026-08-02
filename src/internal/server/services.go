package server

import (
	"fmt"
	"log/slog"

	"github.com/alecdray/two-cents/src/internal/accounts"
	"github.com/alecdray/two-cents/src/internal/auth"
	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/budget"
	"github.com/alecdray/two-cents/src/internal/categorization"
	"github.com/alecdray/two-cents/src/internal/core/app"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/db"
	"github.com/alecdray/two-cents/src/internal/core/task"
	"github.com/alecdray/two-cents/src/internal/fakebank"
	"github.com/alecdray/two-cents/src/internal/home"
	"github.com/alecdray/two-cents/src/internal/plaid"
	"github.com/alecdray/two-cents/src/internal/sweep"
	"github.com/alecdray/two-cents/src/internal/transactions"

	accountsAdapters "github.com/alecdray/two-cents/src/internal/accounts/adapters"
)

// bankProviderFake is the BankProvider config value that selects the
// deterministic in-process stand-in instead of the live Plaid client (ADR-0006).
const bankProviderFake = "fake"

type services struct {
	taskManager           *task.TaskManager
	authService           *auth.Service
	accountsService       *accounts.Service
	transactionsService   *transactions.Service
	categorizationService *categorization.Service
	budgetService         *budget.Service
	homeService           *home.Service
	sweepService          *sweep.Service
	// bankMode is the connect-control mode derived from configuration: "fake"
	// when the deterministic stand-in is selected, "real" otherwise.
	bankMode string
}

func NewServices(application app.App, database *db.DB) (*services, error) {
	s := &services{}

	s.taskManager = task.NewTaskManager(database, slog.Default())

	cfg := application.Config()

	// Auth gates the whole app behind the single local login (ADR-0007). It owns
	// only its credential table and reaches no other module.
	s.authService = auth.NewService(database)

	bankProvider, err := selectBankProvider(cfg)
	if err != nil {
		return nil, err
	}

	s.accountsService = accounts.NewService(database, bankProvider, cfg.EncryptionKey)

	// Build accounts → categorization → transactions. Transactions needs the
	// categorization Service at construction (to resolve each synced row); the
	// re-categorize seam needs the transactions Service at runtime. The closure
	// closes over `s` and reaches s.transactionsService late: it is assigned
	// immediately after transactions.NewService returns, and the closure only runs
	// later, on a Rule mutation, by which time the field is set. This keeps the
	// module graph acyclic — categorization imports neither accounts nor transactions.
	reapplyCategorization := func(ctx contextx.ContextX, substrings []string) (int, error) {
		return s.transactionsService.ApplyCategorization(ctx, substrings)
	}
	s.categorizationService = categorization.NewService(database, reapplyCategorization)
	// The concrete logo fetcher lives with the Plaid adapter (its home for the CDN
	// host allowlist) and is passed where the transactions module's provider-agnostic
	// LogoFetcher interface is expected — satisfied structurally, so transactions never
	// imports plaid. It holds no Plaid credentials, so it is wired regardless of the
	// selected bank provider.
	s.transactionsService = transactions.NewService(database, bankProvider, s.accountsService, s.categorizationService, plaid.NewLogoFetcher())

	// Budget builds on categorization (the Category list it validates limits
	// against and drops archived limits by); it imports neither transactions nor
	// accounts, so it slots in after categorization with no new cycle.
	s.budgetService = budget.NewService(database, s.categorizationService)

	// Home is the read-side dashboard composer — the only module that injects
	// multiple domain services. It builds last, after every service it composes,
	// and owns no tables, so it adds no new persistence edge. It imports no
	// provider client (it reaches the bank only through the services).
	s.homeService = home.NewService(
		s.budgetService,
		s.transactionsService,
		s.categorizationService,
		s.accountsService,
		cfg.AppTimezone,
	)

	// Sweep is the read-side recommendation composer. It reads through accounts,
	// transactions, and budget services and owns no tables itself.
	s.sweepService = sweep.NewService(
		s.accountsService,
		s.transactionsService,
		s.budgetService,
		database,
		cfg.AppTimezone,
		cfg.FixedSafetyMargin,
	)

	s.bankMode = bankMode(cfg)

	// The recurring bank sync drives Accounts (balances + health) first, then
	// pulls each connection's transactions.
	s.taskManager.RegisterCronTask(transactions.NewSyncTask(s.transactionsService))

	// The monthly sweep job fires at 00:00 on the 7th of each month in the
	// configured app timezone, computes and persists the current recommendation.
	s.taskManager.RegisterCronTask(sweep.NewMonthlyTask(s.sweepService, cfg.AppTimezone))

	return s, nil
}

// bankMode maps the configured provider onto the connect-control mode the
// accounts page renders against: the deterministic stand-in posts directly
// ("fake"), every other provider opens the live connect UI ("real").
func bankMode(cfg app.Config) string {
	if cfg.BankProvider == bankProviderFake {
		return accountsAdapters.BankModeFake
	}
	return accountsAdapters.BankModeReal
}

// selectBankProvider chooses the BankProvider to inject from the configuration:
// "fake" yields the deterministic in-process stand-in (no credentials, no
// network); anything else (the default "plaid") builds the live Plaid client.
// The fake branch constructs no Plaid client, so the stand-in is usable with no
// Plaid configuration at all (ADR-0006).
func selectBankProvider(cfg app.Config) (banking.BankProvider, error) {
	if cfg.BankProvider == bankProviderFake {
		return fakebank.NewService(), nil
	}
	return newPlaidProvider(cfg)
}

// newPlaidProvider builds the live Plaid client and wraps it in the provider
// service.
func newPlaidProvider(cfg app.Config) (banking.BankProvider, error) {
	plaidClient, err := plaid.NewClient(
		cfg.Plaid.ClientID,
		cfg.Plaid.Secret,
		plaid.WithOrigin(plaidOrigin(cfg.Plaid.Env)),
		plaid.WithLinkConfig(plaid.LinkConfig{
			ClientName:   cfg.AppName,
			CountryCodes: cfg.Plaid.CountryCodes,
			Products:     cfg.Plaid.Products,
			ClientUserID: cfg.AppName,
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build plaid client: %w", err)
	}
	return plaid.NewService(plaidClient), nil
}

// plaidOrigin maps the configured Plaid environment onto its API base URL. An
// unrecognised value falls back to the production host.
func plaidOrigin(env string) string {
	switch env {
	case "sandbox":
		return "https://sandbox.plaid.com"
	case "development":
		return "https://development.plaid.com"
	default:
		return "https://production.plaid.com"
	}
}
