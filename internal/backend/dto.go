package backend

import (
	"fmt"
	"strings"
)

type InvariantKind string

const (
	InvariantKindScope InvariantKind = "Scope"
	InvariantKindState InvariantKind = "State"
)

type Invariant struct {
	Kind      InvariantKind `json:"kind"`
	Condition string        `json:"condition"`
}

type RawBead struct {
	ID                 string         `json:"id"`
	Aliases            []string       `json:"aliases,omitempty"`
	Title              string         `json:"title"`
	Description        string         `json:"description,omitempty"`
	Notes              string         `json:"notes,omitempty"`
	AcceptanceCriteria string         `json:"acceptance_criteria,omitempty"`
	IssueType          string         `json:"issue_type,omitempty"`
	Status             string         `json:"status,omitempty"`
	Priority           int            `json:"priority,omitempty"`
	Labels             []string       `json:"labels,omitempty"`
	Assignee           string         `json:"assignee,omitempty"`
	Owner              string         `json:"owner,omitempty"`
	Parent             string         `json:"parent,omitempty"`
	Due                string         `json:"due,omitempty"`
	EstimatedMinutes   int            `json:"estimated_minutes,omitempty"`
	CreatedAt          string         `json:"created_at,omitempty"`
	CreatedBy          string         `json:"created_by,omitempty"`
	UpdatedAt          string         `json:"updated_at,omitempty"`
	ClosedAt           string         `json:"closed_at,omitempty"`
	CloseReason        string         `json:"close_reason,omitempty"`
	Metadata           map[string]any `json:"metadata,omitempty"`
	Dependencies       []RawDependency `json:"dependencies,omitempty"`

	Acceptance string `json:"acceptance,omitempty"`
	Estimate   int    `json:"estimate,omitempty"`
	Created    string `json:"created,omitempty"`
	Updated    string `json:"updated,omitempty"`
	Closed     string `json:"closed,omitempty"`
}

type RawDependency struct {
	SourceID string `json:"issue_id"`
	TargetID string `json:"depends_on_id"`
	DepType  string `json:"type"`
}

var validTypes = map[string]bool{
	"bug": true, "feature": true, "task": true, "epic": true,
	"chore": true, "merge-request": true, "molecule": true, "gate": true,
}

func isValidType(t string) bool {
	return validTypes[t]
}

var workflowInitialStates = map[string]bool{
	"ready_for_implementation": true,
	"ready_for_planning":      true,
	"ready_for_review":         true,
}

var workflowKnownStates = map[string]bool{
	"ready_for_implementation":         true,
	"implementation":                   true,
	"ready_for_implementation_review": true,
	"implementation_review":            true,
	"ready_for_integration":           true,
	"integration":                      true,
	"ready_for_integration_review":    true,
	"integration_review":               true,
	"ready_for_shipment":              true,
	"shipment":                         true,
	"ready_for_shipment_review":       true,
	"shipment_review":                  true,
	"shipped":                          true,
	"ready_for_planning":              true,
	"planning":                         true,
	"ready_for_plan_review":           true,
	"plan_review":                      true,
	"deferred":                         true,
	"blocked":                          true,
}

// defaultState returns the workflow state for this bead. It is a total
// function — if rawStatus doesn't match any recognized state it returns it
// verbatim and lets the dispatcher decide whether it is routable.
// Fail-loud for unroutable states lives in the dispatcher (see drive_bead.go
// and ResolveAgentForBead in agent_select.go).
func defaultState(labels []string, rawStatus string) string {
	// Prefer bd's real status when it is already a known workflow state.
	// Agents update status via `bd update --status <next>` but may not
	// touch labels, leaving wf:state:* labels stale. Trusting the
	// authoritative bd status avoids infinite stuck-state loops.
	if workflowKnownStates[rawStatus] {
		return rawStatus
	}
	if workflowInitialStates[rawStatus] {
		return rawStatus
	}
	if rawStatus == "in_progress" || rawStatus == "implementation" || rawStatus == "planning" {
		return rawStatus
	}
	if rawStatus == "shipped" || rawStatus == "closed" || rawStatus == "done" || rawStatus == "abandoned" {
		return rawStatus
	}
	for _, l := range labels {
		if strings.HasPrefix(l, "wf:state:") {
			return strings.TrimPrefix(l, "wf:state:")
		}
	}
	return rawStatus
}

func inferParent(id string, explicitParent string, deps []RawDependency) string {
	if explicitParent != "" {
		return explicitParent
	}
	for _, d := range deps {
		if d.DepType == "parent-child" {
			return d.TargetID
		}
	}
	dotIdx := strings.LastIndex(id, ".")
	if dotIdx == -1 {
		return ""
	}
	return id[:dotIdx]
}

func filterLabels(labels []string) []string {
	if labels == nil {
		return []string{}
	}
	filtered := make([]string, 0, len(labels))
	for _, l := range labels {
		if strings.TrimSpace(l) != "" {
			filtered = append(filtered, l)
		}
	}
	return filtered
}

func extractProfileID(labels []string, metadata map[string]any) string {
	for _, l := range labels {
		if strings.HasPrefix(l, "wf:profile:") {
			return strings.TrimPrefix(l, "wf:profile:")
		}
	}
	if metadata != nil {
		if v, ok := metadata["profileId"].(string); ok && v != "" {
			return v
		}
	}
	return "autopilot_no_planning"
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

const invHeader = "[Invariants]"

func parseInvariantsFromNotes(notes string) (invariants []Invariant, cleanNotes string, found bool) {
	if notes == "" {
		return nil, notes, false
	}
	headerIdx := strings.Index(notes, invHeader)
	if headerIdx == -1 {
		return nil, notes, false
	}

	before := strings.TrimRight(notes[:headerIdx], "\n\r ")
	afterHeader := notes[headerIdx+len(invHeader):]
	lines := strings.Split(afterHeader, "\n")

	var parsed []Invariant
	endIdx := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			endIdx = i + 1
			continue
		}
		colonIdx := strings.Index(trimmed, ":")
		if colonIdx == -1 {
			break
		}
		kind := strings.TrimSpace(trimmed[:colonIdx])
		condition := strings.TrimSpace(trimmed[colonIdx+1:])
		if kind != "Scope" && kind != "State" {
			break
		}
		if condition != "" {
			parsed = append(parsed, Invariant{Kind: InvariantKind(kind), Condition: condition})
		}
		endIdx = i + 1
	}

	if len(parsed) == 0 {
		return nil, notes, false
	}

	remaining := strings.TrimSpace(strings.Join(lines[endIdx:], "\n"))
	var clean string
	if before != "" && remaining != "" {
		clean = before + "\n\n" + remaining
	} else if before != "" {
		clean = before
	} else if remaining != "" {
		clean = remaining
	}
	return parsed, clean, true
}

func embedInvariantsInNotes(notes string, invariants []Invariant) string {
	if len(invariants) == 0 {
		return notes
	}
	var lines []string
	lines = append(lines, invHeader)
	for _, inv := range invariants {
		cond := strings.TrimSpace(inv.Condition)
		if cond == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s: %s", inv.Kind, cond))
	}
	if len(lines) == 1 {
		return notes
	}
	section := strings.Join(lines, "\n")
	if notes == "" {
		return section
	}
	return notes + "\n\n" + section
}

func NormalizeBead(raw RawBead) Bead {
	id := raw.ID
	rawType := firstNonEmpty(raw.IssueType, "task")
	bdType := rawType
	if !isValidType(rawType) {
		bdType = "task"
	}

	rawStatus := firstNonEmpty(raw.Status, "open")
	labels := filterLabels(raw.Labels)

	profileID := extractProfileID(labels, raw.Metadata)

	state := defaultState(labels, rawStatus)

	rawPriority := raw.Priority
	priority := rawPriority
	if priority < 0 || priority > 4 || priority == 0 {
		priority = 2
	}

	parent := inferParent(id, raw.Parent, raw.Dependencies)

	var invariants []Invariant
	notes := raw.Notes
	if inv, clean, found := parseInvariantsFromNotes(notes); found {
		invariants = inv
		notes = clean
	}

	acceptance := firstNonEmpty(raw.AcceptanceCriteria, raw.Acceptance)
	estimate := raw.EstimatedMinutes
	if estimate == 0 {
		estimate = raw.Estimate
	}

	created := firstNonEmpty(raw.CreatedAt, raw.Created)
	updated := firstNonEmpty(raw.UpdatedAt, raw.Updated)
	closed := firstNonEmpty(raw.ClosedAt, raw.Closed)

	metadata := raw.Metadata
	if raw.CloseReason != "" {
		if metadata == nil {
			metadata = make(map[string]any)
		}
		metadata["close_reason"] = raw.CloseReason
	}

	deps := make([]BeadDependency, 0, len(raw.Dependencies))
	for _, d := range raw.Dependencies {
		deps = append(deps, BeadDependency{
			SourceID: d.SourceID,
			TargetID: d.TargetID,
			Type:     d.DepType,
		})
	}

	bead := Bead{
		ID:           id,
		Type:         bdType,
		State:        state,
		Title:        raw.Title,
		Description:  raw.Description,
		Notes:        notes,
		Acceptance:   acceptance,
		Priority:     priority,
		Labels:       labels,
		Assignee:     raw.Assignee,
		Owner:        raw.Owner,
		ParentID:     parent,
		Due:          raw.Due,
		Estimate:     estimate,
		CreatedAt:    created,
		UpdatedAt:    updated,
		ClosedAt:     closed,
		Metadata:     metadata,
		Dependencies: deps,
		ProfileID:    profileID,
	}
	if len(invariants) > 0 {
		bead.Invariants = invariants
	}
	return bead
}

func DenormalizeBead(bead Bead) RawBead {
	status := mapBeadStateToCompatStatus(bead.State)

	labels := make([]string, len(bead.Labels))
	copy(labels, bead.Labels)

	hasStateLabel := false
	hasProfileLabel := false
	for _, l := range labels {
		if strings.HasPrefix(l, "wf:state:") {
			hasStateLabel = true
		}
		if strings.HasPrefix(l, "wf:profile:") {
			hasProfileLabel = true
		}
	}
	if !hasStateLabel && bead.State != "" {
		labels = append(labels, "wf:state:"+bead.State)
	}
	profileID := "autopilot_no_planning"
	if bead.ProfileID != "" {
		profileID = bead.ProfileID
	}
	if !hasProfileLabel {
		labels = append(labels, "wf:profile:"+profileID)
	}

	notesWithInvariants := embedInvariantsInNotes(bead.Notes, bead.Invariants)

	raw := RawBead{
		ID:       bead.ID,
		Title:    bead.Title,
		IssueType: bead.Type,
		Status:   status,
		Priority: bead.Priority,
		Labels:   labels,
		CreatedAt: bead.CreatedAt,
		UpdatedAt: bead.UpdatedAt,
	}
	if bead.Description != "" {
		raw.Description = bead.Description
	}
	if notesWithInvariants != "" {
		raw.Notes = notesWithInvariants
	}
	if bead.Acceptance != "" {
		raw.AcceptanceCriteria = bead.Acceptance
	}
	if bead.Assignee != "" {
		raw.Assignee = bead.Assignee
	}
	if bead.Owner != "" {
		raw.Owner = bead.Owner
	}
	if bead.ParentID != "" {
		raw.Parent = bead.ParentID
	}
	if bead.Due != "" {
		raw.Due = bead.Due
	}
	if bead.Estimate > 0 {
		raw.EstimatedMinutes = bead.Estimate
	}
	if bead.ClosedAt != "" {
		raw.ClosedAt = bead.ClosedAt
	}
	if bead.Metadata != nil {
		if cr, ok := bead.Metadata["close_reason"].(string); ok && cr != "" {
			raw.CloseReason = cr
		}
		raw.Metadata = bead.Metadata
	}
	for _, d := range bead.Dependencies {
		raw.Dependencies = append(raw.Dependencies, RawDependency{
			SourceID: d.SourceID,
			TargetID: d.TargetID,
			DepType:  d.Type,
		})
	}
	return raw
}

func mapBeadStateToCompatStatus(state string) string {
	switch state {
	case "deferred":
		return "deferred"
	case "blocked", "rejected":
		return "blocked"
	case "shipped", "abandoned", "closed", "done", "approved":
		return "closed"
	case "ready_for_implementation", "ready_for_planning", "ready_for_review", "ready_for_integration", "ready_for_integration_review":
		return "open"
	case "planning", "implementation", "shipment_review", "plan_review", "integration", "integration_review":
		return "in_progress"
	default:
		return "open"
	}
}

func clampPriority(p int) int {
	if p < 0 || p > 4 {
		return 2
	}
	return p
}

func parentFromDeps(deps []RawDependency) string {
	for _, d := range deps {
		if d.DepType == "parent-child" {
			return d.TargetID
		}
	}
	return ""
}