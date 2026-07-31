package backend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// decisionRecordGateWorkflow builds a minimal WorkflowDescriptor carrying only
// the "decision_record" gate under test, so these tests do not depend on the
// shape of any builtin profile.
func decisionRecordGateWorkflow() WorkflowDescriptor {
	return WorkflowDescriptor{
		ExitGates: map[string]WorkflowExitGate{
			"implementation": {Type: "decision_record", Path: "<artifact_dir>/decision-record.md"},
		},
	}
}

func fullDecisionRecord() string {
	return `# Decision record

## Decision

Use a regexp-based markdown heading parser instead of a YAML front-matter block.

## Options Considered

1. YAML front matter with fixed keys.
2. A markdown heading parser matching four fixed section names.
3. No structure check at all (status quo).

## Trade-offs

Option 1 is stricter but forces a schema the agent has to be taught up front.
Option 2 reads like natural prose the agent already knows how to write.
Option 3 catches nothing (the defect this gate exists to close).

## Rationale

Option 2 wins: it is parseable without teaching the agent a schema, and it
degrades gracefully - a heading typo fails loud with a named missing section
instead of silently accepting malformed input.
`
}

// TestEvaluateExitGate_DecisionRecord_AllSectionsPresent_Passes proves a
// well-formed record satisfies the gate (acceptance criterion 2).
func TestEvaluateExitGate_DecisionRecord_AllSectionsPresent_Passes(t *testing.T) {
	worktree := t.TempDir()
	artifactDir := t.TempDir()
	path := filepath.Join(artifactDir, "decision-record.md")
	if err := os.WriteFile(path, []byte(fullDecisionRecord()), 0o644); err != nil {
		t.Fatal(err)
	}

	wf := decisionRecordGateWorkflow()
	ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "implementation", WorktreePath: worktree, ArtifactDir: artifactDir, BeadID: "kb-1"})
	if !ok {
		t.Fatalf("expected a complete decision record to pass, got reason=%q", reason)
	}
	if reason != "" {
		t.Errorf("a passing gate must carry no reason, got %q", reason)
	}
}

// TestEvaluateExitGate_DecisionRecord_EmptyFileFails proves the case
// artifact_exists cannot catch: a file that is present but carries nothing
// (acceptance criterion 3).
func TestEvaluateExitGate_DecisionRecord_EmptyFileFails(t *testing.T) {
	worktree := t.TempDir()
	artifactDir := t.TempDir()
	path := filepath.Join(artifactDir, "decision-record.md")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	wf := decisionRecordGateWorkflow()
	ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "implementation", WorktreePath: worktree, ArtifactDir: artifactDir, BeadID: "kb-1"})
	if ok {
		t.Fatal("an empty file must not satisfy the decision_record gate")
	}
	for _, want := range []string{"decision", "options_considered", "trade_offs", "rationale"} {
		if !strings.Contains(reason, want) {
			t.Errorf("reason must name %q as missing, got %q", want, reason)
		}
	}
}

// TestEvaluateExitGate_DecisionRecord_MissingEachSection proves the failure
// reason names precisely which section is absent, one at a time (acceptance
// criterion 1): an agent told "invalid" cannot act on it, one told "missing:
// trade_offs" can.
func TestEvaluateExitGate_DecisionRecord_MissingEachSection(t *testing.T) {
	sections := map[string]string{
		"decision": `
## Options Considered
Option A, option B.

## Trade-offs
A is faster, B is simpler.

## Rationale
B won for simplicity.
`,
		"options_considered": `
## Decision
Use option B.

## Trade-offs
A is faster, B is simpler.

## Rationale
B won for simplicity.
`,
		"trade_offs": `
## Decision
Use option B.

## Options Considered
Option A, option B.

## Rationale
B won for simplicity.
`,
		"rationale": `
## Decision
Use option B.

## Options Considered
Option A, option B.

## Trade-offs
A is faster, B is simpler.
`,
	}

	for missingKey, content := range sections {
		t.Run(missingKey, func(t *testing.T) {
			worktree := t.TempDir()
			artifactDir := t.TempDir()
			path := filepath.Join(artifactDir, "decision-record.md")
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}

			wf := decisionRecordGateWorkflow()
			ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "implementation", WorktreePath: worktree, ArtifactDir: artifactDir, BeadID: "kb-1"})
			if ok {
				t.Fatalf("a record missing %q must not pass", missingKey)
			}
			const prefix = "decision_record_missing_sections: "
			if !strings.HasPrefix(reason, prefix) {
				t.Fatalf("reason must use the decision_record_missing_sections family, got %q", reason)
			}
			reportedKeys := strings.Split(strings.TrimPrefix(reason, prefix), ", ")
			if len(reportedKeys) != 1 || reportedKeys[0] != missingKey {
				t.Errorf("reason must name exactly the missing section %q, got keys %v (reason=%q)", missingKey, reportedKeys, reason)
			}
		})
	}
}

// TestEvaluateExitGate_DecisionRecord_HeadingMatchIsCaseAndPunctuationInsensitive
// proves the parser does not force the agent to reproduce the heading text
// byte-for-byte - "Trade offs" and "TRADE-OFFS" both satisfy "Trade-offs".
func TestEvaluateExitGate_DecisionRecord_HeadingMatchIsCaseAndPunctuationInsensitive(t *testing.T) {
	content := `
## decision
Use option B.

## OPTIONS CONSIDERED
Option A, option B.

### Trade offs
A is faster, B is simpler.

##Rationale
B won for simplicity.
`
	worktree := t.TempDir()
	artifactDir := t.TempDir()
	path := filepath.Join(artifactDir, "decision-record.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	wf := decisionRecordGateWorkflow()
	// "##Rationale" (no space after the hashes) is not a markdown heading by
	// the CommonMark rule this parser follows, so it is expected to still be
	// reported missing - only the case/punctuation variance is under test
	// for the other three.
	ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "implementation", WorktreePath: worktree, ArtifactDir: artifactDir, BeadID: "kb-1"})
	if ok {
		t.Fatal("expected the malformed '##Rationale' heading to still fail the gate")
	}
	if reason != "decision_record_missing_sections: rationale" {
		t.Errorf("expected only rationale to be reported missing, got %q", reason)
	}
}

func TestEvaluateExitGate_DecisionRecord_MissingFile(t *testing.T) {
	worktree := t.TempDir()
	artifactDir := t.TempDir()

	wf := decisionRecordGateWorkflow()
	ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "implementation", WorktreePath: worktree, ArtifactDir: artifactDir, BeadID: "kb-1"})
	if ok {
		t.Fatal("a missing decision record file must not pass")
	}
	if !strings.HasPrefix(reason, "artifact_missing: ") {
		t.Errorf("reason must use the artifact_missing family, got %q", reason)
	}
}

// TestEvaluateExitGate_DecisionRecord_ArtifactDirUnset mirrors the existing
// artifact_verdict/artifact_exists guard: a <artifact_dir> path with no
// ArtifactDir must not silently resolve against the filesystem root.
func TestEvaluateExitGate_DecisionRecord_ArtifactDirUnset(t *testing.T) {
	worktree := t.TempDir()

	wf := decisionRecordGateWorkflow()
	ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "implementation", WorktreePath: worktree, ArtifactDir: "", BeadID: "kb-1"})
	if ok {
		t.Fatal("an unset ArtifactDir must not pass")
	}
	if !strings.HasPrefix(reason, "artifact_dir_unset: ") {
		t.Errorf("reason must use the artifact_dir_unset family, got %q", reason)
	}
}

// TestEvaluateExitGate_DecisionRecord_NeverResolvesInsideWorktree proves the
// gate reads the record from ArtifactDir, never from WorktreePath, even when
// a file of the same name sits inside the worktree - the exact leak the
// <artifact_dir> convention exists to close (see ExitGateContext.ArtifactDir
// and archeion PR #40).
func TestEvaluateExitGate_DecisionRecord_NeverResolvesInsideWorktree(t *testing.T) {
	worktree := t.TempDir()
	artifactDir := t.TempDir()

	// A complete, passing record sitting INSIDE the worktree, where the gate
	// must never look.
	if err := os.WriteFile(filepath.Join(worktree, "decision-record.md"), []byte(fullDecisionRecord()), 0o644); err != nil {
		t.Fatal(err)
	}

	wf := decisionRecordGateWorkflow()
	ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "implementation", WorktreePath: worktree, ArtifactDir: artifactDir, BeadID: "kb-1"})
	if ok {
		t.Fatal("gate must not pass by reading a same-named file inside the worktree")
	}
	if !strings.HasPrefix(reason, "artifact_missing: ") {
		t.Fatalf("expected artifact_missing (record absent from ArtifactDir), got %q", reason)
	}
	if strings.Contains(reason, worktree) {
		t.Errorf("resolved path must not point inside the worktree, got %q", reason)
	}
	if !strings.HasPrefix(reason, "artifact_missing: "+artifactDir) {
		t.Errorf("resolved path must be under ArtifactDir, got %q", reason)
	}
}

// TestBuiltinProfile_AutopilotWithPR_CarriesDecisionRecordGate proves the
// canonical implementation stage carries the new gate end to end via the
// builtin profile, and that the sibling "autopilot" profile (no PR output)
// is untouched, matching TestEvaluateExitGate_Total's premise that
// "autopilot" declares no exit gates at all.
func TestBuiltinProfile_AutopilotWithPR_CarriesDecisionRecordGate(t *testing.T) {
	withPR := BuiltinProfileDescriptor("autopilot_with_pr")
	gate, ok := withPR.ExitGates["implementation"]
	if !ok || gate.Type != "decision_record" || gate.Path != "<artifact_dir>/decision-record.md" {
		t.Errorf("autopilot_with_pr implementation gate = %+v, ok=%v; want decision_record/<artifact_dir>/decision-record.md", gate, ok)
	}

	stage, ok := withPR.Stages["implementation"]
	if !ok || stage.DecisionRecord.Path != "<artifact_dir>/decision-record.md" {
		t.Errorf("autopilot_with_pr implementation stage DecisionRecord = %+v, ok=%v; want <artifact_dir>/decision-record.md", stage.DecisionRecord, ok)
	}

	autopilot := BuiltinProfileDescriptor("autopilot")
	if len(autopilot.ExitGates) != 0 {
		t.Errorf("autopilot must carry no exit gates, got %+v", autopilot.ExitGates)
	}
}
