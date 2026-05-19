package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	State              string
	NextActionState    string
	NextActionOwnerKind ActionOwnerKind
	RequiresHumanAction bool
	IsAgentClaimable   bool
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
	"planning":                ActionOwnerAgent,
	"plan_review":             ActionOwnerAgent,
	"implementation":          ActionOwnerAgent,
	"implementation_review":   ActionOwnerAgent,
	"integration":             ActionOwnerAgent,
	"integration_review":      ActionOwnerAgent,
	"shipment":                ActionOwnerAgent,
	"shipment_review":         ActionOwnerAgent,
}

var semiautoOwners = map[string]ActionOwnerKind{
	"planning":                ActionOwnerAgent,
	"plan_review":             ActionOwnerHuman,
	"implementation":          ActionOwnerAgent,
	"implementation_review":   ActionOwnerHuman,
	"integration":             ActionOwnerAgent,
	"integration_review":      ActionOwnerAgent,
	"shipment":                ActionOwnerAgent,
	"shipment_review":         ActionOwnerAgent,
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
}

var builtinProfiles = []profileConfig{
	{
		ID:                       "epic",
		DisplayName:              "Epic",
		Description:              "Epic lifecycle: integration, integration review, shipment, shipment review",
		PlanningMode:             "skipped",
		ImplementationReviewMode: "skipped",
		Output:                   "pr",
		Owners: map[string]ActionOwnerKind{
			"integration":        ActionOwnerAgent,
			"integration_review": ActionOwnerAgent,
			"shipment":           ActionOwnerAgent,
			"shipment_review":    ActionOwnerAgent,
		},
	},
	{
		ID:                      "autopilot",
		DisplayName:             "Autopilot",
		Description:             "Agent-owned full flow with remote main output",
		PlanningMode:            "required",
		ImplementationReviewMode: "required",
		Output:                  "remote_main",
		Owners:                  agentOwners,
	},
	{
		ID:                      "autopilot_with_pr",
		DisplayName:             "Autopilot (PR)",
		Description:             "Agent-owned full flow with PR output",
		PlanningMode:            "required",
		ImplementationReviewMode: "required",
		Output:                  "pr",
		Owners:                  agentOwners,
	},
	{
		ID:                      "semiauto",
		DisplayName:             "Semiauto",
		Description:             "Human-gated plan and implementation reviews",
		PlanningMode:            "required",
		ImplementationReviewMode: "required",
		Output:                  "remote_main",
		Owners:                  semiautoOwners,
	},
	{
		ID:                      "autopilot_no_planning",
		DisplayName:             "Autopilot (no planning)",
		Description:             "Agent-owned flow starting at implementation",
		PlanningMode:            "skipped",
		ImplementationReviewMode: "required",
		Output:                  "remote_main",
		Owners:                  agentOwners,
	},
	{
		ID:                      "autopilot_with_pr_no_planning",
		DisplayName:             "Autopilot (PR, no planning)",
		Description:             "Agent-owned flow with PR output and no planning",
		PlanningMode:            "skipped",
		ImplementationReviewMode: "required",
		Output:                  "pr",
		Owners:                  agentOwners,
	},
	{
		ID:                      "semiauto_no_planning",
		DisplayName:             "Semiauto (no planning)",
		Description:             "Human-gated implementation review with skipped planning",
		PlanningMode:            "skipped",
		ImplementationReviewMode: "required",
		Output:                  "remote_main",
		Owners:                  semiautoOwners,
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
		{From: "implementation_review", To: "ready_for_integration"},
		{From: "implementation_review", To: "ready_for_implementation"},
		{From: "ready_for_integration", To: "integration"},
		{From: "integration", To: "ready_for_integration_review"},
		{From: "ready_for_integration_review", To: "integration_review"},
		{From: "integration_review", To: "ready_for_shipment"},
		{From: "integration_review", To: "ready_for_integration"},
		{From: "ready_for_shipment", To: "shipment"},
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
		ID:                  cfg.ID,
		BackingWorkflowID:  cfg.ID,
		Label:               cfg.DisplayName,
		Mode:                mode,
		InitialState:        initialState,
		States:              states,
		TerminalStates:      terminalStates,
		Transitions:         transitions,
		FinalCutState:       finalCutState,
		RetakeState:         retakeState,
		PromptProfileID:     cfg.ID,
		ProfileID:           cfg.ID,
		QueueActions:        queueActions,
		QueueStates:         queueStates,
		ActionStates:        actionStates,
		ReviewQueueStates:   reviewQueueStates,
		HumanQueueStates:    humanQueueStates,
		Owners:              cfg.Owners,
		StateOwners:         stateOwners,
	}

	if cfg.Stages != nil {
		desc.Stages = cfg.Stages
	} else {
		desc.Stages = CanonicalStageContracts()
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

// ResolveWorkflow returns the WorkflowDescriptor for a bead, defaulting to
// the "autopilot" built-in profile when the bead has no explicit profile or
// workflow ID.
func ResolveWorkflow(bead *Bead) WorkflowDescriptor {
	profileID := bead.ProfileID
	if profileID == "" {
		profileID = bead.WorkflowID
	}
	return BuiltinProfileDescriptor(profileID)
}

func ForwardTransitionTarget(currentState string, wf WorkflowDescriptor) (string, bool) {
	if len(wf.Transitions) == 0 {
		return "", false
	}

	statePipelineOrder := map[string]int{
		"ready_for_planning":            0,
		"planning":                       1,
		"ready_for_plan_review":          2,
		"plan_review":                    3,
		"ready_for_implementation":       4,
		"implementation":                 5,
		"ready_for_review":               6,
		"ready_for_implementation_review": 6,
		"review":                         7,
		"implementation_review":          7,
		"ready_for_integration":         8,
		"integration":                    9,
		"ready_for_integration_review":  10,
		"integration_review":            11,
		"ready_for_shipment":            12,
		"shipment":                       13,
		"ready_for_shipment_review":      14,
		"shipment_review":                15,
		"shipped":                        16,
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

func EvaluateExitGate(wf WorkflowDescriptor, fromState, worktreePath, beadID string) (passed bool, reason string) {
	gate, ok := wf.ExitGates[fromState]
	if !ok || gate.Type == "" || gate.Type == "agent_exit_zero" {
		return true, ""
	}
	switch gate.Type {
	case "artifact_exists":
		resolved := strings.ReplaceAll(gate.Path, "<bead_id>", beadID)
		abs := filepath.Join(worktreePath, resolved)
		if _, err := os.Stat(abs); os.IsNotExist(err) {
			return false, "artifact_missing: " + resolved
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
			State:                workflowState,
			NextActionOwnerKind:  ActionOwnerNone,
			RequiresHumanAction:  false,
			IsAgentClaimable:     false,
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
		State:                workflowState,
		NextActionState:      resolved.Step,
		NextActionOwnerKind:  ownerKind,
		RequiresHumanAction:  ownerKind == ActionOwnerHuman && resolved.Phase == StepPhaseQueued,
		IsAgentClaimable:     resolved.Phase == StepPhaseQueued && ownerKind == ActionOwnerAgent,
	}
}

func isTerminalState(state string, wf WorkflowDescriptor) bool {
	for _, ts := range wf.TerminalStates {
		if ts == state {
			return true
		}
	}
	if state == "deferred" {
		return true
	}
	return false
}

type BeadTransitionResult struct {
	Bead      *Bead
	NextState string
}

func NextBead(backend BackendPort, beadID string, expectedState string, repoPath string) (*BeadTransitionResult, error) {
	bead, err := backend.Get(beadID, repoPath)
	if err != nil {
		return nil, fmt.Errorf("Failed to load bead %s: %v", beadID, err)
	}
	if bead == nil {
		return nil, fmt.Errorf("Bead %s not found", beadID)
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
		return nil, fmt.Errorf("Failed to update bead %s: %v", beadID, updateErr)
	}

	return &BeadTransitionResult{Bead: bead, NextState: target}, nil
}

func ClaimBead(backend BackendPort, beadID string, repoPath string) (*BeadTransitionResult, error) {
	bead, err := backend.Get(beadID, repoPath)
	if err != nil {
		return nil, fmt.Errorf("Failed to load bead %s: %v", beadID, err)
	}
	if bead == nil {
		return nil, fmt.Errorf("Bead %s not found", beadID)
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
		return nil, fmt.Errorf("Failed to update bead %s: %v", beadID, updateErr)
	}

	return &BeadTransitionResult{Bead: bead, NextState: target}, nil
}