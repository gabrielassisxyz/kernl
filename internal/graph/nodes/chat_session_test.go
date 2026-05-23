package nodes

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

func TestChatSessionRoundtrip(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	ts := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	cs := &ChatSession{
		Messages: []ChatMessage{
			{Role: "user", Content: "Hello", Timestamp: ts},
			{Role: "assistant", Content: "Hi there!", Timestamp: ts.Add(time.Minute)},
		},
		DerivedScopeNodeID: "scope-123",
	}

	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = CreateChatSession(ctx, tx, cs, Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateChatSession: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty id")
	}

	var got *ChatSession
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		got, err = GetChatSession(ctx, tx, id)
		return err
	})
	if err != nil {
		t.Fatalf("GetChatSession: %v", err)
	}

	if got.ID != id {
		t.Errorf("ID = %q, want %q", got.ID, id)
	}
	if len(got.Messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(got.Messages))
	}
	if got.Messages[0].Role != "user" || got.Messages[0].Content != "Hello" {
		t.Errorf("message[0] = %+v", got.Messages[0])
	}
	if got.DerivedScopeNodeID != "scope-123" {
		t.Errorf("DerivedScopeNodeID = %q, want scope-123", got.DerivedScopeNodeID)
	}
}

func TestChatSessionEmptyMessagesSerializesAsEmptyArray(t *testing.T) {
	cs := &ChatSession{
		ID:                 "test-id",
		Messages:           nil,
		DerivedScopeNodeID: "",
	}

	attrs := cs.NodeAttrs()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(attrs, &raw); err != nil {
		t.Fatalf("unmarshal attrs: %v", err)
	}

	// Messages should be present and be "[]" (not null/absent)
	msgRaw, ok := raw["messages"]
	if !ok {
		t.Fatal("expected 'messages' key in attrs")
	}
	if string(msgRaw) != "[]" {
		t.Fatalf("expected messages = [], got %s", string(msgRaw))
	}
}

func TestChatSessionNilPendingPermissionOmitsKey(t *testing.T) {
	cs := &ChatSession{
		ID:                 "test-id",
		Messages:           nil,
		PendingPermission:  nil,
		DerivedScopeNodeID: "",
	}

	attrs := cs.NodeAttrs()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(attrs, &raw); err != nil {
		t.Fatalf("unmarshal attrs: %v", err)
	}

	if _, ok := raw["pending_permission"]; ok {
		t.Fatal("expected 'pending_permission' key to be absent when nil")
	}
}

func TestChatSessionWithPendingPermission(t *testing.T) {
	ts := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	cs := &ChatSession{
		ID: "test-id",
		PendingPermission: &PendingPermissionState{
			ToolCallID:        "tc-1",
			RequestedNodeID:   "node-42",
			RequestedNodePath: "/notes/foo",
			Status:            "pending",
			CreatedAt:         ts,
		},
	}

	attrs := cs.NodeAttrs()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(attrs, &raw); err != nil {
		t.Fatalf("unmarshal attrs: %v", err)
	}

	ppRaw, ok := raw["pending_permission"]
	if !ok {
		t.Fatal("expected 'pending_permission' key when non-nil")
	}

	var pp PendingPermissionState
	if err := json.Unmarshal(ppRaw, &pp); err != nil {
		t.Fatalf("unmarshal pending_permission: %v", err)
	}
	if pp.ToolCallID != "tc-1" || pp.Status != "pending" {
		t.Errorf("pending_permission = %+v", pp)
	}
}

func TestChatSessionGetNotFound(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		_, err := GetChatSession(ctx, tx, "nonexistent")
		return err
	})
	if !errors.Is(err, graph.ErrNotFound) {
		t.Fatalf("expected graph.ErrNotFound, got %v", err)
	}
}

func TestChatSessionSave(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	cs := &ChatSession{
		Messages: []ChatMessage{
			{Role: "user", Content: "before save", Timestamp: time.Now()},
		},
	}

	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = CreateChatSession(ctx, tx, cs, Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateChatSession: %v", err)
	}

	// Now update
	updated := &ChatSession{
		ID: id,
		Messages: []ChatMessage{
			{Role: "user", Content: "before save", Timestamp: time.Now()},
			{Role: "assistant", Content: "after save", Timestamp: time.Now()},
		},
		DerivedScopeNodeID: "scope-updated",
	}
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return SaveChatSession(ctx, tx, updated, Author{Name: "test"})
	})
	if err != nil {
		t.Fatalf("SaveChatSession: %v", err)
	}

	var got *ChatSession
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		got, err = GetChatSession(ctx, tx, id)
		return err
	})
	if err != nil {
		t.Fatalf("GetChatSession after save: %v", err)
	}
	if len(got.Messages) != 2 {
		t.Errorf("messages len after save = %d, want 2", len(got.Messages))
	}
	if got.DerivedScopeNodeID != "scope-updated" {
		t.Errorf("DerivedScopeNodeID = %q, want scope-updated", got.DerivedScopeNodeID)
	}
}

func TestChatSessionFTSFieldsEmpty(t *testing.T) {
	cs := &ChatSession{ID: "id"}
	fts := cs.FTSFields()
	if fts.Title != "" || fts.Body != "" || fts.Tags != "" {
		t.Errorf("expected empty FTSFields, got %+v", fts)
	}
}

func TestChatSessionMessagesReadbackPreservesEmptySlice(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	cs := &ChatSession{
		Messages: []ChatMessage{},
	}

	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = CreateChatSession(ctx, tx, cs, Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateChatSession: %v", err)
	}

	// Verify that empty messages roundtrip as [] not nil
	attrsCheck := cs.NodeAttrs()
	if !strings.Contains(string(attrsCheck), `"messages":[]`) {
		t.Fatalf("expected messages:[] in attrs, got %s", string(attrsCheck))
	}

	var got *ChatSession
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		got, err = GetChatSession(ctx, tx, id)
		return err
	})
	if err != nil {
		t.Fatalf("GetChatSession: %v", err)
	}
	if got.Messages == nil {
		t.Fatal("expected non-nil Messages slice after readback")
	}
	if len(got.Messages) != 0 {
		t.Errorf("expected empty slice, got %d elements", len(got.Messages))
	}
}
