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
