//go:build integration

package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

// ── Helpers ────────────────────────────────────────────────────────────

// eventRecorder implements ChatEventWriter and captures SSE data for assertions.
type eventRecorder struct {
	buf bytes.Buffer
}

func (e *eventRecorder) Write(p []byte) (int, error) { return e.buf.Write(p) }
func (e *eventRecorder) Flush()                      {}

func (e *eventRecorder) events() []map[string]any {
	raw := e.buf.String()
	var out []map[string]any
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var evt map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &evt); err == nil {
			out = append(out, evt)
		}
	}
	return out
}

func (e *eventRecorder) hasEventType(typ string) bool {
	for _, evt := range e.events() {
		if evt["event"] == typ {
			return true
		}
	}
	return false
}

// alwaysAllow is a PermissionChecker that permits everything.
type alwaysAllow struct{}

func (alwaysAllow) CanRead(_ context.Context, _ string) (bool, DenialReason, error) {
	return true, "", nil
}

// alwaysDeny denies everything; used in deny-path scenarios.
type alwaysDeny struct{}

func (alwaysDeny) CanRead(_ context.Context, _ string) (bool, DenialReason, error) {
	return false, "denied by test checker", nil
}

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

func newTestApp(t *testing.T) *app.App {
	t.Helper()
	g := testutil.NewInMemoryTestGraph(t)
	return &app.App{
		Graph: g,
		Config: &config.Config{
			Vault: config.VaultConfig{Root: t.TempDir()},
		},
	}
}

func seedDAIdentity(t *testing.T, a *app.App) {
	t.Helper()
	ctx := context.Background()
	_ = a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := nodes.CreateDAIdentity(ctx, tx, &nodes.DAIdentity{
			SystemPrompt: "You are a test assistant.",
			DisplayName:  "Test Assistant",
		}, nodes.Author{Name: "kernl"})
		return err
	})
}

func seedNote(t *testing.T, a *app.App, title, body string, tags []string) string {
	t.Helper()
	ctx := context.Background()
	var id string
	err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = nodes.CreateNote(ctx, tx, nodes.Note{Title: title, Body: body, Tags: tags}, nodes.Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	return id
}

func createSession(t *testing.T, a *app.App) string {
	t.Helper()
	ctx := context.Background()
	var id string
	err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = nodes.CreateChatSession(ctx, tx, &nodes.ChatSession{
			Messages: []nodes.ChatMessage{},
		}, nodes.Author{Name: "kernl"})
		return err
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return id
}

func appendUserMessage(t *testing.T, a *app.App, sessionID, content, scopeNodeID string) {
	t.Helper()
	ctx := context.Background()
	var cs *nodes.ChatSession
	err := a.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		cs, err = nodes.GetChatSession(ctx, tx, sessionID)
		return err
	})
	if err != nil {
		t.Fatalf("get session for append: %v", err)
	}

	cs.Messages = append(cs.Messages, nodes.ChatMessage{Role: "user", Content: content})
	cs.DerivedScopeNodeID = scopeNodeID

	err = a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return nodes.SaveChatSession(ctx, tx, cs, nodes.Author{Name: "kernl"})
	})
	if err != nil {
		t.Fatalf("save session after append: %v", err)
	}
}

func getSession(t *testing.T, a *app.App, sessionID string) *nodes.ChatSession {
	t.Helper()
	ctx := context.Background()
	var cs *nodes.ChatSession
	err := a.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		cs, err = nodes.GetChatSession(ctx, tx, sessionID)
		return err
	})
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	return cs
}

// ── Tests ──────────────────────────────────────────────────────────────

func TestChatHappyPathSSE(t *testing.T) {
	a := newTestApp(t)
	seedDAIdentity(t, a)
	sessionID := createSession(t, a)
	appendUserMessage(t, a, sessionID, "hello", "")

	mock := newMockLLMClient(ChatResponse{Content: "Hi there!"})
	rec := &eventRecorder{}
	engine, err := NewChatEngine(a, sessionID, rec, mock, alwaysAllow{})
	if err != nil {
		t.Fatalf("NewChatEngine: %v", err)
	}

	if err := engine.RunSession(context.Background()); err != nil {
		t.Fatalf("RunSession: %v", err)
	}

	if !rec.hasEventType("state") {
		t.Error("expected state event")
	}
	if !rec.hasEventType("token") {
		t.Error("expected token event")
	}
	if !rec.hasEventType("done") {
		t.Error("expected done event")
	}
}

func TestChatTelosInjected(t *testing.T) {
	a := newTestApp(t)
	seedDAIdentity(t, a)

	// A telos-tagged note is always-on context; an untagged note is not.
	seedNote(t, a, "My Telos", "I value leverage and shipping the loop.", []string{"telos"})
	seedNote(t, a, "Random", "unrelated body", nil)

	sessionID := createSession(t, a)
	appendUserMessage(t, a, sessionID, "what should I focus on?", "")

	mock := newMockLLMClient(ChatResponse{Content: "Focus on the loop."})
	rec := &eventRecorder{}
	engine, err := NewChatEngine(a, sessionID, rec, mock, alwaysAllow{})
	if err != nil {
		t.Fatalf("NewChatEngine: %v", err)
	}
	if err := engine.RunSession(context.Background()); err != nil {
		t.Fatalf("RunSession: %v", err)
	}

	// The DA system prompt comes first, then the always-on Telos block, then the
	// conversation. Assert the Telos content reached the model.
	var systemBlob string
	for _, m := range mock.Messages {
		if m.Role == "system" {
			systemBlob += m.Content + "\n"
		}
	}
	if !strings.Contains(systemBlob, "I value leverage and shipping the loop.") {
		t.Errorf("expected Telos content in system context, got system messages:\n%s", systemBlob)
	}
	if strings.Contains(systemBlob, "unrelated body") {
		t.Errorf("untagged note leaked into context:\n%s", systemBlob)
	}
}

func TestChatNoTelosNoExtraSystemMessage(t *testing.T) {
	a := newTestApp(t)
	seedDAIdentity(t, a)
	sessionID := createSession(t, a)
	appendUserMessage(t, a, sessionID, "hello", "")

	mock := newMockLLMClient(ChatResponse{Content: "Hi!"})
	rec := &eventRecorder{}
	engine, err := NewChatEngine(a, sessionID, rec, mock, alwaysAllow{})
	if err != nil {
		t.Fatalf("NewChatEngine: %v", err)
	}
	if err := engine.RunSession(context.Background()); err != nil {
		t.Fatalf("RunSession: %v", err)
	}

	// With no telos notes, context building is unchanged: system prompt then user.
	if len(mock.Messages) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(mock.Messages))
	}
	if mock.Messages[0].Role != "system" || mock.Messages[0].Content != "You are a test assistant." {
		t.Errorf("first message = {role=%q content=%q}, want the DA system prompt", mock.Messages[0].Role, mock.Messages[0].Content)
	}
	if mock.Messages[1].Role != "user" {
		t.Errorf("second message role = %q, want user (no empty Telos system noise)", mock.Messages[1].Role)
	}
}

func TestChatPermissionBlockAndResume(t *testing.T) {
	a := newTestApp(t)
	seedDAIdentity(t, a)
	ctx := context.Background()

	// Create a confidential note the permission checker will deny.
	confNodeID := seedNote(t, a, "Secret Note", "secret body", []string{"confidencial"})
	sessionID := createSession(t, a)
	appendUserMessage(t, a, sessionID, "read the secret note", "")

	// Mock: first call → tool_call for the confidential node (triggers block).
	//       second call → final text after permission resolved.
	mock := newMockLLMClient(
		ChatResponse{
			ToolCalls: []ToolCall{{
				ID:   "call_1",
				Type: "function",
				Function: ToolFunction{
					Name:      "read_node",
					Arguments: fmt.Sprintf(`{"node_id":"%s"}`, confNodeID),
				},
			}},
		},
		ChatResponse{Content: "I cannot read that — it is private."},
	)

	permChecker := NewGraphPermissionChecker(a)

	// ── Phase 1: engine hits the permission block ──
	rec1 := &eventRecorder{}
	engine, err := NewChatEngine(a, sessionID, rec1, mock, permChecker)
	if err != nil {
		t.Fatalf("NewChatEngine: %v", err)
	}
	if err := engine.RunSession(ctx); err != nil {
		t.Fatalf("RunSession: %v", err)
	}

	if !rec1.hasEventType("permission_required") {
		t.Fatalf("expected permission_required event, got buffer: %s", rec1.buf.String())
	}

	// Verify session persisted the pending permission.
	cs := getSession(t, a, sessionID)
	if cs.PendingPermission == nil {
		t.Fatal("expected PendingPermission to be set")
	}
	if cs.PendingPermission.RequestedNodeID != confNodeID {
		t.Errorf("RequestedNodeID = %q, want %q", cs.PendingPermission.RequestedNodeID, confNodeID)
	}
	if cs.PendingPermission.Status != "pending" {
		t.Errorf("Status = %q, want pending", cs.PendingPermission.Status)
	}

	// ── Phase 2: simulate approval — clear pending and resume ──
	cs.PendingPermission = nil
	err = a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return nodes.SaveChatSession(ctx, tx, cs, nodes.Author{Name: "kernl"})
	})
	if err != nil {
		t.Fatalf("clear permission: %v", err)
	}

	rec2 := &eventRecorder{}
	engine2, err2 := NewChatEngine(a, sessionID, rec2, mock, permChecker)
	if err2 != nil {
		t.Fatalf("NewChatEngine resume: %v", err2)
	}
	if err := engine2.RunSession(ctx); err != nil {
		t.Fatalf("RunSession resume: %v", err)
	}

	if !rec2.hasEventType("token") {
		t.Errorf("expected token event after resume, got buffer: %s", rec2.buf.String())
	}
	if !rec2.hasEventType("done") {
		t.Errorf("expected done event after resume, got buffer: %s", rec2.buf.String())
	}
}

func TestScopeDerivation(t *testing.T) {
	// Pure logic: DeriveScope.
	t.Run("explicit scope", func(t *testing.T) {
		r := DeriveScope("current", "explicit")
		if len(r.NodeIDs) != 1 || r.NodeIDs[0] != "explicit" {
			t.Errorf("expected [explicit], got %v", r.NodeIDs)
		}
	})
	t.Run("current fallback", func(t *testing.T) {
		r := DeriveScope("current", "")
		if len(r.NodeIDs) != 1 || r.NodeIDs[0] != "current" {
			t.Errorf("expected [current], got %v", r.NodeIDs)
		}
	})
	t.Run("empty", func(t *testing.T) {
		r := DeriveScope("", "")
		if len(r.NodeIDs) != 0 {
			t.Errorf("expected [], got %v", r.NodeIDs)
		}
	})

	// Integration: scope_node_id is persisted on the session and engine builds messages.
	a := newTestApp(t)
	seedDAIdentity(t, a)

	noteID := seedNote(t, a, "Scope Target", "target body", nil)
	sessionID := createSession(t, a)
	appendUserMessage(t, a, sessionID, "show the scoped note", noteID)

	// Verify DerivedScopeNodeID is stored.
	cs := getSession(t, a, sessionID)
	if cs.DerivedScopeNodeID != noteID {
		t.Errorf("DerivedScopeNodeID = %q, want %q", cs.DerivedScopeNodeID, noteID)
	}

	// Verify engine builds messages correctly: system prompt + user message.
	mock := newMockLLMClient(ChatResponse{Content: "OK"})
	rec := &eventRecorder{}
	engine, err := NewChatEngine(a, sessionID, rec, mock, alwaysAllow{})
	if err != nil {
		t.Fatalf("NewChatEngine: %v", err)
	}
	if err := engine.RunSession(context.Background()); err != nil {
		t.Fatalf("RunSession: %v", err)
	}

	if len(mock.Messages) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(mock.Messages))
	}
	if mock.Messages[0].Role != "system" {
		t.Errorf("first message role = %q, want system", mock.Messages[0].Role)
	}
	if mock.Messages[1].Role != "user" || mock.Messages[1].Content != "show the scoped note" {
		t.Errorf("second message = {role=%q content=%q}, want {role=user content='show the scoped note'}",
			mock.Messages[1].Role, mock.Messages[1].Content)
	}
}

func TestDAIdentityApplied(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()

	// Seed with a custom prompt we can verify.
	err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := nodes.CreateDAIdentity(ctx, tx, &nodes.DAIdentity{
			SystemPrompt: "You are custom test assistant.",
			DisplayName:  "Custom DA",
		}, nodes.Author{Name: "kernl"})
		return err
	})
	if err != nil {
		t.Fatalf("create DA identity: %v", err)
	}

	sessionID := createSession(t, a)
	appendUserMessage(t, a, sessionID, "hello", "")

	mock := newMockLLMClient(ChatResponse{Content: "Hello!"})
	rec := &eventRecorder{}
	engine, err := NewChatEngine(a, sessionID, rec, mock, alwaysAllow{})
	if err != nil {
		t.Fatalf("NewChatEngine: %v", err)
	}
	if err := engine.RunSession(ctx); err != nil {
		t.Fatalf("RunSession: %v", err)
	}

	// Verify the mock received the DA system prompt as the first message.
	if len(mock.Messages) == 0 {
		t.Fatal("no messages recorded by mock")
	}
	if mock.Messages[0].Role != "system" {
		t.Errorf("first message role = %q, want system", mock.Messages[0].Role)
	}
	if mock.Messages[0].Content != "You are custom test assistant." {
		t.Errorf("system prompt = %q, want 'You are custom test assistant.'", mock.Messages[0].Content)
	}
}

func TestChatPermissionDenyAndContinue(t *testing.T) {
	a := newTestApp(t)
	seedDAIdentity(t, a)
	ctx := context.Background()

	confNodeID := seedNote(t, a, "Secret Note", "secret body", []string{"confidencial"})
	sessionID := createSession(t, a)
	appendUserMessage(t, a, sessionID, "read the secret note", "")

	// Mock: first call → tool_call to trigger block; second call → text after denial.
	mock := newMockLLMClient(
		ChatResponse{
			ToolCalls: []ToolCall{{
				ID:   "call_1",
				Type: "function",
				Function: ToolFunction{
					Name:      "read_node",
					Arguments: fmt.Sprintf(`{"node_id":"%s"}`, confNodeID),
				},
			}},
		},
		ChatResponse{Content: "I was told I cannot access that."},
	)

	permChecker := NewGraphPermissionChecker(a)
	rec := &eventRecorder{}

	engine, err := NewChatEngine(a, sessionID, rec, mock, permChecker)
	if err != nil {
		t.Fatalf("NewChatEngine: %v", err)
	}
	if err := engine.RunSession(ctx); err != nil {
		t.Fatalf("RunSession: %v", err)
	}

	if !rec.hasEventType("permission_required") {
		t.Fatalf("expected permission_required, got: %s", rec.buf.String())
	}

	// Simulate user DENY — clear pending and resume.
	cs := getSession(t, a, sessionID)
	cs.PendingPermission = nil
	err = a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return nodes.SaveChatSession(ctx, tx, cs, nodes.Author{Name: "kernl"})
	})
	if err != nil {
		t.Fatalf("clear permission: %v", err)
	}

	rec2 := &eventRecorder{}
	engine2, err := NewChatEngine(a, sessionID, rec2, mock, permChecker)
	if err != nil {
		t.Fatalf("NewChatEngine resume: %v", err)
	}
	if err := engine2.RunSession(ctx); err != nil {
		t.Fatalf("RunSession resume: %v", err)
	}

	if !rec2.hasEventType("token") {
		t.Errorf("expected token after deny+resume, got: %s", rec2.buf.String())
	}
	if !rec2.hasEventType("done") {
		t.Errorf("expected done after deny+resume, got: %s", rec2.buf.String())
	}
}

func TestChatConcurrentSessions(t *testing.T) {
	a := newTestApp(t)
	seedDAIdentity(t, a)

	var wg sync.WaitGroup
	errs := make(chan error, 5)

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sid := createSession(t, a)
			appendUserMessage(t, a, sid, fmt.Sprintf("msg-%d", idx), "")

			mock := newMockLLMClient(ChatResponse{Content: fmt.Sprintf("resp-%d", idx)})
			rec := &eventRecorder{}
			engine, egErr := NewChatEngine(a, sid, rec, mock, alwaysAllow{})
			if egErr != nil {
				errs <- fmt.Errorf("goroutine %d NewChatEngine: %w", idx, egErr)
				return
			}
			if egErr := engine.RunSession(context.Background()); egErr != nil {
				errs <- fmt.Errorf("goroutine %d RunSession: %w", idx, egErr)
				return
			}
			if !rec.hasEventType("done") {
				errs <- fmt.Errorf("goroutine %d: expected done event, got buffer: %s", idx, rec.buf.String())
				return
			}
		}(i)
	}
	wg.Wait()
	close(errs)

	for e := range errs {
		t.Error(e)
	}
}

func TestChatSSEReconnectRestoresPendingPermission(t *testing.T) {
	a := newTestApp(t)
	seedDAIdentity(t, a)
	ctx := context.Background()

	confNodeID := seedNote(t, a, "Secret Note", "body", []string{"confidencial"})
	sessionID := createSession(t, a)
	appendUserMessage(t, a, sessionID, "read secret", "")

	mock := newMockLLMClient(
		ChatResponse{ToolCalls: []ToolCall{{
			ID:   "call_1",
			Type: "function",
			Function: ToolFunction{
				Name:      "read_node",
				Arguments: fmt.Sprintf(`{"node_id":"%s"}`, confNodeID),
			},
		}}},
	)

	// First connection — blocks on permission.
	rec1 := &eventRecorder{}
	engine1, err := NewChatEngine(a, sessionID, rec1, mock, NewGraphPermissionChecker(a))
	if err != nil {
		t.Fatalf("NewChatEngine: %v", err)
	}
	_ = engine1.RunSession(ctx)

	if !rec1.hasEventType("permission_required") {
		t.Fatal("expected permission_required on first connection")
	}

	// Second connection — should re-emit state + permission_required.
	rec2 := &eventRecorder{}
	engine2, err := NewChatEngine(a, sessionID, rec2, mock, NewGraphPermissionChecker(a))
	if err != nil {
		t.Fatalf("NewChatEngine reconnect: %v", err)
	}
	_ = engine2.RunSession(ctx)

	if !rec2.hasEventType("state") {
		t.Error("expected state event on reconnect")
	}
	if !rec2.hasEventType("permission_required") {
		t.Error("expected permission_required on reconnect")
	}
}
