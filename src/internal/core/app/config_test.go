package app_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/alecdray/two-cents/src/internal/core/app"
)

// setRequiredSecrets populates the env vars LoadConfig insists on, so a test can
// focus on whatever else it wants to assert.
func setRequiredSecrets(t *testing.T) {
	t.Helper()
	t.Setenv("ENCRYPTION_KEY", "deadbeef")
	t.Setenv("PLAID_CLIENT_ID", "client-123")
	// Both, so a test is free to choose either environment.
	t.Setenv("PLAID_SECRET_SANDBOX", "secret-456")
	t.Setenv("PLAID_SECRET_PRODUCTION", "secret-456")
}

// With every Plaid var and the encryption key set, LoadConfig surfaces each on
// the returned config.
func TestConfigSurfacesPlaidCredentialsAndEncryptionKey(t *testing.T) {
	setRequiredSecrets(t)
	t.Setenv("PLAID_ENV", "sandbox")
	t.Setenv("PLAID_COUNTRY_CODES", "US, CA ,GB")
	t.Setenv("PLAID_PRODUCTS", "transactions,auth")

	cfg := app.LoadConfig()

	if cfg.EncryptionKey != "deadbeef" {
		t.Errorf("EncryptionKey = %q, want %q", cfg.EncryptionKey, "deadbeef")
	}
	if cfg.Plaid.ClientID != "client-123" {
		t.Errorf("Plaid.ClientID = %q, want %q", cfg.Plaid.ClientID, "client-123")
	}
	if cfg.Plaid.Secret != "secret-456" {
		t.Errorf("Plaid.Secret = %q, want %q", cfg.Plaid.Secret, "secret-456")
	}
	if cfg.Plaid.Env != "sandbox" {
		t.Errorf("Plaid.Env = %q, want %q", cfg.Plaid.Env, "sandbox")
	}
	if wantCodes := []string{"US", "CA", "GB"}; !reflect.DeepEqual(cfg.Plaid.CountryCodes, wantCodes) {
		t.Errorf("Plaid.CountryCodes = %v, want %v", cfg.Plaid.CountryCodes, wantCodes)
	}
	if wantProducts := []string{"transactions", "auth"}; !reflect.DeepEqual(cfg.Plaid.Products, wantProducts) {
		t.Errorf("Plaid.Products = %v, want %v", cfg.Plaid.Products, wantProducts)
	}
}

// Plaid env/codes/products fall back to documented defaults when unset.
func TestConfigAppliesPlaidDefaults(t *testing.T) {
	setRequiredSecrets(t)
	// Explicitly cleared: the taskfile loads .env for every test run, so without
	// this the assertion reads whatever the operator's own PLAID_ENV says — and
	// the operator this default exists for is exactly the one whose .env names
	// production. GetEnvWithDefault treats empty as unset.
	t.Setenv("PLAID_ENV", "")

	cfg := app.LoadConfig()

	// Sandbox, not production: an unset PLAID_ENV must never put the app on the
	// operator's real bank logins. Reaching production is an explicit act.
	if cfg.Plaid.Env != "sandbox" {
		t.Errorf("Plaid.Env default = %q, want %q", cfg.Plaid.Env, "sandbox")
	}
	if wantCodes := []string{"US"}; !reflect.DeepEqual(cfg.Plaid.CountryCodes, wantCodes) {
		t.Errorf("Plaid.CountryCodes default = %v, want %v", cfg.Plaid.CountryCodes, wantCodes)
	}
	if wantProducts := []string{"transactions"}; !reflect.DeepEqual(cfg.Plaid.Products, wantProducts) {
		t.Errorf("Plaid.Products default = %v, want %v", cfg.Plaid.Products, wantProducts)
	}
}

// The bank provider defaults to Plaid when BANK_PROVIDER is unset, and reflects
// the env var when set.
func TestBankProviderSelection(t *testing.T) {
	t.Run("defaults to plaid", func(t *testing.T) {
		setRequiredSecrets(t)

		cfg := app.LoadConfig()

		if cfg.BankProvider != "plaid" {
			t.Errorf("BankProvider default = %q, want %q", cfg.BankProvider, "plaid")
		}
	})

	t.Run("honours BANK_PROVIDER", func(t *testing.T) {
		setRequiredSecrets(t)
		t.Setenv("BANK_PROVIDER", "fake")

		cfg := app.LoadConfig()

		if cfg.BankProvider != "fake" {
			t.Errorf("BankProvider = %q, want %q", cfg.BankProvider, "fake")
		}
	})
}

// A missing encryption key is reported, not left silently blank.
func TestMissingEncryptionKeyIsReported(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "")
	t.Setenv("PLAID_CLIENT_ID", "client-123")
	t.Setenv("PLAID_SECRET_SANDBOX", "secret-456")

	assertPanics(t, "ENCRYPTION_KEY", app.LoadConfig)
}

// A missing Plaid client id is reported, not left silently blank.
func TestMissingPlaidClientIDIsReported(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "deadbeef")
	t.Setenv("PLAID_CLIENT_ID", "")
	t.Setenv("PLAID_SECRET_SANDBOX", "secret-456")

	assertPanics(t, "PLAID_CLIENT_ID", app.LoadConfig)
}

// A missing Plaid secret is reported naming the variable actually missing — the
// one for the active environment, not a generic one the template never mentions.
func TestMissingPlaidSecretIsReported(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "deadbeef")
	t.Setenv("PLAID_CLIENT_ID", "client-123")
	t.Setenv("PLAID_ENV", "sandbox")
	t.Setenv("PLAID_SECRET_SANDBOX", "")

	assertPanics(t, "PLAID_SECRET_SANDBOX", app.LoadConfig)
}

func assertPanics(t *testing.T, wantSubstr string, fn func() *app.Config) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected a panic mentioning %q, got none", wantSubstr)
		}
		// Which variable the panic names is the whole point: a config that dies
		// for the wrong reason is as misleading as one that does not die at all.
		if got := fmt.Sprint(r); !strings.Contains(got, wantSubstr) {
			t.Fatalf("panic = %q, want it to mention %q", got, wantSubstr)
		}
	}()
	fn()
}

func TestConfigPlaidEnv(t *testing.T) {
	t.Run("an explicit production is honoured", func(t *testing.T) {
		setRequiredSecrets(t)
		t.Setenv("PLAID_ENV", "production")

		if got := app.LoadConfig().Plaid.Env; got != "production" {
			t.Errorf("Plaid.Env = %q, want production", got)
		}
	})

	t.Run("an unrecognised value is refused rather than silently resolved", func(t *testing.T) {
		// A typo must not fall back to *any* environment. Falling back to
		// production would reach real bank logins; falling back to sandbox would
		// leave a production deployment quietly talking to a sandbox that has none
		// of its data. Both are worse than refusing to start.
		setRequiredSecrets(t)
		t.Setenv("PLAID_ENV", "produciton")

		assertPanics(t, "PLAID_ENV", app.LoadConfig)
	})

	t.Run("development is not a Plaid environment we support", func(t *testing.T) {
		// Retired upstream; keeping an arm for it would be dead surface that still
		// resolves to a host, which is the one thing an unknown value must not do.
		setRequiredSecrets(t)
		t.Setenv("PLAID_ENV", "development")

		assertPanics(t, "PLAID_ENV", app.LoadConfig)
	})

	t.Run("a deployed instance must name its environment", func(t *testing.T) {
		// Outside local development there is no safe default: falling back to
		// sandbox would leave a live instance talking to an environment holding
		// none of its data.
		setRequiredSecrets(t)
		t.Setenv("ENV", "production")
		t.Setenv("HOST", "https://example.test")
		t.Setenv("JWT_SECRET", "jwt")
		t.Setenv("PLAID_ENV", "")

		assertPanics(t, "PLAID_ENV", app.LoadConfig)
	})
}

// The origin is resolved from the same table that validates the environment, so
// a known environment always has one and the two cannot drift apart.
func TestConfigPlaidOrigin(t *testing.T) {
	tests := []struct {
		env  string
		want string
	}{
		{env: "production", want: "https://production.plaid.com"},
		{env: "sandbox", want: "https://sandbox.plaid.com"},
	}
	for _, tc := range tests {
		t.Run(tc.env+" resolves to its own host", func(t *testing.T) {
			setRequiredSecrets(t)
			t.Setenv("PLAID_ENV", tc.env)

			if got := app.LoadConfig().Plaid.Origin; got != tc.want {
				t.Errorf("Plaid.Origin = %q, want %q", got, tc.want)
			}
		})
	}
}
