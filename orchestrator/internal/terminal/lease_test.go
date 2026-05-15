package terminal

import (
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/session"
)

func TestLeaseNickname(t *testing.T) {
	tests := []struct {
		name     string
		input    *EnsureLeaseInput
		expected string
	}{
		{
			name: "uses session ID when available",
			input: &EnsureLeaseInput{
				Source:     LeaseSourceTerminalManagerTake,
				SessionID: "sess-123",
				BeadID:    "bead-1",
			},
			expected: "kernl:terminal_manager_take:sess-123",
		},
		{
			name: "uses execution lease ID when no session ID",
			input: &EnsureLeaseInput{
				Source:           LeaseSourceStructuredPrepareTake,
				ExecutionLeaseID: "lease-456",
				BeadID:           "bead-1",
			},
			expected: "kernl:structured_prepare_take:lease-456",
		},
		{
			name: "uses bead ID when no session or execution lease ID",
			input: &EnsureLeaseInput{
				Source: LeaseSourceStructuredPreparePoll,
				BeadID: "bead-2",
			},
			expected: "kernl:structured_prepare_poll:bead-2",
		},
		{
			name: "falls back to runtime when no IDs",
			input: &EnsureLeaseInput{
				Source: LeaseSourceDoctorActiveLeases,
			},
			expected: "kernl:doctor_active_leases:runtime",
		},
		{
			name: "truncates at 120 chars",
			input: &EnsureLeaseInput{
				Source:     LeaseSourceTerminalManagerTake,
				SessionID: strings.Repeat("x", 200),
			},
			expected: "kernl:terminal_manager_take:" + strings.Repeat("x", 92),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := leaseNickname(tt.input)
			if len(got) > 120 {
				t.Errorf("nickname too long: %d chars", len(got))
			}
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestSetKnotsLease(t *testing.T) {
	entry := newTestSessionEntry()

	agentInfo := &AgentInfo{
		AgentName:     "Codex",
		AgentModel:    "gpt-5.4-codex",
		AgentProvider: "Codex",
	}

	entry.SetKnotsLease("lease-k1", agentInfo)

	if entry.KnotsLeaseID != "lease-k1" {
		t.Errorf("expected KnotsLeaseID=lease-k1, got %s", entry.KnotsLeaseID)
	}
	if entry.KnotsLeaseAgentInfo == nil {
		t.Fatal("expected KnotsLeaseAgentInfo to be set")
	}
	if entry.KnotsLeaseAgentInfo.AgentName != "Codex" {
		t.Errorf("expected AgentName=Codex, got %s", entry.KnotsLeaseAgentInfo.AgentName)
	}
	if entry.LastReleasedLeaseID != "" {
		t.Errorf("expected LastReleasedLeaseID empty after set, got %s", entry.LastReleasedLeaseID)
	}

	entry.SyncSessionLeaseInfo()
	if entry.Session.KnotsLeaseID != "lease-k1" {
		t.Errorf("expected session KnotsLeaseID=lease-k1, got %s", entry.Session.KnotsLeaseID)
	}
	if entry.Session.KnotsAgentInfo == nil {
		t.Fatal("expected session KnotsAgentInfo to be set")
	}
	if entry.Session.KnotsAgentInfo.AgentName != "Codex" {
		t.Errorf("expected session AgentName=Codex, got %s", entry.Session.KnotsAgentInfo.AgentName)
	}
}

func TestSetKnotsLeaseClearsLastReleased(t *testing.T) {
	entry := newTestSessionEntry()
	entry.LastReleasedLeaseID = "lease-old"

	entry.SetKnotsLease("lease-new", nil)

	if entry.LastReleasedLeaseID != "" {
		t.Errorf("expected LastReleasedLeaseID cleared after SetKnotsLease, got %s", entry.LastReleasedLeaseID)
	}
}

func TestClearKnotsLease(t *testing.T) {
	entry := newTestSessionEntry()
	entry.SetKnotsLease("lease-k1", &AgentInfo{AgentName: "Codex"})

	entry.ClearKnotsLease("session_end")

	if entry.KnotsLeaseID != "" {
		t.Errorf("expected KnotsLeaseID cleared, got %s", entry.KnotsLeaseID)
	}
	if entry.KnotsLeaseAgentInfo != nil {
		t.Error("expected KnotsLeaseAgentInfo cleared")
	}
	if entry.LastReleasedLeaseID != "lease-k1" {
		t.Errorf("expected LastReleasedLeaseID=lease-k1, got %s", entry.LastReleasedLeaseID)
	}

	entry.SyncSessionLeaseInfo()
	if entry.Session.KnotsLeaseID != "" {
		t.Errorf("expected session KnotsLeaseID cleared, got %s", entry.Session.KnotsLeaseID)
	}
	if entry.Session.KnotsAgentInfo != nil {
		t.Error("expected session KnotsAgentInfo cleared")
	}
}

func TestClearKnotsLeaseNoExistingLease(t *testing.T) {
	entry := newTestSessionEntry()
	entry.ClearKnotsLease("no_lease")

	if entry.KnotsLeaseID != "" {
		t.Error("expected empty KnotsLeaseID")
	}
	if entry.LastReleasedLeaseID != "" {
		t.Errorf("expected empty LastReleasedLeaseID, got %s", entry.LastReleasedLeaseID)
	}
}

func TestSyncSessionLeaseInfoWithNilAgentInfo(t *testing.T) {
	entry := newTestSessionEntry()
	entry.KnotsLeaseID = "lease-1"
	entry.KnotsLeaseAgentInfo = nil

	entry.SyncSessionLeaseInfo()

	if entry.Session.KnotsLeaseID != "lease-1" {
		t.Errorf("expected KnotsLeaseID=lease-1, got %s", entry.Session.KnotsLeaseID)
	}
	if entry.Session.KnotsAgentInfo != nil {
		t.Error("expected nil KnotsAgentInfo")
	}
}

func TestDisplayAgentName(t *testing.T) {
	tests := []struct {
		name     string
		info     *AgentInfo
		expected string
	}{
		{name: "nil returns empty", info: nil, expected: ""},
		{name: "with name returns name", info: &AgentInfo{AgentName: "Codex"}, expected: "Codex"},
		{name: "empty name returns empty", info: &AgentInfo{AgentName: ""}, expected: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := displayAgentName(tt.info)
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestProviderOrDefault(t *testing.T) {
	tests := []struct {
		name     string
		info     *AgentInfo
		expected string
	}{
		{name: "nil returns empty", info: nil, expected: ""},
		{name: "with provider returns provider", info: &AgentInfo{AgentProvider: "OpenAI"}, expected: "OpenAI"},
		{name: "empty provider returns empty", info: &AgentInfo{AgentProvider: ""}, expected: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := providerOrDefault(tt.info)
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestAgentTypeOrDefault(t *testing.T) {
	got := agentTypeOrDefault(nil)
	if got != "cli" {
		t.Errorf("expected cli, got %s", got)
	}
	got = agentTypeOrDefault(&AgentInfo{AgentModel: "gpt-4"})
	if got != "cli" {
		t.Errorf("expected cli, got %s", got)
	}
}

func TestMakeReleaseKnotsLeaseFunc_NoLease(t *testing.T) {
	entry := newTestSessionEntry()

	fn := MakeReleaseKnotsLeaseFunc(entry, nil)
	fn("abort", "warning", nil)

	if entry.KnotsLeaseID != "" {
		t.Error("KnotsLeaseID should remain empty")
	}
}

func TestMakeReleaseKnotsLeaseFunc_ClearsAfterSet(t *testing.T) {
	entry := newTestSessionEntry()
	entry.SetKnotsLease("lease-test-1", &AgentInfo{AgentName: "Codex"})

	if entry.KnotsLeaseID != "lease-test-1" {
		t.Errorf("expected lease-test-1, got %s", entry.KnotsLeaseID)
	}

	entry.ClearKnotsLease("session_end")

	if entry.KnotsLeaseID != "" {
		t.Errorf("expected empty KnotsLeaseID after clear, got %s", entry.KnotsLeaseID)
	}
	if entry.LastReleasedLeaseID != "lease-test-1" {
		t.Errorf("expected LastReleasedLeaseID=lease-test-1, got %s", entry.LastReleasedLeaseID)
	}
}

func TestBuildCreateLeaseOptions(t *testing.T) {
	info := &AgentInfo{
		AgentName:     "Codex",
		AgentProvider: "Codex",
		AgentModel:    "gpt-5.4-codex",
	}

	input := &EnsureLeaseInput{
		RepoPath:   "/tmp/repo",
		Source:      LeaseSourceTerminalManagerTake,
		SessionID:  "sess-1",
		BeadID:     "bead-1",
		AgentInfo:  info,
	}

	opts := buildCreateLeaseOptions(input)

	if opts.Nickname != "kernl:terminal_manager_take:sess-1" {
		t.Errorf("expected nickname=kernl:terminal_manager_take:sess-1, got %s", opts.Nickname)
	}
	if opts.Type != "agent" {
		t.Errorf("expected type=agent, got %s", opts.Type)
	}
	if opts.AgentName != "Codex" {
		t.Errorf("expected agentName=Codex, got %s", opts.AgentName)
	}
	if opts.AgentType != "cli" {
		t.Errorf("expected agentType=cli, got %s", opts.AgentType)
	}
	if opts.Provider != "Codex" {
		t.Errorf("expected provider=Codex, got %s", opts.Provider)
	}
	if opts.Model != "gpt-5.4-codex" {
		t.Errorf("expected model=gpt-5.4-codex, got %s", opts.Model)
	}
}

func TestBuildCreateLeaseOptionsNilAgentInfo(t *testing.T) {
	input := &EnsureLeaseInput{
		RepoPath:  "/tmp/repo",
		Source:     LeaseSourceDoctorActiveLeases,
		BeadID:    "bead-2",
		AgentInfo: nil,
	}

	opts := buildCreateLeaseOptions(input)

	if opts.AgentName != "" {
		t.Errorf("expected empty agentName, got %s", opts.AgentName)
	}
	if opts.AgentType != "cli" {
		t.Errorf("expected agentType=cli, got %s", opts.AgentType)
	}
	if opts.Provider != "" {
		t.Errorf("expected empty provider, got %s", opts.Provider)
	}
	if opts.Model != "" {
		t.Errorf("expected empty model, got %s", opts.Model)
	}
}

func TestKnotsBackendCreateLeaseOptionsFields(t *testing.T) {
	opts := backend.CreateLeaseOptions{
		Nickname:     "test-lease",
		Type:         "agent",
		AgentName:    "Claude",
		AgentType:    "cli",
		Provider:     "Anthropic",
		Model:        "claude-sonnet-4",
		ModelVersion: "20250115",
	}

	if opts.Nickname != "test-lease" {
		t.Errorf("expected test-lease, got %s", opts.Nickname)
	}
	if opts.Type != "agent" {
		t.Errorf("expected agent, got %s", opts.Type)
	}
	if opts.AgentName != "Claude" {
		t.Errorf("expected Claude, got %s", opts.AgentName)
	}
	if opts.AgentType != "cli" {
		t.Errorf("expected cli, got %s", opts.AgentType)
	}
	if opts.Provider != "Anthropic" {
		t.Errorf("expected Anthropic, got %s", opts.Provider)
	}
	if opts.Model != "claude-sonnet-4" {
		t.Errorf("expected claude-sonnet-4, got %s", opts.Model)
	}
	if opts.ModelVersion != "20250115" {
		t.Errorf("expected 20250115, got %s", opts.ModelVersion)
	}
}

func newTestSessionEntry() *SessionEntry {
	return &SessionEntry{
		Session:                     &TerminalSession{ID: "sess-test"},
		Events:                       make(chan session.TerminalEvent, MaxBuffer),
		Buffer:                       make([]session.TerminalEvent, 0),
		TakeLoopLifecycle:             make(map[int]*TakeLoopIterationTrace),
		PendingApprovals:             make(map[string]*PendingApprovalRecord),
		ClaimsPerQueueType:           make(map[string]int),
		LastAgentPerQueueType:        make(map[string]string),
		FailedAgentsPerQueueType:     make(map[string]map[string]bool),
		FollowUpAttempts:             make(map[string]int),
	}
}