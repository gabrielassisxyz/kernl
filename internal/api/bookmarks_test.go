package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

func TestBookmarkAPI(t *testing.T) {
	oldStartBookmarkArchive := startBookmarkArchive
	var archivedIDs []string
	startBookmarkArchive = func(_ *graph.Graph, _ string, id string) {
		archivedIDs = append(archivedIDs, id)
	}
	t.Cleanup(func() { startBookmarkArchive = oldStartBookmarkArchive })

	// Vault.Root must point at a temp dir: an empty root resolves to a path
	// relative to the test's cwd, polluting internal/api/.kernl in the repo.
	cfg := &config.Config{Vault: config.VaultConfig{Root: t.TempDir()}}
	a := &app.App{
		Config:  cfg,
		Backend: backend.NewBdCliBackend("/tmp/test"),
	}
	a.Graph = testutil.NewInMemoryTestGraph(t)

	mux := http.NewServeMux()
	RegisterBookmarkRoutes(mux, a)

	body := `{"url":"https://example.com"}`
	req := httptest.NewRequest("POST", "/api/bookmarks", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201 Created, got %d", w.Code)
	}

	var resp struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.ID == "" {
		t.Error("expected bookmark ID")
	}
	if len(archivedIDs) != 1 || archivedIDs[0] != resp.ID {
		t.Errorf("archive requested for %v, want created bookmark %q", archivedIDs, resp.ID)
	}

	req = httptest.NewRequest("GET", "/api/bookmarks", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", w.Code)
	}

	var list []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 bookmark, got %d", len(list))
	}
}

// newTestBookmarkMux wires a fresh in-memory app + bookmark routes, matching
// TestBookmarkAPI's setup, for the write-path tests below.
func newTestBookmarkMux(t *testing.T) *http.ServeMux {
	t.Helper()
	cfg := &config.Config{Vault: config.VaultConfig{Root: t.TempDir()}}
	a := &app.App{
		Config:  cfg,
		Backend: backend.NewBdCliBackend("/tmp/test"),
	}
	a.Graph = testutil.NewInMemoryTestGraph(t)

	mux := http.NewServeMux()
	api.RegisterBookmarkRoutes(mux, a)
	return mux
}

func createTestBookmark(t *testing.T, mux *http.ServeMux, body string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/bookmarks", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create bookmark: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	return resp.ID
}

func TestCreateBookmarkTagsRoundTripThroughList(t *testing.T) {
	mux := newTestBookmarkMux(t)
	createTestBookmark(t, mux, `{"url":"https://example.com","tags":["go","reading"]}`)

	req := httptest.NewRequest(http.MethodGet, "/api/bookmarks", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var list []struct {
		Tags []string `json:"tags"`
	}
	if err := json.NewDecoder(w.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 bookmark, got %d", len(list))
	}
	got := list[0].Tags
	want := []string{"go", "reading"}
	if len(got) != len(want) {
		t.Fatalf("tags = %v, want %v", got, want)
	}
	seen := map[string]bool{}
	for _, tg := range got {
		seen[tg] = true
	}
	for _, tg := range want {
		if !seen[tg] {
			t.Errorf("tags %v missing %q", got, tg)
		}
	}
}

func TestPatchBookmarkUpdatesEveryFieldAndLeavesOmittedFieldsUnchanged(t *testing.T) {
	mux := newTestBookmarkMux(t)
	id := createTestBookmark(t, mux, `{"url":"https://example.com","tags":["go"]}`)

	// Patch title and tags only; description and archived state are omitted
	// and must survive untouched.
	req := httptest.NewRequest(http.MethodPatch, "/api/bookmarks/"+id, bytes.NewBufferString(`{"title":"A Better Title","tags":["go","reading"]}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("patch: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var patched struct {
		Title       string   `json:"title"`
		Tags        []string `json:"tags"`
		ArchivedAt  *string  `json:"archivedAt"`
		Description string   `json:"description"`
	}
	if err := json.NewDecoder(w.Body).Decode(&patched); err != nil {
		t.Fatal(err)
	}
	if patched.Title != "A Better Title" {
		t.Errorf("title = %q, want %q", patched.Title, "A Better Title")
	}
	if len(patched.Tags) != 2 {
		t.Errorf("tags = %v, want 2 entries", patched.Tags)
	}
	if patched.ArchivedAt != nil {
		t.Errorf("archivedAt = %v, want nil (omitted field must stay unchanged)", *patched.ArchivedAt)
	}

	// Patch only description; title and tags from the previous patch must
	// survive this one untouched too.
	req = httptest.NewRequest(http.MethodPatch, "/api/bookmarks/"+id, bytes.NewBufferString(`{"description":"a note"}`))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("second patch: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if err := json.NewDecoder(w.Body).Decode(&patched); err != nil {
		t.Fatal(err)
	}
	if patched.Title != "A Better Title" {
		t.Errorf("title after unrelated patch = %q, want it unchanged", patched.Title)
	}
	if len(patched.Tags) != 2 {
		t.Errorf("tags after unrelated patch = %v, want unchanged", patched.Tags)
	}
	if patched.Description != "a note" {
		t.Errorf("description = %q, want %q", patched.Description, "a note")
	}

	// Archive it.
	req = httptest.NewRequest(http.MethodPatch, "/api/bookmarks/"+id, bytes.NewBufferString(`{"archived":true}`))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("archive patch: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if err := json.NewDecoder(w.Body).Decode(&patched); err != nil {
		t.Fatal(err)
	}
	if patched.ArchivedAt == nil {
		t.Error("archivedAt = nil after archiving, want a timestamp")
	}
}

func TestGetAndPatchUnknownBookmarkReturn404(t *testing.T) {
	mux := newTestBookmarkMux(t)

	req := httptest.NewRequest(http.MethodGet, "/api/bookmarks/no-such-id", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("GET unknown id: expected 404, got %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodPatch, "/api/bookmarks/no-such-id", bytes.NewBufferString(`{"title":"x"}`))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("PATCH unknown id: expected 404, got %d", w.Code)
	}
}

func TestGetBookmarkReturnsSameShapeAsListElement(t *testing.T) {
	mux := newTestBookmarkMux(t)
	id := createTestBookmark(t, mux, `{"url":"https://example.com","tags":["go"]}`)

	req := httptest.NewRequest(http.MethodGet, "/api/bookmarks/"+id, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var single map[string]json.RawMessage
	if err := json.NewDecoder(w.Body).Decode(&single); err != nil {
		t.Fatal(err)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/bookmarks", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	var list []map[string]json.RawMessage
	if err := json.NewDecoder(w.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 bookmark, got %d", len(list))
	}
	for key := range list[0] {
		if _, ok := single[key]; !ok {
			t.Errorf("GET /{id} response missing key %q that the list element carries", key)
		}
	}
	for key := range single {
		if _, ok := list[0][key]; !ok {
			t.Errorf("GET /{id} response has extra key %q the list element does not carry", key)
		}
	}
}

func TestListBookmarksTagsFilterReturnsOnlyMatching(t *testing.T) {
	mux := newTestBookmarkMux(t)
	createTestBookmark(t, mux, `{"url":"https://a.example.com","tags":["go"]}`)
	createTestBookmark(t, mux, `{"url":"https://b.example.com","tags":["python"]}`)
	createTestBookmark(t, mux, `{"url":"https://c.example.com"}`)

	req := httptest.NewRequest(http.MethodGet, "/api/bookmarks?tags=go", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var list []struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(w.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].URL != "https://a.example.com" {
		t.Errorf("tags=go filter returned %+v, want only the go bookmark", list)
	}
}

// TestListBookmarksTagsFilterIsMatchAny pins the decision recorded on
// BookmarkFilter: a bookmark matching at least one of the requested tags is
// included, not only a bookmark carrying all of them. Two single-tag
// bookmarks and a query for both tags is what tells match-any and match-all
// apart - a query with only one tag cannot, since both semantics agree there.
func TestListBookmarksTagsFilterIsMatchAny(t *testing.T) {
	mux := newTestBookmarkMux(t)
	createTestBookmark(t, mux, `{"url":"https://go-only.example.com","tags":["go"]}`)
	createTestBookmark(t, mux, `{"url":"https://python-only.example.com","tags":["python"]}`)
	createTestBookmark(t, mux, `{"url":"https://neither.example.com","tags":["rust"]}`)

	req := httptest.NewRequest(http.MethodGet, "/api/bookmarks?tags=go,python", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var list []struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(w.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("tags=go,python (match-any) returned %d bookmarks, want 2: %+v", len(list), list)
	}
}

func TestListBookmarksArchivedParameterFiltersByState(t *testing.T) {
	mux := newTestBookmarkMux(t)
	activeID := createTestBookmark(t, mux, `{"url":"https://active.example.com"}`)
	archivedID := createTestBookmark(t, mux, `{"url":"https://archived.example.com"}`)

	req := httptest.NewRequest(http.MethodPatch, "/api/bookmarks/"+archivedID, bytes.NewBufferString(`{"archived":true}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("archive setup: expected 200, got %d", w.Code)
	}

	cases := []struct {
		query   string
		wantIDs []string
	}{
		{"", []string{activeID, archivedID}},
		{"?archived=false", []string{activeID}},
		{"?archived=true", []string{archivedID}},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, "/api/bookmarks"+tc.query, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		var list []struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(w.Body).Decode(&list); err != nil {
			t.Fatalf("query %q: %v", tc.query, err)
		}
		if len(list) != len(tc.wantIDs) {
			t.Errorf("query %q: got %d bookmarks, want %d", tc.query, len(list), len(tc.wantIDs))
			continue
		}
		got := map[string]bool{}
		for _, b := range list {
			got[b.ID] = true
		}
		for _, id := range tc.wantIDs {
			if !got[id] {
				t.Errorf("query %q: missing id %s in %+v", tc.query, id, list)
			}
		}
	}
}
