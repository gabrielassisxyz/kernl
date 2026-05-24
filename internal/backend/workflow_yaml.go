package backend

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type workflowYAMLDoc struct {
	ID               string                    `yaml:"id"`
	Label            string                    `yaml:"label,omitempty"`
	Mode             string                    `yaml:"mode,omitempty"`
	InitialState     string                    `yaml:"initial_state,omitempty"`
	States           []string                  `yaml:"states,omitempty"`
	TerminalStates   []string                  `yaml:"terminal_states,omitempty"`
	Transitions      []WorkflowTransition      `yaml:"transitions,omitempty"`
	RetakeState      string                    `yaml:"retake_state,omitempty"`
	ExitGates        map[string]WorkflowExitGate `yaml:"exit_gates,omitempty"`
	Stages           map[string]StageContract   `yaml:"stages"`
	Owners           map[string]ActionOwnerKind `yaml:"owners,omitempty"`
	QueueActions     map[string]string          `yaml:"queue_actions,omitempty"`
}

type workflowYAMLTransition struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
}

// LoadWorkflowYAML parses a YAML workflow descriptor file and rejects
// unknown top-level keys so that typos in stage field names are caught
// at load time instead of silently ignored.
func LoadWorkflowYAML(path string) (WorkflowDescriptor, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return WorkflowDescriptor{}, fmt.Errorf("KERNL DISPATCH FAILURE: reading workflow YAML %s: %w", path, err)
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

	return wd, nil
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
	return wd
}
