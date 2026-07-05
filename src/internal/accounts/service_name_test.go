package accounts

import (
	"strings"
	"testing"
)

func TestSetAccountName(t *testing.T) {
	t.Run("sets a custom name that overrides the bank name", func(t *testing.T) {
		svc, ctx, accts := setupOverrideAccounts(t)
		check := accts["p-check"] // bank name "Checking"

		if err := svc.SetAccountName(ctx, check.ID, "Joint Checking"); err != nil {
			t.Fatalf("SetAccountName: %v", err)
		}
		got := getStoredAccount(t, svc, ctx, check.ID)
		if got.CustomName == nil || *got.CustomName != "Joint Checking" {
			t.Errorf("CustomName = %v, want %q", got.CustomName, "Joint Checking")
		}
		if got.DisplayName() != "Joint Checking" {
			t.Errorf("DisplayName() = %q, want %q", got.DisplayName(), "Joint Checking")
		}
		if got.Name != "Checking" {
			t.Errorf("bank Name = %q, want it preserved as %q", got.Name, "Checking")
		}
	})

	t.Run("empty input clears the custom name back to the bank name", func(t *testing.T) {
		svc, ctx, accts := setupOverrideAccounts(t)
		check := accts["p-check"]

		if err := svc.SetAccountName(ctx, check.ID, "Temp"); err != nil {
			t.Fatalf("SetAccountName set: %v", err)
		}
		if err := svc.SetAccountName(ctx, check.ID, "   "); err != nil {
			t.Fatalf("SetAccountName clear: %v", err)
		}
		got := getStoredAccount(t, svc, ctx, check.ID)
		if got.CustomName != nil {
			t.Errorf("CustomName = %q, want nil after clearing", *got.CustomName)
		}
		if got.DisplayName() != "Checking" {
			t.Errorf("DisplayName() = %q, want the bank name %q", got.DisplayName(), "Checking")
		}
	})

	t.Run("trims surrounding whitespace", func(t *testing.T) {
		svc, ctx, accts := setupOverrideAccounts(t)
		check := accts["p-check"]

		if err := svc.SetAccountName(ctx, check.ID, "  Vacation Fund  "); err != nil {
			t.Fatalf("SetAccountName: %v", err)
		}
		got := getStoredAccount(t, svc, ctx, check.ID)
		if got.CustomName == nil || *got.CustomName != "Vacation Fund" {
			t.Errorf("CustomName = %v, want trimmed %q", got.CustomName, "Vacation Fund")
		}
	})

	t.Run("caps the name at 60 characters", func(t *testing.T) {
		svc, ctx, accts := setupOverrideAccounts(t)
		check := accts["p-check"]

		long := strings.Repeat("a", 80)
		if err := svc.SetAccountName(ctx, check.ID, long); err != nil {
			t.Fatalf("SetAccountName: %v", err)
		}
		got := getStoredAccount(t, svc, ctx, check.ID)
		if got.CustomName == nil {
			t.Fatalf("CustomName = nil, want a capped value")
		}
		if n := len([]rune(*got.CustomName)); n != maxCustomNameLen {
			t.Errorf("custom name length = %d, want %d", n, maxCustomNameLen)
		}
	})

	t.Run("a rename survives a sync that refreshes the bank name", func(t *testing.T) {
		svc, ctx, accts := setupOverrideAccounts(t)
		check := accts["p-check"]

		if err := svc.SetAccountName(ctx, check.ID, "My Checking"); err != nil {
			t.Fatalf("SetAccountName: %v", err)
		}
		if err := svc.SyncAccounts(ctx); err != nil {
			t.Fatalf("SyncAccounts: %v", err)
		}
		got := getStoredAccount(t, svc, ctx, check.ID)
		if got.CustomName == nil || *got.CustomName != "My Checking" {
			t.Errorf("CustomName = %v, want the rename preserved across sync", got.CustomName)
		}
	})
}
