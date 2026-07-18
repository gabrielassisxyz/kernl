package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestVersionJSONEmitsMachineReadableMetadata(t *testing.T) {
	var buf bytes.Buffer
	if err := printVersion(&buf, []string{"--json"}); err != nil {
		t.Fatalf("version --json: %v", err)
	}
	var out map[string]string
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, buf.String())
	}
	for _, key := range []string{"version", "commit", "built", "go"} {
		if out[key] == "" {
			t.Errorf("version --json missing %q: %v", key, out)
		}
	}
}

func TestVersionRejectsUnknownFlagWithHint(t *testing.T) {
	err := printVersion(&bytes.Buffer{}, []string{"--jsno"})
	if err == nil || !strings.Contains(err.Error(), `did you mean "--json"?`) {
		t.Fatalf("expected hint for --jsno, got: %v", err)
	}
}

func TestPlanRejectsUnknownFlagBeforeTouchingConfig(t *testing.T) {
	// Flag validation must precede config load so the error is about the
	// actual mistake, not a missing kernl.yaml.
	err := runPlan("definitely-missing.yaml", []string{"--jsno", "topic"})
	if err == nil || !strings.Contains(err.Error(), `did you mean "--json"?`) {
		t.Fatalf("expected plan flag hint, got: %v", err)
	}
	if exitCode(err) != 2 {
		t.Errorf("unknown plan flag is usage error, got %d", exitCode(err))
	}
}

func TestPlanOutputMarshalsCamelCaseWithEmptyNotesArray(t *testing.T) {
	b, err := json.Marshal(newPlanOutput("caching", nil))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, `"notes":[]`) {
		t.Errorf("empty notes must marshal as [], not null: %s", s)
	}
	if !strings.Contains(s, `"topic":"caching"`) {
		t.Errorf("topic missing: %s", s)
	}
}
