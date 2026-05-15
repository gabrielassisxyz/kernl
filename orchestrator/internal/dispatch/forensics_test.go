package dispatch

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/backend"
)

func makePreSnapshot(beadID, state string, stepHistory []StepEntry, leases []backend.Bead) BeadSnapshot {
	return BeadSnapshot{
		Boundary:      "pre_lease",
		CapturedAt:    "2026-04-30T02:19:41.449Z",
		SessionID:     "ses-1",
		BeadID:        beadID,
		LeaseID:       "lease-A",
		Iteration:     1,
		KernlPID:    1,
		Bead:          beatWithSteps(beadID, state, stepHistory),
		Leases:        leases,
	}
}

func makePostSnapshot(beadID, state string, stepHistory []StepEntry, leases []backend.Bead) BeadSnapshot {
	return BeadSnapshot{
		Boundary:      "post_turn_failure",
		CapturedAt:    "2026-04-30T02:19:58.791Z",
		SessionID:     "ses-1",
		BeadID:        beadID,
		LeaseID:       "lease-A",
		Iteration:     1,
		KernlPID:    1,
		Bead:          beatWithSteps(beadID, state, stepHistory),
		Leases:        leases,
	}
}

func beatWithSteps(id, state string, steps []StepEntry) *backend.Bead {
	metadata := map[string]any{}
	if steps != nil {
		entries := make([]map[string]any, len(steps))
		for i, s := range steps {
			entry := map[string]any{"id": s.ID}
			if s.Step != "" {
				entry["step"] = s.Step
			}
			if s.LeaseID != "" {
				entry["lease_id"] = s.LeaseID
			}
			if s.AgentName != "" {
				entry["agent_name"] = s.AgentName
			}
			if s.AgentModel != "" {
				entry["agent_model"] = s.AgentModel
			}
			if s.AgentVersion != "" {
				entry["agent_version"] = s.AgentVersion
			}
			entries[i] = entry
		}
		metadata["step_history"] = entries
	}
	return &backend.Bead{
		ID:       id,
		State:    state,
		Metadata: metadata,
	}
}

func makeLeaseBead(id, state, nickname string) backend.Bead {
	return backend.Bead{
		ID:       id,
		State:    state,
		Metadata: map[string]any{"nickname": nickname, "state": state},
	}
}

func TestClassifyTurnFailure_NothingChanged(t *testing.T) {
	b := &backend.Bead{ID: "b", State: "ready_for_implementation"}
	pre := makePreSnapshot("b", "ready_for_implementation", nil, nil)
	pre.Bead = b
	post := makePostSnapshot("b", "ready_for_implementation", nil, nil)
	post.Bead = b
	result := ClassifyTurnFailure(pre, post, nil)
	if result != nil {
		t.Errorf("expected nil when nothing changed, got %v", result)
	}
}

func TestClassifyTurnFailure_ConcurrentClaim(t *testing.T) {
	postSteps := []StepEntry{
		{ID: "step-1", Step: "implementation", LeaseID: "other-lease", AgentName: "OtherAgent", AgentModel: "other", AgentVersion: "1"},
	}
	otherLease := makeLeaseBead("other-lease", "lease_terminated", "kernl:terminal_manager_take:ses-1")
	pre := makePreSnapshot("b", "ready_for_implementation", nil, []backend.Bead{makeLeaseBead("lease-A", "lease_ready", "kernl:ses-1")})
	post := makePostSnapshot("b", "implementation", postSteps, []backend.Bead{
		makeLeaseBead("lease-A", "lease_ready", "kernl:ses-1"),
		otherLease,
	})

	result := ClassifyTurnFailure(pre, post, nil)
	if result == nil {
		t.Fatal("expected concurrent_claim_detected classification")
	}
	if result.Category != CategoryConcurrentClaim {
		t.Errorf("expected concurrent_claim_detected, got %s", result.Category)
	}
	if !strings.Contains(result.Reasoning, "OtherAgent") {
		t.Errorf("reasoning should contain OtherAgent, got: %s", result.Reasoning)
	}
}

func TestClassifyTurnFailure_DoubleClaim(t *testing.T) {
	postSteps := []StepEntry{
		{ID: "step-1", Step: "implementation", LeaseID: "lease-A"},
		{ID: "step-2", Step: "implementation", LeaseID: "lease-A"},
	}
	leaseA := makeLeaseBead("lease-A", "lease_ready", "kernl:ses-1")
	pre := makePreSnapshot("b", "ready_for_implementation", nil, []backend.Bead{leaseA})
	post := makePostSnapshot("b", "implementation", postSteps, []backend.Bead{leaseA})

	result := ClassifyTurnFailure(pre, post, nil)
	if result == nil {
		t.Fatal("expected double claim classification")
	}
	if result.Category != CategoryDoubleClaim {
		t.Errorf("expected our_agent_double_claim_suspected, got %s", result.Category)
	}
	if !strings.Contains(result.Reasoning, "2 new action steps") {
		t.Errorf("reasoning should mention count, got: %s", result.Reasoning)
	}
}

func TestClassifyTurnFailure_HalfTransition(t *testing.T) {
	postSteps := []StepEntry{
		{ID: "step-1", Step: "implementation", LeaseID: "lease-A"},
	}
	leaseA := makeLeaseBead("lease-A", "lease_ready", "kernl:ses-1")
	pre := makePreSnapshot("b", "ready_for_implementation", nil, []backend.Bead{leaseA})
	post := makePostSnapshot("b", "implementation", postSteps, []backend.Bead{leaseA})
	signals := &ClassifierSignals{AgentClaimExitedNonZero: true}

	result := ClassifyTurnFailure(pre, post, signals)
	if result == nil {
		t.Fatal("expected kno_half_transition_suspected")
	}
	if result.Category != CategoryHalfTransition {
		t.Errorf("expected kno_half_transition_suspected, got %s", result.Category)
	}
}

func TestClassifyTurnFailure_HalfTransitionNotFlaggedWithoutSignal(t *testing.T) {
	postSteps := []StepEntry{
		{ID: "step-1", Step: "implementation", LeaseID: "lease-A"},
	}
	leaseA := makeLeaseBead("lease-A", "lease_ready", "kernl:ses-1")
	pre := makePreSnapshot("b", "ready_for_implementation", nil, []backend.Bead{leaseA})
	post := makePostSnapshot("b", "implementation", postSteps, []backend.Bead{leaseA})

	result := ClassifyTurnFailure(pre, post, nil)
	if result == nil {
		t.Fatal("expected a classification")
	}
	if result.Category != CategoryUnknownStateChange {
		t.Errorf("expected unknown_state_change without signal, got %s", result.Category)
	}
}

func TestClassifyTurnFailure_LeaseTerminated(t *testing.T) {
	preLease := makeLeaseBead("lease-A", "lease_ready", "")
	postLease := makeLeaseBead("lease-A", "lease_terminated", "")
	pre := makePreSnapshot("b", "ready_for_implementation", nil, []backend.Bead{preLease})
	post := makePostSnapshot("b", "ready_for_implementation", nil, []backend.Bead{postLease})

	t.Run("unexpected termination", func(t *testing.T) {
		signals := &ClassifierSignals{KernlInitiatedLeaseTerminate: false}
		result := ClassifyTurnFailure(pre, post, signals)
		if result == nil {
			t.Fatal("expected classification")
		}
		if result.Category != CategoryLeaseTerminated {
			t.Errorf("expected lease_terminated_unexpectedly, got %s", result.Category)
		}
	})

	t.Run("initiated by kernl", func(t *testing.T) {
		signals := &ClassifierSignals{KernlInitiatedLeaseTerminate: true}
		result := ClassifyTurnFailure(pre, post, signals)
		if result != nil {
			t.Errorf("expected nil when kernl initiated, got %v", result)
		}
	})
}

func TestClassifyTurnFailure_UnknownStateChange(t *testing.T) {
	preBead := &backend.Bead{ID: "b", State: "ready_for_implementation"}
	postBead := &backend.Bead{ID: "b", State: "ready_for_review"}
	pre := makePreSnapshot("b", "ready_for_implementation", nil, nil)
	pre.Bead = preBead
	post := makePostSnapshot("b", "ready_for_review", nil, nil)
	post.Bead = postBead

	result := ClassifyTurnFailure(pre, post, nil)
	if result == nil {
		t.Fatal("expected classification")
	}
	if result.Category != CategoryUnknownStateChange {
		t.Errorf("expected unknown_state_change, got %s", result.Category)
	}
}

func TestBuildForensicBannerBody(t *testing.T) {
	body := BuildForensicBannerBody(ForensicBannerInput{
		Category:         CategoryConcurrentClaim,
		BeadID:           "maestro-ca91",
		SessionID:        "ses-1",
		LeaseID:          "lease-A",
		Iteration:        3,
		PreSnapshotPath:  "/p/pre.json",
		PostSnapshotPath: "/p/post.json",
		Reasoning:        "another agent took the bead",
	})
	if !strings.Contains(body, DispatchForensicMarker) {
		t.Error("banner should contain KERNL DISPATCH FORENSIC marker")
	}
	if !strings.Contains(body, "concurrent_claim_detected") {
		t.Error("banner should contain category")
	}
	if !strings.Contains(body, "maestro-ca91") {
		t.Error("banner should contain bead id")
	}
	if !strings.Contains(body, "lease-A") {
		t.Error("banner should contain lease id")
	}
	if !strings.Contains(body, "iteration    = 3") {
		t.Error("banner should contain iteration")
	}
	if !strings.Contains(body, "/p/pre.json") {
		t.Error("banner should contain pre snapshot path")
	}
	if !strings.Contains(body, "/p/post.json") {
		t.Error("banner should contain post snapshot path")
	}
	if !strings.Contains(body, "another agent took the bead") {
		t.Error("banner should contain reasoning")
	}
}

func TestSnapshotPath(t *testing.T) {
	p := SnapshotPath(SnapshotPathInput{
		LogRoot:    "/r",
		Date:       "2026-04-30",
		SessionID:  "ses-1",
		BeadID:     "maestro-ca91",
		Boundary:   "post_turn_failure",
		CapturedAt: "2026-04-30T02:19:48.062Z",
	})
	if !strings.Contains(p, "/r/_dispatch_forensics/2026-04-30/ses-1/") {
		t.Errorf("path should contain expected segments: %s", p)
	}
	if !strings.Contains(p, "post_turn_failure") {
		t.Errorf("path should contain boundary: %s", p)
	}
	if !strings.Contains(p, "maestro-ca91") {
		t.Errorf("path should contain bead id: %s", p)
	}
	if !strings.HasSuffix(p, ".json") {
		t.Errorf("path should end with .json: %s", p)
	}
}

func TestCaptureBeadSnapshot_WritesAndAudits(t *testing.T) {
	writer := NewMemorySnapshotWriter("/test/log-root")
	var auditEvents []struct {
		event   string
		payload map[string]any
	}
	beatData := &backend.Bead{ID: "test-bead", State: "ready_for_implementation"}
	leaseData := makeLeaseBead("lease-A", "lease_ready", "kernl:terminal_manager_take:ses-1:test-bead")

	ctx := CaptureContext{
		SessionID: "ses-1",
		BeadID:    "test-bead",
		RepoPath:   "/repo",
		LeaseID:    "lease-A",
		Iteration:  1,
	}

	snapshot := CaptureBeadSnapshot("pre_lease", ctx, &ForensicDeps{
		Writer: writer,
		ShowKnot: func(beadID, repoPath string) (*backend.Bead, error) {
			return beatData, nil
		},
		ListLeases: func(repoPath string, activeOnly bool) ([]backend.Bead, error) {
			return []backend.Bead{leaseData}, nil
		},
		LogAudit: func(event string, payload map[string]any) {
			auditEvents = append(auditEvents, struct {
				event   string
				payload map[string]any
			}{event: event, payload: payload})
		},
		Now: func() string { return "2026-04-30T02:19:41.449Z" },
	})

	if snapshot.Boundary != "pre_lease" {
		t.Errorf("expected boundary pre_lease, got %s", snapshot.Boundary)
	}
	if snapshot.CapturedAt != "2026-04-30T02:19:41.449Z" {
		t.Errorf("expected fixed timestamp, got %s", snapshot.CapturedAt)
	}
	if snapshot.Bead == nil || snapshot.Bead.State != "ready_for_implementation" {
		t.Errorf("expected bead state ready_for_implementation, got %v", snapshot.Bead)
	}
	if len(snapshot.CaptureErrors) > 0 {
		t.Errorf("expected no capture errors, got %v", snapshot.CaptureErrors)
	}
	if len(writer.Snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(writer.Snapshots))
	}
	written := writer.Snapshots[0]
	if !strings.Contains(written.Path, "_dispatch_forensics") {
		t.Errorf("path should contain forensics slug: %s", written.Path)
	}
	if !strings.Contains(written.Path, "pre_lease") {
		t.Errorf("path should contain boundary: %s", written.Path)
	}
	if !strings.Contains(written.Path, "test-bead") {
		t.Errorf("path should contain bead id: %s", written.Path)
	}
	if len(auditEvents) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(auditEvents))
	}
	if auditEvents[0].event != "beat_snapshot_pre_lease" {
		t.Errorf("expected audit event beat_snapshot_pre_lease, got %s", auditEvents[0].event)
	}
}

func TestCaptureBeadSnapshot_RecordsShowKnotError(t *testing.T) {
	writer := NewMemorySnapshotWriter("/test/log-root")
	ctx := CaptureContext{
		SessionID: "ses-1",
		BeadID:    "test-bead",
		RepoPath:   "/repo",
		LeaseID:    "lease-A",
	}

	snapshot := CaptureBeadSnapshot("post_turn_failure", ctx, &ForensicDeps{
		Writer: writer,
		ShowKnot: func(beadID, repoPath string) (*backend.Bead, error) {
			return nil, fmt.Errorf("boom")
		},
		ListLeases: func(repoPath string, activeOnly bool) ([]backend.Bead, error) {
			return []backend.Bead{}, nil
		},
		LogAudit: func(event string, payload map[string]any) {},
	})

	if snapshot.Bead != nil {
		t.Errorf("expected nil bead, got %v", snapshot.Bead)
	}
	if len(snapshot.CaptureErrors) == 0 {
		t.Error("expected capture errors from showKnot failure")
	}
	if !strings.Contains(snapshot.CaptureErrors[0], "showKnot") {
		t.Errorf("expected showKnot error, got %s", snapshot.CaptureErrors[0])
	}
	if len(writer.Snapshots) != 1 {
		t.Error("snapshot should still be written even with showKnot failure")
	}
}

func TestCaptureBeadSnapshot_DoesNotThrowWhenWriterFails(t *testing.T) {
	failingWriter := &failingSnapshotWriter{}
	ctx := CaptureContext{
		SessionID: "ses-1",
		BeadID:    "test-bead",
		RepoPath:   "/repo",
	}

	snapshot := CaptureBeadSnapshot("post_turn_success", ctx, &ForensicDeps{
		Writer: failingWriter,
		ShowKnot: func(beadID, repoPath string) (*backend.Bead, error) {
			return &backend.Bead{ID: "b", State: "x"}, nil
		},
		ListLeases: func(repoPath string, activeOnly bool) ([]backend.Bead, error) {
			return []backend.Bead{}, nil
		},
		LogAudit: func(event string, payload map[string]any) {},
	})

	if snapshot.Boundary != "post_turn_success" {
		t.Errorf("expected snapshot to be returned even on writer failure, got %s", snapshot.Boundary)
	}
}

func TestRunPostTurnForensics_ClassifiedFailure(t *testing.T) {
	var auditCalls []struct {
		event   string
		payload map[string]any
	}
	var sessionBanners []string

	postSteps := []StepEntry{
		{ID: "step-1", Step: "implementation", LeaseID: "other-lease", AgentName: "OtherAgent"},
	}
	otherLease := makeLeaseBead("other-lease", "lease_terminated", "kernl:OtherAgent")
	leaseA := makeLeaseBead("lease-A", "lease_ready", "kernl:ses-1")
	pre := makePreSnapshot("b", "ready_for_implementation", nil, []backend.Bead{leaseA})
	post := makePostSnapshot("b", "implementation", postSteps, []backend.Bead{leaseA, otherLease})

	result := RunPostTurnForensics(pre, post, "/p/pre.json", "/p/post.json", nil, &ForensicDeps{
		LogAudit: func(event string, payload map[string]any) {
			auditCalls = append(auditCalls, struct {
				event   string
				payload map[string]any
			}{event: event, payload: payload})
		},
		PushBanner: func(banner string) {
			sessionBanners = append(sessionBanners, banner)
		},
	})

	if !result.Classified {
		t.Error("expected classified=true for concurrent claim")
	}
	if !strings.Contains(result.BannerBody, DispatchForensicMarker) {
		t.Error("banner should contain KERNL DISPATCH FORENSIC marker")
	}
	if len(auditCalls) != 1 {
		t.Fatalf("expected 1 audit call, got %d", len(auditCalls))
	}
	if auditCalls[0].event != "dispatch_forensic_classified" {
		t.Errorf("expected audit event dispatch_forensic_classified, got %s", auditCalls[0].event)
	}
	if auditCalls[0].payload["category"] != "concurrent_claim_detected" {
		t.Errorf("expected category concurrent_claim_detected, got %v", auditCalls[0].payload["category"])
	}
	if len(sessionBanners) != 1 {
		t.Fatalf("expected 1 session banner, got %d", len(sessionBanners))
	}
	if !strings.Contains(sessionBanners[0], DispatchForensicMarker) {
		t.Error("banner should contain KERNL DISPATCH FORENSIC marker")
	}
}

func TestRunPostTurnForensics_NoChange(t *testing.T) {
	b := &backend.Bead{ID: "b", State: "ready_for_implementation"}
	leaseA := makeLeaseBead("lease-A", "lease_ready", "")
	pre := makePreSnapshot("b", "ready_for_implementation", nil, []backend.Bead{leaseA})
	pre.Bead = b
	post := makePostSnapshot("b", "ready_for_implementation", nil, []backend.Bead{leaseA})
	post.Bead = b

	result := RunPostTurnForensics(pre, post, "/p/pre.json", "/p/post.json", nil, &ForensicDeps{
		LogAudit:   func(event string, payload map[string]any) {},
		PushBanner: func(banner string) {},
	})

	if result.Classified {
		t.Error("expected classified=false when nothing changed")
	}
}

func TestLeasesForBead(t *testing.T) {
	all := []backend.Bead{
		{ID: "lease-1", Metadata: map[string]any{"nickname": "kernl:terminal:ses-1:bead-x"}},
		{ID: "lease-2", Metadata: map[string]any{"nickname": "kernl:terminal:ses-2:bead-y"}},
		{ID: "lease-3", Metadata: map[string]any{"nickname": "other"}},
	}

	matched := leasesForBead("bead-x", "ses-1", all)
	if len(matched) != 1 || matched[0].ID != "lease-1" {
		t.Errorf("expected lease-1 to match, got %v", matched)
	}

	var short []backend.Bead
	for i := 0; i < 5; i++ {
		short = append(short, backend.Bead{ID: fmt.Sprintf("l-%d", i)})
	}
	matched2 := leasesForBead("nothing", "nothing", short)
	if len(matched2) != 5 {
		t.Errorf("expected all leases when no match and list short, got %d", len(matched2))
	}

	large := make([]backend.Bead, 15)
	matched3 := leasesForBead("nothing", "nothing", large)
	if len(matched3) != 0 {
		t.Errorf("expected empty when no match in large list, got %d", len(matched3))
	}
}

type failingSnapshotWriter struct{}

func (f *failingSnapshotWriter) Write(snapshot BeadSnapshot) (string, error) {
	return "", fmt.Errorf("disk full")
}