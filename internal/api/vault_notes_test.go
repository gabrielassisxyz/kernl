package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The CLI's `note list` reads /api/vault/notes and filters on its author, so
// the field must be resolved by the same rule the /api/notes index uses -
// "da" becomes "agent:da", an absent value becomes "human". Removing the
// resolution would leave the field empty, and this test fails.
func TestVaultNotesResolvesAuthor(t *testing.T) {
	a, _ := newCompanionTestApp(t)

	seedVaultNote(t, a, "mine", "notes/mine.md", "", "")
	seedVaultNote(t, a, "the DA's", "notes/da.md", "da", "")
	seedVaultNote(t, a, "named human", "notes/named.md", "user", "")

	rec := httptest.NewRecorder()
	NewRouter(a).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/vault/notes", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/vault/notes = %d, body %s", rec.Code, rec.Body.String())
	}
	var notes []struct {
		Path   string `json:"path"`
		Author string `json:"author"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &notes); err != nil {
		t.Fatalf("decode: %v", err)
	}
	byPath := map[string]string{}
	for _, n := range notes {
		byPath[n.Path] = n.Author
	}
	for path, want := range map[string]string{
		"notes/mine.md":  "human",
		"notes/da.md":    "agent:da",
		"notes/named.md": "user",
	} {
		if got := byPath[path]; got != want {
			t.Errorf("%s author = %q, want %q", path, got, want)
		}
	}
}
