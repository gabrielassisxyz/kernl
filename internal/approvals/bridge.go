package approvals

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// DefaultTimeout is how long a gate waits for a human before giving up.
//
// The agent side imposes no limit of its own - a permission call held for 15
// minutes was answered normally - so the deadline is entirely kernl's policy,
// and it exists because a gate nobody answers holds an agent process and one
// of the orchestrator's concurrency slots for as long as it waits.
const DefaultTimeout = 30 * time.Minute

// DefaultPollInterval is how often a waiting bridge re-reads the store. The
// store is the transport between processes, so this is the latency between a
// human answering and the agent resuming; a third of a second is below what a
// person notices and costs one stat() per tick.
const DefaultPollInterval = 300 * time.Millisecond

// RequestContext is what the dispatch knows about the run that raised a gate.
// Without it a listing can only say "some agent wants to run Bash", which is
// not enough to decide anything.
type RequestContext struct {
	SessionID string
	BeadID    string
	RepoPath  string
	AgentName string
	Timeout   time.Duration
	Poll      time.Duration
}

// GateOutcome is the answer handed back to the blocked agent.
type GateOutcome struct {
	Allowed    bool
	Reason     string
	ApprovalID string
	// Remembered reports that no human was asked because this session had
	// already answered this tool with always_approve.
	Remembered bool
}

// Gate parks one permission request and blocks until it is answered, the
// deadline passes, or the context is cancelled.
//
// It is the whole policy of the approval subsystem in one function, shared by
// every adapter: the transports differ (an MCP tool call for claude, a
// subprocess for the pi extension) but what a gate MEANS must not.
func Gate(ctx context.Context, s *Store, adapter string, raw map[string]any, rc RequestContext) (GateOutcome, error) {
	req, err := ExtractApprovalRequest(adapter, raw)
	if err != nil {
		return GateOutcome{}, err
	}
	req.SessionID = rc.SessionID
	req.BeadID = rc.BeadID
	req.RepoPath = rc.RepoPath
	req.AgentName = rc.AgentName

	remembered, err := s.RememberedAllow(rc.SessionID, req.ToolName)
	if err != nil {
		return GateOutcome{}, err
	}
	if remembered {
		slog.Info("[approval-bridge] gate.remembered", "tool", req.ToolName, "session", rc.SessionID)
		return GateOutcome{Allowed: true, Remembered: true, Reason: "already approved for this session"}, nil
	}

	timeout := rc.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	created, err := s.Create(req, timeout)
	if err != nil {
		return GateOutcome{}, err
	}
	slog.Info("[approval-bridge] gate.raised",
		"approvalId", created.ID, "tool", created.ToolName, "bead", created.BeadID, "session", created.SessionID)

	return awaitDecision(ctx, s, created, rc.Poll, timeout)
}

func awaitDecision(ctx context.Context, s *Store, req *ApprovalRequest, poll, timeout time.Duration) (GateOutcome, error) {
	if poll <= 0 {
		poll = DefaultPollInterval
	}
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	for {
		decision, ok, err := s.Decision(req.ID)
		if err != nil {
			return GateOutcome{}, err
		}
		if ok {
			return outcomeFor(req.ID, decision), nil
		}

		select {
		case <-ctx.Done():
			// The run was cancelled or the agent went away. Record it so the
			// listing does not keep offering a gate nobody is waiting on.
			return GateOutcome{}, expire(s, req, "the run ended before the approval was answered", ctx.Err())
		case <-deadline.C:
			if _, err := s.Decide(req.ID, ActionExpire, expiryReason(timeout)); err != nil {
				return GateOutcome{}, err
			}
			slog.Info("[approval-bridge] gate.expired", "approvalId", req.ID, "after", timeout.String())
			return GateOutcome{Allowed: false, Reason: expiryReason(timeout), ApprovalID: req.ID}, nil
		case <-ticker.C:
		}
	}
}

func expire(s *Store, req *ApprovalRequest, reason string, cause error) error {
	if _, err := s.Decide(req.ID, ActionExpire, reason); err != nil {
		return err
	}
	return cause
}

func expiryReason(timeout time.Duration) string {
	return fmt.Sprintf("denied automatically: no answer within %s", timeout)
}

func outcomeFor(id string, d *Decision) GateOutcome {
	switch d.Action {
	case ActionAllow, ActionAllowAlways:
		return GateOutcome{Allowed: true, ApprovalID: id, Reason: d.Reason}
	default:
		reason := d.Reason
		if reason == "" {
			reason = "the human declined this action"
		}
		return GateOutcome{Allowed: false, ApprovalID: id, Reason: reason}
	}
}
