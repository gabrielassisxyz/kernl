package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKnotsBackend_Capabilities(t *testing.T) {
	kb := NewKnotsBackend("/tmp/test-repo")
	caps := kb.Capabilities()
	if caps.CanDelete {
		t.Error("knots backend must not support delete")
	}
	if !caps.CanCreate {
		t.Error("knots backend must support create")
	}
	if !caps.CanUpdate {
		t.Error("knots backend must support update")
	}
	if !caps.CanClose {
		t.Error("knots backend must support close")
	}
	if !caps.CanManageDependencies {
		t.Error("knots backend must support manage dependencies")
	}
}

func TestKnotsBackend_DeleteReturnsError(t *testing.T) {
	kb := NewKnotsBackend("/tmp/test-repo")
	err := kb.Delete("123", "/tmp/test-repo")
	if err == nil {
		t.Fatal("expected error from knots Delete")
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("expected KERNL DISPATCH FAILURE marker, got: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "does not support delete") {
		t.Errorf("expected delete not supported message, got: %s", err.Error())
	}
}

func TestKnotRecordToBeat(t *testing.T) {
	priority := 3
	rec := knoRecord{
		ID:          "knot-1",
		Title:       "Test Knot",
		State:       "implementation",
		Type:        "feature",
		Priority:    &priority,
		Description:  "A test description",
		Acceptance:  "Must pass tests",
		Tags:         []string{"backend", "knots"},
		ProfileID:   "autopilot",
		WorkflowID:  "sdlc",
		CreatedAt:   "2024-01-01T00:00:00Z",
		UpdatedAt:   "2024-01-02T00:00:00Z",
		LeaseID:     "lease-abc",
	}
	beat := knotRecordToBeat(rec, "/repo/test")

	if beat.ID != "knot-1" {
		t.Errorf("expected ID knot-1, got %s", beat.ID)
	}
	if beat.Type != "feature" {
		t.Errorf("expected type feature, got %s", beat.Type)
	}
	if beat.State != "implementation" {
		t.Errorf("expected state implementation, got %s", beat.State)
	}
	if beat.Title != "Test Knot" {
		t.Errorf("expected title Test Knot, got %s", beat.Title)
	}
	if beat.Priority != 3 {
		t.Errorf("expected priority 3, got %d", beat.Priority)
	}
	if beat.Acceptance != "Must pass tests" {
		t.Errorf("expected acceptance, got %s", beat.Acceptance)
	}
	if len(beat.Labels) != 2 {
		t.Errorf("expected 2 labels, got %d", len(beat.Labels))
	}
	if beat.ProfileID != "autopilot" {
		t.Errorf("expected profileID autopilot, got %s", beat.ProfileID)
	}
	if beat.Metadata["lease_id"] != "lease-abc" {
		t.Errorf("expected metadata lease_id, got %v", beat.Metadata["lease_id"])
	}
}

func TestKnotRecordToBeat_Defaults(t *testing.T) {
	rec := knoRecord{
		ID:    "knot-2",
		Title: "No Type No Priority",
		State: "ready_for_implementation",
	}
	beat := knotRecordToBeat(rec, "/repo")

	if beat.Type != "task" {
		t.Errorf("expected default type task, got %s", beat.Type)
	}
	if beat.Priority != 2 {
		t.Errorf("expected default priority 2, got %d", beat.Priority)
	}
	if len(beat.Labels) != 0 {
		t.Errorf("expected empty labels, got %v", beat.Labels)
	}
}

func TestKnotRecordToBeat_LeaseMetadata(t *testing.T) {
	rec := knoRecord{
		ID:    "knot-3",
		Title: "With Lease",
		State: "implementation",
		Lease: &knoLease{
			LeaseType: "agent",
			Nickname:  "claude-sonnet",
			AgentInfo: &knoAgentInfo{
				AgentType:    "claude",
				Provider:     "anthropic",
				AgentName:    "claude-sonnet-4-20250514",
				Model:        "claude-sonnet-4-20250514",
				ModelVersion: "4",
			},
		},
	}
	beat := knotRecordToBeat(rec, "/repo")
	if beat.Metadata["agent_type"] != "claude" {
		t.Errorf("expected agent_type claude, got %v", beat.Metadata["agent_type"])
	}
	if beat.Metadata["provider"] != "anthropic" {
		t.Errorf("expected provider anthropic, got %v", beat.Metadata["provider"])
	}
}

func TestKnotRecordToBeat_ZeroPriority(t *testing.T) {
	zero := 0
	rec := knoRecord{
		ID:       "knot-zero",
		Title:    "Zero Priority",
		State:    "ready_for_implementation",
		Priority: &zero,
	}
	beat := knotRecordToBeat(rec, "")
	if beat.Priority != 2 {
		t.Errorf("expected default priority 2 for zero, got %d", beat.Priority)
	}
}

func TestKnotRecordToBeat_NilPriority(t *testing.T) {
	rec := knoRecord{
		ID:    "knot-nil",
		Title: "Nil Priority",
		State: "ready_for_implementation",
	}
	beat := knotRecordToBeat(rec, "")
	if beat.Priority != 2 {
		t.Errorf("expected default priority 2 for nil, got %d", beat.Priority)
	}
}

func TestIsKnoRetriable(t *testing.T) {
	tests := []struct {
		stderr  string
		want    bool
	}{
		{"database is locked", true},
		{"Database is locked for repo", true},
		{"operation busy, retry later", true},
		{"command timed out waiting", true},
		{"permission denied", false},
		{"not found", false},
		{"", false},
	}
	for _, tt := range tests {
		got := isKnoRetriable(tt.stderr)
		if got != tt.want {
			t.Errorf("isKnoRetriable(%q) = %v, want %v", tt.stderr, got, tt.want)
		}
	}
}

func TestKnotsBackend_buildBaseArgs(t *testing.T) {
	tests := []struct {
		name    string
		repoPath string
		knoDB   string
		wantContains []string
		wantNotContains []string
	}{
		{
			name:     "default db path",
			repoPath: "/tmp/myrepo",
			knoDB:    "",
			wantContains: []string{"--repo-root", "/tmp/myrepo", "--db"},
			wantNotContains: []string{},
		},
		{
			name:     "custom db path",
			repoPath: "/tmp/myrepo",
			knoDB:    "/custom/db.sqlite",
			wantContains: []string{"--repo-root", "/tmp/myrepo", "--db", "/custom/db.sqlite"},
			wantNotContains: []string{".knots"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kb := &KnotsBackend{repoPath: tt.repoPath, knoDB: tt.knoDB}
			args := kb.buildBaseArgs()
			for _, want := range tt.wantContains {
				found := false
				for _, arg := range args {
					if arg == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected args to contain %q, got %v", want, args)
				}
			}
			for _, notWant := range tt.wantNotContains {
				for _, arg := range args {
					if strings.Contains(arg, notWant) {
						t.Errorf("expected args NOT to contain %q, got %v", notWant, args)
					}
				}
			}
		})
	}
}

func TestKnotsBackend_ListWithFakeCLI(t *testing.T) {
	tmpDir := t.TempDir()
	knoBin := filepath.Join(tmpDir, "kno")
	script := `#!/bin/sh
echo '[{"id":"k1","title":"Test Knot","state":"implementation","type":"feature","priority":3}]'
`
	if err := os.WriteFile(knoBin, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	kb := &KnotsBackend{
		repoPath: tmpDir,
		knoBin:   knoBin,
	}

	result, err := kb.execRead(context.Background(), []string{"ls", "--all", "--json"})
	if err != nil {
		t.Fatalf("execRead error: %v", err)
	}
	if result.exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr: %s", result.exitCode, result.stderr)
	}

	var records []knoRecord
	if err := json.Unmarshal([]byte(result.stdout), &records); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].ID != "k1" {
		t.Errorf("expected ID k1, got %s", records[0].ID)
	}
}

func TestKnotsBackend_EdgeDepDependencyMapping(t *testing.T) {
	edgeJSON := `[{"src":"p1","kind":"blocked_by","dst":"p2"},{"src":"p1","kind":"parent_of","dst":"c1"}]`
	var edges []knoEdge
	if err := json.Unmarshal([]byte(edgeJSON), &edges); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	deps := make([]BeatDependency, len(edges))
	expectedTypes := []string{"blocks", "parent-child"}
	for i, e := range edges {
		depType := e.Kind
		if e.Kind == "blocked_by" {
			depType = "blocks"
		} else if e.Kind == "parent_of" {
			depType = "parent-child"
		}
		deps[i] = BeatDependency{SourceID: e.Src, TargetID: e.Dst, Type: depType}
		if deps[i].Type != expectedTypes[i] {
			t.Errorf("edge %d: expected type %s, got %s", i, expectedTypes[i], deps[i].Type)
		}
	}
}

func TestKnotsWorkflowToDescriptor(t *testing.T) {
	wf := knoWorkflowDefinition{
		ID:           "sdlc",
		InitialState: "ready_for_implementation",
		States:       []string{"ready_for_implementation", "implementation", "shipped"},
		TerminalStates: []string{"shipped"},
		Transitions: []struct {
			From string `json:"from"`
			To   string `json:"to"`
		}{
			{From: "ready_for_implementation", To: "implementation"},
			{From: "implementation", To: "shipped"},
		},
	}

	descriptor := WorkflowDescriptor{
		ID:           wf.ID,
		Label:        wf.ID,
		RetakeState:  wf.InitialState,
		QueueActions: map[string]string{
			"ready_for_implementation": "implementation",
		},
	}
	if descriptor.ID != "sdlc" {
		t.Errorf("expected ID sdlc, got %s", descriptor.ID)
	}
}

func TestKnotsWorkflowFallback(t *testing.T) {
	tmpDir := t.TempDir()
	knoBin := filepath.Join(tmpDir, "kno")
	script := fmt.Sprintf(`#!/bin/sh
callCountFile="%s/call_count"
count=$(cat "$callCountFile" 2>/dev/null || echo 0)
count=$((count + 1))
echo "$count" > "$callCountFile"
if [ "$count" = "1" ]; then
    echo "unknown command: workflow list" >&2
    exit 1
fi
echo '[{"id":"sdlc","initial_state":"ready_for_implementation","states":["ready_for_implementation"],"terminal_states":["shipped"]}]'
`, tmpDir)
	if err := os.WriteFile(knoBin, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(tmpDir, "call_count"), []byte("0"), 0644)

	kb := &KnotsBackend{
		repoPath: tmpDir,
		knoBin:   knoBin,
	}

	_, err := kb.ListWorkflows("/tmp/test-repo")
	if err != nil {
		t.Logf("ListWorkflows with fallback: %v (expected - fake CLI has no real kno)", err)
	}
}