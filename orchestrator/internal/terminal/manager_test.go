package terminal

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/session"
)

func TestNewTerminalManager(t *testing.T) {
	m := NewTerminalManager()
	if m == nil {
		t.Fatal("expected non-nil manager")
	}
	if m.maxSessions != DefaultMaxSessions {
		t.Errorf("expected maxSessions=%d, got %d", DefaultMaxSessions, m.maxSessions)
	}
}

func TestNewTerminalManagerWithMaxSessions(t *testing.T) {
	m := NewTerminalManager(WithMaxSessions(3))
	if m.maxSessions != 3 {
		t.Errorf("expected maxSessions=3, got %d", m.maxSessions)
	}
}

func TestCreateSession(t *testing.T) {
	m := NewTerminalManager(WithMaxSessions(5))
	entry, err := m.CreateSession(context.Background(), "beat-1", "/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry == nil {
		t.Fatal("expected non-nil entry")
	}
	if entry.Session.ID == "" {
		t.Error("expected non-empty session ID")
	}
	if entry.Session.BeatID != "beat-1" {
		t.Errorf("expected beatId=beat-1, got %s", entry.Session.BeatID)
	}
	if entry.Session.RepoPath != "/repo" {
		t.Errorf("expected repoPath=/repo, got %s", entry.Session.RepoPath)
	}
	if entry.Session.Status != StatusRunning {
		t.Errorf("expected status=running, got %s", entry.Session.Status)
	}
	if entry.Session.StartedAt == "" {
		t.Error("expected non-empty startedAt")
	}
	if cap(entry.Events) != MaxBuffer {
		t.Errorf("expected events cap=%d, got %d", MaxBuffer, cap(entry.Events))
	}
	if entry.TakeLoopLifecycle == nil {
		t.Error("expected non-nil takeLoopLifecycle")
	}
	if entry.PendingApprovals == nil {
		t.Error("expected non-nil pendingApprovals")
	}
	if entry.ClaimsPerQueueType == nil {
		t.Error("expected non-nil claimsPerQueueType")
	}
}

func TestCreateSessionMaxConcurrent(t *testing.T) {
	m := NewTerminalManager(WithMaxSessions(2))

	for i := 0; i < 2; i++ {
		_, err := m.CreateSession(context.Background(), "beat-1", "/repo")
		if err != nil {
			t.Fatalf("unexpected error creating session %d: %v", i, err)
		}
	}

	_, err := m.CreateSession(context.Background(), "beat-1", "/repo")
	if err == nil {
		t.Error("expected error for max concurrent sessions")
	}
}

func TestGetSession(t *testing.T) {
	m := NewTerminalManager()
	entry, _ := m.CreateSession(context.Background(), "beat-1", "/repo")

	found, ok := m.GetSession(entry.Session.ID)
	if !ok {
		t.Error("expected to find session")
	}
	if found.Session.ID != entry.Session.ID {
		t.Error("session ID mismatch")
	}

	_, ok = m.GetSession("nonexistent")
	if ok {
		t.Error("expected not to find nonexistent session")
	}
}

func TestListSessions(t *testing.T) {
	m := NewTerminalManager()
	m.CreateSession(context.Background(), "beat-1", "/repo1")
	m.CreateSession(context.Background(), "beat-2", "/repo2")

	sessions := m.ListSessions()
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestAbortSessionNotFound(t *testing.T) {
	m := NewTerminalManager()
	outcome := m.AbortSession("nonexistent")
	if outcome.OK {
		t.Error("expected OK=false for nonexistent session")
	}
	if outcome.Reason != "not_found" {
		t.Errorf("expected reason=not_found, got %s", outcome.Reason)
	}
}

func TestAbortSession(t *testing.T) {
	m := NewTerminalManager()
	entry, _ := m.CreateSession(context.Background(), "beat-1", "/repo")

	outcome := m.AbortSession(entry.Session.ID)
	if !outcome.OK {
		t.Errorf("expected OK=true, got false")
	}
	if outcome.Session.Status != StatusAborted {
		t.Errorf("expected status=aborted, got %s", outcome.Session.Status)
	}

	entry.mu.RLock()
	status := entry.Session.Status
	entry.mu.RUnlock()
	if status != StatusAborted {
		t.Errorf("expected entry status=aborted, got %s", status)
	}
}

func TestAbortSessionIdempotent(t *testing.T) {
	m := NewTerminalManager()
	entry, _ := m.CreateSession(context.Background(), "beat-1", "/repo")

	m.AbortSession(entry.Session.ID)
	outcome := m.AbortSession(entry.Session.ID)

	if outcome.OK {
		t.Error("expected OK=false for already aborted session")
	}
	if outcome.Reason != "already_exited" {
		t.Errorf("expected reason=already_exited, got %s", outcome.Reason)
	}
}

func TestTerminateSessionNotFound(t *testing.T) {
	m := NewTerminalManager()
	outcome := m.TerminateSession("nonexistent")
	if outcome.OK {
		t.Error("expected OK=false for nonexistent session")
	}
	if outcome.Reason != "not_found" {
		t.Errorf("expected reason=not_found, got %s", outcome.Reason)
	}
}

func TestTerminateSessionAlreadyExited(t *testing.T) {
	m := NewTerminalManager()
	entry, _ := m.CreateSession(context.Background(), "beat-1", "/repo")

	entry.mu.Lock()
	entry.Session.Status = StatusCompleted
	entry.mu.Unlock()

	outcome := m.TerminateSession(entry.Session.ID)
	if outcome.OK {
		t.Error("expected OK=false for already exited session")
	}
	if outcome.Reason != "already_exited" {
		t.Errorf("expected reason=already_exited, got %s", outcome.Reason)
	}
}

type mockProcess struct {
	pid    int
	killed bool
	sig    interface{}
	mu     sync.Mutex
}

func (p *mockProcess) Pid() int         { return p.pid }
func (p *mockProcess) Kill() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.killed = true
	return nil
}
func (p *mockProcess) Signal(sig interface{}) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sig = sig
	return nil
}

func TestTerminateSessionWithProcess(t *testing.T) {
	m := NewTerminalManager()
	entry, _ := m.CreateSession(context.Background(), "beat-1", "/repo")

	proc := &mockProcess{pid: 12345}
	entry.SetProcess(proc)

	entry.SetCancel(func() {})

	outcome := m.TerminateSession(entry.Session.ID)
	if !outcome.OK {
		t.Error("expected OK=true")
	}

	if entry.Session.Status != StatusAborted {
		t.Errorf("expected status=aborted, got %s", entry.Session.Status)
	}
}

func TestKillSessionWithProcess(t *testing.T) {
	m := NewTerminalManager()
	entry, _ := m.CreateSession(context.Background(), "beat-1", "/repo")

	proc := &mockProcess{pid: 12345}
	entry.SetProcess(proc)

	entry.SetCancel(func() {})

	outcome := m.KillSession(entry.Session.ID)
	if !outcome.OK {
		t.Error("expected OK=true")
	}

	if entry.Session.Status != StatusAborted {
		t.Errorf("expected status=aborted, got %s", entry.Session.Status)
	}
}

func TestRemoveSession(t *testing.T) {
	m := NewTerminalManager()
	entry, _ := m.CreateSession(context.Background(), "beat-1", "/repo")

	id := entry.Session.ID
	m.RemoveSession(id)

	_, ok := m.GetSession(id)
	if ok {
		t.Error("expected session to be removed")
	}
}

func TestRemoveSessionNotFound(t *testing.T) {
	m := NewTerminalManager()
	m.RemoveSession("nonexistent")
}

func TestPushEvent(t *testing.T) {
	m := NewTerminalManager()
	entry, _ := m.CreateSession(context.Background(), "beat-1", "/repo")
	id := entry.Session.ID

	evt := session.TerminalEvent{Type: "stdout", Content: "hello", BeatID: "beat-1", Time: time.Now().UnixMilli()}
	m.PushEvent(id, evt)

	buf := m.GetBuffer(id)
	if len(buf) != 1 {
		t.Fatalf("expected 1 event in buffer, got %d", len(buf))
	}
	if buf[0].Content != "hello" {
		t.Errorf("expected content=hello, got %s", buf[0].Content)
	}
}

func TestPushEventBufferOverflow(t *testing.T) {
	m := NewTerminalManager()
	entry, _ := m.CreateSession(context.Background(), "beat-1", "/repo")
	id := entry.Session.ID

	for i := 0; i < MaxBuffer+100; i++ {
		evt := session.TerminalEvent{Type: "stdout", BeatID: "beat-1", Time: time.Now().UnixMilli()}
		m.PushEvent(id, evt)
	}

	buf := m.GetBuffer(id)
	if len(buf) > MaxBuffer {
		t.Errorf("expected buffer capped at %d, got %d", MaxBuffer, len(buf))
	}
}

func TestPushEventNotFound(t *testing.T) {
	m := NewTerminalManager()
	evt := session.TerminalEvent{Type: "stdout", BeatID: "beat-1", Time: time.Now().UnixMilli()}
	m.PushEvent("nonexistent", evt)
}

func TestGetBufferNotFound(t *testing.T) {
	m := NewTerminalManager()
	buf := m.GetBuffer("nonexistent")
	if buf != nil {
		t.Error("expected nil buffer for nonexistent session")
	}
}

func TestRecordPendingApproval(t *testing.T) {
	m := NewTerminalManager()
	entry, _ := m.CreateSession(context.Background(), "beat-1", "/repo")

	rec := &PendingApprovalRecord{
		ApprovalID:      "approval-1",
		Status:           ApprovalPending,
		SupportedActions:  []string{"accept", "decline"},
		Actionable:       true,
	}
	entry.RecordPendingApproval(rec)

	found, ok := entry.GetPendingApproval("approval-1")
	if !ok {
		t.Error("expected to find approval")
	}
	if found.ApprovalID != "approval-1" {
		t.Errorf("expected approvalId=approval-1, got %s", found.ApprovalID)
	}

	entry.mu.RLock()
	count := len(entry.Session.PendingApprovals)
	entry.mu.RUnlock()
	if count != 1 {
		t.Errorf("expected 1 session pending approval, got %d", count)
	}

	_, ok = entry.GetPendingApproval("nonexistent")
	if ok {
		t.Error("expected not to find nonexistent approval")
	}
}

func TestUpdateApprovalStatus(t *testing.T) {
	m := NewTerminalManager()
	entry, _ := m.CreateSession(context.Background(), "beat-1", "/repo")

	rec := &PendingApprovalRecord{
		ApprovalID: "approval-1",
		Status:      ApprovalPending,
		Actionable:  true,
	}
	entry.RecordPendingApproval(rec)

	entry.UpdateApprovalStatus("approval-1", ApprovalApproved)

	found, _ := entry.GetPendingApproval("approval-1")
	if found.Status != ApprovalApproved {
		t.Errorf("expected status=approved, got %s", found.Status)
	}
}

func TestSyncSessionLeaseInfo(t *testing.T) {
	m := NewTerminalManager()
	entry, _ := m.CreateSession(context.Background(), "beat-1", "/repo")

	entry.mu.Lock()
	entry.KnotsLeaseID = "lease-123"
	entry.KnotsLeaseAgentInfo = &AgentInfo{
		AgentName:     "claude",
		AgentModel:    "sonnet",
		AgentVersion:  "4",
		AgentProvider: "anthropic",
	}
	entry.mu.Unlock()

	entry.SyncSessionLeaseInfo()

	if entry.Session.KnotsLeaseID != "lease-123" {
		t.Errorf("expected knotsLeaseId=lease-123, got %s", entry.Session.KnotsLeaseID)
	}
	if entry.Session.KnotsAgentInfo.AgentName != "claude" {
		t.Errorf("expected agentName=claude, got %s", entry.Session.KnotsAgentInfo.AgentName)
	}

	entry.mu.Lock()
	entry.KnotsLeaseID = ""
	entry.KnotsLeaseAgentInfo = nil
	entry.mu.Unlock()

	entry.SyncSessionLeaseInfo()

	if entry.Session.KnotsLeaseID != "" {
		t.Errorf("expected empty knotsLeaseId, got %s", entry.Session.KnotsLeaseID)
	}
	if entry.Session.KnotsAgentInfo != nil {
		t.Error("expected nil knotsAgentInfo")
	}
}

func TestSetProcess(t *testing.T) {
	m := NewTerminalManager()
	entry, _ := m.CreateSession(context.Background(), "beat-1", "/repo")

	proc := &mockProcess{pid: 999}
	entry.SetProcess(proc)

	if entry.Process == nil {
		t.Error("expected process to be set")
	}
	if entry.Process.Pid() != 999 {
		t.Errorf("expected pid=999, got %d", entry.Process.Pid())
	}
}

func TestSetCancel(t *testing.T) {
	m := NewTerminalManager()
	entry, _ := m.CreateSession(context.Background(), "beat-1", "/repo")

	entry.SetCancel(func() {})

	if entry.Cancel == nil {
		t.Error("expected cancel to be set")
	}
}

func TestSetReleaseKnotsLease(t *testing.T) {
	m := NewTerminalManager()
	entry, _ := m.CreateSession(context.Background(), "beat-1", "/repo")

	called := false
	fn := ReleaseKnotsLeaseFunc(func(reason string, outcome string, data map[string]any) {
		called = true
	})
	entry.SetReleaseKnotsLease(fn)

	if entry.ReleaseKnotsLease == nil {
		t.Error("expected releaseKnotsLease to be set")
	}

	entry.ReleaseKnotsLease("test", "warning", nil)
	if !called {
		t.Error("expected releaseKnotsLease to be called")
	}
}

func TestCleanupSessionResources(t *testing.T) {
	m := NewTerminalManager()
	entry, _ := m.CreateSession(context.Background(), "beat-1", "/repo")

	entry.RecordPendingApproval(&PendingApprovalRecord{
		ApprovalID: "approval-1",
		Status:      ApprovalPending,
		Actionable:  true,
	})

	CleanupSessionResources(entry, "test_cleanup")

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

func TestAbortedStatusPreservedOnExit(t *testing.T) {
	m := NewTerminalManager()
	entry, _ := m.CreateSession(context.Background(), "beat-1", "/repo")

	entry.mu.Lock()
	entry.Session.Status = StatusAborted
	entry.mu.Unlock()

	outcome := m.TerminateSession(entry.Session.ID)
	if outcome.OK {
		t.Error("expected OK=false for already exited session")
	}
	if outcome.Reason != "already_exited" {
		t.Errorf("expected reason=already_exited, got %s", outcome.Reason)
	}

	entry.mu.RLock()
	status := entry.Session.Status
	entry.mu.RUnlock()
	if status != StatusAborted {
		t.Errorf("expected status to remain aborted, got %s", status)
	}
}

func TestExitedStatuses(t *testing.T) {
	statuses := []SessionStatus{StatusCompleted, StatusError, StatusAborted, StatusDisconnected}
	for _, s := range statuses {
		if !exitedStatuses[s] {
			t.Errorf("expected %s to be an exited status", s)
		}
	}

	nonExited := []SessionStatus{StatusRunning}
	for _, s := range nonExited {
		if exitedStatuses[s] {
			t.Errorf("expected %s to NOT be an exited status", s)
		}
	}
}

func TestListSessionsIncludesPendingApprovals(t *testing.T) {
	m := NewTerminalManager()
	entry, _ := m.CreateSession(context.Background(), "beat-1", "/repo")

	entry.RecordPendingApproval(&PendingApprovalRecord{
		ApprovalID: "approval-1",
		Status:      ApprovalPending,
		Actionable:  true,
	})

	sessions := m.ListSessions()
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	s := sessions[0]
	if len(s.PendingApprovals) != 1 {
		t.Errorf("expected 1 pending approval, got %d", len(s.PendingApprovals))
	}
	if s.PendingApprovals[0].ApprovalID != "approval-1" {
		t.Errorf("expected approvalId=approval-1, got %s", s.PendingApprovals[0].ApprovalID)
	}
}

func TestConcurrentAccess(t *testing.T) {
	m := NewTerminalManager(WithMaxSessions(100))

	var wg sync.WaitGroup
	errCh := make(chan error, 50)

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := m.CreateSession(context.Background(), fmt.Sprintf("beat-%d", idx), "/repo")
			if err != nil {
				errCh <- err
			}
		}(i)
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("unexpected error: %v", err)
	}

	sessions := m.ListSessions()
	if len(sessions) != 50 {
		t.Errorf("expected 50 sessions, got %d", len(sessions))
	}
}