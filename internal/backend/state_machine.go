package backend

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"
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
	// ExitGates declares, per state, every gate evaluated after an agent
	// exits. All gates declared for a state must pass. Empty means every
	// stage passes on agent_exit_zero (the legacy default).
	ExitGates map[string][]WorkflowExitGate
}

// canonicalImplementationExitGates gates the canonical pipeline's
// "implementation" state on the decision record described in
// CanonicalStageContracts. It is wired onto "autopilot_with_pr" - the
// profile canonical.yaml mirrors - and deliberately not onto "autopilot"
// (TestEvaluateExitGate_Total pins "autopilot" as carrying no exit gates at
// all). "worker" carries the same decision_record gate too, but alongside
// its own pre-existing commit_marker gate on the same state (see
// builtinProfiles below) - now that ExitGates holds a list per state,
// combining the two is exactly what this gate exists to make possible.
//
// "epic" deliberately does NOT carry it. decision_record checks the JSON
// document (one or more decisions, each carrying the four required fields:
// decision, optionsConsidered, tradeOffs, rationale - see
// DecisionRecordEntry) a stage's own implementer writes down before coding -
// that is what the stage contract's Role text asks for and who it
// addresses. Epic's
// "integration" state is not that stage: its real prompt (RenderIntegration
// in cmd/kernl/epic.go, which replaces the generic stage-contract prompt
// entirely for that state) instructs the agent only to merge child
// branches, resolve conflicts, and verify tests - it never asks for a
// decision record, and an ordinary conflict-free merge usually has no open
// design choice to record. Gating a stage on an artifact its own prompt
// never asks for, and which often has nothing honest to contain, does not
// add safety - it forces the agent to fabricate content to get past the
// gate, corrupting the exact record this check exists to keep trustworthy.
var canonicalImplementationExitGates = map[string][]WorkflowExitGate{
	"implementation": {{Type: "decision_record", Path: "<artifact_dir>/decision-record.json"}},
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
		ExitGates: map[string][]WorkflowExitGate{
			// integration agent must leave at least one new commit on the
			// epic branch. It deliberately does NOT also carry
			// decision_record: integration is a merge stage, not an
			// implementer's stage - see canonicalImplementationExitGates for
			// why that distinction matters here.
			"integration": {{Type: "commit_marker"}},
			// integration_review agent must write a PASS verdict artifact.
			"integration_review": {{Type: "artifact_verdict", Path: "<artifact_dir>/integration-review.md"}},
			// shipment agent must record the opened PR URL in the epic description.
			"shipment": {{Type: "description_contains", Path: "pr_url:"}},
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
		ExitGates: map[string][]WorkflowExitGate{
			// implementation agent must leave at least one new commit in the
			// worktree (without this gate a bead that produced no commits
			// silently sails to awaiting_integration - see kernl-gc7j
			// post-mortem) AND record the decisions behind the
			// implementation, same as autopilot_with_pr's
			// canonicalImplementationExitGates. The shared
			// CanonicalStageContracts "implementation" entry already carries
			// a matching DecisionRecord.Path, so no Stages override is
			// needed here (contrast with epic's "integration" above).
			"implementation": {
				{Type: "commit_marker"},
				{Type: "decision_record", Path: "<artifact_dir>/decision-record.json"},
			},
			// implementation_review agent must write a PASS verdict artifact.
			"implementation_review": {{Type: "artifact_verdict", Path: "<artifact_dir>/implementation-review.md"}},
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
		ExitGates:                canonicalImplementationExitGates,
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
		// A YAML-loaded workflow goes through this same containment check in
		// LoadWorkflowYAML before it is ever returned; a builtin never went
		// through the loader, so nothing enforced it here. That is the exact
		// invariant kernl's control files being written outside the bead's
		// worktree exists to guarantee (see legacyInWorktreeArtifactPrefix
		// and validateDecisionRecordPathContained) - a builtin profile is
		// this project's own hardcoded data, not user input, so a violation
		// here is a defect in this file and panicking at first access (the
		// descriptor is built once and cached) is the fail-loud response,
		// matching createConcreteBackend's panic on an unknown backend type.
		if err := ValidateArtifactPaths(desc.Stages, desc.ExitGates); err != nil {
			panic(fmt.Sprintf("KERNL DISPATCH FAILURE: builtin profile %q failed its own artifact path validation: %v", cfg.ID, err))
		}
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

// FindExitGateByType returns the first gate of type gateType declared for
// state, or ok=false if the state carries no gate of that type. ExitGates
// holds a list per state precisely so a state can carry more than one gate
// (e.g. worker's "implementation" carries both commit_marker and
// decision_record); a caller that wants to know "does this state require
// gate type X", or needs that gate's own Path to act on, starts here instead
// of re-deriving the same scan - EvaluateExitGate itself does not use this,
// since it needs every gate on the state, not one by type.
func FindExitGateByType(wf WorkflowDescriptor, state, gateType string) (gate WorkflowExitGate, ok bool) {
	for _, g := range wf.ExitGates[state] {
		if g.Type == gateType {
			return g, true
		}
	}
	return WorkflowExitGate{}, false
}

// EvaluateExitGate decides whether a bead may advance past ctx.FromState
// after its agent exited zero. A state with no declared gates - no entry,
// or an empty list - passes (legacy agent_exit_zero). When a state declares
// more than one gate, ALL of them must pass; every failure is evaluated and
// reported, not just the first one hit, so a run that fixes one failure does
// not have to wait a full stage round trip to discover the next.
func EvaluateExitGate(wf WorkflowDescriptor, ctx ExitGateContext) (passed bool, reason string) {
	gates := wf.ExitGates[ctx.FromState]
	var failures []string
	for _, gate := range gates {
		if ok, gateReason := evaluateSingleExitGate(gate, ctx); !ok {
			failures = append(failures, gateReason)
		}
	}
	if len(failures) > 0 {
		return false, joinExitGateFailures(failures)
	}
	return true, ""
}

// exitGateFailureEscaper escapes a single failure's own text before it is
// joined with others by joinExitGateFailures. Backslash goes first, so the
// escape it introduces is not itself mistaken for one already present in the
// input; the delimiter character goes second.
var exitGateFailureEscaper = strings.NewReplacer(`\`, `\\`, `;`, `\;`)

// joinExitGateFailures joins per-gate failure reasons into the single string
// EvaluateExitGate returns. A plain "; ".join is ambiguous: several of the
// per-gate reason vocabularies embed operator-configured or externally
// sourced free text - a description_contains gate's own configured
// substring, a commit_marker's marker text, a git diagnostic - and none of
// it is validated against containing "; " itself. A gate configured with
// path "foo; artifact_missing: /record" that fails would otherwise produce
// a joined string indistinguishable from two real failures, one of them
// entirely fabricated from the configured text. Escaping each failure's own
// backslashes and semicolons before joining keeps the boundary between
// failures recoverable regardless of what any one gate's detail contains,
// without changing the per-gate vocabulary itself (evaluateSingleExitGate's
// reason strings, and the single-failure case, are unescaped byte-for-byte
// unless a failure happens to contain "\" or ";").
func joinExitGateFailures(failures []string) string {
	if len(failures) == 1 {
		return failures[0]
	}
	escaped := make([]string, len(failures))
	for i, f := range failures {
		escaped[i] = exitGateFailureEscaper.Replace(f)
	}
	return strings.Join(escaped, "; ")
}

// evaluateSingleExitGate judges exactly one gate against ctx. Its reason
// strings are the vocabulary EvaluateExitGate's callers and tests key off of
// (commit_marker_missing, artifact_missing, verdict_not_pass, ...); when a
// state carries several gates, each one's reason already names which check
// failed - joining them (see EvaluateExitGate) does not need to add another
// layer of prefixing on top.
// rejectableVerdictStates are the review stages whose "VERDICT: REJECT" is
// acted on rather than merely failed. Adding a state here without also giving
// it a destination for the rejection turns a review that says "this is wrong"
// into a review that says "this did not pass", which is the same outcome by a
// more confusing route.
var rejectableVerdictStates = map[string]bool{
	"integration_review":    true,
	"implementation_review": true,
}

// IsRejectableVerdictState reports whether a rejection at this state has
// somewhere to go. Exported for the prompt builder: a reviewer that is only
// told how to spell approval invents its own word for the other outcome, and
// an invented word is read as a review that produced no verdict rather than
// as a rejection - which blocks the work for a human instead of sending it
// back. The gate and the prompt have to agree on one word, so they read it
// from the same place.
func IsRejectableVerdictState(state string) bool {
	return rejectableVerdictStates[state]
}

func evaluateSingleExitGate(gate WorkflowExitGate, ctx ExitGateContext) (passed bool, reason string) {
	if gate.Type == "" || gate.Type == "agent_exit_zero" {
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
		// The document must END WITH the literal line "VERDICT: PASS" (or,
		// for integration_review, "VERDICT: REJECT") - not merely contain
		// that text as a trailing substring. HasSuffix on the whole blob
		// would let "NOT A VALID VERDICT: PASS" (one line, no real
		// sentinel at all) pass the gate, because that string IS a suffix
		// of itself. Comparing the trimmed LAST LINE by exact equality is
		// what "ends with the literal line" actually means.
		trimmed := strings.TrimSpace(string(data))
		lastLine := trimmed
		if idx := strings.LastIndexByte(trimmed, '\n'); idx != -1 {
			lastLine = strings.TrimSpace(trimmed[idx+1:])
		}
		if lastLine == "VERDICT: PASS" {
			return true, ""
		}
		// Two stages have somewhere to send a rejection, and only those two:
		// integration_review hands one to the §7 fix-up mechanism, and
		// implementation_review hands one back to its own implementer
		// (DriveBeadToTerminal rewinds the bead to the workflow's retake
		// state). This exact reason string is what both read to tell a
		// reviewer's deliberate "this is wrong" apart from every other way
		// the gate fails - missing file, a reviewer that ran out of budget,
		// one that wrote incoherent output. Those must keep reading as
		// verdict_not_pass, the same generic stage failure they always were,
		// rather than silently becoming rejections that send work back.
		//
		// plan_review and shipment_review still have nowhere to send one, so
		// REJECT stays unrecognised for them: a non-PASS verdict there means
		// only "this did not pass."
		if rejectableVerdictStates[ctx.FromState] && lastLine == "VERDICT: REJECT" {
			return false, "verdict_reject: " + abs
		}
		return false, "verdict_not_pass: " + abs
	case "commit_marker":
		// This gate used to also require gate.Path - a literal string like
		// "stage: implementation" - to appear in a commit message: proof the
		// agent typed kernl's own vocabulary into the target repository's
		// history. ValidateArtifactPaths now refuses to load a workflow
		// whose commit_marker gate still sets a path, so nothing reaches
		// here still expecting that check; what is left is the part that
		// actually answers this gate's purpose - did the stage leave any new
		// commit at all.
		if ctx.BaseSHA == "" {
			return false, "commit_marker_unscoped: " + ctx.FromState
		}
		// `git log <base>..HEAD` means "reachable from HEAD, not reachable
		// from base" - it does NOT require base to be an ancestor of HEAD.
		// If the worktree's history was rewritten under the run (the agent
		// reset or rebased onto an unrelated line of history that already
		// carries commits of its own), that range still evaluates and can
		// admit a commit that has nothing to do with this stage while the
		// stage itself produced nothing - the original defect, reached
		// through a different door. Requiring ancestry first closes it.
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
		// Only the commit hashes are needed - the range's emptiness is the
		// entire question now, not what any commit in it says.
		out, err := exec.Command("git", "-C", ctx.WorktreePath, "log", "--format=%H", ctx.BaseSHA+"..HEAD").CombinedOutput()
		if err != nil {
			return false, "commit_marker_unreadable: " + strings.TrimSpace(string(out))
		}
		if strings.TrimSpace(string(out)) == "" {
			return false, "commit_marker_missing: " + ctx.FromState
		}
		// A non-empty range is not proof of work by itself: `git commit
		// --allow-empty` leaves the tree byte-for-byte identical to base
		// while still landing a commit reachable from HEAD and not from
		// base - the exact bypass the old marker-string check also missed
		// (it only asked for a string in the message, never a change in the
		// tree). `git diff --quiet` exits 0 when the two trees match and 1
		// when they differ, so a clean 0 here means every commit in range
		// was empty. That is the same answer commit_marker_missing already
		// gives ("the stage left nothing"), not a new outcome.
		diffOut, err := exec.Command("git", "-C", ctx.WorktreePath, "diff", "--quiet", ctx.BaseSHA, "HEAD").CombinedOutput()
		if err == nil {
			return false, "commit_marker_missing: " + ctx.FromState
		}
		var diffExitErr *exec.ExitError
		if !errors.As(err, &diffExitErr) || diffExitErr.ExitCode() != 1 {
			return false, "commit_marker_unreadable: " + strings.TrimSpace(string(diffOut))
		}
		// Exit 1 is `git diff --quiet`'s clean "yes, they differ" answer.
		return true, ""
	case "description_contains":
		if !strings.Contains(ctx.BeadDescription, gate.Path) {
			return false, "description_missing: " + gate.Path
		}
		return true, ""
	case "decision_record":
		// Unlike artifact_exists/artifact_verdict, this gate reads the
		// file's structure, not just its existence or its last line - an
		// empty file satisfies artifact_exists but must not satisfy this
		// one (that gap is why this gate type exists at all).
		if strings.Contains(gate.Path, "<artifact_dir>") && ctx.ArtifactDir == "" {
			return false, "artifact_dir_unset: " + gate.Path
		}
		abs := ResolveArtifactFSPath(gate.Path, ctx.BeadID, ctx.WorktreePath, ctx.ArtifactDir)
		data, err := os.ReadFile(abs)
		if err != nil {
			return false, "artifact_missing: " + abs
		}
		// ParseDecisionRecordDocument's own error text already IS the gate
		// reason: a "decision_record_invalid_json"/"decision_record_empty"/
		// "decision_record_missing_fields" family naming exactly what is
		// wrong, the same discipline the old heading parser's
		// "missing_sections" reason followed - an agent told "invalid"
		// cannot fix it, one told which field at which index is missing can.
		if _, err := ParseDecisionRecordDocument(string(data)); err != nil {
			return false, err.Error()
		}
		return true, ""
	default:
		return true, ""
	}
}

// DecisionRecordEntry is one decision inside a decision_record artifact: an
// agent-authored JSON object, not a markdown section under a heading. The
// gate this schema replaced modeled exactly one decision (four fixed
// headings, matched by exact key); an implementer facing more than one
// decision in a single stage had no way to say so and each invented its own
// syntax for it - "## Decision: <subject>", "## Decision 1:" / "## Decision
// 2:" - none of which the heading parser recognized, so the gate rejected
// finished, committed work with a misleading "missing: decision". The
// artifact's top level is a LIST of these for exactly that reason: it is
// the fix, not a convenience.
//
// Title is optional - a record with a single decision does not need one to
// be unambiguous - but is worth writing when a record carries several, so a
// reader (and the run report) has a short label per entry instead of only an
// array index.
type DecisionRecordEntry struct {
	Title             string `json:"title,omitempty"`
	Decision          string `json:"decision"`
	OptionsConsidered string `json:"optionsConsidered"`
	TradeOffs         string `json:"tradeOffs"`
	Rationale         string `json:"rationale"`
}

// decisionRecordDocument is the on-disk envelope for a decision_record
// artifact: {"decisions": [...]}, rather than a bare array, so the schema
// has room to grow a sibling field later without breaking every existing
// record (YAGNI: no such field exists yet, this is just why the envelope is
// an object and not the array itself).
type decisionRecordDocument struct {
	Decisions []DecisionRecordEntry `json:"decisions"`
}

// decisionRecordRequiredField pairs each DecisionRecordEntry field this
// gate requires with its JSON wire name, so a missing-field error can name
// the exact field an agent typed (or omitted) rather than the gate's
// internal vocabulary for it.
type decisionRecordRequiredField struct {
	wireName string
	value    func(DecisionRecordEntry) string
}

var decisionRecordRequiredFields = []decisionRecordRequiredField{
	{"decision", func(e DecisionRecordEntry) string { return e.Decision }},
	{"optionsConsidered", func(e DecisionRecordEntry) string { return e.OptionsConsidered }},
	{"tradeOffs", func(e DecisionRecordEntry) string { return e.TradeOffs }},
	{"rationale", func(e DecisionRecordEntry) string { return e.Rationale }},
}

// maxDecisionRecordEntries bounds how many decisions a single decision_record
// artifact may declare. Every real record this gate's own motivating cases
// produced (the three implementers whose incompatible invented syntaxes
// prompted this whole redesign) carried one to three decisions; twenty is
// already a generous multiple of that. The bound exists because nothing else
// on this path bounds entry count: WriteDecisionRecordNode writes one
// Decision node and up to three has_decision edges PER entry, in one
// transaction, and app.ComposeRunReport's own field-4 composer call runs
// SEQUENTIALLY, one decision at a time, at up to 60 seconds each (see
// app.impactComposeTimeout) - an unbounded entry count turns one runaway
// record (measured: 1000 entries) into a run report that does not finish for
// roughly 16 hours, not a config value anyone chose, and never a shape a
// deliberated decision list has a legitimate reason to take.
const maxDecisionRecordEntries = 20

// maxDecisionRecordFieldBytes bounds any one field's length - title,
// decision, optionsConsidered, tradeOffs, or rationale. This gate's own
// fixtures and this project's one measured production record (see
// realWorldDecisionEntry in app/revert_decision_test.go) run a few hundred
// bytes per field at most; the limit here is two orders of magnitude above
// that, so no genuine record is ever affected, while a field an agent filled
// with, say, an entire source file or a log dump is rejected instead of
// silently becoming the Decision node's Body/Context/Outcome content and
// the composer's prompt input.
const maxDecisionRecordFieldBytes = 20_000

// maxDecisionRecordDocumentBytes bounds the raw artifact file's total size,
// checked before the content is unmarshalled at all - json.Unmarshal has no
// size limit of its own and would otherwise allocate for a file of any size
// before this gate ever gets to look at entry count or field size. Derived
// from the two limits above (worst case: every one of maxDecisionRecordEntries
// entries has all 5 string fields at maxDecisionRecordFieldBytes each) with a
// 2x multiplier for JSON syntax overhead - keys, quoting, commas, whitespace
// - rather than picked independently, so it can never reject a document that
// is otherwise within both of those limits.
const maxDecisionRecordDocumentBytes = maxDecisionRecordEntries * 5 * maxDecisionRecordFieldBytes * 2

// decisionRecordHeadingRe matches an ATX heading ("## Text", 1-6 hashes)
// against a single already-comment-stripped line. It intentionally does not
// require exactly two hashes - any level is accepted, matching how the rest
// of this parser is level-agnostic.
var decisionRecordHeadingRe = regexp.MustCompile(`^[ \t]{0,3}(#{1,6})[ \t]+(.+?)[ \t]*$`)

// setextEqualsRe and setextDashesRe match a setext heading's underline line
// (CommonMark: one or more "=" makes the preceding paragraph an H1, one or
// more "-" makes it an H2) once trimmed of surrounding whitespace.
var setextEqualsRe = regexp.MustCompile(`^=+$`)
var setextDashesRe = regexp.MustCompile(`^-+$`)

// isThematicBreak reports whether a trimmed line is a markdown horizontal
// rule: three or more of the same character from {-, *, _}, optionally
// separated by whitespace. It exists so a section body consisting of nothing
// but a presentation token does not read as content (review finding: "a
// horizontal rule ... counts as content"), and so a "---" is not mistaken
// for real paragraph text when deciding whether it is eligible to become a
// setext heading's underline. Written as an explicit scan rather than a
// regexp with a backreference, since Go's regexp package (RE2) does not
// support backreferences.
func isThematicBreak(trimmed string) bool {
	if trimmed == "" {
		return false
	}
	for _, ch := range []byte{'-', '*', '_'} {
		ok := true
		count := 0
		for i := 0; i < len(trimmed); i++ {
			if trimmed[i] == ch {
				count++
				continue
			}
			if trimmed[i] == ' ' || trimmed[i] == '\t' {
				continue
			}
			ok = false
			break
		}
		if ok && count >= 3 {
			return true
		}
	}
	return false
}

// isAtxHeadingText reports whether a trimmed line reads as an ATX heading -
// used both to recognize real ATX headings and to disqualify a line from
// being reinterpreted as setext heading text (a line that already opens its
// own heading block cannot also be "paragraph text" for the next line).
func isAtxHeadingText(trimmed string) bool {
	return decisionRecordHeadingRe.MatchString(trimmed)
}

// decisionRecordLine is one line of a decision record after block-context
// resolution: fenced code blocks and HTML comments are tracked across line
// boundaries so their content is never mistaken for a heading, and comment
// interiors are blanked (not deleted, so line numbers stay aligned) so they
// never count as section content either.
type decisionRecordLine struct {
	visible string // comment-stripped text; equals the raw line outside comments and fences
	inFence bool   // true for a fence delimiter line or any line inside one
}

// classifyDecisionRecordLines walks a decision record's lines once, tracking
// fenced-code-block state and HTML-comment state, and returns the
// comment-stripped, fence-flagged view the rest of the parser reads instead
// of the raw lines. This is the fix for two bypasses: a heading fenced as
// example code, and a heading (or a section's entire body) hidden inside an
// HTML comment - neither is a real, visible heading or a real, visible
// section body, and the two amount to writing nothing while the file on disk
// looks complete.
func classifyDecisionRecordLines(lines []string) []decisionRecordLine {
	fenceOpenRe := regexp.MustCompile("^[ \t]{0,3}(`{3,}|~{3,})")

	out := make([]decisionRecordLine, len(lines))
	inFence := false
	var fenceChar byte
	fenceLen := 0
	inComment := false

	for i, line := range lines {
		if inFence {
			out[i] = decisionRecordLine{visible: line, inFence: true}
			leading := strings.TrimLeft(line, " \t")
			if len(leading) >= fenceLen {
				run := 0
				for run < len(leading) && leading[run] == fenceChar {
					run++
				}
				if run >= fenceLen && strings.TrimSpace(leading[run:]) == "" {
					inFence = false
				}
			}
			continue
		}

		if m := fenceOpenRe.FindString(line); m != "" {
			marker := strings.TrimLeft(m, " \t")
			fenceChar = marker[0]
			fenceLen = len(marker)
			inFence = true
			out[i] = decisionRecordLine{visible: line, inFence: true}
			continue
		}

		visible, stillOpen := stripHTMLCommentFromLine(line, inComment)
		inComment = stillOpen
		out[i] = decisionRecordLine{visible: visible, inFence: false}
	}
	return out
}

// stripHTMLCommentFromLine blanks any "<!-- ... -->" span on one line,
// carrying comment state across the line boundary in both directions (a
// comment opened here and closed later, or closed here having opened on an
// earlier line). It handles more than one comment per line by design - a
// single-line "<!-- a --> real text <!-- b -->" leaves only "real text".
func stripHTMLCommentFromLine(line string, inComment bool) (string, bool) {
	var b strings.Builder
	i := 0
	for i < len(line) {
		if inComment {
			idx := strings.Index(line[i:], "-->")
			if idx == -1 {
				return b.String(), true
			}
			i += idx + len("-->")
			inComment = false
			continue
		}
		idx := strings.Index(line[i:], "<!--")
		if idx == -1 {
			b.WriteString(line[i:])
			break
		}
		b.WriteString(line[i : i+idx])
		i += idx + len("<!--")
		inComment = true
	}
	return b.String(), inComment
}

// MarkdownSectionsByHeading walks content's ATX/setext headings - fence and
// HTML-comment aware, via classifyDecisionRecordLines - and returns the body
// text under every heading keyFn recognizes. It is the generic engine every
// heading-based artifact parser this package still has is built from -
// ParseIntegrationRejection and ParseImplementationRejection (Phase 6's
// rejection handling) and app.ForkHandover's own fork.md parser - each
// telling a real heading apart from one hidden inside an example fence or an
// HTML comment using the exact same, already-hardened distinction, not a
// second, naively regexp-only parser that a hidden-then-later-revealed
// heading could defeat. The decision_record gate itself no longer uses this
// engine (see ParseDecisionRecordDocument): that artifact is agent-authored
// JSON now, not agent-authored markdown, so there is no heading for an
// adversarial or merely careless agent to hide.
//
// keyFn maps one heading's own text to the canonical key a caller cares
// about; ok=false means this heading is not recognized - it still closes the
// previous section when it sits at the same depth or shallower (an unrelated
// "## Context" preamble does not get folded into whatever section precedes
// it), but contributes no body of its own.
//
// Depth is what decides that. A section runs until the next heading at its
// own level or above; a DEEPER heading is part of its body, because that is
// what nesting means and what an author writing "## Options Considered"
// followed by "### 1." plainly intends. Treating any heading as a terminator
// made exactly that document read as an empty section - a decision record
// with four well-written sections was rejected for a missing one, and the
// twenty minutes of work it documented was discarded with it.
//
// dupKey names the first recognized key two or more real (non-fenced,
// non-commented) headings both produced, or "" when every recognized key
// was seen at most once. Which of two same-keyed headings should win is not
// decidable content: silently keeping the last one lets whichever heading an
// author (or an adversarial agent) places last override one placed first in
// plain view. A caller that wants last-one-wins discards dupKey; a caller
// that must instead refuse an ambiguous document checks it itself.
//
// This is deliberately not a full CommonMark implementation: it recognizes
// exactly the block constructs a hand-written or agent-written document
// realistically contains or could use to hide content in (fences, comments,
// ATX/setext headings, horizontal rules), not the entire spec.
func MarkdownSectionsByHeading(content string, keyFn func(headingText string) (key string, ok bool)) (sections map[string]string, dupKey string) {
	content = strings.TrimPrefix(content, "\uFEFF") // UTF-8 BOM, if present
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")

	lines := strings.Split(content, "\n")
	parsed := classifyDecisionRecordLines(lines)

	type headingHit struct {
		key          string
		headingStart int // first line belonging to the heading marker itself
		bodyStart    int // first line after the heading marker
		// level is the heading's depth (1 for "#" or a "===" underline, 2
		// for "##" or "---", and so on). A section ends at the next heading
		// of the SAME OR SHALLOWER depth; a deeper one is its own content.
		level int
	}
	var hits []headingHit

	for i := 0; i < len(parsed); i++ {
		if parsed[i].inFence {
			continue
		}
		trimmed := strings.TrimSpace(parsed[i].visible)

		if trimmed != "" && (setextEqualsRe.MatchString(trimmed) || setextDashesRe.MatchString(trimmed)) {
			if i > 0 && !parsed[i-1].inFence {
				prevTrimmed := strings.TrimSpace(parsed[i-1].visible)
				if prevTrimmed != "" && !isAtxHeadingText(prevTrimmed) && !isThematicBreak(prevTrimmed) &&
					!setextEqualsRe.MatchString(prevTrimmed) && !setextDashesRe.MatchString(prevTrimmed) {
					key, _ := keyFn(prevTrimmed)
					level := 2
					if setextEqualsRe.MatchString(trimmed) {
						level = 1
					}
					hits = append(hits, headingHit{
						key:          key,
						headingStart: i - 1,
						bodyStart:    i + 1,
						level:        level,
					})
					continue
				}
			}
			// Not adjacent to eligible paragraph text: a run of 3+ is a
			// thematic break, stripped from body content below; anything
			// shorter is left as plain (non-heading) content - CommonMark's
			// own handling of that edge case is genuinely ambiguous and it
			// does not arise in a real decision record.
			continue
		}

		if m := decisionRecordHeadingRe.FindStringSubmatch(parsed[i].visible); m != nil {
			key, _ := keyFn(m[2])
			hits = append(hits, headingHit{
				key:          key,
				headingStart: i,
				bodyStart:    i + 1,
				level:        len(m[1]),
			})
		}
	}

	bodies := make(map[string]string)
	seen := make(map[string]bool, len(hits))
	for i, h := range hits {
		if h.key == "" {
			continue
		}
		if seen[h.key] && dupKey == "" {
			dupKey = h.key
		}
		seen[h.key] = true
		// Scan forward for the next heading at this depth or shallower.
		// Stopping at the immediately following heading regardless of depth
		// is what made a section whose content is organised into
		// subheadings read as empty.
		bodyEnd := len(parsed)
		for j := i + 1; j < len(hits); j++ {
			if hits[j].level <= h.level {
				bodyEnd = hits[j].headingStart
				break
			}
		}
		var visibleLines []string
		for j := h.bodyStart; j < bodyEnd; j++ {
			if !parsed[j].inFence && isThematicBreak(strings.TrimSpace(parsed[j].visible)) {
				continue
			}
			visibleLines = append(visibleLines, parsed[j].visible)
		}
		body := strings.TrimSpace(strings.Join(visibleLines, "\n"))
		if body != "" {
			bodies[h.key] = body
		}
	}
	return bodies, dupKey
}

// duplicateJSONKeyError names where, inside a JSON document, the same object
// key appeared twice: path is the enclosing object's own location ("$" for
// the document root, "$.decisions[2]" for the third array entry), and key is
// the repeated name. See detectDuplicateJSONKeys.
type duplicateJSONKeyError struct {
	path string
	key  string
}

func (e *duplicateJSONKeyError) Error() string {
	return fmt.Sprintf("object key %q appears twice at %s", e.key, e.path)
}

// detectDuplicateJSONKeys walks content's token stream looking for two
// occurrences of the same key within the SAME JSON object, at any depth.
//
// encoding/json.Unmarshal does not report this at all: given
// {"decision":"Use SQLite","decision":"Use Postgres"}, it silently keeps
// only the LAST value and discards the first - exactly the "later data
// silently overrides earlier data, and the gate that was supposed to catch
// it says nothing" defect the withdrawn prefix-matching approach would have
// reintroduced at the markdown layer (see ParseDecisionRecordDocument's own
// doc comment and AGENTS.md), reproduced here one layer down, at the JSON
// layer itself. Detecting it requires walking the raw token stream by hand -
// the standard library exposes no option for this - via json.Decoder.Token,
// which yields one flat sequence of tokens (Delim '{'/'}'/'['/']', or a
// scalar) regardless of nesting depth. This function recurses one level
// per '{' or '[' it opens, tracking only the CURRENT object's own key set
// (a duplicate one level down must never be confused with a duplicate at
// this level - {"a":{"b":1},"c":{"b":2}} is not a duplicate "b").
//
// dec must be freshly positioned at the start of a JSON value (a document's
// root, when called from ParseDecisionRecordDocument). path is that value's
// own location, used only to build a duplicate's error message.
func detectDuplicateJSONKeys(dec *json.Decoder, path string) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	delim, isDelim := tok.(json.Delim)
	if !isDelim {
		return nil // a scalar value (string, number, bool, null) has no keys of its own
	}

	switch delim {
	case '{':
		seen := make(map[string]bool)
		for dec.More() {
			keyTok, err := dec.Token()
			if err != nil {
				return err
			}
			key, _ := keyTok.(string) // a JSON object key token is always a string
			if seen[key] {
				return &duplicateJSONKeyError{path: path, key: key}
			}
			seen[key] = true
			if err := detectDuplicateJSONKeys(dec, path+"."+key); err != nil {
				return err
			}
		}
		_, err := dec.Token() // consume the closing '}'
		return err
	case '[':
		for i := 0; dec.More(); i++ {
			if err := detectDuplicateJSONKeys(dec, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
		_, err := dec.Token() // consume the closing ']'
		return err
	}
	return nil
}

// ParseDecisionRecordDocument parses and validates a decision_record
// artifact's JSON content. It is exported so a caller that already knows the
// decision_record exit gate passed (see evaluateSingleExitGate's
// "decision_record" case) can read the same document into structured data
// without a second, potentially divergent implementation of "what counts as
// valid" - the discipline DecisionRecordSectionBodies enforced for the
// markdown format this schema replaced, kept for the same reason: the gate
// and the later read must never disagree about what they each saw.
//
// The document must be a JSON object shaped like:
//
//	{"decisions": [{"title": "...", "decision": "...", "optionsConsidered": "...", "tradeOffs": "...", "rationale": "..."}]}
//
// "decisions" must contain at least one entry and at most
// maxDecisionRecordEntries, every entry must carry non-blank
// decision/optionsConsidered/tradeOffs/rationale ("title" is optional), and
// no field may exceed maxDecisionRecordFieldBytes. The document is also
// rejected outright if it names a field this schema does not define (a typo
// like "titel" instead of "title" would otherwise be silently ignored by
// json.Unmarshal, and the value the agent meant to set never reaches the
// graph) or if any single JSON object repeats a key (which json.Unmarshal
// resolves by silently keeping only the LAST value - see
// detectDuplicateJSONKeys). A malformed document is rejected with the exact
// problem named - which check failed, and at which array index or object
// path - never a generic "invalid": an agent told that cannot act on it, one
// told "decisions[2].tradeOffs" can.
// maxJSONErrorLineBytes bounds the offending line this quotes back. A
// decision record is prose in JSON strings, so one line can be a whole
// paragraph; the point is to show the agent where to look, not to reproduce
// the file it already has.
const maxJSONErrorLineBytes = 240

// jsonErrorLocation renders where a JSON parse failure is, as
// " on line L: <the line>", or "" when the error carries no position.
//
// It exists because the surrounding parser's own contract - name the exact
// problem, never a generic "invalid", because an agent told "invalid" cannot
// act on it - was met by every check EXCEPT the syntax one. A malformed
// escape came back as `invalid character 'd' in string escape code` with a
// 7 KB file to find it in, and the agent that has to fix it on the next
// attempt cannot. Observed for real: an implementer wrote a regex
// (`\d{4}-\d{2}-\d{2}`) straight into a JSON string, where `\d` is not a
// valid escape.
//
// encoding/json HAS the position - both *json.SyntaxError and
// *json.UnmarshalTypeError carry Offset - it was simply never rendered.
//
// The LINE is reported and the column deliberately is not. The offset of a
// syntax error raised while streaming tokens (which is how the duplicate-key
// pass reads the document) is where the decoder stopped, not where the bad
// character is: on the real malformed record above it landed inside the
// preceding key, 41 columns before the offending escape. A column number
// that points at the wrong character is worse than no column, because it is
// believed. The line is right either way, so the whole line is quoted and
// the agent reads it.
func jsonErrorLocation(content string, err error) string {
	var offset int64 = -1
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	switch {
	case errors.As(err, &syntaxErr):
		offset = syntaxErr.Offset
	case errors.As(err, &typeErr):
		offset = typeErr.Offset
	}
	// encoding/json reports the offset just PAST the byte it stopped on, so
	// a failure on the first byte arrives as 1. Anything outside the
	// document is not a position this can describe honestly.
	if offset <= 0 || offset > int64(len(content)) {
		return ""
	}

	upto := content[:offset]
	line := strings.Count(upto, "\n") + 1
	lineStart := strings.LastIndex(upto, "\n") + 1

	lineEnd := strings.IndexByte(content[lineStart:], '\n')
	if lineEnd < 0 {
		lineEnd = len(content)
	} else {
		lineEnd += lineStart
	}

	text := strings.TrimSpace(content[lineStart:lineEnd])
	if len(text) > maxJSONErrorLineBytes {
		cut := maxJSONErrorLineBytes
		// Back off to a rune boundary so a truncated multi-byte character
		// never reaches the agent as mojibake it then has to hunt through.
		for cut > 0 && !utf8.RuneStart(text[cut]) {
			cut--
		}
		text = text[:cut] + "..."
	}
	if text == "" {
		return fmt.Sprintf(" on line %d", line)
	}
	return fmt.Sprintf(" on line %d: %s", line, text)
}

func ParseDecisionRecordDocument(content string) ([]DecisionRecordEntry, error) {
	if len(content) > maxDecisionRecordDocumentBytes {
		return nil, fmt.Errorf("decision_record_too_large: the record is %d bytes, over the %d byte limit - Fix: a decision_record artifact holds a short, deliberated list of the decisions actually made in this stage, not pasted file or log contents; trim each field to its own decision/options/trade-offs/rationale text", len(content), maxDecisionRecordDocumentBytes)
	}

	if err := detectDuplicateJSONKeys(json.NewDecoder(strings.NewReader(content)), "$"); err != nil {
		var dupErr *duplicateJSONKeyError
		if errors.As(err, &dupErr) {
			return nil, fmt.Errorf(`decision_record_duplicate_key: %w - encoding/json keeps only the LAST value for a repeated object key and silently discards the rest, which can lose an entire decision without saying so - Fix: remove the duplicate key at %s, keeping only the one value that should survive`, dupErr, dupErr.path)
		}
		return nil, fmt.Errorf(`decision_record_invalid_json: %w%s - the file must be a JSON object shaped like {"decisions":[{"decision":"...","optionsConsidered":"...","tradeOffs":"...","rationale":"..."}]}`, err, jsonErrorLocation(content, err))
	}

	dec := json.NewDecoder(strings.NewReader(content))
	dec.DisallowUnknownFields()
	var doc decisionRecordDocument
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf(`decision_record_invalid_json: %w%s - the file must be a JSON object shaped like {"decisions":[{"decision":"...","optionsConsidered":"...","tradeOffs":"...","rationale":"..."}]} with no field beyond title/decision/optionsConsidered/tradeOffs/rationale`, err, jsonErrorLocation(content, err))
	}
	if dec.More() {
		return nil, fmt.Errorf(`decision_record_invalid_json: unexpected content after the top-level JSON object - the file must contain exactly one JSON object shaped like {"decisions":[...]}`)
	}

	if len(doc.Decisions) == 0 {
		return nil, fmt.Errorf(`decision_record_empty: the "decisions" array must contain at least one entry`)
	}
	if len(doc.Decisions) > maxDecisionRecordEntries {
		return nil, fmt.Errorf(`decision_record_too_many_entries: the "decisions" array has %d entries, over the %d entry limit - Fix: a decision_record artifact is the deliberated list of decisions actually made in this stage, not every option ever discussed; if this stage genuinely made this many separate decisions, split the work across separate stages instead of one oversized record`, len(doc.Decisions), maxDecisionRecordEntries)
	}

	var problems []string
	for i, entry := range doc.Decisions {
		for _, field := range decisionRecordRequiredFields {
			if strings.TrimSpace(field.value(entry)) == "" {
				problems = append(problems, fmt.Sprintf("decisions[%d].%s", i, field.wireName))
			}
		}
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("decision_record_missing_fields: %s", strings.Join(problems, ", "))
	}

	var oversized []string
	fieldLength := append([]decisionRecordRequiredField{
		{"title", func(e DecisionRecordEntry) string { return e.Title }},
	}, decisionRecordRequiredFields...)
	for i, entry := range doc.Decisions {
		for _, field := range fieldLength {
			if n := len(field.value(entry)); n > maxDecisionRecordFieldBytes {
				oversized = append(oversized, fmt.Sprintf("decisions[%d].%s is %d bytes, over the %d byte limit", i, field.wireName, n, maxDecisionRecordFieldBytes))
			}
		}
	}
	if len(oversized) > 0 {
		return nil, fmt.Errorf("decision_record_field_too_large: %s - Fix: a decision_record field holds this decision's own reasoning, not pasted file or log contents", strings.Join(oversized, ", "))
	}

	return doc.Decisions, nil
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

// decisionRecordArtifactDirPlaceholder is the only anchor a decision_record
// path may use. Unlike OutputArtifact/artifact_verdict/artifact_exists,
// which may legitimately resolve relative to the worktree - a documented
// pre-existing convention (see legacyInWorktreeArtifactPrefix) - a
// decision_record path has no such fallback: the whole reason this artifact
// lives outside the worktree is to keep it out of a stage's own
// `git add <files>` in the target repository, so a path that resolves
// inside the worktree (or escapes <artifact_dir> via "..") reproduces
// exactly the leak <artifact_dir> exists to prevent (archeion PR #40).
const decisionRecordArtifactDirPlaceholder = "<artifact_dir>"

// validateDecisionRecordPathContained rejects a decision_record path that is
// not anchored at <artifact_dir>, or that escapes it via ".." once
// substituted. It resolves the placeholder against a fixed sentinel
// directory instead of a real one, because at workflow-load time there is no
// real ArtifactDir yet - only the shape of the path is being checked.
func validateDecisionRecordPathContained(raw string) error {
	prefix := decisionRecordArtifactDirPlaceholder + "/"
	if !strings.HasPrefix(raw, prefix) {
		return fmt.Errorf("must be anchored at %s/..., not resolve against the worktree", decisionRecordArtifactDirPlaceholder)
	}
	const sentinel = "/__kernl_artifact_dir__"
	resolved := filepath.Clean(sentinel + strings.TrimPrefix(raw, decisionRecordArtifactDirPlaceholder))
	if !strings.HasPrefix(resolved, sentinel+string(filepath.Separator)) {
		return fmt.Errorf("must stay beneath %s after resolution, not escape it via \"..\"", decisionRecordArtifactDirPlaceholder)
	}
	return nil
}

// ValidateArtifactPaths rejects any stage OutputArtifact/Inputs entry, or
// filesystem-based exit gate Path, that still names the legacy in-worktree
// ".kernl/" location instead of the <artifact_dir> placeholder. Called from
// workflow resolution (LoadWorkflowYAML) so a workflow definition written
// before the artifact directory moved outside the worktree fails loud,
// naming the offending stage, instead of quietly reproducing the defect
// that move fixed.
//
// It additionally enforces two decision_record-specific invariants that the
// other gate types do not need: the path must stay confined beneath
// <artifact_dir> (validateDecisionRecordPathContained), and when both a
// stage's decision_record.path and an exit gate's decision_record path name
// the same state, they must agree - otherwise an implementer who writes
// exactly what the prompt told it to write still fails the gate, because the
// gate was checking a different file the whole time.
func ValidateArtifactPaths(stages map[string]StageContract, exitGates map[string][]WorkflowExitGate) error {
	for name, stage := range stages {
		if strings.Contains(stage.OutputArtifact.Path, legacyInWorktreeArtifactPrefix) {
			return fmt.Errorf("KERNL DISPATCH FAILURE: stage %q output_artifact.path %q uses the legacy in-worktree .kernl/ location - Fix: use <artifact_dir>/... instead, so the artifact is written outside the worktree", name, stage.OutputArtifact.Path)
		}
		if stage.DecisionRecord.Path != "" {
			if strings.Contains(stage.DecisionRecord.Path, legacyInWorktreeArtifactPrefix) {
				return fmt.Errorf("KERNL DISPATCH FAILURE: stage %q decision_record.path %q uses the legacy in-worktree .kernl/ location - Fix: use <artifact_dir>/... instead, so the record is written outside the worktree", name, stage.DecisionRecord.Path)
			}
			if err := validateDecisionRecordPathContained(stage.DecisionRecord.Path); err != nil {
				return fmt.Errorf("KERNL DISPATCH FAILURE: stage %q decision_record.path %q %s", name, stage.DecisionRecord.Path, err)
			}
		}
		for _, inp := range stage.Inputs {
			if strings.Contains(inp, legacyInWorktreeArtifactPrefix) {
				return fmt.Errorf("KERNL DISPATCH FAILURE: stage %q input %q uses the legacy in-worktree .kernl/ location - Fix: use <artifact_dir>/... instead, so the artifact is read from outside the worktree", name, inp)
			}
		}
	}
	for state, gates := range exitGates {
		for _, gate := range gates {
			if gate.Type == "decision_record" {
				if err := validateDecisionRecordPathContained(gate.Path); err != nil {
					return fmt.Errorf("KERNL DISPATCH FAILURE: exit gate %q path %q %s", state, gate.Path, err)
				}
				if stage, ok := stages[state]; ok && stage.DecisionRecord.Path != "" && stage.DecisionRecord.Path != gate.Path {
					return fmt.Errorf("KERNL DISPATCH FAILURE: exit gate %q decision_record path %q disagrees with stage %q decision_record.path %q - Fix: make the two strings identical, so the agent is told to write exactly the file the gate reads", state, gate.Path, state, stage.DecisionRecord.Path)
				}
				continue
			}
			if gate.Type == "commit_marker" {
				// A commit_marker gate no longer reads a marker string from
				// commit messages - it only checks that the stage left a new
				// commit, an ancestry-and-non-empty-range question with
				// nothing left for a path to configure. A workflow that
				// still sets one was written for the retired behavior and
				// would silently get a gate that quietly stopped checking
				// what its author asked for; refusing to load it is louder
				// and cheaper than that surprise.
				if gate.Path != "" {
					return fmt.Errorf("KERNL DISPATCH FAILURE: exit gate %q type commit_marker sets path %q, but commit_marker no longer reads a marker string from commit messages - it only checks that the stage left a new commit - Fix: remove the path field", state, gate.Path)
				}
				continue
			}
			if gate.Type != "artifact_exists" && gate.Type != "artifact_verdict" {
				// description_contains Path values are description
				// substrings, not filesystem paths - a ".kernl/" substring
				// there means nothing.
				continue
			}
			if strings.Contains(gate.Path, legacyInWorktreeArtifactPrefix) {
				return fmt.Errorf("KERNL DISPATCH FAILURE: exit gate %q path %q uses the legacy in-worktree .kernl/ location - Fix: use <artifact_dir>/... instead, so the gate checks outside the worktree", state, gate.Path)
			}
		}
	}
	return nil
}
