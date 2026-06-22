package httpx

import (
	"net/http/httptest"
	"testing"
)

func TestSetHXTrigger(t *testing.T) {
	t.Run("sets HX-Trigger header with event name and detail payload", func(t *testing.T) {
		rec := httptest.NewRecorder()

		if err := SetHXTrigger(rec, "transaction-changed", map[string]string{"id": "abc"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := rec.Header().Get("HX-Trigger")
		want := `{"transaction-changed":{"id":"abc"}}`
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("a nil detail serialises the event with a null payload", func(t *testing.T) {
		rec := httptest.NewRecorder()

		if err := SetHXTrigger(rec, "transaction-changed", nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := rec.Header().Get("HX-Trigger")
		want := `{"transaction-changed":null}`
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}
