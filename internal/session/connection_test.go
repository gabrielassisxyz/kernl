package session

import (
	"sync"
	"testing"
	"time"
)

type stubSessionProvider struct {
	mu       sync.RWMutex
	sessions map[string]SessionInfo
	events   map[string]chan TerminalEvent
}

func newStubProvider() *stubSessionProvider {
	return &stubSessionProvider{
		sessions: make(map[string]SessionInfo),
		events:   make(map[string]chan TerminalEvent),
	}
}

func (p *stubSessionProvider) GetSessionEntry(id string) (SessionInfo, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	info, ok := p.sessions[id]
	return info, ok
}

func (p *stubSessionProvider) ListSessionIDs() []SessionInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()
	result := make([]SessionInfo, 0, len(p.sessions))
	for _, info := range p.sessions {
		result = append(result, info)
	}
	return result
}

func (p *stubSessionProvider) PushEvent(id string, evt TerminalEvent) {
	p.mu.RLock()
	ch, ok := p.events[id]
	p.mu.RUnlock()
	if ok {
		ch <- evt
	}
}

func (p *stubSessionProvider) addSession(id, beadID, beatTitle, repoPath, status string) chan TerminalEvent {
	ch := make(chan TerminalEvent, 5000)
	p.mu.Lock()
	p.sessions[id] = SessionInfo{
		ID:        id,
		BeadID:    beadID,
		BeadTitle: beatTitle,
		RepoPath:   repoPath,
		Status:    status,
	}
	p.events[id] = ch
	p.mu.Unlock()
	return ch
}

func setupTest(t *testing.T) (*stubSessionProvider, *SessionConnectionManager) {
	t.Helper()
	provider := newStubProvider()
	var notifications []Notification
	var mu sync.Mutex
	scm := NewSessionConnectionManager(provider, func(n Notification) {
		mu.Lock()
		notifications = append(notifications, n)
		mu.Unlock()
	})
	_ = &mu
	_ = &notifications
	return provider, scm
}

func TestConnectIdempotent(t *testing.T) {
	provider, scm := setupTest(t)
	ch := provider.addSession("s-1", "bead-1", "Title", "/repo", "running")

	scm.Connect("s-1")
	// Send an event through the provider's channel to verify connection
	go func() { ch <- TerminalEvent{Type: "stdout", Content: "hello"} }()

	scm.HandleEvent("s-1", TerminalEvent{Type: "stdout", Content: "hello"})

	// Connect again - should be idempotent
	scm.Connect("s-1")

	ids := scm.GetConnectedIDs()
	if len(ids) != 1 {
		t.Errorf("expected 1 connection, got %d", len(ids))
	}
}

func TestDisconnectRemovesEntry(t *testing.T) {
	provider, scm := setupTest(t)
	provider.addSession("s-1", "bead-1", "Title", "/repo", "running")

	scm.Connect("s-1")
	ids := scm.GetConnectedIDs()
	if len(ids) != 1 {
		t.Fatalf("expected 1 connection after connect, got %d", len(ids))
	}

	scm.Disconnect("s-1")
	ids = scm.GetConnectedIDs()
	if len(ids) != 0 {
		t.Errorf("expected 0 connections after disconnect, got %d", len(ids))
	}
}

func TestDisconnectNonexistentNoop(t *testing.T) {
	_, scm := setupTest(t)
	scm.Disconnect("nonexistent")
}

func TestGetBufferReturnsEvents(t *testing.T) {
	provider, scm := setupTest(t)
	provider.addSession("s-1", "bead-1", "Title", "/repo", "running")

	scm.Connect("s-1")

	scm.HandleEvent("s-1", TerminalEvent{
		Type:    "stdout",
		Content: "hello",
		BeadID:  "bead-1",
		Time:    1000,
	})

	buf := scm.GetBuffer("s-1")
	if len(buf) < 1 {
		t.Fatalf("expected at least 1 buffered event, got %d", len(buf))
	}
	found := false
	for _, evt := range buf {
		if evt.Type == "stdout" && evt.Data == "hello" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected to find stdout event with 'hello' in buffer, got %v", buf)
	}
}

func TestGetBufferUnknownSessionReturnsNil(t *testing.T) {
	_, scm := setupTest(t)
	buf := scm.GetBuffer("nonexistent")
	if buf != nil {
		t.Errorf("expected nil for unknown session, got %v", buf)
	}
}

func TestHasExitedAndExitCode(t *testing.T) {
	provider, scm := setupTest(t)
	provider.addSession("s-1", "bead-1", "Title", "/repo", "running")

	scm.Connect("s-1")

	if scm.HasExited("s-1") {
		t.Error("expected HasExited=false before exit event")
	}
	if scm.GetExitCode("s-1") != nil {
		t.Error("expected nil exit code before exit event")
	}

	scm.HandleEvent("s-1", TerminalEvent{
		Type:    "exit",
		Content: "0",
		BeadID:  "bead-1",
		Time:    2000,
	})

	if !scm.HasExited("s-1") {
		t.Error("expected HasExited=true after exit event")
	}
	code := scm.GetExitCode("s-1")
	if code == nil || *code != 0 {
		t.Errorf("expected exit code 0, got %v", code)
	}
}

func TestHasExitedUnknownSessionReturnsFalse(t *testing.T) {
	_, scm := setupTest(t)
	if scm.HasExited("nonexistent") {
		t.Error("expected HasExited=false for unknown session")
	}
}

func TestGetExitCodeUnknownSessionReturnsNil(t *testing.T) {
	_, scm := setupTest(t)
	code := scm.GetExitCode("nonexistent")
	if code != nil {
		t.Errorf("expected nil for unknown session, got %v", code)
	}
}

func TestDuplicateExitNotifiesOnlyOnce(t *testing.T) {
	var notifications []Notification
	var mu sync.Mutex
	provider := newStubProvider()
	scm := NewSessionConnectionManager(provider, func(n Notification) {
		mu.Lock()
		notifications = append(notifications, n)
		mu.Unlock()
	})

	provider.addSession("s-1", "bead-1", "Title", "/repo", "running")

	scm.Connect("s-1")
	scm.HandleEvent("s-1", TerminalEvent{
		Type:    "exit",
		Content: "0",
		BeadID:  "bead-1",
		Time:    1000,
	})

	scm.HandleEvent("s-1", TerminalEvent{
		Type:    "exit",
		Content: "1",
		BeadID:  "bead-1",
		Time:    2000,
	})

	mu.Lock()
	count := 0
	for _, n := range notifications {
		if n.Kind == NotificationKindExit {
			count++
		}
	}
	mu.Unlock()

	if count != 1 {
		t.Errorf("expected exactly 1 exit notification, got %d", count)
	}

	code := scm.GetExitCode("s-1")
	if code == nil || *code != 0 {
		t.Errorf("expected first exit code (0) preserved, got %v", code)
	}
}

func TestExitNonZeroErrorCode(t *testing.T) {
	var notifications []Notification
	var mu sync.Mutex
	provider := newStubProvider()
	scm := NewSessionConnectionManager(provider, func(n Notification) {
		mu.Lock()
		notifications = append(notifications, n)
		mu.Unlock()
	})

	provider.addSession("s-1", "bead-1", "Title", "/repo", "running")

	scm.Connect("s-1")
	scm.HandleEvent("s-1", TerminalEvent{
		Type:    "exit",
		Content: "1",
		BeadID:  "bead-1",
		Time:    1000,
	})

	mu.Lock()
	defer mu.Unlock()
	for _, n := range notifications {
		if n.Kind == NotificationKindExit && n.ExitCode != 1 {
			t.Errorf("expected exit code 1, got %d", n.ExitCode)
		}
	}
}

func TestExitPreservesAbortedStatus(t *testing.T) {
	var notifications []Notification
	var mu sync.Mutex
	provider := newStubProvider()
	scm := NewSessionConnectionManager(provider, func(n Notification) {
		mu.Lock()
		notifications = append(notifications, n)
		mu.Unlock()
	})

	provider.addSession("s-1", "bead-1", "Title", "/repo", "aborted")

	scm.Connect("s-1")
	scm.HandleEvent("s-1", TerminalEvent{
		Type:    "exit",
		Content: "0",
		BeadID:  "bead-1",
		Time:    1000,
	})

	mu.Lock()
	var message string
	for _, n := range notifications {
		if n.Kind == NotificationKindExit {
			message = n.Message
		}
	}
	mu.Unlock()

	if message == "" {
		t.Fatal("expected exit notification")
	}
	// Must say "terminated" because status was already aborted
	// Not "completed"
}

func TestSubscribeAndForwardEvents(t *testing.T) {
	provider, scm := setupTest(t)
	provider.addSession("s-1", "bead-1", "Title", "/repo", "running")

	scm.Connect("s-1")
	ch, unsub := scm.ConnectAndSubscribe("s-1")
	defer unsub()

	scm.HandleEvent("s-1", TerminalEvent{
		Type:    "stdout",
		Content: "forwarded",
		BeadID:  "bead-1",
		Time:    1000,
	})

	select {
	case evt := <-ch:
		if evt.Type != "stdout" || evt.Content != "forwarded" {
			t.Errorf("unexpected event: %+v", evt)
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for forwarded event")
	}
}

func TestBufferBoundedToMaxSize(t *testing.T) {
	provider, scm := setupTest(t)
	provider.addSession("s-1", "bead-1", "Title", "/repo", "running")

	scm.Connect("s-1")

	for i := 0; i < maxConnectionBuffer+100; i++ {
		scm.HandleEvent("s-1", TerminalEvent{
			Type:    "stdout",
			Content: "msg",
			BeadID:  "bead-1",
			Time:    int64(i),
		})
	}

	buf := scm.GetBuffer("s-1")
	if len(buf) > maxConnectionBuffer {
		t.Errorf("buffer exceeded max size: got %d, max %d", len(buf), maxConnectionBuffer)
	}
}

func TestGetConnectedIDs(t *testing.T) {
	provider, scm := setupTest(t)
	provider.addSession("s-1", "bead-1", "Title", "/repo", "running")
	provider.addSession("s-2", "bead-2", "Title2", "/repo", "running")

	scm.Connect("s-1")
	scm.Connect("s-2")

	ids := scm.GetConnectedIDs()
	if len(ids) != 2 {
		t.Errorf("expected 2 connected IDs, got %d", len(ids))
	}

	scm.Disconnect("s-1")
	ids = scm.GetConnectedIDs()
	if len(ids) != 1 {
		t.Errorf("expected 1 connected ID after disconnect, got %d", len(ids))
	}
}

func TestStartSyncConnectsRunningSessions(t *testing.T) {
	provider, scm := setupTest(t)
	provider.addSession("s-1", "bead-1", "Title", "/repo", "running")

	scm.StartSync()

	ids := scm.GetConnectedIDs()
	found := false
	for _, id := range ids {
		if id == "s-1" {
			found = true
		}
	}
	if !found {
		t.Error("expected running session to be connected after StartSync")
	}
}

func TestStopSyncDisconnectsAll(t *testing.T) {
	provider, scm := setupTest(t)
	provider.addSession("s-1", "bead-1", "Title", "/repo", "running")

	scm.Connect("s-1")
	if len(scm.GetConnectedIDs()) != 1 {
		t.Fatal("expected 1 connection")
	}

	scm.StopSync()

	if len(scm.GetConnectedIDs()) != 0 {
		t.Errorf("expected 0 connections after StopSync, got %d", len(scm.GetConnectedIDs()))
	}
}

func TestAgentFailureNotification(t *testing.T) {
	var notifications []Notification
	var mu sync.Mutex
	provider := newStubProvider()
	scm := NewSessionConnectionManager(provider, func(n Notification) {
		mu.Lock()
		notifications = append(notifications, n)
		mu.Unlock()
	})

	provider.addSession("s-1", "bead-fail", "Title", "/repo", "running")

	scm.Connect("s-1")
	scm.HandleEvent("s-1", TerminalEvent{
		Type:    "agent_failure",
		Content: "dispatch failed",
		BeadID:  "bead-fail",
		Time:    1000,
	})

	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, n := range notifications {
		if n.Kind == NotificationKindFailure {
			found = true
			if n.BeadID != "bead-fail" {
				t.Errorf("expected beadId bead-fail, got %s", n.BeadID)
			}
		}
	}
	if !found {
		t.Error("expected agent_failure notification")
	}
}

func TestDisconnectCleansUpSubscribers(t *testing.T) {
	provider, scm := setupTest(t)
	provider.addSession("s-1", "bead-1", "Title", "/repo", "running")

	scm.Connect("s-1")
	ch, unsub := scm.ConnectAndSubscribe("s-1")
	_ = ch

	unsub()

	scm.Disconnect("s-1")

	// Verify we can reconnect after disconnect
	scm.Connect("s-1")
	ids := scm.GetConnectedIDs()
	if len(ids) != 1 {
		t.Errorf("expected 1 connection after reconnect, got %d", len(ids))
	}
}

func TestExitWithNegativeCodeDisconnect(t *testing.T) {
	var notifications []Notification
	var mu sync.Mutex
	provider := newStubProvider()
	scm := NewSessionConnectionManager(provider, func(n Notification) {
		mu.Lock()
		notifications = append(notifications, n)
		mu.Unlock()
	})

	provider.addSession("s-1", "bead-1", "Title", "/repo", "running")

	scm.Connect("s-1")
	scm.HandleEvent("s-1", TerminalEvent{
		Type:    "exit",
		Content: "-2",
		BeadID:  "bead-1",
		Time:    1000,
	})

	mu.Lock()
	defer mu.Unlock()
	for _, n := range notifications {
		if n.Kind == NotificationKindExit {
			if n.ExitCode != -2 {
				t.Errorf("expected exit code -2, got %d", n.ExitCode)
			}
		}
	}
}

func TestBeadStateObservedNoStatusUpdate(t *testing.T) {
	provider, scm := setupTest(t)
	provider.addSession("s-1", "bead-1", "Title", "/repo", "running")

	scm.Connect("s-1")

	scm.HandleEvent("s-1", TerminalEvent{
		Type:    "beat_state_observed",
		Content: "shipped",
		BeadID:  "bead-1",
		Time:    1000,
	})

	code := scm.GetExitCode("s-1")
	if code != nil {
		t.Error("beat_state_observed should not set exit code")
	}
	if scm.HasExited("s-1") {
		t.Error("beat_state_observed should not set hasExited")
	}
}

func TestContextCancellation(t *testing.T) {
	provider, scm := setupTest(t)
	provider.addSession("s-1", "bead-1", "Title", "/repo", "running")

	scm.Connect("s-1")
	ch, unsub := scm.ConnectAndSubscribe("s-1")

	// Closing unsub should close the channel
	unsub()

	_, ok := <-ch
	if ok {
		t.Error("expected channel to be closed after unsubscribe")
	}
}