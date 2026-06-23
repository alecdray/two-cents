package transactions

import (
	"testing"
	"time"

	"github.com/alecdray/two-cents/src/internal/accounts"
	"github.com/alecdray/two-cents/src/internal/banking"
)

// TestDisplayDetailRoundTrips proves the read-only bank display detail (ADR-0013)
// the sync upsert writes — the raw descriptor, merchant-identity fields, payment
// channel, category confidence, the nullable authorized/posted timestamps, and the
// counterparties list — survives a store-and-read-back through GetRecentTransaction
// exactly, and that a later upsert of the same id refreshes it (bank-sourced, so it
// is overwritten on every sync, unlike the override facets).
func TestDisplayDetailRoundTrips(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	// Seed one account so the upsert's FK and the read's account JOIN are satisfied.
	const token = "tok-a"
	provider := newStub()
	provider.accountsByToken[token] = []banking.Account{cashAccount("p-check", "Checking")}
	accountsSvc := accounts.NewService(database, provider, testKey)
	registerConnection(t, accountsSvc, token, "item-a")
	accountID := providerToInternal(t, accountsSvc)["p-check"]

	repo := NewRepo(database.Queries())

	authDate := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	authDatetime := time.Date(2026, time.June, 1, 14, 30, 0, 0, time.UTC)
	txn := Transaction{
		ID:                 "t-detail",
		AccountID:          accountID,
		Date:               txnDate(2),
		Amount:             banking.Money{Amount: 42.50, Currency: "USD"},
		Merchant:           "Two Boots",
		Counterparty:       "DD *DOORDASH TWOBOOTSP",
		Category:           banking.Category{Primary: "FOOD_AND_DRINK", Detailed: "FOOD_AND_DRINK_RESTAURANT"},
		Status:             StatusPosted,
		Description:        "DD *DOORDASH TWOBOOTSP",
		MerchantEntityID:   "ent-twoboots",
		LogoURL:            "https://logo.example/twoboots.png",
		Website:            "twoboots.example",
		PaymentChannel:     "online",
		CategoryConfidence: "VERY_HIGH",
		AuthorizedDate:     &authDate,
		// Datetime is left nil to prove a nil timestamp round-trips as nil.
		AuthorizedDatetime: &authDatetime,
		Counterparties: []banking.Counterparty{
			{Name: "DoorDash", Type: "marketplace", LogoURL: "https://logo.example/dd.png", Website: "doordash.com", EntityID: "ent-dd", Confidence: "VERY_HIGH"},
			{Name: "Two Boots", Type: "merchant", LogoURL: "https://logo.example/tb.png", Website: "twoboots.example", EntityID: "ent-tb", Confidence: "HIGH"},
		},
	}

	if err := repo.UpsertTransaction(ctx, txn); err != nil {
		t.Fatalf("UpsertTransaction: %v", err)
	}

	got, err := repo.GetRecentTransaction(ctx, "t-detail")
	if err != nil {
		t.Fatalf("GetRecentTransaction: %v", err)
	}

	t.Run("scalar display fields round-trip", func(t *testing.T) {
		if got.Description != txn.Description {
			t.Errorf("Description = %q, want %q", got.Description, txn.Description)
		}
		if got.MerchantEntityID != txn.MerchantEntityID {
			t.Errorf("MerchantEntityID = %q, want %q", got.MerchantEntityID, txn.MerchantEntityID)
		}
		if got.LogoURL != txn.LogoURL {
			t.Errorf("LogoURL = %q, want %q", got.LogoURL, txn.LogoURL)
		}
		if got.Website != txn.Website {
			t.Errorf("Website = %q, want %q", got.Website, txn.Website)
		}
		if got.PaymentChannel != txn.PaymentChannel {
			t.Errorf("PaymentChannel = %q, want %q", got.PaymentChannel, txn.PaymentChannel)
		}
		if got.CategoryConfidence != txn.CategoryConfidence {
			t.Errorf("CategoryConfidence = %q, want %q", got.CategoryConfidence, txn.CategoryConfidence)
		}
	})

	t.Run("nullable timestamps round-trip, nil stays nil", func(t *testing.T) {
		if got.AuthorizedDate == nil || !got.AuthorizedDate.Equal(authDate) {
			t.Errorf("AuthorizedDate = %v, want %v", got.AuthorizedDate, authDate)
		}
		if got.Datetime != nil {
			t.Errorf("Datetime = %v, want nil (it was never set)", got.Datetime)
		}
		if got.AuthorizedDatetime == nil || !got.AuthorizedDatetime.Equal(authDatetime) {
			t.Errorf("AuthorizedDatetime = %v, want %v", got.AuthorizedDatetime, authDatetime)
		}
	})

	t.Run("counterparties slice round-trips in order with every field", func(t *testing.T) {
		if len(got.Counterparties) != len(txn.Counterparties) {
			t.Fatalf("got %d counterparties, want %d", len(got.Counterparties), len(txn.Counterparties))
		}
		for i, want := range txn.Counterparties {
			if got.Counterparties[i] != want {
				t.Errorf("counterparty %d = %+v, want %+v", i, got.Counterparties[i], want)
			}
		}
	})

	t.Run("re-upsert with changed detail overwrites (bank-sourced refresh)", func(t *testing.T) {
		newDatetime := time.Date(2026, time.June, 3, 9, 0, 0, 0, time.UTC)
		changed := txn
		changed.Description = "SQ *TWO BOOTS PIZZA"
		changed.LogoURL = "https://logo.example/twoboots-v2.png"
		changed.PaymentChannel = "in store"
		changed.AuthorizedDate = nil // a present timestamp clears to nil on refresh
		changed.Datetime = &newDatetime
		changed.Counterparties = []banking.Counterparty{
			{Name: "Square", Type: "payment_app", EntityID: "ent-sq", Confidence: "HIGH"},
		}

		if err := repo.UpsertTransaction(ctx, changed); err != nil {
			t.Fatalf("re-upsert: %v", err)
		}
		got2, err := repo.GetRecentTransaction(ctx, "t-detail")
		if err != nil {
			t.Fatalf("GetRecentTransaction after re-upsert: %v", err)
		}
		if got2.Description != "SQ *TWO BOOTS PIZZA" {
			t.Errorf("Description = %q, want the refreshed value", got2.Description)
		}
		if got2.LogoURL != "https://logo.example/twoboots-v2.png" {
			t.Errorf("LogoURL = %q, want the refreshed value", got2.LogoURL)
		}
		if got2.PaymentChannel != "in store" {
			t.Errorf("PaymentChannel = %q, want the refreshed value", got2.PaymentChannel)
		}
		if got2.AuthorizedDate != nil {
			t.Errorf("AuthorizedDate = %v, want nil after refresh cleared it", got2.AuthorizedDate)
		}
		if got2.Datetime == nil || !got2.Datetime.Equal(newDatetime) {
			t.Errorf("Datetime = %v, want %v after refresh", got2.Datetime, newDatetime)
		}
		if len(got2.Counterparties) != 1 || got2.Counterparties[0].Name != "Square" {
			t.Errorf("Counterparties = %+v, want the single refreshed Square entry", got2.Counterparties)
		}
	})
}
