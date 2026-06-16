package backend

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/config"
)

type BackendType string

const (
	BackendTypeAuto  BackendType = "auto"
	BackendTypeCLI   BackendType = "cli"
	BackendTypeKnots BackendType = "knots"
	BackendTypeBeads BackendType = "beads"
)

type BackendEntry struct {
	Port         BackendPort
	Capabilities BackendCapabilities
}

type repoTypeEntry struct {
	resolvedType BackendType
	cachedAt     time.Time
}

type AutoRoutingBackend struct {
	config         *config.Config
	cache          map[BackendType]BackendEntry
	repoTypeCache  map[string]*repoTypeEntry
	cacheMu        sync.RWMutex
	repoCacheTTL   time.Duration
	detectRepoPath func(string) MemoryManagerType
}

func NewAutoRoutingBackend(cfg *config.Config) *AutoRoutingBackend {
	return &AutoRoutingBackend{
		config:         cfg,
		cache:          make(map[BackendType]BackendEntry),
		repoTypeCache:  make(map[string]*repoTypeEntry),
		repoCacheTTL:   30 * time.Second,
		detectRepoPath: DetectMemoryManager,
	}
}

func (a *AutoRoutingBackend) resolveType(method, repoPath string) (BackendType, error) {
	if repoPath == "" {
		return "", newBackendDispatchError("backend", "", method, "repo_path_missing")
	}

	a.cacheMu.RLock()
	cached, ok := a.repoTypeCache[repoPath]
	a.cacheMu.RUnlock()
	if ok && time.Since(cached.cachedAt) < a.repoCacheTTL {
		return cached.resolvedType, nil
	}

	mm := a.detectRepoPath(repoPath)
	var resolved BackendType
	switch mm {
	case MemoryManagerKnots:
		resolved = BackendTypeKnots
	case MemoryManagerBeads:
		resolved = BackendTypeCLI
	default:
		return "", newBackendDispatchError("backend", repoPath, method, "repo_type_unknown")
	}

	a.cacheMu.Lock()
	a.repoTypeCache[repoPath] = &repoTypeEntry{
		resolvedType: resolved,
		cachedAt:     time.Now(),
	}
	a.cacheMu.Unlock()

	return resolved, nil
}

func (a *AutoRoutingBackend) ClearRepoCache(repoPath string) {
	a.cacheMu.Lock()
	defer a.cacheMu.Unlock()
	if repoPath != "" {
		delete(a.repoTypeCache, repoPath)
	} else {
		a.repoTypeCache = make(map[string]*repoTypeEntry)
	}
}

func (a *AutoRoutingBackend) CapabilitiesForRepo(repoPath string) BackendCapabilities {
	resolved, err := a.resolveType("capabilitiesForRepo", repoPath)
	if err != nil {
		slog.Debug("capabilitiesForRepo resolution failed, returning full capabilities", "error", err)
		return FullCapabilities
	}
	entry := a.getBackend(resolved)
	return entry.Capabilities
}

func (a *AutoRoutingBackend) getBackend(bt BackendType) BackendEntry {
	a.cacheMu.Lock()
	defer a.cacheMu.Unlock()

	if existing, ok := a.cache[bt]; ok {
		return existing
	}
	entry := createConcreteBackend(bt, "")
	a.cache[bt] = entry
	return entry
}

func (a *AutoRoutingBackend) backendFor(method, repoPath string) (BackendPort, error) {
	bt, err := a.resolveType(method, repoPath)
	if err != nil {
		return nil, err
	}
	entry := a.getBackend(bt)
	return entry.Port, nil
}

func (a *AutoRoutingBackend) ListWorkflows(repoPath string) ([]WorkflowDescriptor, error) {
	if repoPath == "" {
		return BuiltinWorkflowDescriptors(), nil
	}
	backend, err := a.backendFor("listWorkflows", repoPath)
	if err != nil {
		return nil, err
	}
	return backend.ListWorkflows(repoPath)
}

func (a *AutoRoutingBackend) List(filters *BeadListFilters, repoPath string) ([]Bead, error) {
	backend, err := a.backendFor("list", repoPath)
	if err != nil {
		return nil, err
	}
	return backend.List(filters, repoPath)
}

func (a *AutoRoutingBackend) ListReady(filters *BeadListFilters, repoPath string) ([]Bead, error) {
	backend, err := a.backendFor("listReady", repoPath)
	if err != nil {
		return nil, err
	}
	return backend.ListReady(filters, repoPath)
}

func (a *AutoRoutingBackend) Get(id string, repoPath string) (*Bead, error) {
	backend, err := a.backendFor("get", repoPath)
	if err != nil {
		return nil, err
	}
	return backend.Get(id, repoPath)
}

func (a *AutoRoutingBackend) Create(input CreateBeadInput, repoPath string) (*Bead, error) {
	backend, err := a.backendFor("create", repoPath)
	if err != nil {
		return nil, err
	}
	return backend.Create(input, repoPath)
}

func (a *AutoRoutingBackend) Update(id string, input UpdateBeadInput, repoPath string) error {
	backend, err := a.backendFor("update", repoPath)
	if err != nil {
		return err
	}
	return backend.Update(id, input, repoPath)
}

func (a *AutoRoutingBackend) Delete(id string, repoPath string) error {
	backend, err := a.backendFor("delete", repoPath)
	if err != nil {
		return err
	}
	return backend.Delete(id, repoPath)
}

func (a *AutoRoutingBackend) Close(id string, reason string, repoPath string) (*TerminalState, error) {
	backend, err := a.backendFor("close", repoPath)
	if err != nil {
		return nil, err
	}
	return backend.Close(id, reason, repoPath)
}

func (a *AutoRoutingBackend) MarkTerminal(id string, targetState string, reason string, repoPath string) error {
	backend, err := a.backendFor("markTerminal", repoPath)
	if err != nil {
		return err
	}
	return backend.MarkTerminal(id, targetState, reason, repoPath)
}

func (a *AutoRoutingBackend) Reopen(id string, reason string, repoPath string) error {
	backend, err := a.backendFor("reopen", repoPath)
	if err != nil {
		return err
	}
	return backend.Reopen(id, reason, repoPath)
}

func (a *AutoRoutingBackend) Rewind(id string, targetState string, reason string, repoPath string) error {
	backend, err := a.backendFor("rewind", repoPath)
	if err != nil {
		return err
	}
	return backend.Rewind(id, targetState, reason, repoPath)
}

func (a *AutoRoutingBackend) Search(query string, filters *BeadListFilters, repoPath string) ([]Bead, error) {
	backend, err := a.backendFor("search", repoPath)
	if err != nil {
		return nil, err
	}
	return backend.Search(query, filters, repoPath)
}

func (a *AutoRoutingBackend) Query(expression string, options *BeadQueryOptions, repoPath string) ([]Bead, error) {
	backend, err := a.backendFor("query", repoPath)
	if err != nil {
		return nil, err
	}
	return backend.Query(expression, options, repoPath)
}

func (a *AutoRoutingBackend) AddDependency(blockerID string, blockedID string, repoPath string) error {
	backend, err := a.backendFor("addDependency", repoPath)
	if err != nil {
		return err
	}
	return backend.AddDependency(blockerID, blockedID, repoPath)
}

func (a *AutoRoutingBackend) RemoveDependency(blockerID string, blockedID string, repoPath string) error {
	backend, err := a.backendFor("removeDependency", repoPath)
	if err != nil {
		return err
	}
	return backend.RemoveDependency(blockerID, blockedID, repoPath)
}

func (a *AutoRoutingBackend) ListDependencies(id string, repoPath string, options *DependencyListOptions) ([]BeadDependency, error) {
	backend, err := a.backendFor("listDependencies", repoPath)
	if err != nil {
		return nil, err
	}
	return backend.ListDependencies(id, repoPath, options)
}

func (a *AutoRoutingBackend) BuildTakePrompt(beadID string, options *TakePromptOptions, repoPath string) (*TakePromptResult, error) {
	backend, err := a.backendFor("buildTakePrompt", repoPath)
	if err != nil {
		return nil, err
	}
	return backend.BuildTakePrompt(beadID, options, repoPath)
}

func (a *AutoRoutingBackend) BuildPollPrompt(options *PollPromptOptions, repoPath string) (*PollPromptResult, error) {
	backend, err := a.backendFor("buildPollPrompt", repoPath)
	if err != nil {
		return nil, err
	}
	return backend.BuildPollPrompt(options, repoPath)
}

func (a *AutoRoutingBackend) Comment(id string, body string, repoPath string) error {
	backend, err := a.backendFor("comment", repoPath)
	if err != nil {
		return err
	}
	return backend.Comment(id, body, repoPath)
}

func (a *AutoRoutingBackend) Capabilities() BackendCapabilities {
	return FullCapabilities
}

func createConcreteBackend(bt BackendType, repoPath string) BackendEntry {
	switch bt {
	case BackendTypeCLI, BackendTypeBeads:
		b := NewBdCliBackend(repoPath)
		return BackendEntry{Port: b, Capabilities: b.Capabilities()}
	case BackendTypeKnots:
		b := NewKnotsBackend(repoPath)
		return BackendEntry{Port: b, Capabilities: b.Capabilities()}
	default:
		panic(fmt.Sprintf("KERNL DISPATCH FAILURE: unknown backend type: %s", bt))
	}
}

type BackendDispatchError struct {
	Kind     string
	RepoPath string
	Method   string
	Reason   string
}

func (e *BackendDispatchError) Error() string {
	banner := fmt.Sprintf(
		"KERNL DISPATCH FAILURE: %s %s — method=%s repoPath=%s",
		e.Kind, e.Reason, e.Method, e.RepoPath,
	)
	if e.RepoPath == "" {
		banner = fmt.Sprintf(
			"KERNL DISPATCH FAILURE: %s %s — method=%s repoPath=(empty)",
			e.Kind, e.Reason, e.Method,
		)
	}
	return banner
}

func newBackendDispatchError(kind, repoPath, method, reason string) *BackendDispatchError {
	return &BackendDispatchError{
		Kind:     kind,
		RepoPath: repoPath,
		Method:   method,
		Reason:   reason,
	}
}

func CreateBackend(bt BackendType, repoPath string) BackendEntry {
	switch bt {
	case BackendTypeAuto:
		panic("KERNL DISPATCH FAILURE: CreateBackend with type 'auto' requires AutoRoutingBackend; use NewAutoRoutingBackend instead")
	case BackendTypeCLI, BackendTypeBeads:
		b := NewBdCliBackend(repoPath)
		return BackendEntry{Port: b, Capabilities: b.Capabilities()}
	case BackendTypeKnots:
		b := NewKnotsBackend(repoPath)
		return BackendEntry{Port: b, Capabilities: b.Capabilities()}
	default:
		panic(fmt.Sprintf("KERNL DISPATCH FAILURE: unknown backend type: %s", bt))
	}
}

// CanonicalStageContracts returns the builtin stage contracts for the
// canonical vibe-coding pipeline. Each entry maps a workflow state name
// (planning, implementation, plan_review, etc.) to its role contract,
// allowed inputs, required output artifact, and forbidden globs.
// Called by descriptorFromProfileConfig when a profile has no custom
// stages block.
func CanonicalStageContracts() map[string]StageContract {
	return map[string]StageContract{
		"planning": {
			Role: "Decompose the bead into an actionable plan. Identify subtasks, dependencies, and acceptance criteria. Do not write source code.",
			Inputs: []string{
				"bead.title",
				"bead.description",
				"bead.acceptance",
				"repo state (read-only)",
			},
			OutputArtifact: StageArtifact{
				Path: ".kernl/<bead_id>/plan.md",
			},
			ForbiddenPaths: []string{
				"**/*.go", "**/*.ts", "**/*.py", "**/*.rs",
			},
		},
		"plan_review": {
			Role: "Review the plan for correctness, completeness, and alignment with the bead's acceptance criteria. Produce a verdict.",
			Inputs: []string{
				"bead.description",
				"bead.acceptance",
				".kernl/<bead_id>/plan.md",
			},
			OutputArtifact: StageArtifact{
				Path:        ".kernl/<bead_id>/plan-review.md",
				MustEndWith: "VERDICT: PASS",
			},
			ForbiddenPaths: []string{
				"**/*.go", "**/*.ts", "**/*.py", "**/*.rs",
				".kernl/**/plan.md",
			},
		},
		"implementation": {
			Role: "Implement the plan. Modify code to satisfy the acceptance criteria and the plan's subtasks. Do not modify the plan.",
			Inputs: []string{
				"bead.acceptance",
				".kernl/<bead_id>/plan.md",
				".kernl/<bead_id>/plan-review.md",
			},
			OutputArtifact: StageArtifact{
				Kind:         "commits",
				CommitMarker: "stage: implementation",
			},
			ForbiddenPaths: []string{
				".kernl/**/plan.md",
				".kernl/**/plan-review.md",
			},
		},
		"implementation_review": {
			Role: "Review the implementation against the plan and acceptance criteria. Verify tests pass, code follows conventions, and all subtasks are addressed. Produce a verdict.",
			Inputs: []string{
				"bead.acceptance",
				".kernl/<bead_id>/plan.md",
				"implementation commits",
			},
			OutputArtifact: StageArtifact{
				Path:        ".kernl/<bead_id>/implementation-review.md",
				MustEndWith: "VERDICT: PASS",
			},
			ForbiddenPaths: []string{
				"**/*.go", "**/*.ts", "**/*.py", "**/*.rs",
			},
		},
		"integration": {
			Role: "Integrate this bead's changes into the parent branch. Resolve merge conflicts, ensure cross-bead consistency, and verify the combined codebase passes all tests.",
			Inputs: []string{
				"bead.acceptance",
				"parent branch state",
				"sibling bead artifacts",
			},
			OutputArtifact: StageArtifact{
				Kind:         "commits",
				CommitMarker: "stage: integration",
			},
			ForbiddenPaths: []string{
				".kernl/**/plan.md",
				".kernl/**/plan-review.md",
			},
		},
		"integration_review": {
			Role: "Review the integration. Verify merge conflicts are resolved, no regressions are introduced, and the combined codebase meets quality gates.",
			Inputs: []string{
				"bead.acceptance",
				"integration commits",
				"test results",
			},
			OutputArtifact: StageArtifact{
				Path:        ".kernl/<bead_id>/integration-review.md",
				MustEndWith: "VERDICT: PASS",
			},
			ForbiddenPaths: []string{
				"**/*.go", "**/*.ts", "**/*.py", "**/*.rs",
			},
		},
		"shipment": {
			Role: "Prepare the bead for final merge. Create the pull request, write the PR description with stage summary, and ensure all CI checks are green.",
			Inputs: []string{
				"bead.acceptance",
				"all stage artifacts in .kernl/<bead_id>/",
			},
			OutputArtifact: StageArtifact{
				Kind:         "commits",
				CommitMarker: "stage: shipment",
			},
			ForbiddenPaths: []string{
				"**/*.go", "**/*.ts", "**/*.py", "**/*.rs",
				".kernl/**/plan.md",
				".kernl/**/plan-review.md",
			},
		},
		"shipment_review": {
			Role: "Review the PR for completeness and correctness. Verify the PR description accurately summarizes the work, all stage artifacts are present, and the bead is ready to merge.",
			Inputs: []string{
				"bead.acceptance",
				"PR description",
				"all stage artifacts in .kernl/<bead_id>/",
			},
			OutputArtifact: StageArtifact{
				Path:        ".kernl/<bead_id>/shipment-review.md",
				MustEndWith: "VERDICT: PASS",
			},
			ForbiddenPaths: []string{
				"**/*.go", "**/*.ts", "**/*.py", "**/*.rs",
			},
		},
	}
}

func BuiltinWorkflowDescriptors() []WorkflowDescriptor {
	return []WorkflowDescriptor{
		{
			ID:                "sdlc",
			BackingWorkflowID: "sdlc",
			Label:             "SDLC",
			Mode:              "semiauto",
			InitialState:      "ready_for_implementation",
			States: []string{
				"ready_for_implementation",
				"implementation",
				"ready_for_review",
				"review",
				"ready_for_shipment",
				"shipment",
				"shipped",
			},
			TerminalStates: []string{"shipped", "abandoned", "closed"},
			Transitions: []WorkflowTransition{
				{From: "ready_for_implementation", To: "implementation"},
				{From: "implementation", To: "ready_for_review"},
				{From: "ready_for_review", To: "review"},
				{From: "review", To: "ready_for_implementation"},
				{From: "review", To: "ready_for_shipment"},
				{From: "ready_for_shipment", To: "shipment"},
				{From: "shipment", To: "shipped"},
			},
			RetakeState: "ready_for_implementation",
			ProfileID:   "sdlc",
			QueueActions: map[string]string{
				"ready_for_implementation": "implementation",
				"ready_for_review":         "review",
				"ready_for_shipment":       "shipment",
			},
			QueueStates: []string{
				"ready_for_implementation",
				"ready_for_review",
				"ready_for_shipment",
			},
			ActionStates: []string{
				"implementation",
				"review",
				"shipment",
			},
			ReviewQueueStates: []string{"ready_for_review"},
			HumanQueueStates:  []string{},
			Owners: map[string]ActionOwnerKind{
				"ready_for_implementation": ActionOwnerAgent,
				"implementation":           ActionOwnerAgent,
				"ready_for_review":         ActionOwnerAgent,
				"review":                   ActionOwnerHuman,
				"ready_for_shipment":       ActionOwnerAgent,
				"shipment":                 ActionOwnerAgent,
			},
			StateOwners: map[string]ActionOwnerKind{
				"implementation": ActionOwnerAgent,
				"review":         ActionOwnerHuman,
				"shipment":       ActionOwnerAgent,
			},
		},
	}
}

func AutoRouteBackendFromConfig(repoPath string, repos []config.RepoEntry) (BackendPort, error) {
	if repoPath == "" {
		return nil, newBackendDispatchError("backend", "", "autoRoute", "repo_path_missing")
	}

	for _, repo := range repos {
		if repo.Path == repoPath {
			mm := repo.MemoryManager
			if mm == "" {
				mm = string(DetectMemoryManager(repoPath))
			}
			switch mm {
			case "knots":
				return NewKnotsBackend(repoPath), nil
			case "beads", "":
				return NewBdCliBackend(repoPath), nil
			default:
				return nil, newBackendDispatchError("backend", repoPath, "autoRoute", "repo_type_unknown")
			}
		}
	}

	return nil, newBackendDispatchError("backend", repoPath, "autoRoute", "repo_path_missing")
}

var detectMemoryManagerForAutoRoute = DetectMemoryManager

func AutoRouteBackendWithDetection(repoPath string) (BackendPort, error) {
	if repoPath == "" {
		return nil, newBackendDispatchError("backend", "", "autoRoute", "repo_path_missing")
	}

	mm := detectMemoryManagerForAutoRoute(repoPath)
	switch mm {
	case MemoryManagerKnots:
		return NewKnotsBackend(repoPath), nil
	case MemoryManagerBeads:
		return NewBdCliBackend(repoPath), nil
	default:
		return nil, newBackendDispatchError("backend", repoPath, "autoRoute", "repo_type_unknown")
	}
}

func AutoRouteFromConfig(cfg *config.Config, repoPath string) (BackendPort, error) {
	if repoPath == "" {
		return nil, newBackendDispatchError("backend", "", "autoRoute", "repo_path_missing")
	}

	for _, repo := range cfg.Registry.Repos {
		if repo.Path == repoPath {
			mm := repo.MemoryManager
			if mm == "" {
				detected := DetectMemoryManager(repoPath)
				mm = string(detected)
			}
			switch mm {
			case "knots":
				return NewKnotsBackend(repoPath), nil
			case "beads", "":
				return NewBdCliBackend(repoPath), nil
			default:
				return nil, newBackendDispatchError("backend", repoPath, "autoRoute", "repo_type_unknown")
			}
		}
	}

	detected := DetectMemoryManager(repoPath)
	switch detected {
	case MemoryManagerKnots:
		return NewKnotsBackend(repoPath), nil
	case MemoryManagerBeads:
		return NewBdCliBackend(repoPath), nil
	default:
		return nil, newBackendDispatchError("backend", repoPath, "autoRoute", "repo_type_unknown")
	}
}
