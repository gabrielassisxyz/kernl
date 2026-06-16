package workflow_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/workflow"
)

func newStore(t *testing.T) *workflow.AgentStateStore {
	t.Helper()
	dir := t.TempDir()
	s, err := workflow.NewAgentStateStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestAgentStateStore_ReadMissing_ReturnsDefaults(t *testing.T) {
	s := newStore(t)
	got, err := s.Load("kernl-abc")
	if err != nil {
		t.Fatal(err)
	}
	if got.AgentState != "" || got.FollowUpCount != 0 {
		t.Fatalf("expected zero-value, got %+v", got)
	}
}

func TestAgentStateStore_WriteThenRead_Roundtrip(t *testing.T) {
	s := newStore(t)
	in := workflow.AgentRuntime{
		AgentState:      workflow.AgentWorking,
		AgentSessionID:  "sess-1",
		AgentStartedAt:  time.Date(2026, 5, 15, 14, 23, 0, 0, time.UTC),
		LastHeartbeatAt: time.Date(2026, 5, 15, 14, 25, 30, 0, time.UTC),
		FollowUpCount:   2,
	}
	if err := s.Save("kernl-abc", in); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load("kernl-abc")
	if err != nil {
		t.Fatal(err)
	}
	if got != in {
		t.Fatalf("roundtrip mismatch:\n got %+v\nwant %+v", got, in)
	}
}

func TestAgentStateStore_CorruptedJSON_RecoverWithDefaults(t *testing.T) {
	dir := t.TempDir()
	s, _ := workflow.NewAgentStateStore(dir)
	path := filepath.Join(dir, "kernl-abc.json")
	if err := os.WriteFile(path, []byte("{not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load("kernl-abc")
	if err != nil {
		t.Fatalf("expected recover, got err: %v", err)
	}
	if got.AgentState != "" {
		t.Fatalf("expected defaults after corrupt, got %+v", got)
	}
}

func TestAgentStateStore_AtomicWrite_NoTempLeftover(t *testing.T) {
	s := newStore(t)
	if err := s.Save("kernl-abc", workflow.AgentRuntime{AgentState: workflow.AgentWorking}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(s.Dir())
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Fatalf("temp file leaked: %s", e.Name())
		}
	}
	data, _ := os.ReadFile(filepath.Join(s.Dir(), "kernl-abc.json"))
	var v workflow.AgentRuntime
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("final file not JSON: %v", err)
	}
}

func TestAgentStateStore_ConcurrentWrites_SameBead_Serialized(t *testing.T) {
	s := newStore(t)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = s.Save("kernl-abc", workflow.AgentRuntime{AgentState: workflow.AgentWorking, FollowUpCount: i})
		}(i)
	}
	wg.Wait()
	got, err := s.Load("kernl-abc")
	if err != nil {
		t.Fatal(err)
	}
	if got.AgentState != workflow.AgentWorking {
		t.Fatalf("torn write: %+v", got)
	}
}

func TestAgentStateStore_Purge_RemovesFile(t *testing.T) {
	s := newStore(t)
	_ = s.Save("kernl-abc", workflow.AgentRuntime{AgentState: workflow.AgentWorking})
	if err := s.Purge("kernl-abc"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.Dir(), "kernl-abc.json")); !os.IsNotExist(err) {
		t.Fatalf("expected file gone, err=%v", err)
	}
	if err := s.Purge("kernl-abc"); err != nil {
		t.Fatalf("purge should be idempotent, got %v", err)
	}
}

func TestAgentStateStore_ContextPayload_Roundtrip(t *testing.T) {
	s := newStore(t)

	// Missing bead should return empty
	got, err := s.Load("missing-bead")
	if err != nil {
		t.Fatal(err)
	}
	if got.ContextPayload != "" {
		t.Fatalf("expected empty context payload for missing bead, got %q", got.ContextPayload)
	}

	// Save context payload
	in := workflow.AgentRuntime{
		AgentState:     workflow.AgentWorking,
		ContextPayload: "# Custom Context\naccumulated context",
	}
	if err := s.Save("bead-123", in); err != nil {
		t.Fatal(err)
	}

	got, err = s.Load("bead-123")
	if err != nil {
		t.Fatal(err)
	}
	if got.ContextPayload != in.ContextPayload {
		t.Fatalf("context payload mismatch: got %q, want %q", got.ContextPayload, in.ContextPayload)
	}
}

func TestAgentStateStore_ContextPayload_LargePayload(t *testing.T) {
	s := newStore(t)

	// Generate a ~1MB context payload (1,000,000 characters)
	sb := strings.Builder{}
	for i := 0; i < 100000; i++ {
		sb.WriteString("1234567890")
	}
	largePayload := sb.String()

	in := workflow.AgentRuntime{
		AgentState:     workflow.AgentWorking,
		ContextPayload: largePayload,
	}

	if err := s.Save("bead-large", in); err != nil {
		t.Fatal(err)
	}

	got, err := s.Load("bead-large")
	if err != nil {
		t.Fatal(err)
	}

	if got.ContextPayload != largePayload {
		t.Fatalf("large context payload corrupted during roundtrip, size got %d, want %d", len(got.ContextPayload), len(largePayload))
	}
}
