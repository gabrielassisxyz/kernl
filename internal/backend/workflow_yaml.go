package backend

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type workflowYAMLDoc struct {
	ID             string                        `yaml:"id"`
	Label          string                        `yaml:"label,omitempty"`
	Mode           string                        `yaml:"mode,omitempty"`
	InitialState   string                        `yaml:"initial_state,omitempty"`
	States         []string                      `yaml:"states,omitempty"`
	TerminalStates []string                      `yaml:"terminal_states,omitempty"`
	Transitions    []WorkflowTransition          `yaml:"transitions,omitempty"`
	RetakeState    string                        `yaml:"retake_state,omitempty"`
	ExitGates      map[string][]WorkflowExitGate `yaml:"exit_gates,omitempty"`
	Stages         map[string]StageContract      `yaml:"stages"`
	Owners         map[string]ActionOwnerKind    `yaml:"owners,omitempty"`
	QueueActions   map[string]string             `yaml:"queue_actions,omitempty"`
}

// LoadWorkflowYAML parses a YAML workflow descriptor file and rejects
// unknown top-level keys so that typos in stage field names are caught
// at load time instead of silently ignored.
func LoadWorkflowYAML(path string) (WorkflowDescriptor, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return WorkflowDescriptor{}, fmt.Errorf("KERNL DISPATCH FAILURE: reading workflow YAML %s: %w", path, err)
	}

	if err := rejectLegacyExitGatesShape(path, data); err != nil {
		return WorkflowDescriptor{}, err
	}

	var doc workflowYAMLDoc
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil {
		return WorkflowDescriptor{}, fmt.Errorf("KERNL DISPATCH FAILURE: parsing workflow YAML %s: %w", path, err)
	}
	if doc.ID == "" {
		return WorkflowDescriptor{}, fmt.Errorf("KERNL DISPATCH FAILURE: workflow YAML %s missing required field 'id'", path)
	}

	wd := doc.toDescriptor()
	if err := ValidateStages(wd.Stages); err != nil {
		return WorkflowDescriptor{}, err
	}
	if err := ValidateArtifactPaths(wd.Stages, wd.ExitGates); err != nil {
		return WorkflowDescriptor{}, err
	}

	return wd, nil
}

// rejectLegacyExitGatesShape rejects a workflow YAML file whose exit_gates
// block still uses the pre-list shape (a single gate object directly under
// the state key) before the strict struct decode in LoadWorkflowYAML gets a
// chance to fail on it. That decode's own error - yaml.v3's default
// "cannot unmarshal !!map into []backend.WorkflowExitGate" - names a line
// number, not the offending state, and gives no hint of what to write
// instead; a workflow author cannot act on it. This walks the raw document
// node instead of the typed workflowYAMLDoc so the error can name the exact
// state and show the fix, matching the decision (see Part 2 of the exit
// gate cardinality change): a workflow whose gates quietly changed meaning
// is worse than one that refuses to load, so the old shape is never
// silently accepted alongside the new one.
func rejectLegacyExitGatesShape(path string, data []byte) error {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		// Malformed YAML entirely - the real decode below reports this with
		// its own adequate syntax error; nothing to add here.
		return nil
	}
	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return nil
	}

	exitGatesNode := resolveAlias(mappingValue(root.Content[0], "exit_gates"))
	if exitGatesNode == nil || exitGatesNode.Kind != yaml.MappingNode {
		return nil
	}

	for i := 0; i+1 < len(exitGatesNode.Content); i += 2 {
		stateKey := exitGatesNode.Content[i]
		stateVal := resolveAlias(exitGatesNode.Content[i+1])
		if stateVal.Kind != yaml.MappingNode {
			continue
		}

		exampleType := nodeValueOrPlaceholder(mappingValue(stateVal, "type"), "gate_type")
		examplePath := nodeValueOrPlaceholder(mappingValue(stateVal, "path"), "gate_path")
		return fmt.Errorf(
			"KERNL DISPATCH FAILURE: workflow YAML %s: exit_gates.%s uses the old single-gate shape - exit_gates is now a list of gates per state, so a state can carry more than one. Fix: write it as\nexit_gates:\n  %s:\n    - type: %s\n      path: %s",
			path, stateKey.Value, stateKey.Value, yamlDoubleQuoted(exampleType), yamlDoubleQuoted(examplePath),
		)
	}
	return nil
}

// resolveAlias follows a YAML alias node (e.g. two states sharing one gate
// via "&legacy" / "*legacy" anchors) to the node it refers to. An AliasNode
// carries no Kind of its own to classify - skipping this step would let a
// state written as an alias to an old-shape gate object slip past the
// MappingNode check below unnoticed, falling through to yaml.v3's own
// unreadable decode error instead of the actionable one this file exists to
// produce. A node that is not an alias (the overwhelmingly common case) is
// returned unchanged.
func resolveAlias(n *yaml.Node) *yaml.Node {
	if n != nil && n.Kind == yaml.AliasNode && n.Alias != nil {
		return n.Alias
	}
	return n
}

// mappingValue returns the value node paired with key in a YAML mapping
// node, or nil if the mapping has no such key. Returns nil (rather than
// panicking) for a malformed odd-length content list, which yaml.v3 never
// produces for a valid MappingNode but a defensive caller should not assume.
func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// nodeValueOrPlaceholder returns node's scalar value, or placeholder when
// node is nil (the old-shape gate omitted that field) - the error message
// this feeds still needs something to show in the "write it as" example.
func nodeValueOrPlaceholder(node *yaml.Node, placeholder string) string {
	if node == nil || node.Value == "" {
		return placeholder
	}
	return node.Value
}

// yamlDoubleQuoted renders s as a double-quoted YAML scalar. The "Fix: write
// it as" example embeds a value taken verbatim from the file under
// diagnosis - and a real gate value can itself contain ": " (commit_marker's
// own path is literally "stage: implementation"). Interpolated unquoted,
// that reads as a second mapping key and the suggested fix does not parse
// ("mapping values are not allowed here"); an actionable error that emits
// invalid YAML is not actionable. Escaping is the minimum a double-quoted
// YAML scalar needs: a backslash first (so an already-escaped sequence in s
// is not double-unescaped by whatever re-parses this), then the quote that
// would otherwise close the scalar early.
func yamlDoubleQuoted(s string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
	return `"` + escaped + `"`
}

func (d *workflowYAMLDoc) toDescriptor() WorkflowDescriptor {
	wd := WorkflowDescriptor{
		ID:        d.ID,
		Label:     d.Label,
		Mode:      d.Mode,
		Stages:    d.Stages,
		ExitGates: d.ExitGates,
	}
	if d.InitialState != "" {
		wd.InitialState = d.InitialState
	}
	if len(d.States) > 0 {
		wd.States = d.States
	}
	if len(d.TerminalStates) > 0 {
		wd.TerminalStates = d.TerminalStates
	}
	if len(d.Transitions) > 0 {
		wd.Transitions = d.Transitions
	}
	if d.RetakeState != "" {
		wd.RetakeState = d.RetakeState
	}
	if len(d.Owners) > 0 {
		wd.Owners = d.Owners
	}
	if len(d.QueueActions) > 0 {
		wd.QueueActions = d.QueueActions
	}

	// Derive QueueStates, ActionStates, QueueActions, StateOwners, and Human/Review queues
	terminalStates := []string{"shipped", "abandoned"}
	if len(wd.TerminalStates) > 0 {
		terminalStates = wd.TerminalStates
	}

	qStates, actStates, qActions := deriveWorkflowStructureFromConfig(wd.States, wd.Transitions, wd.Owners, terminalStates)
	wd.QueueStates = qStates
	wd.ActionStates = actStates
	if len(wd.QueueActions) == 0 {
		wd.QueueActions = qActions
	}

	var reviewQueueStates []string
	for _, q := range wd.QueueStates {
		if action, ok := wd.QueueActions[q]; ok && strings.HasSuffix(action, "_review") {
			reviewQueueStates = append(reviewQueueStates, q)
		}
	}
	wd.ReviewQueueStates = reviewQueueStates

	var humanQueueStates []string
	for _, q := range wd.QueueStates {
		if action, ok := wd.QueueActions[q]; ok && stepOwnerKind(wd.Owners, action) == ActionOwnerHuman {
			humanQueueStates = append(humanQueueStates, q)
		}
	}
	wd.HumanQueueStates = humanQueueStates

	if len(wd.HumanQueueStates) > 0 {
		wd.FinalCutState = wd.HumanQueueStates[0]
	}

	var stateOwners map[string]ActionOwnerKind
	for _, s := range wd.ActionStates {
		if stateOwners == nil {
			stateOwners = make(map[string]ActionOwnerKind)
		}
		stateOwners[s] = stepOwnerKind(wd.Owners, s)
	}
	wd.StateOwners = stateOwners

	return wd
}
