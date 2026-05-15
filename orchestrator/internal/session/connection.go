package session

import (
	"log/slog"
	"strconv"
	"sync"
)

const maxConnectionBuffer = 5000

type BufferedEvent struct {
	Type string `json:"type"`
	Data string `json:"data"`
}

type NotificationKind string

const (
	NotificationKindExit      NotificationKind = "exit"
	NotificationKindApproval  NotificationKind = "approval"
	NotificationKindFailure   NotificationKind = "failure"
	NotificationKindConnected NotificationKind = "connected"
)

type Notification struct {
	Kind      NotificationKind `json:"kind"`
	Message   string           `json:"message"`
	BeadID    string           `json:"beadId,omitempty"`
	RepoPath   string           `json:"repoPath,omitempty"`
	SessionID string           `json:"sessionId,omitempty"`
	ExitCode  int              `json:"exitCode,omitempty"`
}

type SessionInfo struct {
	ID         string
	BeadID     string
	BeadTitle  string
	RepoPath    string
	Status     string
}

type SessionProvider interface {
	GetSessionEntry(id string) (SessionInfo, bool)
	ListSessionIDs() []SessionInfo
	PushEvent(id string, evt TerminalEvent)
}

type sseListener struct {
	id uint64
	ch chan TerminalEvent
}

type connection struct {
	listeners    map[uint64]chan TerminalEvent
	buffer       []BufferedEvent
	exitReceived bool
	exitCode     *int
	mu           sync.Mutex
	nextListener uint64
}

type SessionConnectionManager struct {
	mu          sync.RWMutex
	connections map[string]*connection
	provider    SessionProvider
	notify      func(Notification)
}

func NewSessionConnectionManager(provider SessionProvider, notify func(Notification)) *SessionConnectionManager {
	if notify == nil {
		notify = func(n Notification) {}
	}
	return &SessionConnectionManager{
		connections: make(map[string]*connection),
		provider:    provider,
		notify:      notify,
	}
}

// Connect creates an SSE fan-out for the given session. Idempotent: repeated
// calls for the same sessionID do not create duplicate connections.
func (m *SessionConnectionManager) Connect(sessionID string) <-chan TerminalEvent {
	m.mu.Lock()
	defer m.mu.Unlock()

	if conn, exists := m.connections[sessionID]; exists {
		ch := make(chan TerminalEvent, 500)
		conn.mu.Lock()
		conn.nextListener++
		key := conn.nextListener
		conn.listeners[key] = ch
		conn.mu.Unlock()
		return ch
	}

	conn := &connection{
		listeners:    make(map[uint64]chan TerminalEvent),
		buffer:       make([]BufferedEvent, 0, maxConnectionBuffer),
		nextListener: 1,
	}

	m.connections[sessionID] = conn

	info, exists := m.provider.GetSessionEntry(sessionID)
	if !exists {
		slog.Warn("[connection-manager] connect: session not found, buffering anyway",
			"sessionId", sessionID)
		return nil
	}
	_ = info

	return nil
}

func (m *SessionConnectionManager) ConnectAndSubscribe(sessionID string) (<-chan TerminalEvent, func()) {
	m.mu.Lock()

	if conn, exists := m.connections[sessionID]; exists {
		conn.mu.Lock()
		key := conn.nextListener + 1
		conn.nextListener = key
		ch := make(chan TerminalEvent, 500)
		conn.listeners[key] = ch
		conn.mu.Unlock()
		m.mu.Unlock()

		unsub := func() {
			conn.mu.Lock()
			delete(conn.listeners, key)
			close(ch)
			conn.mu.Unlock()
		}
		return ch, unsub
	}

	conn := &connection{
		listeners:    make(map[uint64]chan TerminalEvent),
		buffer:       make([]BufferedEvent, 0, maxConnectionBuffer),
		nextListener: 1,
	}

	m.connections[sessionID] = conn
	m.mu.Unlock()

	ch := make(chan TerminalEvent, 5000)
	conn.mu.Lock()
	conn.listeners[1] = ch
	conn.mu.Unlock()

	return ch, func() {
		conn.mu.Lock()
		delete(conn.listeners, 1)
		close(ch)
		conn.mu.Unlock()
	}
}

// HandleEvent processes a TerminalEvent for the given session, buffering it,
// forwarding to subscribers, and handling special event types.
func (m *SessionConnectionManager) HandleEvent(sessionID string, evt TerminalEvent) {
	m.mu.RLock()
	conn, exists := m.connections[sessionID]
	m.mu.RUnlock()

	if !exists {
		return
	}

	m.handleEventForConn(conn, sessionID, evt)
}

func (m *SessionConnectionManager) handleEventForConn(conn *connection, sessionID string, evt TerminalEvent) {
	conn.mu.Lock()

	if len(conn.buffer) < maxConnectionBuffer {
		conn.buffer = append(conn.buffer, BufferedEvent{
			Type: evt.Type,
			Data: evt.Content,
		})
	}

	exitAlreadyReceived := conn.exitReceived

	if evt.Type == "exit" && !exitAlreadyReceived {
		conn.exitReceived = true
		code := 0
		trimmed := trimSpace(evt.Content)
		if trimmed != "" {
			if parsed, err := strconv.Atoi(trimmed); err == nil {
				code = parsed
			}
		}
		conn.exitCode = &code
	}

	listeners := make([]chan TerminalEvent, 0, len(conn.listeners))
	for _, ch := range conn.listeners {
		listeners = append(listeners, ch)
	}
	exitJustReceived := !exitAlreadyReceived && conn.exitReceived
	conn.mu.Unlock()

	for _, ch := range listeners {
		select {
		case ch <- evt:
		default:
			slog.Warn("[connection-manager] subscriber channel full, dropping event",
				"type", evt.Type, "sessionId", sessionID)
		}
	}

	if exitJustReceived {
		m.handleExitNotification(sessionID, conn, evt)
	}

	if evt.Type == "beat_state_observed" {
		slog.Debug("[connection-manager] beat_state_observed, queries invalidated",
			"sessionId", sessionID, "beadId", evt.BeadID)
	}

	if evt.Type == "agent_failure" {
		m.handleAgentFailure(sessionID, evt)
	}
}

func (m *SessionConnectionManager) handleAgentFailure(sessionID string, evt TerminalEvent) {
	info, exists := m.provider.GetSessionEntry(sessionID)
	beadID := evt.BeadID
	repoPath := ""
	if exists {
		if beadID == "" {
			beadID = info.BeadID
		}
		repoPath = info.RepoPath
	}

	m.notify(Notification{
		Kind:      NotificationKindFailure,
		Message:   evt.Content,
		BeadID:    beadID,
		RepoPath:   repoPath,
		SessionID: sessionID,
	})
}

func (m *SessionConnectionManager) handleExitNotification(sessionID string, conn *connection, evt TerminalEvent) {
	conn.mu.Lock()
	exitCode := 0
	if conn.exitCode != nil {
		exitCode = *conn.exitCode
	}
	conn.mu.Unlock()

	info, exists := m.provider.GetSessionEntry(sessionID)

	alreadyAborted := false
	beatTitle := ""
	beadID := ""
	repoPath := ""
	if exists {
		alreadyAborted = info.Status == "aborted"
		beatTitle = info.BeadTitle
		beadID = info.BeadID
		repoPath = info.RepoPath
	}

	statusLabel := "completed"
	if alreadyAborted {
		statusLabel = "terminated"
	} else if exitCode == -2 {
		statusLabel = "disconnected (server may have restarted)"
	} else if exitCode != 0 {
		statusLabel = "exited with error"
	}

	errorDetail := ""
	if exitCode != 0 && exitCode != -2 {
		conn.mu.Lock()
		var stderrEvents []BufferedEvent
		for _, e := range conn.buffer {
			if e.Type == "stderr" {
				stderrEvents = append(stderrEvents, e)
			}
		}
		last3 := stderrEvents
		if len(last3) > 3 {
			last3 = last3[len(last3)-3:]
		}
		detail := ""
		for _, e := range last3 {
			trimmed := trimSpace(e.Data)
			if trimmed != "" {
				if detail != "" {
					detail += " "
				}
				detail += trimmed
			}
		}
		if len(detail) > 200 {
			detail = detail[:200]
		}
		if detail != "" {
			errorDetail = " — " + detail
		}
		conn.mu.Unlock()
	}

	message := ""
	if beatTitle != "" {
		message = "\"" + beatTitle + "\" session " + statusLabel + errorDetail
	} else {
		message = "session " + statusLabel + errorDetail
	}

	m.notify(Notification{
		Kind:      NotificationKindExit,
		Message:   message,
		BeadID:    beadID,
		RepoPath:   repoPath,
		SessionID: sessionID,
		ExitCode:  exitCode,
	})
}

// Disconnect closes all subscriber channels and removes the session entry.
func (m *SessionConnectionManager) Disconnect(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	conn, exists := m.connections[sessionID]
	if !exists {
		return
	}

	conn.mu.Lock()
	for _, ch := range conn.listeners {
		close(ch)
	}
	conn.mu.Unlock()

	delete(m.connections, sessionID)
}

// GetBuffer returns buffered events for replay. Returns nil for unknown sessions.
func (m *SessionConnectionManager) GetBuffer(sessionID string) []BufferedEvent {
	m.mu.RLock()
	conn, exists := m.connections[sessionID]
	m.mu.RUnlock()

	if !exists {
		return nil
	}

	conn.mu.Lock()
	defer conn.mu.Unlock()

	buf := make([]BufferedEvent, len(conn.buffer))
	copy(buf, conn.buffer)
	return buf
}

// HasExited returns true after an exit event has been received for the session.
func (m *SessionConnectionManager) HasExited(sessionID string) bool {
	m.mu.RLock()
	conn, exists := m.connections[sessionID]
	m.mu.RUnlock()

	if !exists {
		return false
	}

	conn.mu.Lock()
	defer conn.mu.Unlock()
	return conn.exitReceived
}

// GetExitCode returns the parsed exit code, or nil if not yet exited.
func (m *SessionConnectionManager) GetExitCode(sessionID string) *int {
	m.mu.RLock()
	conn, exists := m.connections[sessionID]
	m.mu.RUnlock()

	if !exists {
		return nil
	}

	conn.mu.Lock()
	defer conn.mu.Unlock()
	return conn.exitCode
}

// GetConnectedIDs returns all currently-connected session IDs.
func (m *SessionConnectionManager) GetConnectedIDs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.connections))
	for id := range m.connections {
		ids = append(ids, id)
	}
	return ids
}

// StartSync connects to all currently running sessions.
func (m *SessionConnectionManager) StartSync() {
	sessions := m.provider.ListSessionIDs()
	for _, s := range sessions {
		if s.Status == "running" {
			m.Connect(s.ID)
		}
	}
}

// StopSync disconnects all sessions and clears the connection map.
func (m *SessionConnectionManager) StopSync() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id := range m.connections {
		conn := m.connections[id]
		conn.mu.Lock()
		for _, ch := range conn.listeners {
			close(ch)
		}
		conn.mu.Unlock()
	}

	m.connections = make(map[string]*connection)
}

func trimSpace(s string) string {
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}