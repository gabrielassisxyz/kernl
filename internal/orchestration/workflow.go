package orchestration

import (
	"fmt"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/workflow"
)

// legacyToWorkflowState maps legacy bead state names to workflow.IssueStatus
// string values for backward compatibility during mechanical refactoring.
// deferred and abandoned map to StatusClosed so wildcard transitions produce
// a terminal state in the new model.
var legacyToWorkflowState = map[string]string{
	"ready_for_implementation":       string(workflow.StatusOpen),
	"implementation":                 string(workflow.StatusInProgress),
	"implementation_review":          string(workflow.StatusAwaitingIntegration),
	"ready_for_integration":          string(workflow.StatusAwaitingIntegration),
	"integration":                    string(workflow.StatusAwaitingIntegration),
	"integration_review":             string(workflow.StatusAwaitingIntegration),
	"ready_for_shipment":             string(workflow.StatusAwaitingIntegration),
	"shipment":                       string(workflow.StatusClosed),
	"shipment_review":                string(workflow.StatusClosed),
	"shipped":                        string(workflow.StatusClosed),
	"closed":                         string(workflow.StatusClosed),
	"done":                           string(workflow.StatusClosed),
	"approved":                       string(workflow.StatusClosed),
	"deferred":                       string(workflow.StatusClosed),
	"abandoned":                      string(workflow.StatusClosed),
}

type Step struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Kind  string `json:"kind"`
	State string `json:"state"`
}

type StepPhase string

const (
	PhaseQueued   StepPhase = "queued"
	PhaseActive   StepPhase = "active"
	PhaseTerminal StepPhase = "terminal"
)

type ResolvedStep struct {
	Step  string
	Phase StepPhase
}

type WorkflowRuntimeState struct {
	State                string
	NextActionState      string
	NextActionOwnerKind  backend.ActionOwnerKind
	RequiresHumanAction  bool
	IsAgentClaimable      bool
}

func ResolveStep(steps []Step, stepID string) (*Step, error) {
	for i := range steps {
		if steps[i].ID == stepID {
			return &steps[i], nil
		}
	}
	return nil, fmt.Errorf("KERNL DISPATCH FAILURE: step %s not found in workflow", stepID)
}

var statePipelineOrder = map[string]int{
	"ready_for_planning":              0,
	"planning":                        1,
	"ready_for_plan_review":            2,
	"plan_review":                     3,
	"ready_for_implementation":         4,
	"implementation":                  5,
	"ready_for_implementation_review": 6,
	"implementation_review":           7,
	"ready_for_integration":           8,
	"integration":                     9,
	"ready_for_integration_review":    10,
	"integration_review":            11,
	"ready_for_shipment":              12,
	"shipment":                        13,
	"ready_for_shipment_review":       14,
	"shipment_review":                 15,
	"shipped":                         16,
	string(workflow.StatusOpen):                4,
	string(workflow.StatusInProgress):          5,
	string(workflow.StatusAwaitingIntegration): 8,
	string(workflow.StatusClosed):              16,
}

var legacyRetakeStates = map[string]bool{
	"retake": true, "retry": true, "rejected": true, "refining": true, "rework": true,
}

var legacyInProgressStates = map[string]bool{
	"in_progress": true, "implementing": true, "implemented": true, "reviewing": true,
}

const DefaultProfileID = "autopilot"
const LegacyBeadsCoarseWorkflowID = "beads-coarse"

func NormalizeProfileID(profileID string) string {
	v := strings.TrimSpace(strings.ToLower(profileID))
	if v == "" {
		return DefaultProfileID
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

func IsWorkflowStateLabel(label string) bool {
	return strings.HasPrefix(label, "wf:state:")
}

func IsWorkflowProfileLabel(label string) bool {
	return strings.HasPrefix(label, "wf:profile:")
}

func ExtractWorkflowStateLabel(labels []string) string {
	for _, l := range labels {
		if !IsWorkflowStateLabel(l) {
			continue
		}
		raw := strings.TrimPrefix(l, "wf:state:")
		state := strings.TrimSpace(strings.ToLower(raw))
		if state != "" {
			return state
		}
	}
	return ""
}

func ExtractWorkflowProfileLabel(labels []string) string {
	for _, l := range labels {
		if !IsWorkflowProfileLabel(l) {
			continue
		}
		raw := strings.TrimPrefix(l, "wf:profile:")
		normalized := NormalizeProfileID(raw)
		if normalized != "" {
			return normalized
		}
	}
	return ""
}

func WithWorkflowStateLabel(labels []string, workflowState string) []string {
	var next []string
	for _, l := range labels {
		if !IsWorkflowStateLabel(l) {
			next = append(next, l)
		}
	}
	trimmed := strings.TrimSpace(workflowState)
	normalized := "open"
	if trimmed != "" {
		normalized = strings.ToLower(trimmed)
	}
	next = append(next, "wf:state:"+normalized)
	return dedupStrings(next)
}

func WithWorkflowProfileLabel(labels []string, profileID string) []string {
	var next []string
	for _, l := range labels {
		if !IsWorkflowProfileLabel(l) {
			next = append(next, l)
		}
	}
	normalized := NormalizeProfileID(profileID)
	if normalized == "" {
		normalized = DefaultProfileID
	}
	next = append(next, "wf:profile:"+normalized)
	return dedupStrings(next)
}

func dedupStrings(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	var result []string
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

func DeriveProfileID(labels []string, metadata map[string]any) string {
	if metadata != nil {
		for _, key := range []string{"profileId", "kernlProfileId", "workflowProfileId", "knotsProfileId"} {
			if v, ok := metadata[key]; ok {
				if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
					normalized := NormalizeProfileID(s)
					if normalized != "" {
						return normalized
					}
				}
			}
		}
	}
	explicit := ExtractWorkflowProfileLabel(labels)
	if explicit != "" {
		return explicit
	}
	return DefaultProfileID
}

func DeriveWorkflowState(labels []string, wf *backend.WorkflowDescriptor) string {
	explicit := ExtractWorkflowStateLabel(labels)
	if explicit != "" {
		return NormalizeStateForWorkflow(explicit, wf)
	}
	if wf != nil {
		return wf.InitialState
	}
	desc := backend.BuiltinProfileDescriptor(DefaultProfileID)
	return desc.InitialState
}

func firstActionState(wf *backend.WorkflowDescriptor) string {
	if len(wf.ActionStates) > 0 {
		return wf.ActionStates[0]
	}
	if wf.States != nil {
		for _, s := range wf.States {
			if s == string(workflow.StatusInProgress) || legacyToWorkflowState[s] == string(workflow.StatusInProgress) {
				return string(workflow.StatusInProgress)
			}
		}
	}
	return string(workflow.StatusInProgress)
}

func terminalStateForClosed(wf *backend.WorkflowDescriptor) string {
	for _, s := range wf.States {
		if s == string(workflow.StatusClosed) || legacyToWorkflowState[s] == string(workflow.StatusClosed) {
			return string(workflow.StatusClosed)
		}
	}
	for _, ts := range wf.TerminalStates {
		if ts == string(workflow.StatusClosed) {
			return string(workflow.StatusClosed)
		}
	}
	if len(wf.TerminalStates) > 0 {
		return wf.TerminalStates[0]
	}
	return string(workflow.StatusClosed)
}

func NormalizeStateForWorkflow(workflowState string, wf *backend.WorkflowDescriptor) string {
	normalized := strings.TrimSpace(strings.ToLower(workflowState))
	if normalized == "" {
		return wf.InitialState
	}

	if mapped, ok := legacyToWorkflowState[normalized]; ok {
		return mapped
	}

	stateSet := make(map[string]bool, len(wf.States)*2)
	for _, s := range wf.States {
		stateSet[s] = true
		if mapped, ok := legacyToWorkflowState[s]; ok {
			stateSet[mapped] = true
		}
	}
	if stateSet[normalized] {
		return normalized
	}

	if normalized == "impl" {
		if stateSet[string(workflow.StatusInProgress)] {
			return string(workflow.StatusInProgress)
		}
		return firstActionState(wf)
	}

	if normalized == string(workflow.StatusClosed) {
		return normalized
	}

	if normalized == "open" || normalized == "idea" || normalized == "work_item" {
		return wf.InitialState
	}

	if legacyInProgressStates[normalized] {
		return firstActionState(wf)
	}

	if normalized == "ready_for_review" || normalized == "reviewing" {
		if stateSet["ready_for_implementation_review"] {
			return "ready_for_implementation_review"
		}
		return firstActionState(wf)
	}

	if legacyRetakeStates[normalized] {
		retake := wf.RetakeState
		if retake != "" && stateSet[retake] {
			return retake
		}
		return wf.InitialState
	}

	return wf.InitialState
}

func WorkflowStatePhase(wf *backend.WorkflowDescriptor, state string) StepPhase {
	normalized := strings.TrimSpace(strings.ToLower(state))
	if normalized == "" {
		return PhaseQueued
	}

	for _, ts := range wf.TerminalStates {
		if ts == normalized || legacyToWorkflowState[ts] == normalized || legacyToWorkflowState[normalized] == ts {
			return PhaseTerminal
		}
	}

	for _, qs := range wf.QueueStates {
		if qs == normalized || legacyToWorkflowState[qs] == normalized || legacyToWorkflowState[normalized] == qs {
			return PhaseQueued
		}
	}
	if wf.QueueActions != nil {
		if _, ok := wf.QueueActions[normalized]; ok {
			return PhaseQueued
		}
		if mapped, ok := legacyToWorkflowState[normalized]; ok {
			if _, ok2 := wf.QueueActions[mapped]; ok2 {
				return PhaseQueued
			}
		}
	}

	for _, as := range wf.ActionStates {
		if as == normalized || legacyToWorkflowState[as] == normalized || legacyToWorkflowState[normalized] == as {
			return PhaseActive
		}
	}
	if wf.QueueActions != nil {
		for _, a := range wf.QueueActions {
			if a == normalized || legacyToWorkflowState[a] == normalized || legacyToWorkflowState[normalized] == a {
				return PhaseActive
			}
		}
	}

	return PhaseQueued
}

func ResolveStepForWorkflow(state string, wf *backend.WorkflowDescriptor) *ResolvedStep {
	actionStates := wf.ActionStates
	if actionStates == nil {
		actionStates = []string{}
	}
	for _, s := range actionStates {
		if s == state {
			return &ResolvedStep{Step: state, Phase: PhaseActive}
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
					return &ResolvedStep{Step: action, Phase: PhaseQueued}
				}
			}
			return nil
		}
	}
	return nil
}

func IsAgentClaimable(step *Step) bool {
	return step.Kind == "agent"
}

func RequiresHumanAction(step *Step) bool {
	return step.Kind == "human"
}

func IsAgentClaimableForStep(wf *backend.WorkflowDescriptor, state string) bool {
	rs := ResolveStepForWorkflow(state, wf)
	if rs == nil {
		return false
	}
	if rs.Phase != PhaseQueued {
		return false
	}
	ownerKind := OwnerKindForState(wf, rs.Step)
	return ownerKind == backend.ActionOwnerAgent
}

func RequiresHumanActionForStep(wf *backend.WorkflowDescriptor, state string) bool {
	rs := ResolveStepForWorkflow(state, wf)
	if rs == nil {
		return false
	}
	if rs.Phase == PhaseQueued {
		ownerKind := OwnerKindForState(wf, rs.Step)
		return ownerKind == backend.ActionOwnerHuman
	}
	return false
}

func OwnerKindForState(wf *backend.WorkflowDescriptor, step string) backend.ActionOwnerKind {
	if wf.StateOwners != nil {
		if k, ok := wf.StateOwners[step]; ok {
			return k
		}
	}
	if wf.Owners != nil {
		if k, ok := wf.Owners[step]; ok {
			return k
		}
	}
	return backend.ActionOwnerAgent
}

func IsQueueOrTerminalWorkflow(state string, wf *backend.WorkflowDescriptor) bool {
	for _, ts := range wf.TerminalStates {
		if ts == state {
			return true
		}
	}
	phase := WorkflowStatePhase(wf, state)
	return phase != PhaseActive
}

func IsQueueOrTerminal(state string) bool {
	switch state {
	case "ready_for_planning", "ready_for_review",
		"ready_for_plan_review", "ready_for_implementation_review",
		"ready_for_shipment_review",
		string(workflow.StatusOpen), "ready_for_implementation":
		return true
	case string(workflow.StatusAwaitingIntegration), "ready_for_shipment":
		return true
	case string(workflow.StatusClosed), "shipped", "done", "abandoned", "deferred":
		return true
	}
	return false
}

func IsReviewStep(step *Step) bool {
	return step.Kind == "agent_review" || step.Kind == "human_review"
}

func IsReviewStepForWorkflow(step string, wf *backend.WorkflowDescriptor) bool {
	reviewSet := make(map[string]bool, len(wf.ReviewQueueStates))
	for _, q := range wf.ReviewQueueStates {
		reviewSet[q] = true
	}
	queue := QueueStateForStep(step, wf)
	if queue == "" {
		return false
	}
	return reviewSet[queue]
}

func PriorActionStep(step string, wf *backend.WorkflowDescriptor) string {
	if !IsReviewStepForWorkflow(step, wf) {
		return ""
	}
	reviewQueue := QueueStateForStep(step, wf)
	if reviewQueue == "" {
		return ""
	}
	actionSet := make(map[string]bool, len(wf.ActionStates))
	for _, s := range wf.ActionStates {
		actionSet[s] = true
	}
	for _, t := range wf.Transitions {
		if t.To == reviewQueue && actionSet[t.From] {
			return t.From
		}
	}
	return ""
}

func QueueStateForStep(step string, wf *backend.WorkflowDescriptor) string {
	if wf.QueueActions == nil {
		return ""
	}
	for q, a := range wf.QueueActions {
		if a == step {
			return q
		}
	}
	return ""
}

func NextQueueStateForStep(step string, wf *backend.WorkflowDescriptor) string {
	order := OrderedActionStates(wf)
	idx := -1
	for i, s := range order {
		if s == step {
			idx = i
			break
		}
	}
	if idx < 0 || idx >= len(order)-1 {
		return ""
	}
	return QueueStateForStep(order[idx+1], wf)
}

func PriorQueueStateForStep(step string, wf *backend.WorkflowDescriptor) string {
	order := OrderedActionStates(wf)
	idx := -1
	for i, s := range order {
		if s == step {
			idx = i
			break
		}
	}
	if idx <= 0 {
		return ""
	}
	return QueueStateForStep(order[idx-1], wf)
}

func WorkflowActionStateForState(wf *backend.WorkflowDescriptor, state string) string {
	normalized := strings.TrimSpace(strings.ToLower(state))
	if normalized == "" {
		return ""
	}
	phase := WorkflowStatePhase(wf, normalized)
	if phase == PhaseActive {
		return normalized
	}
	if phase != PhaseQueued {
		return ""
	}
	if wf.QueueActions != nil {
		if action, ok := wf.QueueActions[normalized]; ok {
			return action
		}
	}
	return ""
}

func WorkflowQueueStateForState(wf *backend.WorkflowDescriptor, state string) string {
	normalized := strings.TrimSpace(strings.ToLower(state))
	if normalized == "" {
		return ""
	}
	phase := WorkflowStatePhase(wf, normalized)
	if phase == PhaseQueued {
		return normalized
	}
	if phase != PhaseActive {
		return ""
	}
	if wf.QueueActions == nil {
		return ""
	}
	for q, a := range wf.QueueActions {
		if a == normalized {
			return q
		}
	}
	return ""
}

func OrderedActionStates(wf *backend.WorkflowDescriptor) []string {
	actionSet := make(map[string]bool, len(wf.ActionStates))
	for _, s := range wf.ActionStates {
		actionSet[s] = true
	}
	if len(actionSet) == 0 {
		return []string{}
	}

	successor := make(map[string]string)
	for _, t := range wf.Transitions {
		if t.From == "*" {
			continue
		}
		if _, ok := successor[t.From]; !ok {
			successor[t.From] = t.To
		}
	}

	visited := make(map[string]bool)
	var ordered []string
	cursor := wf.InitialState
	for cursor != "" && !visited[cursor] {
		visited[cursor] = true
		if actionSet[cursor] {
			ordered = append(ordered, cursor)
		}
		cursor = successor[cursor]
	}

	for _, action := range wf.ActionStates {
		if !visited[action] {
			ordered = append(ordered, action)
		}
	}
	return ordered
}

func DeriveWorkflowRuntimeState(wf *backend.WorkflowDescriptor, workflowState string) WorkflowRuntimeState {
	normalizedState := NormalizeStateForWorkflow(workflowState, wf)
	phase := WorkflowStatePhase(wf, normalizedState)

	if phase == PhaseTerminal {
		return WorkflowRuntimeState{
			State:                normalizedState,
			NextActionOwnerKind:  backend.ActionOwnerNone,
			RequiresHumanAction:  false,
			IsAgentClaimable:      false,
		}
	}

	resolved := ResolveStepForWorkflow(normalizedState, wf)
	if resolved == nil {
		return WorkflowRuntimeState{
			State:                normalizedState,
			NextActionOwnerKind:  backend.ActionOwnerNone,
			RequiresHumanAction:  false,
			IsAgentClaimable:      false,
		}
	}

	ownerKind := backend.ActionOwnerNone
	if wf.StateOwners != nil {
		if k, ok := wf.StateOwners[normalizedState]; ok {
			ownerKind = k
		}
	}
	if ownerKind == backend.ActionOwnerNone {
		if wf.Owners != nil {
			if k, ok := wf.Owners[resolved.Step]; ok {
				ownerKind = k
			}
		}
	}
	if ownerKind == backend.ActionOwnerNone {
		ownerKind = backend.ActionOwnerAgent
	}

	if phase == PhaseQueued {
		return WorkflowRuntimeState{
			State:                normalizedState,
			NextActionState:      resolved.Step,
			NextActionOwnerKind:  ownerKind,
			RequiresHumanAction:  ownerKind == backend.ActionOwnerHuman,
			IsAgentClaimable:      ownerKind == backend.ActionOwnerAgent,
		}
	}

	return WorkflowRuntimeState{
		State:                normalizedState,
		NextActionOwnerKind:  ownerKind,
		RequiresHumanAction:  false,
		IsAgentClaimable:      false,
	}
}

func DeriveWorkflowStructure(states []string, transitions []backend.WorkflowTransition, owners map[string]backend.ActionOwnerKind, terminalStates []string) (queueStates, actionStates []string, queueActions map[string]string) {
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

func BuiltinWorkflowDescriptors() []backend.WorkflowDescriptor {
	profileIDs := []string{
		"autopilot", "autopilot_with_pr", "semiauto",
		"autopilot_no_planning", "autopilot_with_pr_no_planning", "semiauto_no_planning",
	}
	descriptors := make([]backend.WorkflowDescriptor, 0, len(profileIDs))
	for _, id := range profileIDs {
		descriptors = append(descriptors, backend.BuiltinProfileDescriptor(id))
	}
	return descriptors
}

func BuiltinProfileDescriptor(profileID string) backend.WorkflowDescriptor {
	return backend.BuiltinProfileDescriptor(profileID)
}

func DefaultWorkflowDescriptor() backend.WorkflowDescriptor {
	return backend.BuiltinProfileDescriptor(DefaultProfileID)
}

func InferWorkflowMode(workflowID string, description string, states []string) string {
	hint := strings.ToLower(workflowID + " " + description + " " + strings.Join(states, " "))
	if strings.Contains(hint, "semiauto") || strings.Contains(hint, "coarse") || strings.Contains(hint, "human") || strings.Contains(hint, "gated") || strings.Contains(hint, "pull request") || strings.Contains(hint, "pr") {
		return "coarse_human_gated"
	}
	return "granular_autonomous"
}

func InferFinalCutState(states []string) string {
	preferred := []string{"ready_for_plan_review", "ready_for_implementation_review", "ready_for_shipment_review", "reviewing"}
	for _, candidate := range preferred {
		for _, s := range states {
			if s == candidate {
				return candidate
			}
		}
	}
	return ""
}

func InferRetakeState(states []string, initialState string) string {
	preferred := []string{string(workflow.StatusOpen), "ready_for_implementation", "retake", "retry", "rejected", "refining"}
	for _, candidate := range preferred {
		for _, s := range states {
			if s == candidate {
				return candidate
			}
		}
	}
	return initialState
}

func WorkflowDescriptorByID(workflows []backend.WorkflowDescriptor) map[string]*backend.WorkflowDescriptor {
	m := make(map[string]*backend.WorkflowDescriptor, len(workflows)*3)
	for i := range workflows {
		m[workflows[i].ID] = &workflows[i]
		m[workflows[i].BackingWorkflowID] = &workflows[i]
		if workflows[i].ProfileID != "" {
			m[workflows[i].ProfileID] = &workflows[i]
		}
	}

	for i := range workflows {
		if workflows[i].ID == "autopilot" {
			m[LegacyBeadsCoarseWorkflowID] = &workflows[i]
			m["knots-granular"] = &workflows[i]
			m["knots-granular-autonomous"] = &workflows[i]
		}
		if workflows[i].ID == "semiauto" {
			m["knots-coarse"] = &workflows[i]
			m["knots-coarse-human-gated"] = &workflows[i]
			m["beads-coarse-human-gated"] = &workflows[i]
		}
	}
	return m
}

func ResolveWorkflowForBead(bead *backend.Bead, workflowsByID map[string]*backend.WorkflowDescriptor, fallback ...*backend.WorkflowDescriptor) *backend.WorkflowDescriptor {
	profileID := NormalizeProfileID(bead.ProfileID)
	if profileID != "" {
		if wf, ok := workflowsByID[profileID]; ok {
			return wf
		}
	}
	if bead.WorkflowID != "" {
		if wf, ok := workflowsByID[bead.WorkflowID]; ok {
			return wf
		}
	}
	if len(fallback) > 0 && fallback[0] != nil {
		return fallback[0]
	}
	return nil
}

func BeadRequiresHumanAction(bead *backend.Bead, workflowsByID map[string]*backend.WorkflowDescriptor) bool {
	if bead.RequiresHumanAction {
		return true
	}
	wf := ResolveWorkflowForBead(bead, workflowsByID)
	if wf == nil {
		return false
	}
	rs := DeriveWorkflowRuntimeState(wf, bead.State)
	return rs.RequiresHumanAction
}

func BeadInFinalCut(bead *backend.Bead, workflowsByID map[string]*backend.WorkflowDescriptor) bool {
	return BeadRequiresHumanAction(bead, workflowsByID)
}

func BeadInRetake(bead *backend.Bead, workflowsByID map[string]*backend.WorkflowDescriptor) bool {
	normalized := strings.TrimSpace(strings.ToLower(bead.State))
	if legacyRetakeStates[normalized] {
		return true
	}

	wf := ResolveWorkflowForBead(bead, workflowsByID)
	if wf == nil {
		return false
	}
	return strings.TrimSpace(strings.ToLower(wf.RetakeState)) == normalized
}

func CompareWorkflowStatePriority(left, right string) int {
	leftIdx, leftOk := statePipelineOrder[left]
	rightIdx, rightOk := statePipelineOrder[right]

	if leftOk && rightOk {
		if leftIdx != rightIdx {
			return leftIdx - rightIdx
		}
		if left < right {
			return -1
		}
		if left > right {
			return 1
		}
		return 0
	}
	if leftOk {
		return -1
	}
	if rightOk {
		return 1
	}
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func IsRollbackTransition(from, to string) bool {
	fromIdx, fromOk := statePipelineOrder[from]
	toIdx, toOk := statePipelineOrder[to]
	if !fromOk || !toOk {
		return false
	}
	return toIdx < fromIdx
}

func ForwardTransitionTarget(currentState string, wf *backend.WorkflowDescriptor) string {
	if len(wf.Transitions) == 0 {
		return ""
	}
	for _, t := range wf.Transitions {
		if t.From != currentState {
			continue
		}
		if !IsRollbackTransition(t.From, t.To) {
			return t.To
		}
	}
	return ""
}

func WfToSteps(wf *backend.WorkflowDescriptor) []Step {
	if wf == nil {
		return nil
	}
	steps := make([]Step, 0, len(wf.States))
	for _, s := range wf.States {
		kind := "agent"
		if wf.StateOwners != nil {
			if k, ok := wf.StateOwners[s]; ok {
				kind = string(k)
			}
		}
		steps = append(steps, Step{
			ID:   s,
			Name: s,
			Kind: kind,
			State: s,
		})
	}
	return steps
}