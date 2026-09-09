package sweep

import (
	"testing"
	"time"
)

// snapshots builds a newest-first history, the order the repo returns.
func snapshots(ids ...string) []Recommendation {
	out := make([]Recommendation, 0, len(ids))
	base := time.Date(2026, time.September, 8, 12, 0, 0, 0, time.UTC)
	for i, id := range ids {
		out = append(out, Recommendation{
			ID:         id,
			Kind:       KindNumeric,
			ComputedAt: base.Add(-time.Duration(i) * time.Hour),
		})
	}
	return out
}

// With no snapshot named, the page opens on the newest one.
func TestNeighborsDefaultsToNewest(t *testing.T) {
	got, found := neighbors(snapshots("c", "b", "a"), "")

	if !found {
		t.Fatal("found=false with a non-empty history")
	}
	if got.Recommendation.ID != "c" {
		t.Errorf("target: want the newest snapshot c, got %s", got.Recommendation.ID)
	}
	if got.NewerID != "" {
		t.Errorf("NewerID: want none at the newest snapshot, got %s", got.NewerID)
	}
	if got.OlderID != "b" {
		t.Errorf("OlderID: want b, got %s", got.OlderID)
	}
}

// Stepping from a middle snapshot reaches its neighbours in both directions.
func TestNeighborsFromTheMiddle(t *testing.T) {
	got, found := neighbors(snapshots("c", "b", "a"), "b")

	if !found {
		t.Fatal("found=false for a snapshot that is in the history")
	}
	if got.Recommendation.ID != "b" {
		t.Errorf("target: want b, got %s", got.Recommendation.ID)
	}
	if got.NewerID != "c" {
		t.Errorf("NewerID: want c, got %s", got.NewerID)
	}
	if got.OlderID != "a" {
		t.Errorf("OlderID: want a, got %s", got.OlderID)
	}
}

// The oldest snapshot has nowhere older to step, so the control has no target.
func TestNeighborsAtTheOldest(t *testing.T) {
	got, found := neighbors(snapshots("c", "b", "a"), "a")

	if !found {
		t.Fatal("found=false for the oldest snapshot")
	}
	if got.OlderID != "" {
		t.Errorf("OlderID: want none at the oldest snapshot, got %s", got.OlderID)
	}
	if got.NewerID != "b" {
		t.Errorf("NewerID: want b, got %s", got.NewerID)
	}
}

// A single snapshot is both ends of the history at once.
func TestNeighborsWithOneSnapshot(t *testing.T) {
	got, found := neighbors(snapshots("only"), "")

	if !found {
		t.Fatal("found=false with one snapshot stored")
	}
	if got.OlderID != "" || got.NewerID != "" {
		t.Errorf("want no steps available, got older=%q newer=%q", got.OlderID, got.NewerID)
	}
}

// Before any run, there is nothing to show — the first-run empty state, which is
// distinct from a stored needs-attention snapshot.
func TestNeighborsWithEmptyHistory(t *testing.T) {
	if _, found := neighbors(nil, ""); found {
		t.Error("found=true with no snapshots stored")
	}
}

// A deep link to a snapshot that does not exist is not the newest one silently:
// the caller needs to be able to tell the difference and 404.
func TestNeighborsUnknownIDIsNotFound(t *testing.T) {
	if _, found := neighbors(snapshots("c", "b", "a"), "nope"); found {
		t.Error("found=true for an id that is not in the history")
	}
}
