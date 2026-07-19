package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

// seedChatSessionForContract writes a session exercising every shape that
// reaches the wire: a message carrying a learned candidate, and a pending
// permission. Both are nested structs whose json tags are the STORAGE format,
// which is exactly why the wire needs its own DTO.
func seedChatSessionForContract(t *testing.T, a *app.App) string {
	t.Helper()
	ctx := context.Background()
	ts := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	cs := &nodes.ChatSession{
		Messages: []nodes.ChatMessage{{
			Role:      "assistant",
			Content:   "noted",
			Timestamp: ts,
			LearnedCandidate: &nodes.LearnedCandidateState{
				Subject:   "go-programming",
				Statement: "the user prefers early returns",
			},
		}},
		PendingPermission: &nodes.PendingPermissionState{
			ToolCallID:        "tc1",
			RequestedNodeID:   "n1",
			RequestedNodePath: "/notes/foo.md",
			Status:            "pending",
			CreatedAt:         ts,
		},
		DerivedScopeNodeID:  "scope-1",
		DiscardedCandidates: []string{"rejected statement"},
		DraftRouting:        "- note: something\n",
	}

	var id string
	if err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = nodes.CreateChatSession(ctx, tx, cs, nodes.Author{Name: "test"})
		return err
	}); err != nil {
		t.Fatalf("seed chat session: %v", err)
	}
	return id
}

func TestGetChatSessionJSONContract(t *testing.T) {
	a := newTestAppWithGraphWithLLM(t)
	r := NewRouter(a)
	id := seedChatSessionForContract(t, a)

	req := httptest.NewRequest(http.MethodGet, "/api/chat/sessions/"+id, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	assertJSONKeys(t, obj,
		[]string{"id", "createdAt", "updatedAt", "messages", "pendingPermission", "derivedScopeNodeId", "discardedCandidates", "draftRouting"},
		[]string{
			"ID", "CreatedAt", "UpdatedAt", "Messages", "PendingPermission", "DerivedScopeNodeID", "DiscardedCandidates", "DraftRouting",
			"pending_permission", "derived_scope_node_id", "discarded_candidates", "draft_routing",
		},
	)

	var msgs []map[string]json.RawMessage
	if err := json.Unmarshal(obj["messages"], &msgs); err != nil {
		t.Fatalf("unmarshal messages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	assertJSONKeys(t, msgs[0],
		[]string{"role", "content", "timestamp", "learnedCandidate"},
		[]string{"learned_candidate", "Role", "Content", "Timestamp", "LearnedCandidate"},
	)

	var pp map[string]json.RawMessage
	if err := json.Unmarshal(obj["pendingPermission"], &pp); err != nil {
		t.Fatalf("unmarshal pendingPermission: %v", err)
	}
	assertJSONKeys(t, pp,
		[]string{"toolCallId", "requestedNodeId", "requestedNodePath", "status", "createdAt"},
		[]string{"tool_call_id", "requested_node_id", "requested_node_path", "created_at", "ToolCallID", "RequestedNodeID"},
	)
}

func TestCreateChatSessionJSONContract(t *testing.T) {
	a := newTestAppWithGraphWithLLM(t)
	r := NewRouter(a)

	req := httptest.NewRequest(http.MethodPost, "/api/chat/sessions", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	assertJSONKeys(t, obj, []string{"id", "createdAt"}, []string{"created_at", "ID", "CreatedAt"})
}

func TestResolvePermissionAcceptsCamelCaseToolCallID(t *testing.T) {
	a := newTestAppWithGraphWithLLM(t)
	r := NewRouter(a)
	id := seedChatSessionForContract(t, a)

	body := strings.NewReader(`{"toolCallId":"tc1","action":"deny"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/chat/sessions/"+id+"/resolve-permission", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
