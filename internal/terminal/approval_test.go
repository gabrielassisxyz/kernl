package terminal

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestPerformApprovalAction_NotFound(t *testing.T) {
	m := NewTerminalManager()
	entry, _ := m.CreateSession(context.TODO(), "bead-1", "/repo")

	result := PerformApprovalAction(entry, "nonexistent", ActionAccept)
	if result.OK {
		t.Error("expected OK=false for nonexistent approval")
	}
	if result.HTTPStatus != 404 {
		t.Errorf("expected httpStatus=404, got %d", result.HTTPStatus)
	}
	if result.Error != "Approval request not found" {
		t.Errorf("unexpected error: %s", result.Error)
	}
}

func TestPerformApprovalAction_UnsupportedAction(t *testing.T) {
	m := NewTerminalManager()
	entry, _ := m.CreateSession(context.TODO(), "bead-1", "/repo")

	rec := &PendingApprovalRecord{
		ApprovalID:       "approval-1",
		Status:           ApprovalPending,
		SupportedActions: []string{"decline"},
		ReplyTarget:      &ApprovalReplyTarget{Adapter: "opencode", Transport: ReplyTransportHTTP},
		Actionable:       true,
	}
	entry.RecordPendingApproval(rec)

	result := PerformApprovalAction(entry, "approval-1", ActionAccept)
	if result.OK {
		t.Error("expected OK=false for unsupported action")
	}
	if result.HTTPStatus != 409 {
		t.Errorf("expected httpStatus=409, got %d", result.HTTPStatus)
	}
	if result.Code != "approval_action_not_supported" {
		t.Errorf("expected code=approval_action_not_supported, got %s", result.Code)
	}

	found, _ := entry.GetPendingApproval("approval-1")
	if found.Status != ApprovalUnsupported {
		t.Errorf("expected status=unsupported, got %s", found.Status)
	}
	if found.FailureReason != "approval_action_not_supported" {
		t.Errorf("expected failureReason=approval_action_not_supported, got %s", found.FailureReason)
	}
}

func TestPerformApprovalAction_MissingReplyTarget(t *testing.T) {
	m := NewTerminalManager()
	entry, _ := m.CreateSession(context.TODO(), "bead-1", "/repo")

	rec := &PendingApprovalRecord{
		ApprovalID:       "approval-1",
		Status:           ApprovalPending,
		SupportedActions: []string{"accept"},
		ReplyTarget:      nil,
		Actionable:       true,
	}
	entry.RecordPendingApproval(rec)

	result := PerformApprovalAction(entry, "approval-1", ActionAccept)
	if result.OK {
		t.Error("expected OK=false for missing reply target")
	}
	if result.Code != "approval_reply_target_missing" {
		t.Errorf("expected code=approval_reply_target_missing, got %s", result.Code)
	}
}

func TestPerformApprovalAction_NoResponder(t *testing.T) {
	m := NewTerminalManager()
	entry, _ := m.CreateSession(context.TODO(), "bead-1", "/repo")

	rec := &PendingApprovalRecord{
		ApprovalID:       "approval-1",
		Status:           ApprovalPending,
		SupportedActions: []string{"accept"},
		ReplyTarget:      &ApprovalReplyTarget{Adapter: "opencode", Transport: ReplyTransportHTTP},
		Actionable:       true,
	}
	entry.RecordPendingApproval(rec)

	result := PerformApprovalAction(entry, "approval-1", ActionAccept)
	if result.OK {
		t.Error("expected OK=false for no responder")
	}
	if result.Code != "approval_responder_unavailable" {
		t.Errorf("expected code=approval_responder_unavailable, got %s", result.Code)
	}
}

func TestPerformApprovalAction_Success(t *testing.T) {
	m := NewTerminalManager()
	entry, _ := m.CreateSession(context.TODO(), "bead-1", "/repo")

	rec := &PendingApprovalRecord{
		ApprovalID:       "approval-1",
		Status:           ApprovalPending,
		SupportedActions: []string{"accept", "decline"},
		ReplyTarget:      &ApprovalReplyTarget{Adapter: "opencode", Transport: ReplyTransportHTTP, NativeSessionID: "ses_1", PermissionID: "perm_1"},
		Actionable:       true,
	}
	entry.RecordPendingApproval(rec)

	var responderCalled bool
	entry.SetApprovalResponder(func(record *PendingApprovalRecord, action ApprovalAction) (*ApprovalReplyResult, error) {
		responderCalled = true
		return &ApprovalReplyResult{OK: true, Status: "approved"}, nil
	})

	result := PerformApprovalAction(entry, "approval-1", ActionAccept)
	if !result.OK {
		t.Errorf("expected OK=true, got false")
	}
	if result.HTTPStatus != 200 {
		t.Errorf("expected httpStatus=200, got %d", result.HTTPStatus)
	}
	if !responderCalled {
		t.Error("expected responder to be called")
	}

	found, _ := entry.GetPendingApproval("approval-1")
	if found.Status != ApprovalApproved {
		t.Errorf("expected status=approved, got %s", found.Status)
	}
	if found.FailureReason != "" {
		t.Errorf("expected empty failureReason, got %s", found.FailureReason)
	}
}

func TestPerformApprovalAction_DeclineRejects(t *testing.T) {
	m := NewTerminalManager()
	entry, _ := m.CreateSession(context.TODO(), "bead-1", "/repo")

	rec := &PendingApprovalRecord{
		ApprovalID:       "approval-2",
		Status:           ApprovalPending,
		SupportedActions: []string{"accept", "decline"},
		ReplyTarget:      &ApprovalReplyTarget{Adapter: "opencode", Transport: ReplyTransportHTTP, PermissionID: "perm_1"},
		Actionable:       true,
	}
	entry.RecordPendingApproval(rec)

	entry.SetApprovalResponder(func(record *PendingApprovalRecord, action ApprovalAction) (*ApprovalReplyResult, error) {
		return &ApprovalReplyResult{OK: true, Status: "rejected"}, nil
	})

	result := PerformApprovalAction(entry, "approval-2", ActionDecline)
	if !result.OK {
		t.Errorf("expected OK=true, got false")
	}

	found, _ := entry.GetPendingApproval("approval-2")
	if found.Status != ApprovalRejected {
		t.Errorf("expected status=rejected, got %s", found.Status)
	}
}

func TestPerformApprovalAction_AlwaysApprove(t *testing.T) {
	m := NewTerminalManager()
	entry, _ := m.CreateSession(context.TODO(), "bead-1", "/repo")

	rec := &PendingApprovalRecord{
		ApprovalID:       "approval-3",
		Status:           ApprovalPending,
		SupportedActions: []string{"accept", "always_approve", "decline"},
		ReplyTarget:      &ApprovalReplyTarget{Adapter: "opencode", Transport: ReplyTransportHTTP},
		Actionable:       true,
	}
	entry.RecordPendingApproval(rec)

	entry.SetApprovalResponder(func(record *PendingApprovalRecord, action ApprovalAction) (*ApprovalReplyResult, error) {
		return &ApprovalReplyResult{OK: true, Status: "always_approved"}, nil
	})

	result := PerformApprovalAction(entry, "approval-3", ActionAlwaysApprove)
	if !result.OK {
		t.Errorf("expected OK=true, got false")
	}

	found, _ := entry.GetPendingApproval("approval-3")
	if found.Status != ApprovalAlwaysApproved {
		t.Errorf("expected status=always_approved, got %s", found.Status)
	}
}

func TestPerformApprovalAction_ReplyFailed(t *testing.T) {
	m := NewTerminalManager()
	entry, _ := m.CreateSession(context.TODO(), "bead-1", "/repo")

	rec := &PendingApprovalRecord{
		ApprovalID:       "approval-4",
		Status:           ApprovalPending,
		SupportedActions: []string{"accept"},
		ReplyTarget:      &ApprovalReplyTarget{Adapter: "opencode", Transport: ReplyTransportHTTP, NativeSessionID: "ses_1", PermissionID: "perm_1"},
		Actionable:       true,
	}
	entry.RecordPendingApproval(rec)

	entry.SetApprovalResponder(func(record *PendingApprovalRecord, action ApprovalAction) (*ApprovalReplyResult, error) {
		return &ApprovalReplyResult{OK: false, Status: "reply_failed", Reason: "network_down"}, nil
	})

	result := PerformApprovalAction(entry, "approval-4", ActionAccept)
	if result.OK {
		t.Error("expected OK=false")
	}
	if result.HTTPStatus != 502 {
		t.Errorf("expected httpStatus=502, got %d", result.HTTPStatus)
	}
	if result.Code != "approval_reply_failed" {
		t.Errorf("expected code=approval_reply_failed, got %s", result.Code)
	}

	found, _ := entry.GetPendingApproval("approval-4")
	if found.Status != ApprovalReplyFailed {
		t.Errorf("expected status=reply_failed, got %s", found.Status)
	}
	if found.FailureReason != "network_down" {
		t.Errorf("expected failureReason=network_down, got %s", found.FailureReason)
	}

	if len(entry.Buffer) == 0 {
		t.Error("expected failure event in buffer")
	} else {
		last := entry.Buffer[len(entry.Buffer)-1]
		if last.Type != "stderr" {
			t.Errorf("expected event type=stderr, got %s", last.Type)
		}
		if !strings.Contains(last.Content, "network_down") {
			t.Errorf("expected event content to contain 'network_down', got %s", last.Content)
		}
		if !strings.Contains(last.Content, "approval-4") {
			t.Errorf("expected event content to contain 'approval-4', got %s", last.Content)
		}
	}
}

func TestPerformApprovalAction_RetrySuccessClearsFailureReason(t *testing.T) {
	m := NewTerminalManager()
	entry, _ := m.CreateSession(context.TODO(), "bead-1", "/repo")

	rec := &PendingApprovalRecord{
		ApprovalID:       "approval-5",
		Status:           ApprovalPending,
		SupportedActions: []string{"accept"},
		ReplyTarget:      &ApprovalReplyTarget{Adapter: "opencode", Transport: ReplyTransportHTTP, NativeSessionID: "ses_1", PermissionID: "perm_1"},
		Actionable:       true,
	}
	entry.RecordPendingApproval(rec)

	callCount := 0
	entry.SetApprovalResponder(func(record *PendingApprovalRecord, action ApprovalAction) (*ApprovalReplyResult, error) {
		callCount++
		if callCount == 1 {
			return &ApprovalReplyResult{OK: false, Reason: "opencode_http_404"}, nil
		}
		return &ApprovalReplyResult{OK: true, Status: "approved"}, nil
	})

	failed := PerformApprovalAction(entry, "approval-5", ActionAccept)
	if failed.OK {
		t.Error("expected first call to fail")
	}

	found, _ := entry.GetPendingApproval("approval-5")
	if found.FailureReason != "opencode_http_404" {
		t.Errorf("expected failureReason=opencode_http_404, got %s", found.FailureReason)
	}

	ok := PerformApprovalAction(entry, "approval-5", ActionAccept)
	if !ok.OK {
		t.Error("expected second call to succeed")
	}

	found, _ = entry.GetPendingApproval("approval-5")
	if found.Status != ApprovalApproved {
		t.Errorf("expected status=approved, got %s", found.Status)
	}
	if found.FailureReason != "" {
		t.Errorf("expected empty failureReason after success, got %s", found.FailureReason)
	}
}

func TestPerformApprovalAction_ResponderError(t *testing.T) {
	m := NewTerminalManager()
	entry, _ := m.CreateSession(context.TODO(), "bead-1", "/repo")

	rec := &PendingApprovalRecord{
		ApprovalID:       "approval-6",
		Status:           ApprovalPending,
		SupportedActions: []string{"accept"},
		ReplyTarget:      &ApprovalReplyTarget{Adapter: "opencode", Transport: ReplyTransportHTTP},
		Actionable:       true,
	}
	entry.RecordPendingApproval(rec)

	entry.SetApprovalResponder(func(record *PendingApprovalRecord, action ApprovalAction) (*ApprovalReplyResult, error) {
		return nil, fmt.Errorf("connection_refused")
	})

	result := PerformApprovalAction(entry, "approval-6", ActionAccept)
	if result.OK {
		t.Error("expected OK=false")
	}
	if result.Code != "approval_reply_failed" {
		t.Errorf("expected code=approval_reply_failed, got %s", result.Code)
	}

	found, _ := entry.GetPendingApproval("approval-6")
	if found.Status != ApprovalReplyFailed {
		t.Errorf("expected status=reply_failed, got %s", found.Status)
	}
	if found.FailureReason != "connection_refused" {
		t.Errorf("expected failureReason=connection_refused, got %s", found.FailureReason)
	}
}

func TestPerformApprovalAction_ClaudeBridgeSuccess(t *testing.T) {
	m := NewTerminalManager()
	entry, _ := m.CreateSession(context.TODO(), "bead-1", "/repo")

	rec := &PendingApprovalRecord{
		ApprovalID:       "approval-7",
		Status:           ApprovalPending,
		SupportedActions: []string{"accept", "decline"},
		ReplyTarget:      &ApprovalReplyTarget{Adapter: "claude-bridge", Transport: ReplyTransportStdio, RequestID: "toolu_1"},
		Actionable:       true,
	}
	entry.RecordPendingApproval(rec)

	entry.SetApprovalResponder(func(record *PendingApprovalRecord, action ApprovalAction) (*ApprovalReplyResult, error) {
		if record.ReplyTarget.Adapter == "claude-bridge" {
			return &ApprovalReplyResult{OK: true}, nil
		}
		return &ApprovalReplyResult{OK: false, Status: "unsupported"}, nil
	})

	result := PerformApprovalAction(entry, "approval-7", ActionAccept)
	if !result.OK {
		t.Errorf("expected OK=true for Claude bridge approval, got false")
	}

	found, _ := entry.GetPendingApproval("approval-7")
	if found.Status != ApprovalApproved {
		t.Errorf("expected status=approved, got %s", found.Status)
	}
}

func TestApprovalStatusForAction(t *testing.T) {
	tests := []struct {
		action   ApprovalAction
		expected ApprovalStatus
	}{
		{ActionAccept, ApprovalApproved},
		{ActionAlwaysApprove, ApprovalAlwaysApproved},
		{ActionDecline, ApprovalRejected},
	}
	for _, tt := range tests {
		result := ApprovalStatusForAction(tt.action)
		if result != tt.expected {
			t.Errorf("ApprovalStatusForAction(%s) = %s, want %s", tt.action, result, tt.expected)
		}
	}
}

func TestIsTerminalApprovalStatus(t *testing.T) {
	terminal := []ApprovalStatus{ApprovalApproved, ApprovalAlwaysApproved, ApprovalRejected, ApprovalDismissed}
	for _, s := range terminal {
		if !IsTerminalApprovalStatus(s) {
			t.Errorf("expected %s to be terminal", s)
		}
	}

	nonTerminal := []ApprovalStatus{ApprovalPending, ApprovalResponding, ApprovalReplyFailed, ApprovalUnsupported}
	for _, s := range nonTerminal {
		if IsTerminalApprovalStatus(s) {
			t.Errorf("expected %s to NOT be terminal", s)
		}
	}
}

func TestNormalizeSupportedActions(t *testing.T) {
	result := NormalizeSupportedActions([]ApprovalAction{ActionAccept, ActionDecline})
	if len(result) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(result))
	}
	if result[0] != ActionAccept || result[1] != ActionDecline {
		t.Errorf("unexpected actions: %v", result)
	}

	result = NormalizeSupportedActions(nil)
	if len(result) != 0 {
		t.Errorf("expected empty for nil, got %v", result)
	}

	invalid := NormalizeSupportedActions([]ApprovalAction{"invalid_action"})
	if len(invalid) != 0 {
		t.Errorf("expected empty for invalid actions, got %v", invalid)
	}
}

func TestCleanupSessionResources_MarksManualRequired(t *testing.T) {
	m := NewTerminalManager()
	entry, _ := m.CreateSession(context.TODO(), "bead-1", "/repo")

	entry.RecordPendingApproval(&PendingApprovalRecord{
		ApprovalID: "approval-1",
		Status:     ApprovalPending,
		Actionable: true,
	})

	CleanupSessionResources(entry, "session_aborted")

	rec, _ := entry.GetPendingApproval("approval-1")
	if rec.Status != ApprovalManualRequired {
		t.Errorf("expected status=manual_required, got %s", rec.Status)
	}
	if rec.Actionable {
		t.Error("expected actionable=false")
	}
	if rec.ActionableReason != "approval_responder_unavailable" {
		t.Errorf("expected actionableReason=approval_responder_unavailable, got %s", rec.ActionableReason)
	}
}

func TestCleanupSessionResources_MultipleApprovals(t *testing.T) {
	m := NewTerminalManager()
	entry, _ := m.CreateSession(context.TODO(), "bead-1", "/repo")

	for i := 0; i < 3; i++ {
		entry.RecordPendingApproval(&PendingApprovalRecord{
			ApprovalID: fmt.Sprintf("approval-%d", i),
			Status:     ApprovalPending,
			Actionable: true,
		})
	}

	CleanupSessionResources(entry, "session_aborted")

	for i := 0; i < 3; i++ {
		rec, _ := entry.GetPendingApproval(fmt.Sprintf("approval-%d", i))
		if rec.Status != ApprovalManualRequired {
			t.Errorf("approval-%d: expected status=manual_required, got %s", i, rec.Status)
		}
		if rec.Actionable {
			t.Errorf("approval-%d: expected actionable=false", i)
		}
	}
}

func TestPendingApprovalRecord_Fields(t *testing.T) {
	m := NewTerminalManager()
	entry, _ := m.CreateSession(context.TODO(), "bead-1", "/repo")

	rec := &PendingApprovalRecord{
		ApprovalID:       "approval-1",
		Status:           ApprovalPending,
		SupportedActions: []string{"accept", "decline"},
		NativeSessionID:  "ses_1",
		ReplyTarget: &ApprovalReplyTarget{
			Adapter:         "opencode",
			Transport:       ReplyTransportHTTP,
			NativeSessionID: "ses_1",
			PermissionID:    "perm_1",
		},
		Actionable:   true,
		BeadID:       "bead-1",
		BeadTitle:    "Fix bug",
		RepoPath:     "/repo",
		Adapter:      "opencode",
		Source:       "permission.asked",
		RequestID:    "req_1",
		PermissionID: "perm_1",
		AgentName:    "OpenCode",
	}
	entry.RecordPendingApproval(rec)

	found, ok := entry.GetPendingApproval("approval-1")
	if !ok {
		t.Fatal("expected to find approval")
	}
	if found.BeadID != "bead-1" {
		t.Errorf("expected beadId=bead-1, got %s", found.BeadID)
	}
	if found.NativeSessionID != "ses_1" {
		t.Errorf("expected nativeSessionId=ses_1, got %s", found.NativeSessionID)
	}
	if found.ReplyTarget == nil {
		t.Error("expected non-nil replyTarget")
	} else {
		if found.ReplyTarget.Adapter != "opencode" {
			t.Errorf("expected adapter=opencode, got %s", found.ReplyTarget.Adapter)
		}
		if found.ReplyTarget.Transport != ReplyTransportHTTP {
			t.Errorf("expected transport=http, got %s", found.ReplyTarget.Transport)
		}
	}
	if found.AgentName != "OpenCode" {
		t.Errorf("expected agentName=OpenCode, got %s", found.AgentName)
	}
}
