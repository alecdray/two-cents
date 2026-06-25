package views

// The DOM ids of the two management pages' shared swap regions — the <main>
// elements each page renders its content into and every mutation form targets
// for an innerHTML swap. The pages own the ids here so the forms and the pages
// agree on the region names without hard-coding the strings at the call sites.
const (
	categoriesRegionID = "categories"
	rulesRegionID      = "rules"
)

// CategoriesRegionID returns the categories swap region's DOM id for callers that
// need to name the region (e.g. a mutation form's hx-target).
func CategoriesRegionID() string { return categoriesRegionID }

// RulesRegionID returns the rules swap region's DOM id.
func RulesRegionID() string { return rulesRegionID }

// The DOM ids of the rule editor modal: the <dialog> the shared shell names (so its
// close form can dismiss it) and the editor body region the save re-renders in
// place, so a validation error swaps itself back in and the modal stays open. Both
// are owned here once, shared by the GET open render and the POST re-render.
const (
	ruleEditorModalID  = "rule-editor-modal"
	ruleEditorRegionID = "rule-editor"
)

// RuleEditorRegionID returns the editor body region's DOM id — the hx-target the
// editor form swaps its re-render into.
func RuleEditorRegionID() string { return ruleEditorRegionID }

// formFailure scopes a recoverable inline error to a single row's form: the id of
// the entity whose mutation failed and the message to show beside it. The zero
// value (empty message) means no failure, so a page threads it everywhere and
// only the matching row renders the error.
type formFailure struct {
	id      string
	message string
}

// matches reports whether the failure belongs to the given entity — true only
// when there is a message and the ids agree.
func (f formFailure) matches(id string) bool {
	return f.message != "" && f.id == id
}

// NewFormFailure builds a row-scoped inline failure for the handler to thread
// into a page's props (e.g. a rename or edit that failed validation).
func NewFormFailure(id, message string) formFailure {
	return formFailure{id: id, message: message}
}
