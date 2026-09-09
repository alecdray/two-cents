package sweep

// Snapshot is one recommendation positioned in the history: the snapshot itself
// plus where the user can step from it. OlderID and NewerID are empty at the
// ends of the timeline, which is how the page knows to disable a control.
type Snapshot struct {
	Recommendation Recommendation
	OlderID        string
	NewerID        string
}

// neighbors locates id in a newest-first history and reports its neighbours. An
// empty id selects the newest snapshot, which is what a plain page load wants.
// found is false for an empty history and for an id that is not in it — a deep
// link to a snapshot that does not exist must not silently show a different one.
func neighbors(all []Recommendation, id string) (Snapshot, bool) {
	if len(all) == 0 {
		return Snapshot{}, false
	}

	i := 0
	if id != "" {
		i = -1
		for n, rec := range all {
			if rec.ID == id {
				i = n
				break
			}
		}
		if i < 0 {
			return Snapshot{}, false
		}
	}

	// The list runs newest-first, so the older neighbour is the next element and
	// the newer one is the previous.
	snap := Snapshot{Recommendation: all[i]}
	if i+1 < len(all) {
		snap.OlderID = all[i+1].ID
	}
	if i > 0 {
		snap.NewerID = all[i-1].ID
	}
	return snap, true
}
