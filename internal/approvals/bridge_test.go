package approvals

import (
	"context"
	"strings"
	"testing"
	"time"
)

func claudePayload(tool string, input map[string]any) map[string]any {
	return map[string]any{"tool_name": tool, "input": input, "tool_use_id": "toolu_1"}
}

func fastContext() RequestContext {
	return RequestContext{
		SessionID: "sess-1", BeadID: "kernl-1", RepoPath: "/repo", AgentName: "claude",
		Timeout: 2 * time.Second, Poll: 5 * time.Millisecond,
	}
}

// The gate must actually block: an approval that returns before a human
// answered is not a gate, it is a log line.
func TestGateBlocksUntilAnswered(t *testing.T) {
	s := newTestStore(t)
	done := make(chan GateOutcome, 1)

	go func() {
		outcome, err := Gate(context.Background(), s, "claude", claudePayload("Bash", map[string]any{"command": "rm -rf /"}), fastContext())
		if err != nil {
			t.Errorf("Gate: %v", err)
		}
		done <- outcome
	}()

	id := waitForPending(t, s)
	select {
	case outcome := <-done:
		t.Fatalf("Gate returned before anyone answered: %+v", outcome)
	case <-time.After(50 * time.Millisecond):
	}

	if _, err := s.Decide(id, ActionAllow, ""); err != nil {
		t.Fatalf("Decide: %v", err)
	}
	select {
	case outcome := <-done:
		if !outcome.Allowed {
			t.Errorf("an allowed gate must come back allowed: %+v", outcome)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Gate never noticed the answer")
	}
}

func TestGateCarriesTheDispatchContext(t *testing.T) {
	s := newTestStore(t)
	go func() {
		_, _ = Gate(context.Background(), s, "claude", claudePayload("Write", map[string]any{"file_path": "/tmp/x"}), fastContext())
	}()

	id := waitForPending(t, s)
	req, err := s.Get(id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if req.BeadID != "kernl-1" || req.SessionID != "sess-1" || req.RepoPath != "/repo" || req.AgentName != "claude" {
		t.Errorf("the listing cannot be read without the run's context: %+v", req)
	}
	if req.ParameterSummary != "/tmp/x" {
		t.Errorf("want the path as the summary, got %q", req.ParameterSummary)
	}
}

func TestGateDeniedCarriesTheReasonBack(t *testing.T) {
	s := newTestStore(t)
	done := make(chan GateOutcome, 1)
	go func() {
		outcome, _ := Gate(context.Background(), s, "claude", claudePayload("Bash", nil), fastContext())
		done <- outcome
	}()

	id := waitForPending(t, s)
	if _, err := s.Decide(id, ActionDeny, "not on production"); err != nil {
		t.Fatalf("Decide: %v", err)
	}

	outcome := <-done
	if outcome.Allowed {
		t.Fatal("a denied gate must not report allowed")
	}
	if outcome.Reason != "not on production" {
		t.Errorf("the agent must be told why, got %q", outcome.Reason)
	}
}

// D6: an unanswered gate is denied with a reason, and the record says it
// expired rather than that someone refused it.
func TestGateExpiresIntoADenial(t *testing.T) {
	s := newTestStore(t)
	rc := fastContext()
	rc.Timeout = 30 * time.Millisecond

	outcome, err := Gate(context.Background(), s, "claude", claudePayload("Bash", nil), rc)
	if err != nil {
		t.Fatalf("Gate: %v", err)
	}
	if outcome.Allowed {
		t.Fatal("an unanswered gate must never come back allowed")
	}
	if !strings.Contains(outcome.Reason, "no answer") {
		t.Errorf("the denial must say why, got %q", outcome.Reason)
	}

	req, err := s.Get(outcome.ApprovalID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if req.Status != StatusExpired {
		t.Errorf("want the record to read %s, got %s", StatusExpired, req.Status)
	}
}

func TestGateHonoursARememberedAllow(t *testing.T) {
	s := newTestStore(t)
	seed := mustCreate(t, s, &ApprovalRequest{SessionID: "sess-1", ToolName: "Bash"})
	if _, err := s.Decide(seed.ID, ActionAllowAlways, ""); err != nil {
		t.Fatalf("Decide: %v", err)
	}

	// No answerer is running, so anything but an immediate return hangs.
	outcome, err := Gate(context.Background(), s, "claude", claudePayload("Bash", nil), fastContext())
	if err != nil {
		t.Fatalf("Gate: %v", err)
	}
	if !outcome.Allowed || !outcome.Remembered {
		t.Errorf("a remembered session decision must allow without asking again: %+v", outcome)
	}

	pending, err := s.List(ApprovalFilter{ActiveOnly: true})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("a remembered allow must not park a new gate, got %d", len(pending))
	}
}

// A cancelled run leaves no answerer behind, so the parked request must not
// keep inviting an answer nobody is waiting on.
func TestGateCancelledRunClosesTheRequest(t *testing.T) {
	s := newTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := Gate(ctx, s, "claude", claudePayload("Bash", nil), fastContext())
		errCh <- err
	}()

	id := waitForPending(t, s)
	cancel()

	if err := <-errCh; err == nil {
		t.Fatal("a cancelled gate must report an error, not a decision")
	}
	req, err := s.Get(id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if req.Actionable {
		t.Error("a gate whose run is gone must stop being actionable")
	}
}

func TestGateRefusesAnUnknownAdapter(t *testing.T) {
	s := newTestStore(t)
	_, err := Gate(context.Background(), s, "codex", claudePayload("Bash", nil), fastContext())
	if err == nil {
		t.Fatal("an adapter with no measured payload layout must be refused by name")
	}
	if !strings.Contains(err.Error(), "codex") {
		t.Errorf("the error must name the adapter, got %v", err)
	}
}

func waitForPending(t *testing.T, s *Store) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		pending, err := s.List(ApprovalFilter{ActiveOnly: true})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(pending) == 1 {
			return pending[0].ID
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("no gate was ever parked")
	return ""
}
