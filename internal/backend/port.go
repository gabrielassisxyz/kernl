package backend

type ActionOwnerKind string

const (
	ActionOwnerAgent ActionOwnerKind = "agent"
	ActionOwnerHuman ActionOwnerKind = "human"
	ActionOwnerNone   ActionOwnerKind = "none"
)

type BackendPort interface {
	ListWorkflows(repoPath string) ([]WorkflowDescriptor, error)
	List(filters *BeadListFilters, repoPath string) ([]Bead, error)
	ListReady(filters *BeadListFilters, repoPath string) ([]Bead, error)
	Get(id string, repoPath string) (*Bead, error)
	Create(input CreateBeadInput, repoPath string) (*Bead, error)
	Update(id string, input UpdateBeadInput, repoPath string) error
	Delete(id string, repoPath string) error
	Close(id string, reason string, repoPath string) (*TerminalState, error)
	MarkTerminal(id string, targetState string, reason string, repoPath string) error
	Reopen(id string, reason string, repoPath string) error
	Rewind(id string, targetState string, reason string, repoPath string) error
	Search(query string, filters *BeadListFilters, repoPath string) ([]Bead, error)
	Query(expression string, options *BeadQueryOptions, repoPath string) ([]Bead, error)
	AddDependency(blockerID string, blockedID string, repoPath string) error
	RemoveDependency(blockerID string, blockedID string, repoPath string) error
	ListDependencies(id string, repoPath string, options *DependencyListOptions) ([]BeadDependency, error)
	BuildTakePrompt(beadID string, options *TakePromptOptions, repoPath string) (*TakePromptResult, error)
	BuildPollPrompt(options *PollPromptOptions, repoPath string) (*PollPromptResult, error)
	Comment(id string, body string, repoPath string) error
	Capabilities() BackendCapabilities
}

type BackendResult[T any] struct {
	OK    bool
	Data  *T
	Error *BackendError
}

func OkResult[T any](data T) BackendResult[T] {
	return BackendResult[T]{OK: true, Data: &data}
}

func ErrResult[T any](err *BackendError) BackendResult[T] {
	return BackendResult[T]{OK: false, Error: err}
}

type WorkflowDescriptor struct {
	ID               string                        `json:"id"`
	BackingWorkflowID string                       `json:"backingWorkflowId"`
	Label            string                        `json:"label"`
	Mode             string                        `json:"mode"`
	InitialState     string                        `json:"initialState"`
	States           []string                      `json:"states"`
	TerminalStates   []string                      `json:"terminalStates"`
	Transitions      []WorkflowTransition          `json:"transitions,omitempty"`
	FinalCutState    string                        `json:"finalCutState,omitempty"`
	RetakeState      string                        `json:"retakeState"`
	PromptProfileID  string                       `json:"promptProfileId"`
	ProfileID        string                        `json:"profileId,omitempty"`
	Owners           map[string]ActionOwnerKind   `json:"owners,omitempty"`
	StateOwners      map[string]ActionOwnerKind   `json:"stateOwners,omitempty"`
	QueueActions     map[string]string            `json:"queueActions,omitempty"`
	QueueStates      []string                     `json:"queueStates,omitempty"`
	ActionStates     []string                     `json:"actionStates,omitempty"`
	ReviewQueueStates []string                    `json:"reviewQueueStates,omitempty"`
	HumanQueueStates  []string                    `json:"humanQueueStates,omitempty"`
	ExitGates         map[string]WorkflowExitGate `json:"exitGates,omitempty"`
	Stages            map[string]StageContract     `json:"stages,omitempty" yaml:"stages,omitempty"`
}

type WorkflowExitGate struct {
	Type string `json:"type"`
	Path string `json:"path,omitempty"`
}

type StageContract struct {
	Role           string        `json:"role"             yaml:"role"`
	Inputs         []string      `json:"inputs,omitempty"  yaml:"inputs,omitempty"`
	OutputArtifact StageArtifact `json:"outputArtifact"    yaml:"output_artifact"`
	ForbiddenPaths []string      `json:"forbiddenPaths,omitempty" yaml:"forbidden_paths,omitempty"`
}

type StageArtifact struct {
	Path         string `json:"path,omitempty"          yaml:"path,omitempty"`
	Kind         string `json:"kind,omitempty"          yaml:"kind,omitempty"`
	CommitMarker string `json:"commitMarker,omitempty"  yaml:"commit_marker,omitempty"`
	MustEndWith  string `json:"mustEndWith,omitempty"   yaml:"must_end_with,omitempty"`
}

type WorkflowTransition struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type Bead struct {
	ID                  string         `json:"id"`
	Aliases             []string       `json:"aliases,omitempty"`
	Type                string         `json:"type"`
	State               string         `json:"state"`
	Title               string         `json:"title"`
	Description         string         `json:"description,omitempty"`
	Notes               string         `json:"notes,omitempty"`
	Acceptance          string         `json:"acceptance,omitempty"`
	Priority            int            `json:"priority"`
	Labels              []string       `json:"labels"`
	Assignee            string         `json:"assignee,omitempty"`
	Owner               string         `json:"owner,omitempty"`
	ParentID            string         `json:"parentId,omitempty"`
	Due                 string         `json:"due,omitempty"`
	Estimate            int            `json:"estimate"`
	CreatedAt           string         `json:"createdAt"`
	UpdatedAt           string         `json:"updatedAt"`
	ClosedAt            string         `json:"closedAt,omitempty"`
	RepoPath            string         `json:"repoPath,omitempty"`
	Metadata            map[string]any `json:"metadata,omitempty"`
	Invariants          []Invariant   `json:"invariants,omitempty"`
	Dependencies        []BeadDependency `json:"dependencies,omitempty"`
	ProfileID           string         `json:"profileId,omitempty"`
	WorkflowID          string         `json:"workflowId,omitempty"`
	WorkflowMode        string         `json:"workflowMode,omitempty"`
	NextActionState     string         `json:"nextActionState,omitempty"`
	NextActionOwnerKind ActionOwnerKind `json:"nextActionOwnerKind,omitempty"`
	RequiresHumanAction bool           `json:"requiresHumanAction,omitempty"`
	IsAgentClaimable    bool           `json:"isAgentClaimable,omitempty"`
}

type CreateBeadInput struct {
	Title        string   `json:"title"`
	Description  string   `json:"description,omitempty"`
	Type         string   `json:"type,omitempty"`
	Priority     int      `json:"priority"`
	Labels       []string `json:"labels,omitempty"`
	Assignee     string   `json:"assignee,omitempty"`
	Due          string   `json:"due,omitempty"`
	Acceptance   string   `json:"acceptance,omitempty"`
	Notes        string   `json:"notes,omitempty"`
	ParentID     string   `json:"parentId,omitempty"`
	Estimate     int      `json:"estimate,omitempty"`
	Invariants   []Invariant `json:"invariants,omitempty"`
	ProfileID   string   `json:"profileId,omitempty"`
	WorkflowID   string   `json:"workflowId,omitempty"`
}

type UpdateBeadInput struct {
	Title           string     `json:"title,omitempty"`
	Description     string     `json:"description,omitempty"`
	Type            string     `json:"type,omitempty"`
	State           string     `json:"state,omitempty"`
	ProfileID       string     `json:"profileId,omitempty"`
	Priority        *int       `json:"priority,omitempty"`
	ParentID        string     `json:"parentId,omitempty"`
	Labels          []string   `json:"labels,omitempty"`
	SetLabels       []string   `json:"setLabels,omitempty"`
	RemoveLabels    []string   `json:"removeLabels,omitempty"`
	Assignee        string     `json:"assignee,omitempty"`
	Due             string     `json:"due,omitempty"`
	Acceptance      string     `json:"acceptance,omitempty"`
	Notes           string     `json:"notes,omitempty"`
	AddHandoffCapsule string   `json:"addHandoffCapsule,omitempty"`
	Estimate        *int       `json:"estimate,omitempty"`
	AddInvariants   []Invariant `json:"addInvariants,omitempty"`
	RemoveInvariants []Invariant `json:"removeInvariants,omitempty"`
	ClearInvariants bool       `json:"clearInvariants,omitempty"`
}

type BeadListFilters struct {
	Type                string          `json:"type,omitempty"`
	State               string          `json:"state,omitempty"`
	WorkflowID          string          `json:"workflowId,omitempty"`
	Priority            int             `json:"priority,omitempty"`
	Label               string          `json:"label,omitempty"`
	Assignee            string          `json:"assignee,omitempty"`
	Owner               string          `json:"owner,omitempty"`
	Parent              string          `json:"parent,omitempty"`
	ProfileID           string          `json:"profileId,omitempty"`
	RequiresHumanAction bool            `json:"requiresHumanAction,omitempty"`
	NextOwnerKind       ActionOwnerKind `json:"nextOwnerKind,omitempty"`
}

type BeadQueryOptions struct {
	Limit int    `json:"limit,omitempty"`
	Sort  string `json:"sort,omitempty"`
}

type DependencyListOptions struct {
	Type string `json:"type,omitempty"`
}

type BeadDependency struct {
	ID             string `json:"id,omitempty"`
	Aliases        []string `json:"aliases,omitempty"`
	Type           string `json:"type,omitempty"`
	SourceID       string `json:"sourceId,omitempty"`
	TargetID       string `json:"targetId,omitempty"`
	DependencyType string `json:"dependencyType,omitempty"`
	Title          string `json:"title,omitempty"`
	Description    string `json:"description,omitempty"`
	State          string `json:"state,omitempty"`
	Priority       int    `json:"priority,omitempty"`
	IssueType      string `json:"issueType,omitempty"`
	Owner          string `json:"owner,omitempty"`
}

type TerminalState struct {
	State  string `json:"state"`
	Reason string `json:"reason"`
}

type TakePromptOptions struct {
	IsParent      bool     `json:"isParent,omitempty"`
	ChildBeadIDs  []string `json:"childBeadIds,omitempty"`
	KnotsLeaseID  string   `json:"knotsLeaseId,omitempty"`
}

type TakePromptResult struct {
	Prompt  string `json:"prompt"`
	Claimed  bool   `json:"claimed,omitempty"`
}

type PollPromptOptions struct {
	AgentName    string `json:"agentName,omitempty"`
	AgentModel   string `json:"agentModel,omitempty"`
	AgentVersion string `json:"agentVersion,omitempty"`
}

type PollPromptResult struct {
	Prompt    string `json:"prompt"`
	ClaimedID string `json:"claimedId"`
}

type Dependency struct {
	SourceID string `json:"sourceId"`
	TargetID string `json:"targetId"`
	Type     string `json:"type"`
}

var _ = BeadInput{}

type BeadInput = CreateBeadInput