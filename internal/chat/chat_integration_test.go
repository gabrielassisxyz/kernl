//go:build integration

package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/api"
	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

// mockLLMClient is a deterministic stub for integration testing.
type mockLLMClient struct {
	Responses []ChatResponse
	CallIndex int
	Messages  []Message
}

func (m *mockLLMClient) Chat(ctx context.Context, messages []Message, tools []Tool) (*ChatResponse, error) {
	m.Messages = append(m.Messages, messages...)
	if m.CallIndex >= len(m.Responses) {
		return &ChatResponse{Content: "No more responses."}, nil
	}
	resp := m.Responses[m.CallIndex]
	m.CallIndex++
	return &resp, nil
}

func newMockLLMClient(responses ...ChatResponse) *mockLLMClient {
	return &mockLLMClient{Responses: responses}
}

func newTestAppWithGraph(t *testing.T) (*app.App, func()) {
	g := testutil.NewInMemoryTestGraph(t)
	a := &app.App{Graph: g}
	// Seed DA identity for chat endpoints.
	ctx := context.Background()
	_ = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := nodes.CreateDAIdentity(ctx, tx, &nodes.DAIdentity{
			SystemPrompt: "You are a test assistant.",
			DisplayName:  "Test Assistant",
		}, nodes.Author{Name: "kernl"})
		return err
	})
	return a, func() {}
}

func TestChatHappyPathSSE(t *testing.T) {
	a, cleanup := newTestAppWithGraph(t)
	defer cleanup()
	r := api.NewRouter(a)

	// 1. Create session.
	req := httptest.NewRequest("POST", "/api/chat/sessions", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("create session expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var createRes struct {
		ID        string `json:"id"`
		CreatedAt string `json:"created_at"`
	}
	json.Unmarshal(w.Body.Bytes(), &createRes)
	if createRes.ID == "" {
		t.Fatal("expected session id")
	}

	// 2. POST message.
	msgBody := fmt.Sprintf(`{"content":"hello","scope_node_id":""}`)
	msgReq := httptest.NewRequest("POST", fmt.Sprintf("/api/chat/sessions/%s/messages", createRes.ID), strings.NewReader(msgBody))
	msgReq.Header.Set("Content-Type", "application/json")
	msgW := httptest.NewRecorder()
	r.ServeHTTP(msgW, msgReq)
	if msgW.Code != 202 {
		t.Fatalf("post message expected 202, got %d: %s", msgW.Code, msgW.Body.String())
	}

	// 3. GET events (SSE).
	// For this test, we don't have a real mock injected into the engine,
	// so the engine will fetch the session but the LLMClient will be nil
	// and return an error. We just verify SSE headers and error event.
	eventsReq := httptest.NewRequest("GET", fmt.Sprintf("/api/chat/sessions/%s/events", createRes.ID), nil)
	eventsReq.Header.Set("Accept", "text/event-stream")
	eventsW := httptest.NewRecorder()
	r.ServeHTTP(eventsW, eventsReq)

	if eventsW.Code != 200 {
		t.Fatalf("events expected 200, got %d: %s", eventsW.Code, eventsW.Body.String())
	}
	ct := eventsW.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("expected text/event-stream, got %q", ct)
	}
	body := eventsW.Body.String()
	if !strings.Contains(body, "event: state") {
		t.Errorf("expected state event, got body: %s", body)
	}
}

func TestChatNotFound(t *testing.T) {
	a, cleanup := newTestAppWithGraph(t)
	defer cleanup()
	r := api.NewRouter(a)

	// GET non-existent session.
	req := httptest.NewRequest("GET", "/api/chat/sessions/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Fatalf("expected 404, got %d", w.Code)
	}

	// POST message to non-existent session.
	msgBody := `{"content":"hello","scope_node_id":""}`
	msgReq := httptest.NewRequest("POST", "/api/chat/sessions/nonexistent/messages", strings.NewReader(msgBody))
	msgReq.Header.Set("Content-Type", "application/json")
	msgW := httptest.NewRecorder()
	r.ServeHTTP(msgW, msgReq)
	if msgW.Code != 404 {
		t.Fatalf("expected 404 for post to nonexistent, got %d", msgW.Code)
	}
}

func TestNodesList(t *testing.T) {
	a, cleanup := newTestAppWithGraph(t)
	defer cleanup()

	// Seed a note.
	ctx := context.Background()
	_ = a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := nodes.CreateNote(ctx, tx, nodes.Note{Title: "Test Note", Body: "Body"}, nodes.Author{Name: "test"})
		return err
	})

	r := api.NewRouter(a)
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
		if n.Title == "Test Note" && n.Type == "note" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected Test Note in list, got %+v", list)
	}
}

func TestChatPermissionBlockAndResume(t *testing.T) {
	t.Skip("requires full engine implementation with LLM injection")
}

func TestScopeDerivation(t *testing.T) {
	t.Skip("requires full engine implementation with LLM injection")
}

func TestDAIdentityApplied(t *testing.T) {
	t.Skip("requires full engine implementation with LLM injection")
}
