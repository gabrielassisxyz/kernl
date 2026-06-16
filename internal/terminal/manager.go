package terminal

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/session"
)

const (
	MaxBuffer          = 5000
	DefaultMaxSessions = 5
	CleanupDelay       = 5 * time.Minute
)

type SessionStatus string

const (
	StatusRunning      SessionStatus = "running"
	StatusCompleted    SessionStatus = "completed"
	StatusError        SessionStatus = "error"
	StatusAborted      SessionStatus = "aborted"
	StatusDisconnected SessionStatus = "disconnected"
)

var exitedStatuses = map[SessionStatus]bool{
	StatusCompleted:    true,
	StatusError:        true,
	StatusAborted:      true,
	StatusDisconnected: true,
}

type TerminalSession struct {
	ID               string                   `json:"id"`
	BeadID           string                   `json:"beadId"`
	BeadTitle        string                   `json:"beatTitle,omitempty"`
	RepoPath         string                   `json:"repoPath,omitempty"`
	Status           SessionStatus            `json:"status"`
	StartedAt        string                   `json:"startedAt"`
	KnotsLeaseID     string                   `json:"knotsLeaseId,omitempty"`
	KnotsAgentInfo   *AgentInfo               `json:"knotsAgentInfo,omitempty"`
	PendingApprovals []*PendingApprovalRecord `json:"pendingApprovals,omitempty"`
}

type AgentInfo struct {
	AgentName     string `json:"agentName"`
	AgentModel    string `json:"agentModel,omitempty"`
	AgentVersion  string `json:"agentVersion,omitempty"`
	AgentProvider string `json:"agentProvider,omitempty"`
}

type ApprovalReplyTransport string

const (
	ReplyTransportHTTP    ApprovalReplyTransport = "http"
	ReplyTransportJSONRPC ApprovalReplyTransport = "jsonrpc"
	ReplyTransportACP     ApprovalReplyTransport = "acp"
	ReplyTransportStdio   ApprovalReplyTransport = "stdio"
)

type ApprovalReplyTarget struct {
	Adapter         string                 `json:"adapter"`
	Transport       ApprovalReplyTransport `json:"transport"`
	NativeSessionID string                 `json:"nativeSessionId,omitempty"`
	RequestID       string                 `json:"requestId,omitempty"`
	PermissionID    string                 `json:"permissionId,omitempty"`
}

type PendingApprovalRecord struct {
	ApprovalID        string               `json:"approvalId"`
	Status            ApprovalStatus       `json:"status"`
	SupportedActions  []string             `json:"supportedActions,omitempty"`
	NativeSessionID   string               `json:"nativeSessionId,omitempty"`
	ReplyTarget       *ApprovalReplyTarget `json:"replyTarget,omitempty"`
	FailureReason     string               `json:"failureReason,omitempty"`
	Actionable        bool                 `json:"actionable"`
	ActionableReason  string               `json:"actionableReason,omitempty"`
	BeadID            string               `json:"beadId,omitempty"`
	BeadTitle         string               `json:"beatTitle,omitempty"`
	RepoPath          string               `json:"repoPath,omitempty"`
	Adapter           string               `json:"adapter,omitempty"`
	Source            string               `json:"source,omitempty"`
	Message           string               `json:"message,omitempty"`
	Question          string               `json:"question,omitempty"`
	ServerName        string               `json:"serverName,omitempty"`
	ToolName          string               `json:"toolName,omitempty"`
	ToolParamsDisplay string               `json:"toolParamsDisplay,omitempty"`
	ParameterSummary  string               `json:"parameterSummary,omitempty"`
	ToolUseID         string               `json:"toolUseId,omitempty"`
	RequestID         string               `json:"requestId,omitempty"`
	PermissionID      string               `json:"permissionId,omitempty"`
	PermissionName    string               `json:"permissionName,omitempty"`
	Patterns          []string             `json:"patterns,omitempty"`
	Options           []string             `json:"options,omitempty"`
	NotificationKey   string               `json:"notificationKey,omitempty"`
	TerminalSessionID string               `json:"terminalSessionId,omitempty"`
	AgentName         string               `json:"agentName,omitempty"`
	AgentModel        string               `json:"agentModel,omitempty"`
	AgentVersion      string               `json:"agentVersion,omitempty"`
	CreatedAt         int64                `json:"createdAt,omitempty"`
	UpdatedAt         int64                `json:"updatedAt,omitempty"`
}

type ApprovalStatus string

const (
	ApprovalPending        ApprovalStatus = "pending"
	ApprovalApproved       ApprovalStatus = "approved"
	ApprovalAlwaysApproved ApprovalStatus = "always_approved"
	ApprovalRejected       ApprovalStatus = "rejected"
	ApprovalManualRequired ApprovalStatus = "manual_required"
	ApprovalDismissed      ApprovalStatus = "dismissed"
	ApprovalResponding     ApprovalStatus = "responding"
	ApprovalReplyFailed    ApprovalStatus = "reply_failed"
	ApprovalUnsupported    ApprovalStatus = "unsupported"
)

type ApprovalAction string

const (
	ActionAccept        ApprovalAction = "accept"
	ActionAlwaysApprove ApprovalAction = "always_approve"
	ActionDecline       ApprovalAction = "decline"
)

type ApprovalReplyResult struct {
	OK     bool   `json:"ok"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

type TakeLoopIterationTrace struct {
	Iteration            int    `json:"iteration"`
	AgentID              string `json:"agentId,omitempty"`
	ClaimedState         string `json:"claimedState,omitempty"`
	PostExitState        string `json:"postExitState,omitempty"`
	ExitCode             int    `json:"exitCode,omitempty"`
	Success              bool   `json:"success"`
	RolledBack           bool   `json:"rolledBack"`
	AlternativeAvailable bool   `json:"alternativeAgentAvailable,omitempty"`
}

type ReleaseKnotsLeaseFunc func(reason string, outcome string, data map[string]any)

type ApprovalResponderFunc func(record *PendingApprovalRecord, action ApprovalAction) (*ApprovalReplyResult, error)

type SessionEntry struct {
	mu       sync.RWMutex
	Session  *TerminalSession
	Events   chan session.TerminalEvent
	Buffer   []session.TerminalEvent
	Runtime  *session.SessionRuntime
	Watchdog *session.Watchdog
	Cancel   context.CancelFunc
	Process  ProcessHandle

	KnotsLeaseID        string
	KnotsLeaseSeq       int
	KnotsLeaseStep      string
	KnotsLeaseAgentInfo *AgentInfo
	LastReleasedLeaseID string

	ReleaseKnotsLease   ReleaseKnotsLeaseFunc
	ApprovalResponder   ApprovalResponderFunc
	ApprovalBridgeURL   string
	ApprovalBridgeToken string

	TakeLoopLifecycle        map[int]*TakeLoopIterationTrace
	PendingApprovals         map[string]*PendingApprovalRecord
	ClaimsPerQueueType       map[string]int
	LastAgentPerQueueType    map[string]string
	FailedAgentsPerQueueType map[string]map[string]bool
	FollowUpAttempts         map[string]int

	InteractionLog InteractionLog
}

type ProcessHandle interface {
	Pid() int
	Signal(sig interface{}) error
	Kill() error
}

type InteractionLog interface {
	LogBeadState(beadID, state, phase, label string)
	LogEnd(exitCode int, status string)
	LogTokenUsage(beadID string, usage session.TokenUsageCounts)
}

type TerminalManager struct {
	mu          sync.RWMutex
	sessions    map[string]*SessionEntry
	maxSessions int
	idCounter   int64
}

func NewTerminalManager(opts ...ManagerOption) *TerminalManager {
	cfg := &managerConfig{
		maxSessions: DefaultMaxSessions,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return &TerminalManager{
		sessions:    make(map[string]*SessionEntry),
		maxSessions: cfg.maxSessions,
	}
}

type managerConfig struct {
	maxSessions int
}

type ManagerOption func(*managerConfig)

func WithMaxSessions(n int) ManagerOption {
	return func(cfg *managerConfig) {
		cfg.maxSessions = n
	}
}

func (m *TerminalManager) CreateSession(ctx context.Context, beadID, repoPath string) (*SessionEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	running := 0
	for _, e := range m.sessions {
		e.mu.RLock()
		s := e.Session.Status
		e.mu.RUnlock()
		if s == StatusRunning {
			running++
		}
	}
	if running >= m.maxSessions {
		return nil, fmt.Errorf("max concurrent sessions (%d) reached", m.maxSessions)
	}

	id := m.generateID()
	sess := &TerminalSession{
		ID:        id,
		BeadID:    beadID,
		RepoPath:  repoPath,
		Status:    StatusRunning,
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	}

	onKill := func(pid int) {
		slog.Warn("[terminal-manager] [watchdog] timeout_fired", "pid", pid)
	}
	watchdog := session.NewWatchdog(0, onKill)

	entry := &SessionEntry{
		Session:                  sess,
		Events:                   make(chan session.TerminalEvent, MaxBuffer),
		Buffer:                   make([]session.TerminalEvent, 0, MaxBuffer),
		Runtime:                  session.NewSessionRuntime(beadID, repoPath),
		Watchdog:                 watchdog,
		TakeLoopLifecycle:        make(map[int]*TakeLoopIterationTrace),
		PendingApprovals:         make(map[string]*PendingApprovalRecord),
		ClaimsPerQueueType:       make(map[string]int),
		LastAgentPerQueueType:    make(map[string]string),
		FailedAgentsPerQueueType: make(map[string]map[string]bool),
		FollowUpAttempts:         make(map[string]int),
	}

	m.sessions[id] = entry
	slog.Info("[terminal-manager] session created",
		"sessionId", id, "beadId", beadID, "repoPath", repoPath)
	return entry, nil
}

func (m *TerminalManager) AbortSession(id string) SignalOutcome {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.sessions[id]
	if !ok {
		return SignalOutcome{OK: false, Reason: "not_found"}
	}

	entry.mu.Lock()
	defer entry.mu.Unlock()

	alreadyExited := exitedStatuses[entry.Session.Status]
	if alreadyExited {
		status := entry.Session.Status
		return SignalOutcome{OK: false, Reason: "already_exited", Status: string(status)}
	}

	entry.Session.Status = StatusAborted

	if entry.Cancel != nil {
		entry.Cancel()
	}
	if entry.Runtime != nil {
		entry.Runtime.Stop()
	}
	if entry.Watchdog != nil {
		entry.Watchdog.Stop()
	}

	if entry.ReleaseKnotsLease != nil {
		entry.ReleaseKnotsLease("abort", "warning", nil)
	}

	slog.Info("[terminal-manager] session aborted", "sessionId", id)
	return SignalOutcome{OK: true, Session: entry.Session}
}

func (m *TerminalManager) TerminateSession(id string) SignalOutcome {
	return m.signalSession(id, "SIGTERM")
}

func (m *TerminalManager) KillSession(id string) SignalOutcome {
	return m.signalSession(id, "SIGKILL")
}

type SignalOutcome struct {
	OK      bool             `json:"ok"`
	Reason  string           `json:"reason,omitempty"`
	Status  string           `json:"status,omitempty"`
	Session *TerminalSession `json:"session,omitempty"`
}

func (m *TerminalManager) signalSession(id, sig string) SignalOutcome {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.sessions[id]
	if !ok {
		return SignalOutcome{OK: false, Reason: "not_found"}
	}

	entry.mu.Lock()
	defer entry.mu.Unlock()

	alreadyExited := exitedStatuses[entry.Session.Status]
	if alreadyExited {
		status := entry.Session.Status
		return SignalOutcome{OK: false, Reason: "already_exited", Status: string(status)}
	}

	if entry.Process == nil {
		if entry.Cancel != nil {
			entry.Cancel()
		}
		entry.Session.Status = StatusAborted
		return SignalOutcome{OK: false, Reason: "already_exited", Status: string(StatusAborted)}
	}

	entry.Session.Status = StatusAborted

	if entry.Cancel != nil {
		entry.Cancel()
	}

	pid := entry.Process.Pid()
	if pid > 0 {
		session.TerminateProcessGroup(pid, "signal_"+sig, session.DefaultKillDelay)
	}

	if sig == "SIGKILL" {
		entry.Process.Kill()
	} else {
		entry.Process.Signal(sig)
	}

	slog.Info("[terminal-manager] session signaled",
		"sessionId", id, "signal", sig, "pid", pid)
	return SignalOutcome{OK: true, Session: entry.Session}
}

func (m *TerminalManager) GetSession(id string) (*SessionEntry, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry, ok := m.sessions[id]
	return entry, ok
}

func (m *TerminalManager) ListSessions() []*TerminalSession {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*TerminalSession, 0, len(m.sessions))
	for _, entry := range m.sessions {
		entry.mu.RLock()
		s := *entry.Session
		pending := make([]*PendingApprovalRecord, 0)
		for _, rec := range entry.PendingApprovals {
			pending = append(pending, rec)
		}
		s.PendingApprovals = pending
		entry.mu.RUnlock()
		result = append(result, &s)
	}
	return result
}

func (m *TerminalManager) RemoveSession(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.sessions[id]
	if !ok {
		return
	}

	entry.mu.Lock()
	if entry.Cancel != nil {
		entry.Cancel()
	}
	if entry.Watchdog != nil {
		entry.Watchdog.Stop()
	}
	entry.mu.Unlock()

	delete(m.sessions, id)
}

func (m *TerminalManager) PushEvent(id string, evt session.TerminalEvent) {
	m.mu.RLock()
	entry, ok := m.sessions[id]
	m.mu.RUnlock()

	if !ok {
		return
	}

	entry.mu.Lock()
	defer entry.mu.Unlock()

	if len(entry.Buffer) >= MaxBuffer {
		entry.Buffer = entry.Buffer[1:]
	}
	entry.Buffer = append(entry.Buffer, evt)

	select {
	case entry.Events <- evt:
	default:
		slog.Warn("[terminal-manager] event channel full, dropping event",
			"type", evt.Type, "sessionId", id)
	}
}

func (m *TerminalManager) GetBuffer(id string) []session.TerminalEvent {
	m.mu.RLock()
	entry, ok := m.sessions[id]
	m.mu.RUnlock()

	if !ok {
		return nil
	}

	entry.mu.RLock()
	defer entry.mu.RUnlock()

	buf := make([]session.TerminalEvent, len(entry.Buffer))
	copy(buf, entry.Buffer)
	return buf
}

func (e *SessionEntry) SetProcess(p ProcessHandle) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Process = p
}

func (e *SessionEntry) SetCancel(cancel context.CancelFunc) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Cancel = cancel
}

func (e *SessionEntry) SetReleaseKnotsLease(fn ReleaseKnotsLeaseFunc) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ReleaseKnotsLease = fn
}

func (e *SessionEntry) SetApprovalResponder(fn ApprovalResponderFunc) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ApprovalResponder = fn
}

func (e *SessionEntry) RecordPendingApproval(record *PendingApprovalRecord) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.PendingApprovals[record.ApprovalID] = record
	e.Session.PendingApprovals = append(e.Session.PendingApprovals, record)
}

func (e *SessionEntry) GetPendingApproval(approvalID string) (*PendingApprovalRecord, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	rec, ok := e.PendingApprovals[approvalID]
	return rec, ok
}

func (e *SessionEntry) UpdateApprovalStatus(approvalID string, status ApprovalStatus) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if rec, ok := e.PendingApprovals[approvalID]; ok {
		rec.Status = status
	}
}

func (e *SessionEntry) SetKnotsLease(leaseID string, agentInfo *AgentInfo) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.KnotsLeaseID = leaseID
	e.KnotsLeaseAgentInfo = agentInfo
	e.LastReleasedLeaseID = ""
	e.SyncSessionLeaseInfoLocked()
}

func (e *SessionEntry) ClearKnotsLease(reason string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.KnotsLeaseID != "" {
		e.LastReleasedLeaseID = e.KnotsLeaseID
	}
	e.KnotsLeaseID = ""
	e.KnotsLeaseAgentInfo = nil
	e.SyncSessionLeaseInfoLocked()
	slog.Info("[terminal-manager] lease cleared",
		"sessionId", e.Session.ID, "reason", reason,
		"lastReleasedLeaseId", e.LastReleasedLeaseID)
}

func (e *SessionEntry) SyncSessionLeaseInfo() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.SyncSessionLeaseInfoLocked()
}

func (e *SessionEntry) SyncSessionLeaseInfoLocked() {
	e.Session.KnotsLeaseID = e.KnotsLeaseID
	if e.KnotsLeaseAgentInfo != nil {
		e.Session.KnotsAgentInfo = &AgentInfo{
			AgentName:     e.KnotsLeaseAgentInfo.AgentName,
			AgentModel:    e.KnotsLeaseAgentInfo.AgentModel,
			AgentVersion:  e.KnotsLeaseAgentInfo.AgentVersion,
			AgentProvider: e.KnotsLeaseAgentInfo.AgentProvider,
		}
	} else {
		e.Session.KnotsAgentInfo = nil
	}
}

func (m *TerminalManager) generateID() string {
	m.idCounter++
	return fmt.Sprintf("term-%d-%06d", time.Now().UnixMilli(), m.idCounter%1000000)
}

func CleanupSessionResources(entry *SessionEntry, reason string) {
	entry.mu.Lock()
	defer entry.mu.Unlock()

	for id, rec := range entry.PendingApprovals {
		rec.Status = ApprovalManualRequired
		rec.Actionable = false
		rec.ActionableReason = "approval_responder_unavailable"
		slog.Warn("[terminal-manager] cleanup: marking approval as manual_required",
			"approvalId", id, "reason", reason)
	}

	if entry.ReleaseKnotsLease != nil {
		entry.ReleaseKnotsLease(reason, "warning", nil)
	}

	if entry.Watchdog != nil {
		entry.Watchdog.Stop()
	}
	if entry.Cancel != nil {
		entry.Cancel()
	}
}
