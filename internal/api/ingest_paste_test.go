package api_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/api"
	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
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
		Graph:  testutil.NewInMemoryTestGraph(t),
		Config: &config.Config{Vault: config.VaultConfig{Root: t.TempDir()}},
	}
	mux := http.NewServeMux()
	api.RegisterIngestRoutes(mux, a)
	return a, mux
}

func TestIngestPasteCreatesReview(t *testing.T) {
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

func TestIngestUploadRejectsNonText(t *testing.T) {
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
