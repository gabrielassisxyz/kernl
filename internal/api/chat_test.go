package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

// flushableRecorder wraps httptest.ResponseRecorder with Flush() for SSE tests.
type flushableRecorder struct {
	*httptest.ResponseRecorder
}

func (f *flushableRecorder) Flush() {}

func TestCreateChatSession(t *testing.T) {
	a := newTestAppWithGraphWithLLM(t)
	r := NewRouter(a)

	req := httptest.NewRequest("POST", "/api/chat/sessions", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var res struct {
		ID        string `json:"id"`
		CreatedAt string `json:"createdAt"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if res.ID == "" {
		t.Fatal("expected id")
	}
}

func TestGetChatSessionNotFound(t *testing.T) {
	a := newTestAppWithGraphWithLLM(t)
	r := NewRouter(a)

	req := httptest.NewRequest("GET", "/api/chat/sessions/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestPostMessageAndGetSession(t *testing.T) {
	a := newTestAppWithGraphWithLLM(t)
	r := NewRouter(a)

	// Create session.
	createReq := httptest.NewRequest("POST", "/api/chat/sessions", nil)
	createW := httptest.NewRecorder()
	r.ServeHTTP(createW, createReq)
	var createRes struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(createW.Body.Bytes(), &createRes)

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
	_ = json.Unmarshal(getW.Body.Bytes(), &cs)
	if len(cs.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(cs.Messages))
	}
	if cs.Messages[0].Content != "hello" {
		t.Errorf("content = %q, want hello", cs.Messages[0].Content)
	}
}

func TestChatEventsSSE(t *testing.T) {
	a := newTestAppWithGraphWithLLM(t)

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
	_ = json.Unmarshal(createW.Body.Bytes(), &createRes)

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

func TestChatEventsSSE_SendsCorrectEventSequence(t *testing.T) {
	a := newTestAppWithGraphWithLLM(t)

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
	createW := httptest.NewRecorder()
	r.ServeHTTP(createW, createReq)
	var createRes struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(createW.Body.Bytes(), &createRes)

	msgBody := `{"content":"hello","scope_node_id":""}`
	msgReq := httptest.NewRequest("POST", "/api/chat/sessions/"+createRes.ID+"/messages", strings.NewReader(msgBody))
	msgReq.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(httptest.NewRecorder(), msgReq)

	eventsReq := httptest.NewRequest("GET", "/api/chat/sessions/"+createRes.ID+"/events", nil)
	eventsReq.Header.Set("Accept", "text/event-stream")
	eventsW := &flushableRecorder{ResponseRecorder: httptest.NewRecorder()}
	r.ServeHTTP(eventsW, eventsReq)

	if eventsW.Code != 200 {
		t.Fatalf("expected 200, got %d", eventsW.Code)
	}
	body := eventsW.Body.String()
	// Order: state first, then tokens, then done.
	stateIdx := strings.Index(body, `"event":"state"`)
	tokenIdx := strings.Index(body, `"event":"token"`)
	doneIdx := strings.Index(body, `"event":"done"`)
	if stateIdx == -1 {
		t.Error("expected state event")
	}
	if tokenIdx == -1 {
		t.Error("expected token event")
	}
	if doneIdx == -1 {
		t.Error("expected done event")
	}
	if stateIdx > tokenIdx || tokenIdx > doneIdx {
		t.Errorf("events out of order: state=%d token=%d done=%d", stateIdx, tokenIdx, doneIdx)
	}
}

func TestChatEventsSSE_NonexistentSession(t *testing.T) {
	a := newTestAppWithGraphWithLLM(t)
	r := NewRouter(a)

	eventsReq := httptest.NewRequest("GET", "/api/chat/sessions/nonexistent/events", nil)
	eventsReq.Header.Set("Accept", "text/event-stream")
	eventsW := httptest.NewRecorder()
	r.ServeHTTP(eventsW, eventsReq)

	if eventsW.Code != 404 {
		t.Fatalf("expected 404, got %d: %s", eventsW.Code, eventsW.Body.String())
	}
}

func TestListNodes(t *testing.T) {
	a := newTestAppWithGraphWithLLM(t)
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
	_ = json.Unmarshal(w.Body.Bytes(), &list)
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

// postLearned drives the learned-candidate endpoint and returns the recorder.
func postLearned(t *testing.T, r http.Handler, sessionID, payload string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/chat/sessions/"+sessionID+"/learned", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func countMemoryClaims(t *testing.T, a *app.App) int {
	t.Helper()
	var n int
	_ = a.Graph.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT COUNT(*) FROM nodes WHERE type = 'memory_claim' AND deleted_at IS NULL`).Scan(&n)
	})
	return n
}

func TestLearnedKeepPersistsClaimWithSourceEdge(t *testing.T) {
	a := newTestAppWithGraphWithLLM(t)
	r := NewRouter(a)

	sessionID := createTestSession(t, r)

	w := postLearned(t, r, sessionID, `{"action":"keep","subject":"tools","statement":"Prefers Google Meet over Zoom."}`)
	if w.Code != 200 {
		t.Fatalf("keep: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var res struct {
		Status string `json:"status"`
		ID     string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if res.Status != "kept" || res.ID == "" {
		t.Fatalf("unexpected response: %+v", res)
	}

	// The claim exists with the kept statement and a `source` edge to the session.
	err := a.Graph.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		claim, err := nodes.GetMemoryClaim(context.Background(), tx, res.ID)
		if err != nil {
			return err
		}
		if claim.Statement != "Prefers Google Meet over Zoom." {
			t.Errorf("statement = %q, want the kept text", claim.Statement)
		}
		out, err := edges.Outgoing(context.Background(), tx, res.ID)
		if err != nil {
			return err
		}
		if len(out) != 1 || out[0].Dst != sessionID || out[0].Label != "source" {
			t.Errorf("expected one source edge to the session, got %+v", out)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify claim: %v", err)
	}
}

func TestLearnedEditPersistsModifiedStatement(t *testing.T) {
	a := newTestAppWithGraphWithLLM(t)
	r := NewRouter(a)
	sessionID := createTestSession(t, r)

	// Edit is Keep with a corrected statement — the modified text must win.
	w := postLearned(t, r, sessionID, `{"action":"keep","subject":"tools","statement":"Prefers Google Meet for all video calls."}`)
	if w.Code != 200 {
		t.Fatalf("keep(edited): expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var res struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &res)

	err := a.Graph.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		claim, err := nodes.GetMemoryClaim(context.Background(), tx, res.ID)
		if err != nil {
			return err
		}
		if claim.Statement != "Prefers Google Meet for all video calls." {
			t.Errorf("statement = %q, want the edited text", claim.Statement)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify edited claim: %v", err)
	}
}

func TestLearnedDiscardPersistsNoClaimAndSuppresses(t *testing.T) {
	a := newTestAppWithGraphWithLLM(t)
	r := NewRouter(a)
	sessionID := createTestSession(t, r)

	before := countMemoryClaims(t, a)

	w := postLearned(t, r, sessionID, `{"action":"discard","statement":"Prefers Google Meet over Zoom."}`)
	if w.Code != 200 {
		t.Fatalf("discard: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if after := countMemoryClaims(t, a); after != before {
		t.Errorf("discard created a claim: before=%d after=%d", before, after)
	}

	// The discard is recorded on the session so the candidate isn't re-proposed.
	var cs *nodes.ChatSession
	if err := a.Graph.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		var err error
		cs, err = nodes.GetChatSession(context.Background(), tx, sessionID)
		return err
	}); err != nil {
		t.Fatalf("get session: %v", err)
	}
	if len(cs.DiscardedCandidates) != 1 || cs.DiscardedCandidates[0] != "Prefers Google Meet over Zoom." {
		t.Errorf("DiscardedCandidates = %v, want the discarded statement recorded", cs.DiscardedCandidates)
	}
}

func TestLearnedRejectsBadInput(t *testing.T) {
	a := newTestAppWithGraphWithLLM(t)
	r := NewRouter(a)
	sessionID := createTestSession(t, r)

	if w := postLearned(t, r, sessionID, `{"action":"keep","statement":""}`); w.Code != 400 {
		t.Errorf("empty keep statement: expected 400, got %d", w.Code)
	}
	if w := postLearned(t, r, sessionID, `{"action":"bogus"}`); w.Code != 400 {
		t.Errorf("unknown action: expected 400, got %d", w.Code)
	}
	if w := postLearned(t, r, "nonexistent", `{"action":"discard","statement":"x"}`); w.Code != 404 {
		t.Errorf("missing session: expected 404, got %d", w.Code)
	}
}

// createTestSession creates a chat session via the API and returns its id.
func createTestSession(t *testing.T, r http.Handler) string {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/chat/sessions", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var res struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if res.ID == "" {
		t.Fatal("create session: empty id")
	}
	return res.ID
}

func testCfgWithLLM() *config.Config {
	cfg := testCfg()
	cfg.LLM = config.LLMConfig{
		Provider: "noop",
		Model:    "test-model",
	}
	return cfg
}

func newTestAppWithGraphWithLLM(t *testing.T) *app.App {
	t.Helper()
	g := testutil.NewInMemoryTestGraph(t)
	return &app.App{
		Graph:  g,
		Config: testCfgWithLLM(),
	}
}
