package chat

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

// bufferEventWriter captures SSE frames without a network or a ResponseWriter.
type bufferEventWriter struct {
	buf bytes.Buffer
}

func (b *bufferEventWriter) Write(p []byte) (int, error) { return b.buf.Write(p) }
func (b *bufferEventWriter) Flush()                      {}

// decodeSSEFrame pulls the JSON payload out of the single `data: …` frame.
func decodeSSEFrame(t *testing.T, raw string) map[string]json.RawMessage {
	t.Helper()
	payload := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "data:"))
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payload), &obj); err != nil {
		t.Fatalf("unmarshal SSE payload %q: %v", payload, err)
	}
	return obj
}

func assertEventKeys(t *testing.T, obj map[string]json.RawMessage, want, reject []string) {
	t.Helper()
	for _, k := range want {
		if _, ok := obj[k]; !ok {
			t.Errorf("missing camelCase key %q in SSE event", k)
		}
	}
	for _, k := range reject {
		if _, ok := obj[k]; ok {
			t.Errorf("snake_case key %q leaked into the SSE event contract", k)
		}
	}
}

func TestStateEventUsesCamelCaseKeys(t *testing.T) {
	w := &bufferEventWriter{}
	e := &ChatEngine{eventWriter: w}
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
	}

	if err := e.emitStateEvent(cs); err != nil {
		t.Fatalf("emitStateEvent: %v", err)
	}

	obj := decodeSSEFrame(t, w.buf.String())
	assertEventKeys(t, obj,
		[]string{"event", "messages", "pendingPermission"},
		[]string{"pending_permission"},
	)

	var msgs []map[string]json.RawMessage
	if err := json.Unmarshal(obj["messages"], &msgs); err != nil {
		t.Fatalf("unmarshal messages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	assertEventKeys(t, msgs[0],
		[]string{"role", "content", "learnedCandidate"},
		[]string{"learned_candidate"},
	)

	var pp map[string]json.RawMessage
	if err := json.Unmarshal(obj["pendingPermission"], &pp); err != nil {
		t.Fatalf("unmarshal pendingPermission: %v", err)
	}
	assertEventKeys(t, pp,
		[]string{"toolCallId", "requestedNodeId", "requestedNodePath", "status", "createdAt"},
		[]string{"tool_call_id", "requested_node_id", "requested_node_path", "created_at"},
	)
}

func TestPermissionRequiredEventUsesCamelCaseKeys(t *testing.T) {
	w := &bufferEventWriter{}
	e := &ChatEngine{eventWriter: w}

	pp := &nodes.PendingPermissionState{
		ToolCallID:        "tc1",
		RequestedNodeID:   "n1",
		RequestedNodePath: "/notes/foo.md",
	}
	if err := e.emitPermissionRequiredEvent(pp); err != nil {
		t.Fatalf("emitPermissionRequiredEvent: %v", err)
	}

	obj := decodeSSEFrame(t, w.buf.String())
	assertEventKeys(t, obj,
		[]string{"event", "toolCallId", "nodeId", "nodePath", "description"},
		[]string{"tool_call_id", "node_id", "node_path"},
	)
}
