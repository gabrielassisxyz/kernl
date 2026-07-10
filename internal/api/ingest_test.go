package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

func TestIngestAPI(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	tempDir := t.TempDir()

	cfg := &config.Config{
		Vault: config.VaultConfig{Root: tempDir},
		Registry: config.RegistryConfig{
			Repos: []config.RepoEntry{{Path: tempDir}},
		},
	}

	a := &app.App{
		Config: cfg,
		Graph:  g,
	}

	mux := http.NewServeMux()
	RegisterIngestRoutes(mux, a)

	// Test 1: Trigger ingest is disabled without an LLM.
	testFile := filepath.Join(tempDir, "test.md")
	_ = os.WriteFile(testFile, []byte("hello"), 0644)

	body := `{"file_path":"` + testFile + `", "node_id":"n1"}`
	req := httptest.NewRequest("POST", "/api/ingest/trigger", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Trigger expected 503, got %d", w.Code)
	}

	// Test 2: List queue (insert dummy first)
	var id string
	_ = g.DoWrite(req.Context(), func(tx *graph.WriteTx) error {
		var err error
		id, err = nodes.CreateIngestReview(req.Context(), tx, nodes.IngestReview{Title: "Test"}, nodes.Author{Name: "test"})
		return err
	})

	req2 := httptest.NewRequest("GET", "/api/ingest/queue", nil)
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("Queue expected 200, got %d", w2.Code)
	}

	var items []nodes.IngestReview
	_ = json.NewDecoder(w2.Body).Decode(&items)
	if len(items) == 0 {
		t.Error("Expected at least one item in queue")
	}

	// Test 3: Resolve is disabled without an LLM so stale reviews cannot be
	// mutated while ingest is off.
	req3 := httptest.NewRequest("POST", "/api/ingest/queue/"+id+"/resolve", nil)
	w3 := httptest.NewRecorder()
	mux.ServeHTTP(w3, req3)

	if w3.Code != http.StatusServiceUnavailable {
		t.Errorf("Resolve expected 503, got %d", w3.Code)
	}

	_ = g.DoRead(req.Context(), func(tx *graph.ReadTx) error {
		_, err := nodes.GetIngestReview(req.Context(), tx, id)
		if err != nil {
			t.Error("Expected node to remain queued")
		}
		return nil
	})
}
