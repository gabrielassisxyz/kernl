package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/api"
	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

func TestInboxAPI(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "graph.db")
	g, err := graph.Open(ctx, graph.Config{Path: dbPath})
	if err != nil {
		t.Fatalf("graph.Open: %v", err)
	}
	defer g.Close()

	var captureID string
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		id, err := nodes.CreateCapture(ctx, tx, nodes.Capture{
			Title: "API Capture",
			Body:  "https://example.com",
			Tags:  []string{"pending"},
		}, nodes.Author{Name: "tester"})
		captureID = id
		return err
	})
	if err != nil {
		t.Fatalf("CreateCapture: %v", err)
	}

	a := &app.App{
		Graph: g,
		Config: &config.Config{
			Vault: config.VaultConfig{
				Root: t.TempDir(),
			},
		},
	}

	mux := http.NewServeMux()
	api.RegisterInboxRoutes(mux, a)

	// Test GET /api/inbox/pending
	req := httptest.NewRequest(http.MethodGet, "/api/inbox/pending", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("GET /api/inbox/pending returned %d, expected %d", rr.Code, http.StatusOK)
	}

	var caps []nodes.Capture
	if err := json.Unmarshal(rr.Body.Bytes(), &caps); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(caps) != 1 || caps[0].ID != captureID {
		t.Errorf("expected 1 capture with ID %q, got %v", captureID, caps)
	}

	// Test POST /api/inbox/{id}/convert
	body := `{"action": "bookmark"}`
	req = httptest.NewRequest(http.MethodPost, "/api/inbox/"+captureID+"/convert", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("POST convert returned %d, expected %d", rr.Code, http.StatusOK)
	}

	// Verify Capture is triaged
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		cap, err := nodes.GetCapture(ctx, tx, captureID)
		if err != nil {
			return err
		}

		hasTriaged := false
		for _, tag := range cap.Tags {
			if tag == "triaged" {
				hasTriaged = true
			}
		}
		if !hasTriaged {
			t.Errorf("expected 'triaged' tag on capture, got %v", cap.Tags)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("DoRead: %v", err)
	}
}

// TestInboxPendingDTOShape verifies /api/inbox/pending emits the camelCase,
// UI-shaped fields web/pages/inbox.vue reads (id, subtitle), not the raw
// PascalCase Capture struct.
func TestInboxPendingDTOShape(t *testing.T) {
	ctx := context.Background()
	g, err := graph.Open(ctx, graph.Config{Path: filepath.Join(t.TempDir(), "graph.db")})
	if err != nil {
		t.Fatalf("graph.Open: %v", err)
	}
	defer g.Close()

	var captureID string
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		id, err := nodes.CreateCapture(ctx, tx, nodes.Capture{Body: "buy milk", Tags: []string{"pending"}}, nodes.Author{Name: "tester"})
		captureID = id
		return err
	}); err != nil {
		t.Fatalf("CreateCapture: %v", err)
	}

	a := &app.App{Graph: g, Config: &config.Config{Vault: config.VaultConfig{Root: t.TempDir()}}}
	mux := http.NewServeMux()
	api.RegisterInboxRoutes(mux, a)

	req := httptest.NewRequest(http.MethodGet, "/api/inbox/pending", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	var items []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &items); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	it := items[0]
	if it["id"] != captureID {
		t.Errorf("expected camelCase id=%q, got %v (keys: %v)", captureID, it["id"], keysOf(it))
	}
	if it["subtitle"] != "buy milk" {
		t.Errorf("expected subtitle to carry the body, got %v", it["subtitle"])
	}
	// Title is derived from the body when the capture has no explicit title.
	if it["title"] != "buy milk" {
		t.Errorf("expected derived title 'buy milk', got %v", it["title"])
	}
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
