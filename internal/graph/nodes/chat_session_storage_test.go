package nodes

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

// TestChatSessionStorageFormatIsSnakeCase pins the ON-DISK attrs keys.
//
// The REST layer owes its clients camelCase, and the obvious way to pay that
// debt is to retag these structs. That would be silent data loss: NodeAttrs
// marshals them into `attrs` and chatSessionAttrs reads them back, so every
// session already stored would come back with zeroed messages and no pending
// permission. The wire format is a DTO in the API layer precisely so this stays
// still — if this test fails, storage was migrated without a migration.
func TestChatSessionStorageFormatIsSnakeCase(t *testing.T) {
	ts := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	cs := ChatSession{
		Messages: []ChatMessage{{
			Role:             "assistant",
			Content:          "noted",
			Timestamp:        ts,
			LearnedCandidate: &LearnedCandidateState{Subject: "go", Statement: "prefers early returns"},
		}},
		PendingPermission: &PendingPermissionState{
			ToolCallID:        "tc1",
			RequestedNodeID:   "n1",
			RequestedNodePath: "/notes/foo.md",
			Status:            "pending",
			CreatedAt:         ts,
		},
		DerivedScopeNodeID:  "scope-1",
		DiscardedCandidates: []string{"rejected"},
		DraftRouting:        "- note: something\n",
	}

	var attrs map[string]json.RawMessage
	if err := json.Unmarshal(cs.NodeAttrs(), &attrs); err != nil {
		t.Fatalf("unmarshal attrs: %v", err)
	}
	for _, k := range []string{"messages", "derived_scope_node_id", "discarded_candidates", "draft_routing", "pending_permission"} {
		if _, ok := attrs[k]; !ok {
			t.Errorf("storage attrs lost key %q", k)
		}
	}

	var msgs []map[string]json.RawMessage
	if err := json.Unmarshal(attrs["messages"], &msgs); err != nil {
		t.Fatalf("unmarshal stored messages: %v", err)
	}
	if _, ok := msgs[0]["learned_candidate"]; !ok {
		t.Errorf("storage message lost key %q; got %v", "learned_candidate", msgs[0])
	}

	var pp map[string]json.RawMessage
	if err := json.Unmarshal(attrs["pending_permission"], &pp); err != nil {
		t.Fatalf("unmarshal stored pending_permission: %v", err)
	}
	for _, k := range []string{"tool_call_id", "requested_node_id", "requested_node_path", "status", "created_at"} {
		if _, ok := pp[k]; !ok {
			t.Errorf("stored pending_permission lost key %q", k)
		}
	}
}

// TestChatSessionRoundtripPreservesNestedState proves a session survives a full
// write/read cycle with the nested shapes the wire DTO now re-spells: the
// learned candidate on a message, and the pending permission. Storage is
// untouched by the wire change, and this is what says so.
func TestChatSessionRoundtripPreservesNestedState(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()
	ts := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	cs := &ChatSession{
		Messages: []ChatMessage{
			{Role: "user", Content: "hello", Timestamp: ts},
			{
				Role:             "assistant",
				Content:          "noted",
				Timestamp:        ts.Add(time.Minute),
				LearnedCandidate: &LearnedCandidateState{Subject: "go", Statement: "prefers early returns"},
			},
		},
		PendingPermission: &PendingPermissionState{
			ToolCallID:        "tc1",
			RequestedNodeID:   "n1",
			RequestedNodePath: "/notes/foo.md",
			Status:            "pending",
			CreatedAt:         ts,
		},
		DerivedScopeNodeID:  "scope-1",
		DiscardedCandidates: []string{"rejected"},
		DraftRouting:        "- note: something\n",
	}

	var id string
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = CreateChatSession(ctx, tx, cs, Author{Name: "test"})
		return err
	}); err != nil {
		t.Fatalf("CreateChatSession: %v", err)
	}

	var got *ChatSession
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		got, err = GetChatSession(ctx, tx, id)
		return err
	}); err != nil {
		t.Fatalf("GetChatSession: %v", err)
	}

	if len(got.Messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(got.Messages))
	}
	lc := got.Messages[1].LearnedCandidate
	if lc == nil {
		t.Fatal("learned candidate did not survive the round-trip")
	}
	if lc.Subject != "go" || lc.Statement != "prefers early returns" {
		t.Errorf("learned candidate = %+v", lc)
	}
	if got.Messages[1].Timestamp.UTC() != ts.Add(time.Minute).UTC() {
		t.Errorf("timestamp = %v, want %v", got.Messages[1].Timestamp, ts.Add(time.Minute))
	}

	pp := got.PendingPermission
	if pp == nil {
		t.Fatal("pending permission did not survive the round-trip")
	}
	if pp.ToolCallID != "tc1" || pp.RequestedNodeID != "n1" || pp.RequestedNodePath != "/notes/foo.md" || pp.Status != "pending" {
		t.Errorf("pending permission = %+v", pp)
	}
	if pp.CreatedAt.UTC() != ts.UTC() {
		t.Errorf("pending permission createdAt = %v, want %v", pp.CreatedAt, ts)
	}

	if got.DerivedScopeNodeID != "scope-1" || got.DraftRouting != "- note: something\n" {
		t.Errorf("scalar fields lost: %+v", got)
	}
	if len(got.DiscardedCandidates) != 1 || got.DiscardedCandidates[0] != "rejected" {
		t.Errorf("discarded candidates = %v", got.DiscardedCandidates)
	}
}
