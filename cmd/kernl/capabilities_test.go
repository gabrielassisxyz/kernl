package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestCapabilitiesCoversEveryDispatchableVerb(t *testing.T) {
	var buf bytes.Buffer
	if err := runCapabilities(&buf, nil); err != nil {
		t.Fatalf("capabilities: %v", err)
	}
	var out capabilitiesOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	if out.ContractVersion == "" || out.Tool != "kernl" {
		t.Errorf("contract identity missing: %+v", out)
	}
	names := map[string]bool{}
	for _, c := range out.Commands {
		names[c.Name] = true
	}
	for _, verb := range []string{"serve", "doctor", "epic", "bead", "sweep", "bookmark", "capture", "plan", "capabilities", "version"} {
		if !names[verb] {
			t.Errorf("capabilities missing verb %q", verb)
		}
	}
	if len(out.ExitCodes) != 3 {
		t.Errorf("exit-code dictionary must have 3 entries, got %d", len(out.ExitCodes))
	}
	if len(out.EnvVars) == 0 {
		t.Error("env vars must be documented")
	}
}

// R2-003: the machine contract must expose the two most load-bearing surfaces
// for the parity verbs — --server and KERNL_SERVER, the only way to run them
// without a local kernl.yaml — and carry each command's flag documentation
// (Details), not leave it human-only in --help.
func TestCapabilitiesExposesServerAndFlagDetails(t *testing.T) {
	var buf bytes.Buffer
	if err := runCapabilities(&buf, nil); err != nil {
		t.Fatalf("capabilities: %v", err)
	}
	var out capabilitiesOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}

	hasEnv := false
	for _, e := range out.EnvVars {
		if e.Name == "KERNL_SERVER" {
			hasEnv = true
		}
	}
	if !hasEnv {
		t.Error("KERNL_SERVER must appear in capabilities.envVars")
	}
	hasFlag := false
	for _, f := range out.GlobalFlags {
		if f.Name == "--server" {
			hasFlag = true
		}
	}
	if !hasFlag {
		t.Error("--server must appear in capabilities.globalFlags")
	}

	// A command with documented flags must carry them in details.
	var project *capabilityCommand
	for i := range out.Commands {
		if out.Commands[i].Name == "project" {
			project = &out.Commands[i]
		}
	}
	if project == nil {
		t.Fatal("project verb missing from capabilities")
	}
	var setSub *capabilityCommand
	for i := range project.Subcommands {
		if project.Subcommands[i].Name == "set" {
			setSub = &project.Subcommands[i]
		}
	}
	if setSub == nil || !strings.Contains(setSub.Details, "--status") {
		t.Errorf("project set details must carry its flag contract, got: %q", details(setSub))
	}
}

func details(c *capabilityCommand) string {
	if c == nil {
		return "<nil>"
	}
	return c.Details
}

func TestCapabilitiesJSONFlagAcceptedAndTypoHinted(t *testing.T) {
	var buf bytes.Buffer
	if err := runCapabilities(&buf, []string{"--json"}); err != nil {
		t.Fatalf("capabilities --json: %v", err)
	}
	err := runCapabilities(&buf, []string{"--jsn"})
	if err == nil || !strings.Contains(err.Error(), `did you mean "--json"?`) {
		t.Fatalf("expected hint, got: %v", err)
	}
}
