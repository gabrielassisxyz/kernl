package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/chat"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
	"github.com/gabrielassisxyz/kernl/internal/ingest"
)

// queueLen polls the ingest queue until it reaches want or the deadline passes;
// paste/upload process in a detached goroutine, so the review appears async.
func waitForQueue(t *testing.T, mux http.Handler, want int) int {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var last int
	for time.Now().Before(deadline) {
		req := httptest.NewRequest("GET", "/api/ingest/queue", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		var items []map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &items)
		last = len(items)
		if last >= want {
			return last
		}
		time.Sleep(30 * time.Millisecond)
	}
	return last
}

func newIngestApp(t *testing.T) (*app.App, http.Handler) {
	t.Helper()
	a := &app.App{
		Graph: testutil.NewInMemoryTestGraph(t),
		Config: &config.Config{
			Vault: config.VaultConfig{Root: t.TempDir()},
			LLM:   config.LLMConfig{Provider: "test"},
		},
	}
	mux := http.NewServeMux()
	RegisterIngestRoutes(mux, a)
	return a, mux
}

type fakeIngestLLM struct {
	content string
}

func (f fakeIngestLLM) Chat(ctx context.Context, messages []chat.Message, tools []chat.Tool) (*chat.ChatResponse, error) {
	return &chat.ChatResponse{Content: f.content}, nil
}

func withFakeIngestLLM(t *testing.T, content string) {
	t.Helper()
	old := newIngestLLM
	newIngestLLM = func(cfg chat.LLMProviderConfig) (chat.LLMClient, error) {
		if cfg.Provider == "" {
			return nil, errors.New("missing provider")
		}
		return fakeIngestLLM{content: content}, nil
	}
	t.Cleanup(func() { newIngestLLM = old })
}

func TestIngestPasteCreatesReview(t *testing.T) {
	withFakeIngestLLM(t, `[{"type":"Create Page","title":"Meeting decision","payload":"Decided to prioritize X."}]`)
	_, mux := newIngestApp(t)

	body, _ := json.Marshal(map[string]string{"text": "Meeting notes: decided to prioritize X."})
	req := httptest.NewRequest("POST", "/api/ingest/paste", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
	if n := waitForQueue(t, mux, 1); n < 1 {
		t.Errorf("expected the pasted content to produce a review item, queue=%d", n)
	}
}

func TestIngestPasteRejectsEmpty(t *testing.T) {
	withFakeIngestLLM(t, `[]`)
	_, mux := newIngestApp(t)

	body, _ := json.Marshal(map[string]string{"text": "   "})
	req := httptest.NewRequest("POST", "/api/ingest/paste", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty paste, got %d", w.Code)
	}
}

func TestIngestUploadCreatesReview(t *testing.T) {
	withFakeIngestLLM(t, `[{"type":"Create Page","title":"Room measurement","payload":"Detail measurement by room."}]`)
	_, mux := newIngestApp(t)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "notes.md")
	_, _ = fw.Write([]byte("# Reuniao\n\nDecided to detail measurement by room.\n"))
	mw.Close()

	req := httptest.NewRequest("POST", "/api/ingest/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
	if n := waitForQueue(t, mux, 1); n < 1 {
		t.Errorf("expected the uploaded file to produce a review item, queue=%d", n)
	}
}

func TestIngestDisabledWithoutLLM(t *testing.T) {
	a := &app.App{
		Graph:  testutil.NewInMemoryTestGraph(t),
		Config: &config.Config{Vault: config.VaultConfig{Root: t.TempDir()}},
	}
	mux := http.NewServeMux()
	RegisterIngestRoutes(mux, a)

	body, _ := json.Marshal(map[string]string{"text": "Meeting notes"})
	req := httptest.NewRequest("POST", "/api/ingest/paste", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for paste without LLM, got %d: %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest("GET", "/api/ingest/queue", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("queue should remain readable without LLM, got %d", w.Code)
	}
}

func TestIngestSourceCreatesLinkedReview(t *testing.T) {
	withFakeIngestLLM(t, `[{"type":"Create Page","title":"Repo overview","payload":"The repo documents ai-memory routing."}]`)
	oldFetcher := defaultIngestSourceFetcher
	defaultIngestSourceFetcher = ingest.StaticSourceFetcher{Document: ingest.SourceDocument{
		Kind:    ingest.SourceKindGitHub,
		URL:     "https://github.com/example/ai-memory",
		Title:   "example/ai-memory",
		Content: "ai-memory stores durable project knowledge.",
	}}
	t.Cleanup(func() { defaultIngestSourceFetcher = oldFetcher })

	a, mux := newIngestApp(t)
	body, _ := json.Marshal(map[string]string{"url": "https://github.com/example/ai-memory", "kind": "github_repo"})
	req := httptest.NewRequest("POST", "/api/ingest/source", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
	var response map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	sourceID := response["sourceNodeId"]
	if sourceID == "" {
		t.Fatalf("expected sourceNodeId in response")
	}
	if n := waitForQueue(t, mux, 1); n < 1 {
		t.Fatalf("expected source ingest to produce a review item, queue=%d", n)
	}
	var reviews []*nodes.IngestReview
	if err := a.Graph.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		var err error
		reviews, err = nodes.ListIngestReviews(context.Background(), tx, nodes.IngestReviewFilter{})
		return err
	}); err != nil {
		t.Fatalf("ListIngestReviews: %v", err)
	}
	if len(reviews) != 1 {
		t.Fatalf("expected 1 review, got %d", len(reviews))
	}
	if reviews[0].SourceNodeID != sourceID {
		t.Fatalf("review SourceNodeID = %q, want %q", reviews[0].SourceNodeID, sourceID)
	}
}

// TestIngestTriggerUsesFilePathAndNodeID is the load-bearing check on the
// filePath/nodeId wire contract: encoding/json leaves an unmatched field at
// its zero value with no error, so a body decoded against the wrong tags
// still returns 202 and processes nothing, in silence. Posting a real file
// and asserting the review that comes out carries the posted node id is the
// only way to prove the server actually read filePath and nodeId, rather
// than just accepting the request.
func TestIngestTriggerUsesFilePathAndNodeID(t *testing.T) {
	withFakeIngestLLM(t, `[{"type":"Create Page","title":"Trigger note","payload":"triggered content"}]`)
	a, mux := newIngestApp(t)

	testFile := filepath.Join(t.TempDir(), "trigger.md")
	if err := os.WriteFile(testFile, []byte("triggered content"), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"filePath": testFile, "nodeId": "trigger-node-1"})
	req := httptest.NewRequest("POST", "/api/ingest/trigger", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}

	if n := waitForQueue(t, mux, 1); n < 1 {
		t.Fatalf("expected the triggered file to produce a review item, queue=%d", n)
	}
	var reviews []*nodes.IngestReview
	if err := a.Graph.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		var err error
		reviews, err = nodes.ListIngestReviews(context.Background(), tx, nodes.IngestReviewFilter{})
		return err
	}); err != nil {
		t.Fatalf("ListIngestReviews: %v", err)
	}
	if len(reviews) != 1 {
		t.Fatalf("expected 1 review, got %d", len(reviews))
	}
	if reviews[0].SourceNodeID != "trigger-node-1" {
		t.Fatalf("review SourceNodeID = %q, want %q (nodeId did not reach the handler)", reviews[0].SourceNodeID, "trigger-node-1")
	}
}

func TestIngestUploadRejectsNonText(t *testing.T) {
	withFakeIngestLLM(t, `[]`)
	_, mux := newIngestApp(t)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "image.png")
	_, _ = fw.Write([]byte{0x89, 0x50, 0x4e, 0x47})
	mw.Close()

	req := httptest.NewRequest("POST", "/api/ingest/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnsupportedMediaType {
		t.Errorf("expected 415 for a .png upload, got %d", w.Code)
	}
}
