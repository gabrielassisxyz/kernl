package backend

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

type StepPhase string

const (
	StepPhaseQueued StepPhase = "queued"
	StepPhaseActive StepPhase = "active"
)

type ResolvedStep struct {
	Step  string
	Phase StepPhase
}

type WorkflowRuntimeState struct {
	State               string
	NextActionState     string
	NextActionOwnerKind ActionOwnerKind
	RequiresHumanAction bool
	IsAgentClaimable    bool
}

func stateMismatchError(beadID, expectedState, currentState string) error {
	return fmt.Errorf("Bead %s: expected state '%s' but currently '%s'", beadID, expectedState, currentState)
}

func normalizeProfileID(profileID string) string {
	v := strings.TrimSpace(strings.ToLower(profileID))
	if v == "" {
		return "autopilot"
	}
	switch v {
	case "beads-coarse":
		return "autopilot"
	case "beads-coarse-human-gated":
		return "semiauto"
	case "automatic":
		return "autopilot"
	case "workflow":
		return "semiauto"
	case "knots-granular", "knots-granular-autonomous":
		return "autopilot"
	case "knots-coarse", "knots-coarse-human-gated":
		return "semiauto"
	}
	return v
}

var agentOwners = map[string]ActionOwnerKind{
	"planning":              ActionOwnerAgent,
	"plan_review":           ActionOwnerAgent,
	"implementation":        ActionOwnerAgent,
	"implementation_review": ActionOwnerAgent,
	"integration":           ActionOwnerAgent,
	"integration_review":    ActionOwnerAgent,
	"shipment":              ActionOwnerAgent,
	"shipment_review":       ActionOwnerAgent,
}

var semiautoOwners = map[string]ActionOwnerKind{
	"planning":              ActionOwnerAgent,
	"plan_review":           ActionOwnerHuman,
	"implementation":        ActionOwnerAgent,
	"implementation_review": ActionOwnerHuman,
	"integration":           ActionOwnerAgent,
	"integration_review":    ActionOwnerAgent,
	"shipment":              ActionOwnerAgent,
	"shipment_review":       ActionOwnerAgent,
}

type profileConfig struct {
	ID                       string
	DisplayName              string
	Description              string
	PlanningMode             string
	ImplementationReviewMode string
	Output                   string
	Owners                   map[string]ActionOwnerKind
	InitialState             string // override; empty means compute from PlanningMode
	Stages                   map[string]StageContract
	// ExplicitStates, when non-empty, replaces the derived state list from
	// buildStates. Used by the epic profile, whose lifecycle is a bespoke
	// tail (integration -> integration_review -> shipment -> awaiting_pr_review)
	// that does not match the canonical planning/implementation filter.
	ExplicitStates []string
	// TerminalStates overrides the default {"shipped","abandoned"} stop set.
	TerminalStates []string
	// ExitGates declares per-state exit gates evaluated after an agent exits.
	// Empty means every stage passes on agent_exit_zero (the legacy default).
	ExitGates map[string]WorkflowExitGate
}

var builtinProfiles = []profileConfig{
	{
		ID:                       "epic",
		DisplayName:              "Epic",
		Description:              "Epic lifecycle: integration, integration review, shipment, then awaiting human PR review",
		PlanningMode:             "skipped",
		ImplementationReviewMode: "skipped",
		Output:                   "pr",
		InitialState:             "ready_for_integration",
		Owners: map[string]ActionOwnerKind{
			"integration":        ActionOwnerAgent,
			"integration_review": ActionOwnerAgent,
			"shipment":           ActionOwnerAgent,
		},
		ExplicitStates: []string{
			"ready_for_integration", "integration",
			"ready_for_integration_review", "integration_review",
			"ready_for_shipment", "shipment",
			"awaiting_pr_review",
			"deferred", "abandoned",
		},
		TerminalStates: []string{"awaiting_pr_review", "abandoned"},
		ExitGates: map[string]WorkflowExitGate{
			// integration agent must leave a marker commit on the epic branch.
			"integration": {Type: "commit_marker", Path: "stage: integration"},
			// integration_review agent must write a PASS verdict artifact.
			"integration_review": {Type: "artifact_verdict", Path: "<artifact_dir>/integration-review.md"},
			// shipment agent must record the opened PR URL in the epic description.
			"shipment": {Type: "description_contains", Path: "pr_url:"},
		},
	},
	{
		// worker is the per-child profile inside an epic: it does the bead's
		// own work and STOPS at awaiting_integration, handing the branch to the
		// epic-level integration stage. It deliberately does NOT own integration
		// or shipment - those belong to the epic profile. The orchestrator
		// applies this profile to epic children automatically (epic run).
		ID:                       "worker",
		DisplayName:              "Worker",
		Description:              "Per-child epic worker: implement + review, then hand off at awaiting_integration",
		PlanningMode:             "skipped",
		ImplementationReviewMode: "required",
		Output:                   "branch",
		InitialState:             "ready_for_implementation",
		Owners: map[string]ActionOwnerKind{
			"implementation":        ActionOwnerAgent,
			"implementation_review": ActionOwnerAgent,
		},
		ExplicitStates: []string{
			"ready_for_implementation", "implementation",
			"ready_for_implementation_review", "implementation_review",
			"awaiting_integration",
			"deferred", "abandoned",
		},
		TerminalStates: []string{"awaiting_integration", "abandoned"},
		ExitGates: map[string]WorkflowExitGate{
			// implementation agent must leave a marker commit in the worktree.
			// Without this gate a bead that produced no commits silently sails
			// to awaiting_integration (see kernl-gc7j post-mortem).
			"implementation": {Type: "commit_marker", Path: "stage: implementation"},
			// implementation_review agent must write a PASS verdict artifact.
			"implementation_review": {Type: "artifact_verdict", Path: "<artifact_dir>/implementation-review.md"},
		},
	},
	{
		ID:                       "autopilot",
		DisplayName:              "Autopilot",
		Description:              "Agent-owned full flow with remote main output",
		PlanningMode:             "required",
		ImplementationReviewMode: "required",
		Output:                   "remote_main",
		Owners:                   agentOwners,
	},
	{
		ID:                       "autopilot_with_pr",
		DisplayName:              "Autopilot (PR)",
		Description:              "Agent-owned full flow with PR output",
		PlanningMode:             "required",
		ImplementationReviewMode: "required",
		Output:                   "pr",
		Owners:                   agentOwners,
	},
	{
		ID:                       "semiauto",
		DisplayName:              "Semiauto",
		Description:              "Human-gated plan and implementation reviews",
		PlanningMode:             "required",
		ImplementationReviewMode: "required",
		Output:                   "remote_main",
		Owners:                   semiautoOwners,
	},
	{
		ID:                       "autopilot_no_planning",
		DisplayName:              "Autopilot (no planning)",
		Description:              "Agent-owned flow starting at implementation",
		PlanningMode:             "skipped",
		ImplementationReviewMode: "required",
		Output:                   "remote_main",
		Owners:                   agentOwners,
	},
	{
		ID:                       "autopilot_with_pr_no_planning",
		DisplayName:              "Autopilot (PR, no planning)",
		Description:              "Agent-owned flow with PR output and no planning",
		PlanningMode:             "skipped",
		ImplementationReviewMode: "required",
		Output:                   "pr",
		Owners:                   agentOwners,
	},
	{
		ID:                       "semiauto_no_planning",
		DisplayName:              "Semiauto (no planning)",
		Description:              "Human-gated implementation review with skipped planning",
		PlanningMode:             "skipped",
		ImplementationReviewMode: "required",
		Output:                   "remote_main",
		Owners:                   semiautoOwners,
	},
}

func canonicalTransitions() []WorkflowTransition {
	return []WorkflowTransition{
		{From: "ready_for_planning", To: "planning"},
		{From: "planning", To: "ready_for_plan_review"},
		{From: "ready_for_plan_review", To: "plan_review"},
		{From: "plan_review", To: "ready_for_implementation"},
		{From: "plan_review", To: "ready_for_planning"},
		{From: "ready_for_implementation", To: "implementation"},
		{From: "implementation", To: "ready_for_implementation_review"},
		{From: "ready_for_implementation_review", To: "implementation_review"},
		{From: "implementation_review", To: "awaiting_integration"},
		{From: "implementation_review", To: "ready_for_integration"},
		{From: "implementation_review", To: "ready_for_implementation"},
		{From: "ready_for_integration", To: "integration"},
		{From: "integration", To: "ready_for_integration_review"},
		{From: "ready_for_integration_review", To: "integration_review"},
		{From: "integration_review", To: "ready_for_shipment"},
		{From: "integration_review", To: "ready_for_integration"},
		{From: "ready_for_shipment", To: "shipment"},
		{From: "shipment", To: "awaiting_pr_review"},
		{From: "shipment", To: "ready_for_shipment_review"},
		{From: "ready_for_shipment_review", To: "shipment_review"},
		{From: "shipment_review", To: "shipped"},
		{From: "shipment_review", To: "ready_for_implementation"},
		{From: "shipment_review", To: "ready_for_shipment"},
		{From: "*", To: "deferred"},
		{From: "*", To: "abandoned"},
	}
}

func buildStates(cfg profileConfig) []string {
	if len(cfg.ExplicitStates) > 0 {
		return cfg.ExplicitStates
	}
	all := []string{
		"ready_for_planning", "planning",
		"ready_for_plan_review", "plan_review",
		"ready_for_implementation", "implementation",
		"ready_for_implementation_review", "implementation_review",
		"ready_for_integration", "integration",
		"ready_for_integration_review", "integration_review",
		"ready_for_shipment", "shipment",
		"ready_for_shipment_review", "shipment_review",
		"shipped", "deferred", "abandoned",
	}
	skipPlanning := cfg.PlanningMode != "required"
	skipImplReview := cfg.ImplementationReviewMode != "required"

	filtered := make([]string, 0, len(all))
	for _, s := range all {
		if skipPlanning && (s == "ready_for_planning" || s == "planning" || s == "ready_for_plan_review" || s == "plan_review") {
			continue
		}
		if skipImplReview && (s == "ready_for_implementation_review" || s == "implementation_review") {
			continue
		}
		filtered = append(filtered, s)
	}
	return filtered
}

func filterTransitions(states []string, cfg profileConfig) []WorkflowTransition {
	stateSet := make(map[string]bool, len(states))
	for _, s := range states {
		stateSet[s] = true
	}

	var result []WorkflowTransition
	seen := make(map[string]bool)

	ct := canonicalTransitions()
	if cfg.PlanningMode != "required" {
		ct = append(ct, WorkflowTransition{From: "ready_for_planning", To: "ready_for_implementation"})
	}
	if cfg.ImplementationReviewMode != "required" {
		ct = append(ct, WorkflowTransition{From: "implementation", To: "ready_for_integration"})
	}

	for _, t := range ct {
		if t.From != "*" && !stateSet[t.From] {
			continue
		}
		if !stateSet[t.To] {
			continue
		}
		key := t.From + "->" + t.To
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, t)
	}
	return result
}

func stepOwnerKind(owners map[string]ActionOwnerKind, step string) ActionOwnerKind {
	if k, ok := owners[step]; ok {
		return k
	}
	return ActionOwnerAgent
}

func deriveWorkflowStructureFromConfig(states []string, transitions []WorkflowTransition, owners map[string]ActionOwnerKind, terminalStates []string) (queueStates, actionStates []string, queueActions map[string]string) {
	actionSet := make(map[string]bool, len(owners))
	for k := range owners {
		actionSet[k] = true
	}
	terminalSet := make(map[string]bool, len(terminalStates))
	for _, s := range terminalStates {
		terminalSet[s] = true
	}

	for _, s := range states {
		if actionSet[s] {
			actionStates = append(actionStates, s)
		} else if !terminalSet[s] {
			queueStates = append(queueStates, s)
		}
	}

	queueActions = make(map[string]string)
	for _, q := range queueStates {
		for _, t := range transitions {
			if t.From == q && actionSet[t.To] {
				queueActions[q] = t.To
				break
			}
		}
	}
	return
}

func descriptorFromProfileConfig(cfg profileConfig) WorkflowDescriptor {
	states := buildStates(cfg)
	transitions := filterTransitions(states, cfg)
	terminalStates := []string{"shipped", "abandoned"}
	if len(cfg.TerminalStates) > 0 {
		terminalStates = cfg.TerminalStates
	}
	queueStates, actionStates, queueActions := deriveWorkflowStructureFromConfig(states, transitions, cfg.Owners, terminalStates)

	initialState := "ready_for_planning"
	if cfg.InitialState != "" {
		initialState = cfg.InitialState
	} else if cfg.PlanningMode != "required" {
		initialState = "ready_for_implementation"
	}

	retakeState := "ready_for_implementation"
	hasImpl := false
	for _, s := range states {
		if s == "ready_for_implementation" {
			hasImpl = true
			break
		}
	}
	if !hasImpl {
		retakeState = initialState
	}

	var reviewQueueStates []string
	for _, q := range queueStates {
		if action, ok := queueActions[q]; ok && strings.HasSuffix(action, "_review") {
			reviewQueueStates = append(reviewQueueStates, q)
		}
	}

	var humanQueueStates []string
	for _, q := range queueStates {
		if action, ok := queueActions[q]; ok && stepOwnerKind(cfg.Owners, action) == ActionOwnerHuman {
			humanQueueStates = append(humanQueueStates, q)
		}
	}

	var finalCutState string
	if len(humanQueueStates) > 0 {
		finalCutState = humanQueueStates[0]
	}

	mode := "granular_autonomous"
	for _, k := range cfg.Owners {
		if k == ActionOwnerHuman {
			mode = "coarse_human_gated"
			break
		}
	}

	var stateOwners map[string]ActionOwnerKind
	for _, s := range actionStates {
		if stateOwners == nil {
			stateOwners = make(map[string]ActionOwnerKind)
		}
		stateOwners[s] = stepOwnerKind(cfg.Owners, s)
	}

	desc := WorkflowDescriptor{
		ID:                cfg.ID,
		BackingWorkflowID: cfg.ID,
		Label:             cfg.DisplayName,
		Mode:              mode,
		InitialState:      initialState,
		States:            states,
		TerminalStates:    terminalStates,
		Transitions:       transitions,
		FinalCutState:     finalCutState,
		RetakeState:       retakeState,
		PromptProfileID:   cfg.ID,
		ProfileID:         cfg.ID,
		QueueActions:      queueActions,
		QueueStates:       queueStates,
		ActionStates:      actionStates,
		ReviewQueueStates: reviewQueueStates,
		HumanQueueStates:  humanQueueStates,
		Owners:            cfg.Owners,
		StateOwners:       stateOwners,
	}

	if cfg.Stages != nil {
		desc.Stages = cfg.Stages
	} else {
		desc.Stages = CanonicalStageContracts()
	}
	if cfg.ExitGates != nil {
		desc.ExitGates = cfg.ExitGates
	}
	return desc
}

var builtinWorkflowCache map[string]WorkflowDescriptor

func initBuiltinWorkflows() map[string]WorkflowDescriptor {
	if builtinWorkflowCache != nil {
		return builtinWorkflowCache
	}
	builtinWorkflowCache = make(map[string]WorkflowDescriptor)
	for _, cfg := range builtinProfiles {
		desc := descriptorFromProfileConfig(cfg)
		builtinWorkflowCache[cfg.ID] = desc
	}
	return builtinWorkflowCache
}

func BuiltinProfileDescriptor(profileID string) WorkflowDescriptor {
	normalized := normalizeProfileID(profileID)
	m := initBuiltinWorkflows()
	if desc, ok := m[normalized]; ok {
		return desc
	}
	return m["autopilot"]
}

var (
	workflowRegistryMu sync.RWMutex
	workflowRegistry   = make(map[string]WorkflowDescriptor)
)

// RegisterWorkflow adds a custom workflow descriptor to the package-level registry.
func RegisterWorkflow(wd WorkflowDescriptor) {
	workflowRegistryMu.Lock()
	defer workflowRegistryMu.Unlock()
	normalized := normalizeProfileID(wd.ID)
	workflowRegistry[normalized] = wd
}

// ClearWorkflowRegistry clears all registered custom workflows (used for test isolation).
func ClearWorkflowRegistry() {
	workflowRegistryMu.Lock()
	defer workflowRegistryMu.Unlock()
	workflowRegistry = make(map[string]WorkflowDescriptor)
}

// ResolveWorkflow returns the WorkflowDescriptor for a bead, defaulting to
// the "autopilot" built-in profile when the bead has no explicit profile or
// workflow ID.
func ResolveWorkflow(bead *Bead) WorkflowDescriptor {
	profileID := bead.ProfileID
	if profileID == "" {
		profileID = bead.WorkflowID
	}
	normalized := normalizeProfileID(profileID)
	workflowRegistryMu.RLock()
	wd, ok := workflowRegistry[normalized]
	workflowRegistryMu.RUnlock()
	if ok {
		return wd
	}
	return BuiltinProfileDescriptor(profileID)
}

func ForwardTransitionTarget(currentState string, wf WorkflowDescriptor) (string, bool) {
	if len(wf.Transitions) == 0 {
		return "", false
	}

	statePipelineOrder := map[string]int{
		"ready_for_planning":              0,
		"planning":                        1,
		"ready_for_plan_review":           2,
		"plan_review":                     3,
		"ready_for_implementation":        4,
		"implementation":                  5,
		"ready_for_review":                6,
		"ready_for_implementation_review": 6,
		"review":                          7,
		"implementation_review":           7,
		"awaiting_integration":            8,
		"ready_for_integration":           8,
		"integration":                     9,
		"ready_for_integration_review":    10,
		"integration_review":              11,
		"ready_for_shipment":              12,
		"shipment":                        13,
		"awaiting_pr_review":              14,
		"ready_for_shipment_review":       14,
		"shipment_review":                 15,
		"shipped":                         16,
	}

	for _, t := range wf.Transitions {
		if t.From != currentState {
			continue
		}
		fromIdx, fromOk := statePipelineOrder[t.From]
		toIdx, toOk := statePipelineOrder[t.To]
		if fromOk && toOk && toIdx < fromIdx {
			continue
		}
		return t.To, true
	}
	return "", false
}

// ExitGateContext is everything EvaluateExitGate needs to judge one stage's
// exit. It replaced a five-string positional call that was about to grow a
// sixth (artifact directory) and seventh (base SHA) parameter - a struct
// keeps the call site self-describing instead of a wall of same-typed
// strings that differ only by argument position.
type ExitGateContext struct {
	// FromState is the workflow state the agent just ran in.
	FromState string
	// WorktreePath is the bead's git worktree. commit_marker scans it for
	// the stage's own commits; a gate.Path that does not use the
	// <artifact_dir> placeholder (a custom workflow's own worktree-relative
	// file, or the pre-existing ".kernl/<bead_id>/..." convention) also
	// resolves relative to it.
	WorktreePath string
	// ArtifactDir is where kernl writes exit-gate artifacts for this bead -
	// absolute, and deliberately outside WorktreePath, so a stage's own
	// `git add <files>` can never sweep kernl's control files into the
	// target repository's commits (archeion PR #40 shipped
	// .kernl/<bead>/*.md this way). A gate.Path using <artifact_dir>
	// resolves against it instead of the worktree.
	ArtifactDir string
	BeadID      string
	// BeadDescription is the bead's current description, read fresh after
	// the agent ran, so description_contains gates see markers the agent
	// just wrote there.
	BeadDescription string
	// BaseSHA is the worktree HEAD captured before the agent was dispatched.
	// commit_marker scopes its scan to BaseSHA..HEAD so it only sees commits
	// this stage produced. Empty means no pre-dispatch capture happened;
	// commit_marker fails rather than falling back to scanning the whole
	// branch, which is how an ancestor commit (a sibling merge, the base
	// branch's own history) used to satisfy a gate the stage never earned.
	BaseSHA string
}

// ResolveArtifactPath expands the <bead_id> and <artifact_dir> placeholders
// used in a stage contract's or exit gate's Path/Inputs strings. It is pure
// text substitution - a string with neither placeholder (e.g. "bead.title",
// a descriptive Inputs entry) comes back unchanged, so callers can run every
// entry through it without first checking whether it names a real file.
func ResolveArtifactPath(raw, beadID, artifactDir string) string {
	resolved := strings.ReplaceAll(raw, "<bead_id>", beadID)
	return strings.ReplaceAll(resolved, "<artifact_dir>", artifactDir)
}

// ResolveArtifactFSPath turns a stage contract's or exit gate's Path into the
// real file it names on disk. A path using <artifact_dir> already resolves to
// an absolute location outside the worktree once substituted. A path that
// does not - the pre-existing ".kernl/<bead_id>/..." convention, or a custom
// workflow's own worktree-relative file such as
// examples/custom-workflow's "qa_verdict.txt" - keeps resolving relative to
// the worktree, exactly as it always has, so nothing that predates the
// <artifact_dir> placeholder changes behavior.
func ResolveArtifactFSPath(raw, beadID, worktreePath, artifactDir string) string {
	if strings.Contains(raw, "<artifact_dir>") {
		return ResolveArtifactPath(raw, beadID, artifactDir)
	}
	return filepath.Join(worktreePath, ResolveArtifactPath(raw, beadID, artifactDir))
}

// EvaluateExitGate decides whether a bead may advance past ctx.FromState
// after its agent exited zero. An empty/unknown gate type passes (legacy
// agent_exit_zero).
func EvaluateExitGate(wf WorkflowDescriptor, ctx ExitGateContext) (passed bool, reason string) {
	gate, ok := wf.ExitGates[ctx.FromState]
	if !ok || gate.Type == "" || gate.Type == "agent_exit_zero" {
		return true, ""
	}
	switch gate.Type {
	case "artifact_exists":
		// A <artifact_dir> path with no ArtifactDir would substitute to an
		// empty string and resolve against the filesystem root - looking
		// like a real (missing) path instead of the unresolvable one it is.
		if strings.Contains(gate.Path, "<artifact_dir>") && ctx.ArtifactDir == "" {
			return false, "artifact_dir_unset: " + gate.Path
		}
		abs := ResolveArtifactFSPath(gate.Path, ctx.BeadID, ctx.WorktreePath, ctx.ArtifactDir)
		if _, err := os.Stat(abs); os.IsNotExist(err) {
			return false, "artifact_missing: " + abs
		}
		return true, ""
	case "artifact_verdict":
		if strings.Contains(gate.Path, "<artifact_dir>") && ctx.ArtifactDir == "" {
			return false, "artifact_dir_unset: " + gate.Path
		}
		abs := ResolveArtifactFSPath(gate.Path, ctx.BeadID, ctx.WorktreePath, ctx.ArtifactDir)
		data, err := os.ReadFile(abs)
		if err != nil {
			return false, "artifact_missing: " + abs
		}
		if !strings.HasSuffix(strings.TrimSpace(string(data)), "VERDICT: PASS") {
			return false, "verdict_not_pass: " + abs
		}
		return true, ""
	case "commit_marker":
		if ctx.BaseSHA == "" {
			return false, "commit_marker_unscoped: " + gate.Path
		}
		// `git log <base>..HEAD` means "reachable from HEAD, not reachable
		// from base" - it does NOT require base to be an ancestor of HEAD.
		// If the worktree's history was rewritten under the run (the agent
		// reset or rebased onto a line of history that already contains the
		// marker), that range still evaluates and can admit the marker from
		// an unrelated commit while the stage itself produced nothing -
		// the original defect, reached through a different door. Requiring
		// ancestry first closes it.
		ancestorOut, err := exec.Command("git", "-C", ctx.WorktreePath, "merge-base", "--is-ancestor", ctx.BaseSHA, "HEAD").CombinedOutput()
		if err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
				// Exit code 1 from --is-ancestor is a clean negative answer,
				// not a broken command: base resolved fine, it just is not
				// reachable from HEAD anymore.
				return false, "commit_marker_history_rewritten: base " + ctx.BaseSHA + " is not an ancestor of HEAD"
			}
			return false, "commit_marker_unreadable: " + strings.TrimSpace(string(ancestorOut))
		}
		out, err := exec.Command("git", "-C", ctx.WorktreePath, "log", "--format=%B", ctx.BaseSHA+"..HEAD").CombinedOutput()
		if err != nil {
			return false, "commit_marker_unreadable: " + strings.TrimSpace(string(out))
		}
		if !strings.Contains(string(out), gate.Path) {
			return false, "commit_marker_missing: " + gate.Path
		}
		return true, ""
	case "description_contains":
		if !strings.Contains(ctx.BeadDescription, gate.Path) {
			return false, "description_missing: " + gate.Path
		}
		return true, ""
	default:
		return true, ""
	}
}

func ResolveStepForWorkflow(state string, wf WorkflowDescriptor) (*ResolvedStep, error) {
	actionStates := wf.ActionStates
	if actionStates == nil {
		actionStates = []string{}
	}
	for _, s := range actionStates {
		if s == state {
			return &ResolvedStep{Step: state, Phase: StepPhaseActive}, nil
		}
	}

	queueStates := wf.QueueStates
	if queueStates == nil {
		queueStates = []string{}
	}
	for _, q := range queueStates {
		if q == state {
			if wf.QueueActions != nil {
				if action, ok := wf.QueueActions[q]; ok {
					return &ResolvedStep{Step: action, Phase: StepPhaseQueued}, nil
				}
			}
			return &ResolvedStep{Step: state, Phase: StepPhaseQueued}, nil
		}
	}
	return nil, fmt.Errorf("KERNL DISPATCH FAILURE: state %s not found in workflow", state)
}

func DeriveWorkflowRuntimeState(wf WorkflowDescriptor, workflowState string) WorkflowRuntimeState {
	resolved, err := ResolveStepForWorkflow(workflowState, wf)
	if err != nil {
		return WorkflowRuntimeState{
			State:               workflowState,
			NextActionOwnerKind: ActionOwnerNone,
			RequiresHumanAction: false,
			IsAgentClaimable:    false,
		}
	}

	ownerKind := ActionOwnerNone
	if wf.StateOwners != nil {
		if k, ok := wf.StateOwners[workflowState]; ok {
			ownerKind = k
		}
	}

	if ownerKind == ActionOwnerNone {
		actionState := resolved.Step
		ownerKind = stepOwnerKind(wf.Owners, actionState)
	}

	return WorkflowRuntimeState{
		State:               workflowState,
		NextActionState:     resolved.Step,
		NextActionOwnerKind: ownerKind,
		RequiresHumanAction: ownerKind == ActionOwnerHuman && resolved.Phase == StepPhaseQueued,
		IsAgentClaimable:    resolved.Phase == StepPhaseQueued && ownerKind == ActionOwnerAgent,
	}
}

func isTerminalState(state string, wf WorkflowDescriptor) bool {
	for _, ts := range wf.TerminalStates {
		if ts == state {
			return true
		}
	}
	return state == "deferred"
}

type BeadTransitionResult struct {
	Bead      *Bead
	NextState string
}

func NextBead(backend BackendPort, beadID string, expectedState string, repoPath string) (*BeadTransitionResult, error) {
	bead, err := backend.Get(beadID, repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load bead %s: %v", beadID, err)
	}
	if bead == nil {
		return nil, fmt.Errorf("bead %s not found", beadID)
	}

	if bead.State != expectedState {
		return nil, stateMismatchError(beadID, expectedState, bead.State)
	}

	wf := ResolveWorkflow(bead)
	target, ok := ForwardTransitionTarget(bead.State, wf)
	if !ok {
		if isTerminalState(bead.State, wf) {
			return nil, fmt.Errorf("KERNL DISPATCH FAILURE: state %q is terminal; no forward transition", bead.State)
		}
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: no forward transition from state %q", bead.State)
	}

	updateErr := backend.Update(beadID, UpdateBeadInput{State: target}, repoPath)
	if updateErr != nil {
		return nil, fmt.Errorf("failed to update bead %s: %v", beadID, updateErr)
	}

	return &BeadTransitionResult{Bead: bead, NextState: target}, nil
}

func ClaimBead(backend BackendPort, beadID string, repoPath string) (*BeadTransitionResult, error) {
	bead, err := backend.Get(beadID, repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load bead %s: %v", beadID, err)
	}
	if bead == nil {
		return nil, fmt.Errorf("bead %s not found", beadID)
	}

	wf := ResolveWorkflow(bead)
	resolved, resolveErr := ResolveStepForWorkflow(bead.State, wf)
	if resolveErr != nil || resolved.Phase != StepPhaseQueued {
		return nil, stateMismatchError(beadID, "queued", bead.State)
	}

	runtime := DeriveWorkflowRuntimeState(wf, bead.State)
	if !runtime.IsAgentClaimable {
		return nil, fmt.Errorf("Bead %s: expected state 'agent-claimable' but currently '%s' is not claimable", beadID, bead.State)
	}

	target, ok := ForwardTransitionTarget(bead.State, wf)
	if !ok {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: no forward transition from state %q for bead %s", bead.State, beadID)
	}

	updateErr := backend.Update(beadID, UpdateBeadInput{State: target}, repoPath)
	if updateErr != nil {
		return nil, fmt.Errorf("failed to update bead %s: %v", beadID, updateErr)
	}

	return &BeadTransitionResult{Bead: bead, NextState: target}, nil
}

func ValidateStages(stages map[string]StageContract) error {
	for name, stage := range stages {
		switch stage.Kind {
		case "subprocess":
			if stage.Subprocess == nil || len(stage.Subprocess.Command) == 0 {
				return fmt.Errorf("KERNL DISPATCH FAILURE: %s subprocess stage missing script/command", name)
			}
			if stage.Role != "" {
				return fmt.Errorf("KERNL DISPATCH FAILURE: %s setting both native-only and subprocess fields", name)
			}
		case "native", "":
			if stage.Subprocess != nil {
				return fmt.Errorf("KERNL DISPATCH FAILURE: %s setting both native-only and subprocess fields", name)
			}
		default:
			return fmt.Errorf("KERNL DISPATCH FAILURE: %s unknown stage kind %q", name, stage.Kind)
		}
	}
	return nil
}

// legacyInWorktreeArtifactPrefix is the pre-fix convention for exit-gate
// artifacts: a path rooted inside the bead's own worktree. Kernl's own
// .gitignore covers .kernl/, but the target repository's does not, so an
// artifact written there and then committed - a stage's own
// `git add <files>`, or a workflow definition that predates this fix -
// travels straight into that repository's own commits (the defect PR #40 on
// archeion made public). A workflow that still names this location is not
// "the old convention still supported": nothing consumes it as a fallback,
// so honoring it silently would mean teaching every workflow written from
// an unmigrated example the same bug this project exists to close.
const legacyInWorktreeArtifactPrefix = ".kernl/"

// ValidateArtifactPaths rejects any stage OutputArtifact/Inputs entry, or
// filesystem-based exit gate Path, that still names the legacy in-worktree
// ".kernl/" location instead of the <artifact_dir> placeholder. Called from
// workflow resolution (LoadWorkflowYAML) so a workflow definition written
// before the artifact directory moved outside the worktree fails loud,
// naming the offending stage, instead of quietly reproducing the defect
// that move fixed.
func ValidateArtifactPaths(stages map[string]StageContract, exitGates map[string]WorkflowExitGate) error {
	for name, stage := range stages {
		if strings.Contains(stage.OutputArtifact.Path, legacyInWorktreeArtifactPrefix) {
			return fmt.Errorf("KERNL DISPATCH FAILURE: stage %q output_artifact.path %q uses the legacy in-worktree .kernl/ location - Fix: use <artifact_dir>/... instead, so the artifact is written outside the worktree", name, stage.OutputArtifact.Path)
		}
		for _, inp := range stage.Inputs {
			if strings.Contains(inp, legacyInWorktreeArtifactPrefix) {
				return fmt.Errorf("KERNL DISPATCH FAILURE: stage %q input %q uses the legacy in-worktree .kernl/ location - Fix: use <artifact_dir>/... instead, so the artifact is read from outside the worktree", name, inp)
			}
		}
	}
	for state, gate := range exitGates {
		if gate.Type != "artifact_exists" && gate.Type != "artifact_verdict" {
			// commit_marker and description_contains Path values are marker
			// text and description substrings, not filesystem paths - a
			// ".kernl/" substring there means nothing.
			continue
		}
		if strings.Contains(gate.Path, legacyInWorktreeArtifactPrefix) {
			return fmt.Errorf("KERNL DISPATCH FAILURE: exit gate %q path %q uses the legacy in-worktree .kernl/ location - Fix: use <artifact_dir>/... instead, so the gate checks outside the worktree", state, gate.Path)
		}
	}
	return nil
}
