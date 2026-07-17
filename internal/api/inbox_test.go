package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
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

func TestInboxBatchPreviewAndCreate(t *testing.T) {
	ctx := context.Background()
	g, err := graph.Open(ctx, graph.Config{Path: filepath.Join(t.TempDir(), "graph.db")})
	if err != nil {
		t.Fatalf("graph.Open: %v", err)
	}
	defer g.Close()

	a := &app.App{Graph: g, Config: &config.Config{Vault: config.VaultConfig{Root: t.TempDir()}}}
	mux := http.NewServeMux()
	api.RegisterInboxRoutes(mux, a)

	body := `{"text":"[06/07/2026, 14:32] Me: Project idea\n[06/07/2026, 14:33] Me: Task idea","source":"whatsapp","splitMode":"whatsapp","contextTitle":"Planning dump"}`
	req := httptest.NewRequest(http.MethodPost, "/api/inbox/batch/analyze", bytes.NewBufferString(`{"text":"[06/07/2026, 14:32] Me: Project idea\n[06/07/2026, 14:33] Me: Task idea"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("analyze returned %d: %s", rr.Code, rr.Body.String())
	}
	var analysis struct {
		Source                string           `json:"source"`
		Separator             string           `json:"separator"`
		SuggestedContextTitle string           `json:"suggestedContextTitle"`
		Segments              []map[string]any `json:"segments"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &analysis); err != nil {
		t.Fatalf("unmarshal analysis: %v", err)
	}
	if analysis.Source != "whatsapp" || analysis.Separator != "whatsapp" || analysis.SuggestedContextTitle == "" || len(analysis.Segments) != 2 {
		t.Fatalf("unexpected analysis: %#v", analysis)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/inbox/batch/preview", bytes.NewBufferString(body))
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("preview returned %d: %s", rr.Code, rr.Body.String())
	}
	var preview struct {
		Segments []map[string]any `json:"segments"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &preview); err != nil {
		t.Fatalf("unmarshal preview: %v", err)
	}
	if len(preview.Segments) != 2 {
		t.Fatalf("expected 2 preview segments, got %d", len(preview.Segments))
	}

	req = httptest.NewRequest(http.MethodPost, "/api/inbox/batch", bytes.NewBufferString(body))
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create returned %d: %s", rr.Code, rr.Body.String())
	}
	var created struct {
		BatchID string   `json:"batchId"`
		IDs     []string `json:"ids"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create: %v", err)
	}
	if created.BatchID == "" || len(created.IDs) != 2 {
		t.Fatalf("unexpected create response: %#v", created)
	}

	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		cap, err := nodes.GetCapture(ctx, tx, created.IDs[0])
		if err != nil {
			return err
		}
		if cap.BatchID != created.BatchID {
			t.Fatalf("BatchID = %q, want %q", cap.BatchID, created.BatchID)
		}
		if cap.BatchContextTitle != "Planning dump" {
			t.Fatalf("BatchContextTitle = %q", cap.BatchContextTitle)
		}
		if cap.BatchTimestamp != "06/07/2026 14:32" {
			t.Fatalf("BatchTimestamp = %q", cap.BatchTimestamp)
		}
		return nil
	}); err != nil {
		t.Fatalf("DoRead: %v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/inbox/batch-log?batchId="+created.BatchID, nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("batch read returned %d: %s", rr.Code, rr.Body.String())
	}
	var batchLog map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &batchLog); err != nil {
		t.Fatalf("unmarshal batch read: %v", err)
	}
	if batchLog["batchId"] != created.BatchID {
		t.Fatalf("batchId = %v", batchLog["batchId"])
	}
	if len(batchLog["rawEntries"].([]any)) != 2 {
		t.Fatalf("expected 2 raw entries, got %v", batchLog["rawEntries"])
	}
	if len(batchLog["finalEntries"].([]any)) != 2 {
		t.Fatalf("expected 2 final entries, got %v", batchLog["finalEntries"])
	}
	if len(batchLog["createdCaptureIds"].([]any)) != 2 {
		t.Fatalf("expected 2 created capture ids, got %v", batchLog["createdCaptureIds"])
	}
}

func TestInboxBatchAnalyzeWithLLM(t *testing.T) {
	ctx := context.Background()
	g, err := graph.Open(ctx, graph.Config{Path: filepath.Join(t.TempDir(), "graph.db")})
	if err != nil {
		t.Fatalf("graph.Open: %v", err)
	}
	defer g.Close()

	a := &app.App{
		Graph: g,
		Config: &config.Config{
			Vault: config.VaultConfig{Root: t.TempDir()},
			LLM: config.LLMConfig{
				Provider: "noop",
			},
		},
	}
	mux := http.NewServeMux()
	api.RegisterInboxRoutes(mux, a)

	body := `{"text":"[06/07/2026, 14:32] Me: Build an ai-memory explainer project\n[06/07/2026, 14:33] Me: Task: map the repo architecture\n[06/07/2026, 14:34] Me: Task: write usage examples","source":"whatsapp","splitMode":"whatsapp","contextTitle":"ai-memory planning"}`
	req := httptest.NewRequest(http.MethodPost, "/api/inbox/batch/analyze", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("analyze returned %d: %s", rr.Code, rr.Body.String())
	}
	var analysis map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &analysis); err != nil {
		t.Fatalf("unmarshal analysis: %v", err)
	}
	if analysis["source"] != "whatsapp" || analysis["separator"] != "whatsapp" {
		t.Fatalf("unexpected analysis: %#v", analysis)
	}
	if analysis["enrichmentStatus"] != "unavailable" && analysis["enrichmentStatus"] != "failed" {
		t.Fatalf("expected unavailable or failed enrichment, got %v", analysis["enrichmentStatus"])
	}
}

func TestInboxBatchCreateWithLLMGrouping(t *testing.T) {
	ctx := context.Background()
	g, err := graph.Open(ctx, graph.Config{Path: filepath.Join(t.TempDir(), "graph.db")})
	if err != nil {
		t.Fatalf("graph.Open: %v", err)
	}
	defer g.Close()

	a := &app.App{
		Graph: g,
		Config: &config.Config{
			Vault: config.VaultConfig{Root: t.TempDir()},
			LLM: config.LLMConfig{
				Provider: "noop",
			},
		},
	}
	mux := http.NewServeMux()
	api.RegisterInboxRoutes(mux, a)

	body := `{"text":"Build an ai-memory explainer project. Task: map the repo architecture. Task: write usage examples.","source":"text","splitMode":"semantic","contextTitle":"ai-memory planning"}`
	req := httptest.NewRequest(http.MethodPost, "/api/inbox/batch", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create returned %d: %s", rr.Code, rr.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create: %v", err)
	}
	if created["batchId"] == "" {
		t.Fatalf("missing batchId")
	}
	// The noop provider returns deterministic fallback; without a real mock
	// response we assert the deterministic path still creates one capture.
	if len(created["ids"].([]any)) != 1 {
		t.Fatalf("expected 1 fallback capture, got %v", created["ids"])
	}
	if created["rawSegmentCount"] != float64(1) {
		t.Fatalf("expected rawSegmentCount=1, got %v", created["rawSegmentCount"])
	}
	if created["enrichmentStatus"] != "unavailable" && created["enrichmentStatus"] != "failed" {
		t.Fatalf("expected unavailable or failed enrichment, got %v", created["enrichmentStatus"])
	}
}

// TestInboxAutoClassifyGetPut covers the live switch round-trip: GET reflects the
// current state (plus whether an LLM is configured), PUT flips it, and a
// subsequent GET sees the new value.
func TestInboxAutoClassifyGetPut(t *testing.T) {
	ctx := context.Background()
	g, err := graph.Open(ctx, graph.Config{Path: filepath.Join(t.TempDir(), "graph.db")})
	if err != nil {
		t.Fatalf("graph.Open: %v", err)
	}
	defer g.Close()

	a := &app.App{Graph: g, Config: &config.Config{
		Vault: config.VaultConfig{Root: t.TempDir()},
		LLM:   config.LLMConfig{Provider: "noop"},
	}}
	a.SetAutoClassify(true)

	mux := http.NewServeMux()
	api.RegisterInboxRoutes(mux, a)

	// GET reflects the seeded ON state and reports the LLM as configured.
	get := func() map[string]bool {
		req := httptest.NewRequest(http.MethodGet, "/api/inbox/auto-classify", nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("GET auto-classify returned %d: %s", rr.Code, rr.Body.String())
		}
		var out map[string]bool
		if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		return out
	}
	if state := get(); !state["enabled"] || !state["llmConfigured"] {
		t.Fatalf("expected enabled=true llmConfigured=true, got %#v", state)
	}

	// PUT flips it off and echoes the new state.
	req := httptest.NewRequest(http.MethodPut, "/api/inbox/auto-classify", bytes.NewBufferString(`{"enabled":false}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT auto-classify returned %d: %s", rr.Code, rr.Body.String())
	}
	var put map[string]bool
	if err := json.Unmarshal(rr.Body.Bytes(), &put); err != nil {
		t.Fatalf("unmarshal put: %v", err)
	}
	if put["enabled"] {
		t.Fatalf("PUT did not disable the switch: %#v", put)
	}
	if a.AutoClassifyEnabled() {
		t.Fatalf("app flag still enabled after PUT")
	}
	if state := get(); state["enabled"] {
		t.Fatalf("GET after PUT still reports enabled: %#v", state)
	}
}

// TestInboxClassifyNoLLM asserts the on-demand pass fails loud (not a silent
// no-op) when no LLM is configured.
func TestInboxClassifyNoLLM(t *testing.T) {
	ctx := context.Background()
	g, err := graph.Open(ctx, graph.Config{Path: filepath.Join(t.TempDir(), "graph.db")})
	if err != nil {
		t.Fatalf("graph.Open: %v", err)
	}
	defer g.Close()

	a := &app.App{Graph: g, Config: &config.Config{Vault: config.VaultConfig{Root: t.TempDir()}}}
	mux := http.NewServeMux()
	api.RegisterInboxRoutes(mux, a)

	req := httptest.NewRequest(http.MethodPost, "/api/inbox/classify", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("classify without LLM returned %d, want 503: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "KERNL DISPATCH FAILURE") {
		t.Fatalf("expected fail-loud marker, got %s", rr.Body.String())
	}
}

// TestInboxClassifyRunsPass drives one on-demand pass with the noop provider and
// asserts the previously-unclassified capture comes back with a suggestion.
func TestInboxClassifyRunsPass(t *testing.T) {
	ctx := context.Background()
	g, err := graph.Open(ctx, graph.Config{Path: filepath.Join(t.TempDir(), "graph.db")})
	if err != nil {
		t.Fatalf("graph.Open: %v", err)
	}
	defer g.Close()

	var capID string
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		id, err := nodes.CreateCapture(ctx, tx, nodes.Capture{Body: "buy milk", Tags: []string{"pending"}}, nodes.Author{Name: "tester"})
		capID = id
		return err
	}); err != nil {
		t.Fatalf("CreateCapture: %v", err)
	}

	a := &app.App{Graph: g, Config: &config.Config{
		Vault: config.VaultConfig{Root: t.TempDir()},
		LLM:   config.LLMConfig{Provider: "noop"},
	}}
	mux := http.NewServeMux()
	api.RegisterInboxRoutes(mux, a)

	req := httptest.NewRequest(http.MethodPost, "/api/inbox/classify", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("classify returned %d: %s", rr.Code, rr.Body.String())
	}

	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		cap, err := nodes.GetCapture(ctx, tx, capID)
		if err != nil {
			return err
		}
		if len(cap.SuggestedActions) == 0 {
			t.Fatalf("capture still unclassified after classify pass")
		}
		return nil
	}); err != nil {
		t.Fatalf("DoRead: %v", err)
	}
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
