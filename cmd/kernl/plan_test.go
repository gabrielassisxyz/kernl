package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/planning"
)

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
