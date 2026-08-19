package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/planning"
)

// TestParsePlanArgsLimit pins the --limit flag contract: the default stays 8,
// an explicit value is honoured, and a non-positive or non-numeric value is
// refused with an error that names the flag and the value - never silently
// falling back to the default, which would quietly measure the wrong depth.
func TestParsePlanArgsLimit(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		wantLimit int
		wantTopic string
		wantErr   string
	}{
		{name: "default", args: []string{"caching"}, wantLimit: 8, wantTopic: "caching"},
		{name: "explicit", args: []string{"--limit", "25", "caching"}, wantLimit: 25, wantTopic: "caching"},
		{name: "zero", args: []string{"--limit", "0", "caching"}, wantErr: `--limit must be positive, got 0`},
		{name: "negative", args: []string{"--limit", "-3", "caching"}, wantErr: `--limit must be positive, got -3`},
		{name: "non-numeric", args: []string{"--limit", "abc", "caching"}, wantErr: `--limit needs a positive integer, got "abc"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pa, err := parsePlanArgs(c.args)
			if c.wantErr != "" {
				if err == nil {
					t.Fatalf("parsePlanArgs(%v) = %+v, want error containing %q", c.args, pa, c.wantErr)
				}
				if !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("parsePlanArgs(%v) error = %q, want it to contain %q", c.args, err, c.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parsePlanArgs(%v) unexpected error: %v", c.args, err)
			}
			if pa.limit != c.wantLimit {
				t.Errorf("parsePlanArgs(%v).limit = %d, want %d", c.args, pa.limit, c.wantLimit)
			}
			if pa.topic != c.wantTopic {
				t.Errorf("parsePlanArgs(%v).topic = %q, want %q", c.args, pa.topic, c.wantTopic)
			}
		})
	}
}

// TestPlanJSONClaimPathIsNull pins the null convention at the wire: a claim has
// no file on disk, so its path must serialize as an explicit JSON null, never
// as "" (which would be indistinguishable from "failed to resolve the path").
func TestPlanJSONClaimPathIsNull(t *testing.T) {
	out := newPlanOutput("deploy", []planning.ContextNote{
		{ID: "claim-1", Title: "Deploy cadence", Via: "claim", Snippet: "canary rollout"},
	})
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"path":null`) {
		t.Errorf("claim path must serialize as null, got: %s", raw)
	}
	if strings.Contains(string(raw), `"path":""`) {
		t.Errorf("claim path must not serialize as an empty string, got: %s", raw)
	}
}

// TestPlanJSONNoteCarriesIDAndPath verifies a content note's id and resolved
// path both land in the JSON, so the output can drive `kernl note read` with no
// extra lookup.
func TestPlanJSONNoteCarriesIDAndPath(t *testing.T) {
	p := "notes/caching.md"
	out := newPlanOutput("caching", []planning.ContextNote{
		{ID: "note-1", Title: "Caching", Via: "content", Snippet: "LRU", Path: &p},
	})
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, want := range []string{`"id":"note-1"`, `"path":"notes/caching.md"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("plan JSON missing %s, got: %s", want, raw)
		}
	}
}

// TestPlanJSONCarriesType verifies the node type lands in the JSON, so a
// consumer can tell a note from a task without guessing from a path.
func TestPlanJSONCarriesType(t *testing.T) {
	out := newPlanOutput("composer", []planning.ContextNote{
		{ID: "note-1", Title: "Caching", Via: "content", Type: "note", Snippet: "LRU"},
		{ID: "task-1", Title: "Report composer", Via: "content", Type: "task", Snippet: "markdown report"},
	})
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, want := range []string{`"type":"note"`, `"type":"task"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("plan JSON missing %s, got: %s", want, raw)
		}
	}
}
