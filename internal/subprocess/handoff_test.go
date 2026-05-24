package subprocess_test

import (
	"encoding/json"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/subprocess"
)

func TestHandoffRequest_JSONContract(t *testing.T) {
	req := subprocess.HandoffRequest{
		EpicID:         "epic-123",
		BeadID:         "bead-456",
		WorktreePath:   "/path/to/worktree",
		ContextPayload: "# Context Markdown\nSome data here.",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal HandoffRequest: %v", err)
	}

	// Verify exact rigid keys are present in the JSON representation.
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal raw map: %v", err)
	}

	expectedKeys := []string{"epic_id", "bead_id", "worktree_path", "context_payload"}
	for _, key := range expectedKeys {
		if _, ok := raw[key]; !ok {
			t.Errorf("expected key %q in JSON, but it was missing", key)
		}
	}

	// Verify roundtrip
	var roundtripped subprocess.HandoffRequest
	if err := json.Unmarshal(data, &roundtripped); err != nil {
		t.Fatalf("failed to unmarshal HandoffRequest: %v", err)
	}

	if roundtripped != req {
		t.Errorf("roundtrip mismatch:\n got %+v\nwant %+v", roundtripped, req)
	}
}

func TestHandoffResponse_OpenOutput_ToleratesExtraKeys(t *testing.T) {
	// JSON with extra unexpected keys should be tolerated without error (open output principle).
	jsonInput := `{
		"context_payload": "some-accumulated-context",
		"unexpected_extra_key": "some-value",
		"nested_extra": {
			"another": 123
		}
	}`

	var resp subprocess.HandoffResponse
	if err := json.Unmarshal([]byte(jsonInput), &resp); err != nil {
		t.Fatalf("failed to unmarshal HandoffResponse with extra keys: %v", err)
	}

	if resp.ContextPayload != "some-accumulated-context" {
		t.Errorf("expected ContextPayload to be %q, got %q", "some-accumulated-context", resp.ContextPayload)
	}
}

func TestHandoff_EmptyContextPayload(t *testing.T) {
	// Verify that empty context payload roundtrips correctly and does not cause a panic.
	req := subprocess.HandoffRequest{
		EpicID:         "epic-id",
		BeadID:         "bead-id",
		WorktreePath:   "/some/path",
		ContextPayload: "",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var req2 subprocess.HandoffRequest
	if err := json.Unmarshal(data, &req2); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req2.ContextPayload != "" {
		t.Errorf("expected empty context payload, got %q", req2.ContextPayload)
	}

	respJSON := `{"context_payload": ""}`
	var resp subprocess.HandoffResponse
	if err := json.Unmarshal([]byte(respJSON), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.ContextPayload != "" {
		t.Errorf("expected empty response context payload, got %q", resp.ContextPayload)
	}
}
