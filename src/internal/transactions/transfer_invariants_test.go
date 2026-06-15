package transactions

import (
	"testing"

	"github.com/alecdray/two-cents/src/internal/accounts"
	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/categorization"
	"github.com/alecdray/two-cents/src/internal/core/db"
)

// These tests assert the emergent, whole-product properties of the assembled
// transfer-subtype-pairing feature — invariants that span the pure pairing
// engine in categorization and the transaction writer/reconciler in this module,
// and so cannot be proven by either alone. Each drives the real, assembled
// services (a real accounts.Service over a migrated temp SQLite database, the
// stub provider as the only swapped part) through SyncTransactions / ReCategorize
// / MarkTransferDestination — the same paths the running app composes.

// savingsPairWithDB stands up a connection holding a checking account and a
// counts-as-savings account and syncs a single $500 outflow→inflow transfer pair,
// so the outflow leg t-out auto-resolves to a savings contribution into the
// savings account and the inflow mirror t-in is left unlabeled. It returns the
// database (for direct facet reads), the wired transactions service, and the
// provider→internal account-id map. Unlike syncedSavingsPair it hands back the
// database so a test can sum the stored ledger directly.
func savingsPairWithDB(t *testing.T) (*db.DB, *Service, map[string]string) {
	t.Helper()
	database := newTestDB(t)

	const token = "tok-a"
	provider := newStub()
	provider.accountsByToken[token] = []banking.Account{
		cashAccount("p-check", "Everyday Checking"),
		savingsAccount("p-save", "High-Yield Savings"),
	}
	provider.syncByToken[token] = func(cursor string) (banking.TransactionChanges, error) {
		if cursor == "" {
			return banking.TransactionChanges{
				Added: []banking.Transaction{
					transferBankTxn("t-out", "p-check", 4, 500.00),
					transferBankTxn("t-in", "p-save", 4, -500.00),
				},
				Cursor: "c1",
			}, nil
		}
		return banking.TransactionChanges{Cursor: cursor}, nil
	}

	accountsSvc := accounts.NewService(database, provider, testKey)
	registerConnection(t, accountsSvc, token, "item-a")
	svc := NewService(database, provider, accountsSvc, newCategorization(database))
	if err := svc.SyncTransactions(testCtx()); err != nil {
		t.Fatalf("SyncTransactions: %v", err)
	}
	return database, svc, providerToInternal(t, accountsSvc)
}

// savingsContributionLeg is one stored transaction carrying the
// savings_contribution subtype, the row an aggregation would count toward a
// savings total.
type savingsContributionLeg struct {
	id     string
	amount float64
}

// savingsContributionLegs reads every stored transaction whose recorded subtype
// is a savings contribution — the rows a "how much did I save this period" sum
// would add up. The whole point of the subtype living only on the outflow leg is
// that this set never double-counts a single move.
func savingsContributionLegs(t *testing.T, database *db.DB) []savingsContributionLeg {
	t.Helper()
	rows, err := database.Sql().Query(
		"SELECT id, amount_amount FROM transactions WHERE transfer_subtype = ?",
		string(categorization.SubtypeSavingsContribution),
	)
	if err != nil {
		t.Fatalf("query savings-contribution legs: %v", err)
	}
	defer rows.Close()

	var legs []savingsContributionLeg
	for rows.Next() {
		var leg savingsContributionLeg
		if err := rows.Scan(&leg.id, &leg.amount); err != nil {
			t.Fatalf("scan savings-contribution leg: %v", err)
		}
		legs = append(legs, leg)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate savings-contribution legs: %v", err)
	}
	return legs
}

// --- A savings move is counted once, on the outflow leg ---

// TestSavingsContributionCountedOnce proves that after a sync auto-pairs a
// savings transfer, the $500 is counted exactly once: only the OUTFLOW leg
// carries the savings_contribution subtype and the inflow mirror carries none, so
// summing the savings-contribution legs adds the move a single time. This is the
// emergent guarantee behind "the subtype lives only on the source leg" — without
// it a transfer between the user's own accounts would inflate savings twice.
func TestSavingsContributionCountedOnce(t *testing.T) {
	database, svc, internal := savingsPairWithDB(t)

	t.Run("exactly one stored leg carries the savings-contribution subtype", func(t *testing.T) {
		legs := savingsContributionLegs(t, database)
		if len(legs) != 1 {
			t.Fatalf("found %d savings-contribution legs, want exactly 1 (the move is counted once): %+v", len(legs), legs)
		}
	})

	t.Run("the carrier is the outflow leg, not the inflow mirror", func(t *testing.T) {
		legs := savingsContributionLegs(t, database)
		if len(legs) != 1 {
			t.Fatalf("precondition: want exactly 1 savings-contribution leg, got %d", len(legs))
		}
		if legs[0].id != "t-out" {
			t.Errorf("savings-contribution leg = %q, want the outflow leg t-out", legs[0].id)
		}
		if legs[0].amount <= 0 {
			t.Errorf("savings-contribution leg amount = %v, want a positive outflow amount", legs[0].amount)
		}
		// And the inflow mirror is explicitly unlabeled.
		if mirror := getTransferFacet(t, svc, "t-in"); mirror.Subtype != categorization.SubtypeNone {
			t.Errorf("inflow mirror subtype = %q, want empty (the mirror is never the carrier)", mirror.Subtype)
		}
	})

	t.Run("summing the savings-contribution legs counts the $500 exactly once", func(t *testing.T) {
		legs := savingsContributionLegs(t, database)
		var total float64
		for _, leg := range legs {
			total += leg.amount
		}
		if total != 500.00 {
			t.Errorf("savings-contribution total = %v, want 500 (the single move, counted once)", total)
		}
		// Sanity: the outflow leg's resolved destination is the savings account, so
		// the contribution is attributed to the right place.
		if facet := getTransferFacet(t, svc, "t-out"); facet.DestinationAccountID == nil || *facet.DestinationAccountID != internal["p-save"] {
			t.Errorf("outflow destination = %v, want the savings account id %q", facet.DestinationAccountID, internal["p-save"])
		}
	})
}

// --- The categorization facet and the transfer facet never perturb each other ---

// TestTransferAndCategorizationFacetsAreIndependent proves the two sticky facets
// stay isolated across a full cycle — sync, then a manual ReCategorize, then a
// manual MarkTransferDestination on the same row, then a re-sync. A write to one
// facet never moves the other, in both directions, and the final sync (with both
// facets now overridden) leaves the whole row alone.
func TestTransferAndCategorizationFacetsAreIndependent(t *testing.T) {
	_, svc, _ := savingsPairWithDB(t)
	ctx := testCtx()

	// The auto pass classified t-out a Transfer and resolved it to a savings
	// contribution; neither facet is overridden yet.
	if cat := getCategorizationFacet(t, svc, "t-out"); cat.Classification != categorization.Transfer || cat.Overridden {
		t.Fatalf("precondition: t-out should auto-classify transfer and not be overridden, got %+v", cat)
	}
	if tr := getTransferFacet(t, svc, "t-out"); tr.Subtype != categorization.SubtypeSavingsContribution || tr.Overridden {
		t.Fatalf("precondition: t-out should auto-resolve to a savings contribution and not be overridden, got %+v", tr)
	}

	t.Run("re-categorizing the row leaves its transfer facet untouched", func(t *testing.T) {
		before := getTransferFacet(t, svc, "t-out")

		// Manually re-categorize (keeping it a Transfer so it stays markable below);
		// this is a real write to the categorization facet — it flips that override.
		if err := svc.ReCategorize(ctx, "t-out", categorization.Transfer, nil); err != nil {
			t.Fatalf("ReCategorize: %v", err)
		}
		if cat := getCategorizationFacet(t, svc, "t-out"); !cat.Overridden {
			t.Fatalf("the re-categorize did not record its own override flag")
		}

		after := getTransferFacet(t, svc, "t-out")
		if after.Subtype != before.Subtype {
			t.Errorf("transfer subtype moved on a re-categorize: %q -> %q", before.Subtype, after.Subtype)
		}
		if !equalStringPtr(after.DestinationAccountID, before.DestinationAccountID) {
			t.Errorf("transfer destination moved on a re-categorize")
		}
		if after.Overridden {
			t.Errorf("transfer_destination_overridden was set by a re-categorize; the facets must stay independent")
		}
	})

	t.Run("marking the transfer destination leaves its categorization facet untouched", func(t *testing.T) {
		before := getCategorizationFacet(t, svc, "t-out")

		// Mark a value the auto pass would not pick (plain, no destination); this is
		// a real write to the transfer facet — it flips that override.
		if err := svc.MarkTransferDestination(ctx, "t-out", nil, categorization.SubtypePlain); err != nil {
			t.Fatalf("MarkTransferDestination: %v", err)
		}
		if tr := getTransferFacet(t, svc, "t-out"); !tr.Overridden || tr.Subtype != categorization.SubtypePlain {
			t.Fatalf("the mark did not record its own facet (got overridden=%v subtype=%q)", tr.Overridden, tr.Subtype)
		}

		after := getCategorizationFacet(t, svc, "t-out")
		if after.Classification != before.Classification {
			t.Errorf("classification moved on a transfer mark: %q -> %q", before.Classification, after.Classification)
		}
		if !equalStringPtr(after.CategoryID, before.CategoryID) {
			t.Errorf("category_id moved on a transfer mark")
		}
		if after.Overridden != before.Overridden {
			t.Errorf("categorization_overridden moved on a transfer mark: %v -> %v", before.Overridden, after.Overridden)
		}
	})

	t.Run("a final re-sync, with both facets overridden, perturbs neither", func(t *testing.T) {
		catBefore := getCategorizationFacet(t, svc, "t-out")
		trBefore := getTransferFacet(t, svc, "t-out")

		if err := svc.SyncTransactions(ctx); err != nil {
			t.Fatalf("SyncTransactions (re-run): %v", err)
		}

		catAfter := getCategorizationFacet(t, svc, "t-out")
		if catAfter.Classification != catBefore.Classification || !equalStringPtr(catAfter.CategoryID, catBefore.CategoryID) || catAfter.Overridden != catBefore.Overridden {
			t.Errorf("categorization facet drifted on re-sync: before %+v, after %+v", catBefore, catAfter)
		}
		trAfter := getTransferFacet(t, svc, "t-out")
		if trAfter.Subtype != trBefore.Subtype || !equalStringPtr(trAfter.DestinationAccountID, trBefore.DestinationAccountID) || trAfter.Overridden != trBefore.Overridden {
			t.Errorf("transfer facet drifted on re-sync: before %+v, after %+v", trBefore, trAfter)
		}
	})
}

// --- A manual override survives re-sync, including the pending→posted reconcile ---

// TestStickyOverridesSurviveResync proves both sticky overrides outlive a re-sync
// that also reconciles a pending row to posted. A transfer is marked to a value
// the auto pass would never pick (plain, no destination, against an auto savings
// pair) and a separate row's categorization is overridden; the very row carrying
// that categorization override is then driven pending→posted. The reconcile
// re-touches the row, yet neither override is reverted.
func TestStickyOverridesSurviveResync(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	const token = "tok-a"
	provider := newStub()
	provider.accountsByToken[token] = []banking.Account{
		cashAccount("p-check", "Everyday Checking"),
		savingsAccount("p-save", "High-Yield Savings"),
	}
	provider.syncByToken[token] = func(cursor string) (banking.TransactionChanges, error) {
		switch cursor {
		case "":
			return banking.TransactionChanges{
				Added: []banking.Transaction{
					// A savings pair the auto pass resolves to a savings contribution.
					transferBankTxn("t-out", "p-check", 4, 500.00),
					transferBankTxn("t-in", "p-save", 4, -500.00),
					// A pending spending charge that will reconcile to posted below.
					bankTxnCat("t-pending", "p-check", "Whole Foods", 1, 84.32, "GENERAL_MERCHANDISE", true),
				},
				Cursor: "c1",
			}, nil
		case "c1":
			// The pending charge posts — a modify that re-touches the row.
			return banking.TransactionChanges{
				Modified: []banking.Transaction{
					bankTxnCat("t-pending", "p-check", "Whole Foods", 1, 84.32, "GENERAL_MERCHANDISE", false),
				},
				Cursor: "c2",
			}, nil
		default:
			return banking.TransactionChanges{Cursor: cursor}, nil
		}
	}

	accountsSvc := accounts.NewService(database, provider, testKey)
	registerConnection(t, accountsSvc, token, "item-a")
	svc := NewService(database, provider, accountsSvc, newCategorization(database))
	if err := svc.SyncTransactions(ctx); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// Preconditions: the transfer auto-resolved to a savings contribution, and the
	// pending charge auto-categorized to its spending Category.
	if tr := getTransferFacet(t, svc, "t-out"); tr.Subtype != categorization.SubtypeSavingsContribution {
		t.Fatalf("precondition: t-out should auto-resolve to a savings contribution, got %+v", tr)
	}
	if class, _, _ := readCategorization(t, database, "t-pending"); class != string(categorization.Spending) {
		t.Fatalf("precondition: t-pending should auto-classify spending, got %q", class)
	}

	// Pin the transfer to a value the auto pass would never pick (plain, no
	// destination), and override the pending row's categorization to Income.
	if err := svc.MarkTransferDestination(ctx, "t-out", nil, categorization.SubtypePlain); err != nil {
		t.Fatalf("MarkTransferDestination: %v", err)
	}
	if err := svc.ReCategorize(ctx, "t-pending", categorization.Income, nil); err != nil {
		t.Fatalf("ReCategorize: %v", err)
	}

	// Re-sync: this reconciles t-pending pending→posted (a modify that re-touches
	// the very row carrying the categorization override).
	if err := svc.SyncTransactions(ctx); err != nil {
		t.Fatalf("reconcile sync: %v", err)
	}

	t.Run("the reconcile actually moved the pending row to posted", func(t *testing.T) {
		var status string
		if err := database.Sql().QueryRow("SELECT status FROM transactions WHERE id = ?", "t-pending").Scan(&status); err != nil {
			t.Fatalf("read status: %v", err)
		}
		if status != string(StatusPosted) {
			t.Fatalf("t-pending status = %q, want posted (the reconcile must have run)", status)
		}
	})

	t.Run("the marked transfer facet is unchanged by the re-sync", func(t *testing.T) {
		facet := getTransferFacet(t, svc, "t-out")
		if !facet.Overridden {
			t.Fatalf("transfer override was cleared; a manual mark must stay sticky")
		}
		if facet.Subtype != categorization.SubtypePlain {
			t.Errorf("transfer subtype = %q, want %q (not re-paired to savings)", facet.Subtype, categorization.SubtypePlain)
		}
		if facet.DestinationAccountID != nil {
			t.Errorf("transfer destination = %q, want nil (the manual choice preserved)", *facet.DestinationAccountID)
		}
	})

	t.Run("the separately-overridden categorization facet survives the reconcile", func(t *testing.T) {
		class, categoryID, overridden := readCategorization(t, database, "t-pending")
		if overridden != 1 {
			t.Fatalf("categorization override was cleared by the reconcile; it must stay sticky")
		}
		if class != string(categorization.Income) {
			t.Errorf("classification = %q, want income (the override must hold across the reconcile)", class)
		}
		if categoryID.Valid {
			t.Errorf("category_id = %v, want none (an income override carries no Category)", categoryID)
		}
	})
}

// --- Savings is never guessed: it needs exactly one match into a savings account ---

// TestSavingsContributionRequiresExactlyOneSavingsMatch proves the pairing is
// conservative end-to-end: an outflow Transfer is labeled a savings contribution
// only when it has exactly one unambiguous matching inflow into a counts-as-savings
// account. With zero matches, or with two ambiguous matches, the leg is never a
// savings contribution and its destination stays unknown — a missing or ambiguous
// pair is never guessed into a contribution, since a false pair would silently
// hide real spending.
func TestSavingsContributionRequiresExactlyOneSavingsMatch(t *testing.T) {
	t.Run("zero matches: a lone outflow transfer is not a savings contribution", func(t *testing.T) {
		database := newTestDB(t)
		ctx := testCtx()

		const token = "tok-a"
		provider := newStub()
		provider.accountsByToken[token] = []banking.Account{
			cashAccount("p-check", "Everyday Checking"),
			savingsAccount("p-save", "High-Yield Savings"),
		}
		provider.syncByToken[token] = func(cursor string) (banking.TransactionChanges, error) {
			if cursor == "" {
				// A lone $500 outflow transfer with no matching inflow leg anywhere.
				return banking.TransactionChanges{
					Added:  []banking.Transaction{transferBankTxn("t-out", "p-check", 4, 500.00)},
					Cursor: "c1",
				}, nil
			}
			return banking.TransactionChanges{Cursor: cursor}, nil
		}

		accountsSvc := accounts.NewService(database, provider, testKey)
		registerConnection(t, accountsSvc, token, "item-a")
		svc := NewService(database, provider, accountsSvc, newCategorization(database))
		if err := svc.SyncTransactions(ctx); err != nil {
			t.Fatalf("SyncTransactions: %v", err)
		}

		facet := getTransferFacet(t, svc, "t-out")
		if facet.Subtype == categorization.SubtypeSavingsContribution {
			t.Errorf("subtype = savings_contribution with no matching inflow; pairing must be conservative")
		}
		if facet.Subtype != categorization.SubtypePlain {
			t.Errorf("subtype = %q, want plain (no match → unknown, not savings)", facet.Subtype)
		}
		if facet.DestinationAccountID != nil {
			t.Errorf("destination = %q, want nil (no match)", *facet.DestinationAccountID)
		}
		if legs := savingsContributionLegs(t, database); len(legs) != 0 {
			t.Errorf("found %d savings-contribution legs, want 0", len(legs))
		}
	})

	t.Run("two ambiguous matches: a $500 outflow with two same-amount in-window inflows is not a savings contribution", func(t *testing.T) {
		database := newTestDB(t)
		ctx := testCtx()

		const token = "tok-a"
		provider := newStub()
		provider.accountsByToken[token] = []banking.Account{
			cashAccount("p-check", "Everyday Checking"),
			savingsAccount("p-save", "High-Yield Savings"),
			cashAccount("p-other", "Second Checking"),
		}
		provider.syncByToken[token] = func(cursor string) (banking.TransactionChanges, error) {
			if cursor == "" {
				// One $500 outflow, two same-amount inflows in the window — one into
				// the savings account, one into another checking. The pair is
				// ambiguous, so the engine must refuse to call it savings.
				return banking.TransactionChanges{
					Added: []banking.Transaction{
						transferBankTxn("t-out", "p-check", 4, 500.00),
						transferBankTxn("t-in-save", "p-save", 4, -500.00),
						transferBankTxn("t-in-other", "p-other", 5, -500.00),
					},
					Cursor: "c1",
				}, nil
			}
			return banking.TransactionChanges{Cursor: cursor}, nil
		}

		accountsSvc := accounts.NewService(database, provider, testKey)
		registerConnection(t, accountsSvc, token, "item-a")
		svc := NewService(database, provider, accountsSvc, newCategorization(database))
		if err := svc.SyncTransactions(ctx); err != nil {
			t.Fatalf("SyncTransactions: %v", err)
		}

		facet := getTransferFacet(t, svc, "t-out")
		if facet.Subtype == categorization.SubtypeSavingsContribution {
			t.Errorf("subtype = savings_contribution on an ambiguous pair; two matches must not be guessed into a contribution")
		}
		if facet.Subtype != categorization.SubtypePlain {
			t.Errorf("subtype = %q, want plain (ambiguous → unknown, not savings)", facet.Subtype)
		}
		if facet.DestinationAccountID != nil {
			t.Errorf("destination = %q, want nil (ambiguous pairing resolves to no destination)", *facet.DestinationAccountID)
		}
		if legs := savingsContributionLegs(t, database); len(legs) != 0 {
			t.Errorf("found %d savings-contribution legs on an ambiguous pair, want 0", len(legs))
		}
	})
}
