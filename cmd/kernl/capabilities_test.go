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
