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
