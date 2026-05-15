package dispatch

import "github.com/gabrielassisxyz/kernl/internal/backend"

type DispatchForensicBoundary string

const (
	BoundaryPreLease       DispatchForensicBoundary = "pre_lease"
	BoundaryPostLease      DispatchForensicBoundary = "post_lease"
	BoundaryPrePromptBuild DispatchForensicBoundary = "pre_prompt_build"
	BoundaryPrePromptSend  DispatchForensicBoundary = "pre_prompt_send"
	BoundaryPostPromptAck  DispatchForensicBoundary = "post_prompt_ack"
	BoundaryPeriodic       DispatchForensicBoundary = "periodic"
	BoundaryPreFollowup    DispatchForensicBoundary = "pre_followup"
	BoundaryPostTurnSuccess DispatchForensicBoundary = "post_turn_success"
	BoundaryPostTurnFailure DispatchForensicBoundary = "post_turn_failure"
	BoundaryPostRollback   DispatchForensicBoundary = "post_rollback"
)

type ForensicCategory string

const (
	CategoryConcurrentClaim       ForensicCategory = "concurrent_claim_detected"
	CategoryDoubleClaim            ForensicCategory = "our_agent_double_claim_suspected"
	CategoryHalfTransition         ForensicCategory = "kno_half_transition_suspected"
	CategoryLeaseTerminated        ForensicCategory = "lease_terminated_unexpectedly"
	CategoryUnknownStateChange     ForensicCategory = "unknown_state_change"
)

type ForensicClassification struct {
	Category         ForensicCategory
	Reasoning        string
	ConflictingLease *backend.Beat
}

type StepEntry struct {
	ID          string `json:"id,omitempty"`
	Step        string `json:"step,omitempty"`
	LeaseID     string `json:"lease_id,omitempty"`
	AgentName   string `json:"agent_name,omitempty"`
	AgentModel  string `json:"agent_model,omitempty"`
	AgentVersion string `json:"agent_version,omitempty"`
	StartedAt   string `json:"started_at,omitempty"`
	EndedAt     string `json:"ended_at,omitempty"`
	FromState   string `json:"from_state,omitempty"`
	ToState     string `json:"to_state,omitempty"`
}

type BeatSnapshot struct {
	Boundary      DispatchForensicBoundary `json:"boundary"`
	CapturedAt    string                   `json:"capturedAt"`
	SessionID     string                   `json:"sessionId"`
	BeatID        string                   `json:"beatId"`
	AgentInfo     *ExecutionAgentInfo      `json:"agentInfo,omitempty"`
	LeaseID       string                   `json:"leaseId,omitempty"`
	Iteration     int                      `json:"iteration,omitempty"`
	ObservedState string                   `json:"observedState,omitempty"`
	ExpectedStep  string                   `json:"expectedStep,omitempty"`
	KernlPID    int                      `json:"kernlpid"`
	ChildPID      int                      `json:"childPid,omitempty"`
	Beat          *backend.Beat            `json:"beat,omitempty"`
	Leases        []backend.Beat           `json:"leases,omitempty"`
	CaptureErrors []string                 `json:"captureErrors,omitempty"`
}

type CaptureContext struct {
	SessionID     string
	BeatID        string
	RepoPath      string
	Iteration     int
	LeaseID       string
	AgentInfo     *ExecutionAgentInfo
	ExpectedStep  string
	ObservedState string
	ChildPID      int
}

type ClassifierSignals struct {
	AgentClaimExitedNonZero   bool
	KernlInitiatedLeaseTerminate bool
}

type SnapshotWriter interface {
	Write(snapshot BeatSnapshot) (string, error)
}

type MemorySnapshotWriter struct {
	Snapshots []MemorySnapshotEntry
	LogRoot   string
}

type MemorySnapshotEntry struct {
	Path     string
	Snapshot BeatSnapshot
}

func NewMemorySnapshotWriter(logRoot string) *MemorySnapshotWriter {
	return &MemorySnapshotWriter{LogRoot: logRoot}
}

func (m *MemorySnapshotWriter) Write(snapshot BeatSnapshot) (string, error) {
	date := snapshot.CapturedAt[:10]
	p := SnapshotPath(SnapshotPathInput{
		LogRoot:    m.LogRoot,
		Date:       date,
		SessionID:  snapshot.SessionID,
		BeatID:     snapshot.BeatID,
		Boundary:   snapshot.Boundary,
		CapturedAt: snapshot.CapturedAt,
	})
	m.Snapshots = append(m.Snapshots, MemorySnapshotEntry{Path: p, Snapshot: snapshot})
	return p, nil
}

type SnapshotPathInput struct {
	LogRoot    string
	Date       string
	SessionID  string
	BeatID     string
	Boundary   DispatchForensicBoundary
	CapturedAt string
}

type PostTurnForensicResult struct {
	Classified bool
	BannerBody string
}

type ForensicDeps struct {
	ShowKnot    func(beatID, repoPath string) (*backend.Beat, error)
	ListLeases  func(repoPath string, activeOnly bool) ([]backend.Beat, error)
	Writer      SnapshotWriter
	LogAudit    func(event string, payload map[string]any)
	PushBanner  func(banner string)
	Now         func() string
}