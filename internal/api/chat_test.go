package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

// flushableRecorder wraps httptest.ResponseRecorder with Flush() for SSE tests.
type flushableRecorder struct {
	*httptest.ResponseRecorder
}

func (f *flushableRecorder) Flush() {}

func TestCreateChatSession(t *testing.T) {
	a := newTestAppWithGraph(t)
	r := NewRouter(a)

	req := httptest.NewRequest("POST", "/api/chat/sessions", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var res struct {
		ID        string `json:"id"`
		CreatedAt string `json:"created_at"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if res.ID == "" {
		t.Fatal("expected id")
	}
}

func TestGetChatSessionNotFound(t *testing.T) {
	a := newTestAppWithGraph(t)
	r := NewRouter(a)

	req := httptest.NewRequest("GET", "/api/chat/sessions/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestPostMessageAndGetSession(t *testing.T) {
	a := newTestAppWithGraph(t)
	r := NewRouter(a)

	// Create session.
	createReq := httptest.NewRequest("POST", "/api/chat/sessions", nil)
	createW := httptest.NewRecorder()
	r.ServeHTTP(createW, createReq)
	var createRes struct {
		ID string `json:"id"`
	}
	json.Unmarshal(createW.Body.Bytes(), &createRes)

	// Post message.
	body := `{"content":"hello","scope_node_id":""}`
	msgReq := httptest.NewRequest("POST", "/api/chat/sessions/"+createRes.ID+"/messages", strings.NewReader(body))
	msgReq.Header.Set("Content-Type", "application/json")
	msgW := httptest.NewRecorder()
	r.ServeHTTP(msgW, msgReq)
	if msgW.Code != 202 {
		t.Fatalf("expected 202, got %d: %s", msgW.Code, msgW.Body.String())
	}

	// Get session.
	getReq := httptest.NewRequest("GET", "/api/chat/sessions/"+createRes.ID, nil)
	getW := httptest.NewRecorder()
	r.ServeHTTP(getW, getReq)
	if getW.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", getW.Code, getW.Body.String())
	}
	var cs nodes.ChatSession
	json.Unmarshal(getW.Body.Bytes(), &cs)
	if len(cs.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(cs.Messages))
	}
	if cs.Messages[0].Content != "hello" {
		t.Errorf("content = %q, want hello", cs.Messages[0].Content)
	}
}

func TestChatEventsSSE(t *testing.T) {
	a := newTestAppWithGraph(t)

	// Seed DA identity so engine doesn't error.
	ctx := context.Background()
	_ = a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := nodes.CreateDAIdentity(ctx, tx, &nodes.DAIdentity{
			SystemPrompt: "You are a test assistant.",
			DisplayName:  "Test Assistant",
		}, nodes.Author{Name: "kernl"})
		return err
	})

	r := NewRouter(a)

	createReq := httptest.NewRequest("POST", "/api/chat/sessions", nil)
	createW := &flushableRecorder{ResponseRecorder: httptest.NewRecorder()}
	r.ServeHTTP(createW, createReq)
	var createRes struct {
		ID string `json:"id"`
	}
	json.Unmarshal(createW.Body.Bytes(), &createRes)

	msgBody := `{"content":"hello","scope_node_id":""}`
	msgReq := httptest.NewRequest("POST", "/api/chat/sessions/"+createRes.ID+"/messages", strings.NewReader(msgBody))
	msgReq.Header.Set("Content-Type", "application/json")
	msgW := httptest.NewRecorder()
	r.ServeHTTP(msgW, msgReq)

	// SSE request needs a writer that supports Flush.
	eventsReq := httptest.NewRequest("GET", "/api/chat/sessions/"+createRes.ID+"/events", nil)
	eventsReq.Header.Set("Accept", "text/event-stream")
	eventsW := &flushableRecorder{ResponseRecorder: httptest.NewRecorder()}
	r.ServeHTTP(eventsW, eventsReq)

	if eventsW.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", eventsW.Code, eventsW.Body.String())
	}
	ct := eventsW.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("expected text/event-stream, got %q", ct)
	}
	body := eventsW.Body.String()
	if !strings.Contains(body, "state") {
		t.Errorf("expected state event, got body: %s", body)
	}
}

func TestListNodes(t *testing.T) {
	a := newTestAppWithGraph(t)
	// Seed a note
	ctx := context.Background()
	_ = a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := nodes.CreateNote(ctx, tx, nodes.Note{Title: "Test Note", Body: "Body"}, nodes.Author{Name: "test"})
		return err
	})

	r := NewRouter(a)
	req := httptest.NewRequest("GET", "/api/nodes", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var list []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
		Type  string `json:"type"`
	}
	json.Unmarshal(w.Body.Bytes(), &list)
	if len(list) == 0 {
		t.Fatal("expected at least one node")
	}
	found := false
	for _, n := range list {
		if n.Type == "note" && n.Title == "Test Note" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected Test Note in list, got %+v", list)
	}
}
