package app_test

import (
	"reflect"
	"testing"

	"github.com/alecdray/two-cents/src/internal/core/app"
)

// setRequiredSecrets populates the env vars LoadConfig insists on, so a test can
// focus on whatever else it wants to assert.
func setRequiredSecrets(t *testing.T) {
	t.Helper()
	t.Setenv("ENCRYPTION_KEY", "deadbeef")
	t.Setenv("PLAID_CLIENT_ID", "client-123")
	t.Setenv("PLAID_SECRET", "secret-456")
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

	cfg := app.LoadConfig()

	if cfg.Plaid.Env != "production" {
		t.Errorf("Plaid.Env default = %q, want %q", cfg.Plaid.Env, "production")
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
	t.Setenv("PLAID_SECRET", "secret-456")

	assertPanics(t, "ENCRYPTION_KEY", app.LoadConfig)
}

// A missing Plaid client id is reported, not left silently blank.
func TestMissingPlaidClientIDIsReported(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "deadbeef")
	t.Setenv("PLAID_CLIENT_ID", "")
	t.Setenv("PLAID_SECRET", "secret-456")

	assertPanics(t, "PLAID_CLIENT_ID", app.LoadConfig)
}

// A missing Plaid secret is reported, not left silently blank.
func TestMissingPlaidSecretIsReported(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "deadbeef")
	t.Setenv("PLAID_CLIENT_ID", "client-123")
	t.Setenv("PLAID_SECRET", "")

	assertPanics(t, "PLAID_SECRET", app.LoadConfig)
}

func assertPanics(t *testing.T, wantSubstr string, fn func() *app.Config) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected a panic mentioning %q, got none", wantSubstr)
		}
	}()
	fn()
}
