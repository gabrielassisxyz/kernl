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
		ExitGates: map[string][]WorkflowExitGate{
			"implementation": {{Type: "decision_record", Path: "<artifact_dir>/decision-record.json"}},
		},
	}
}

// oneDecisionRecord is a single well-formed decision, the normal shape of a
// stage that made exactly one decision.
func oneDecisionRecord() string {
	return `{"decisions":[{"title":"Heading parser vs a schema","decision":"Use a JSON schema instead of a markdown heading parser.","optionsConsidered":"1. YAML front matter with fixed keys.\n2. A markdown heading parser matching four fixed section names.\n3. A JSON document validated against a schema.","tradeOffs":"Option 2 reads like natural prose but cannot honestly model more than one decision. Option 3 is stricter but parses and validates precisely.","rationale":"Option 3 wins: it models a list of decisions instead of exactly one, and a schema violation names precisely what is wrong instead of a misleading missing-section reason."}]}`
}

// threeDecisionsRecord is the acceptance case this gate exists to fix: a
// stage that made more than one decision in a single record, exactly the
// shape three real agents could not express under the old markdown gate
// (each inventing its own incompatible syntax for "more than one").
func threeDecisionsRecord() string {
	return `{"decisions":[` +
		`{"title":"First","decision":"Decision one.","optionsConsidered":"A or B.","tradeOffs":"A is faster.","rationale":"A wins."},` +
		`{"title":"Second","decision":"Decision two.","optionsConsidered":"C or D.","tradeOffs":"D is simpler.","rationale":"D wins."},` +
		`{"title":"Third","decision":"Decision three.","optionsConsidered":"E or F.","tradeOffs":"E is safer.","rationale":"E wins."}` +
		`]}`
}

// TestEvaluateExitGate_DecisionRecord_OneDecisionPasses proves the normal,
// single-decision shape still satisfies the gate.
func TestEvaluateExitGate_DecisionRecord_OneDecisionPasses(t *testing.T) {
	worktree := t.TempDir()
	artifactDir := t.TempDir()
	path := filepath.Join(artifactDir, "decision-record.json")
	if err := os.WriteFile(path, []byte(oneDecisionRecord()), 0o644); err != nil {
		t.Fatal(err)
	}

	wf := decisionRecordGateWorkflow()
	ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "implementation", WorktreePath: worktree, ArtifactDir: artifactDir, BeadID: "kb-1"})
	if !ok {
		t.Fatalf("expected a single well-formed decision to pass, got reason=%q", reason)
	}
	if reason != "" {
		t.Errorf("a passing gate must carry no reason, got %q", reason)
	}
}

// TestEvaluateExitGate_DecisionRecord_ThreeDecisionsPasses is the
// acceptance criterion this gate exists to fix (measured defect: a stage
// that made three decisions could not express that under the old
// single-section markdown gate, and each of three real agents invented a
// different, gate-incompatible syntax trying to). A record whose "decisions"
// array holds three entries must pass in one shot, with nothing discarded.
func TestEvaluateExitGate_DecisionRecord_ThreeDecisionsPasses(t *testing.T) {
	worktree := t.TempDir()
	artifactDir := t.TempDir()
	path := filepath.Join(artifactDir, "decision-record.json")
	if err := os.WriteFile(path, []byte(threeDecisionsRecord()), 0o644); err != nil {
		t.Fatal(err)
	}

	wf := decisionRecordGateWorkflow()
	ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "implementation", WorktreePath: worktree, ArtifactDir: artifactDir, BeadID: "kb-1"})
	if !ok {
		t.Fatalf("expected three well-formed decisions to pass, got reason=%q", reason)
	}
	if reason != "" {
		t.Errorf("a passing gate must carry no reason, got %q", reason)
	}
}

// TestParseDecisionRecordDocument_ThreeDecisionsNoneDropped proves, by
// construction, that every one of several decisions survives parsing
// distinctly - the exact failure mode the withdrawn prefix-matching approach
// would have introduced (two same-keyed headings collapsing to
// last-one-wins, silently discarding every decision but the last). A list
// preserves position and count; nothing here can make that happen.
func TestParseDecisionRecordDocument_ThreeDecisionsNoneDropped(t *testing.T) {
	entries, err := ParseDecisionRecordDocument(threeDecisionsRecord())
	if err != nil {
		t.Fatalf("ParseDecisionRecordDocument: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("got %d decisions, want 3 - a decision was dropped", len(entries))
	}
	wantTitles := []string{"First", "Second", "Third"}
	for i, want := range wantTitles {
		if entries[i].Title != want {
			t.Errorf("entries[%d].Title = %q, want %q - order or identity was not preserved", i, entries[i].Title, want)
		}
	}
}

// TestEvaluateExitGate_DecisionRecord_EmptyFileFails proves the case
// artifact_exists cannot catch: a file that is present but carries nothing.
func TestEvaluateExitGate_DecisionRecord_EmptyFileFails(t *testing.T) {
	worktree := t.TempDir()
	artifactDir := t.TempDir()
	path := filepath.Join(artifactDir, "decision-record.json")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	wf := decisionRecordGateWorkflow()
	ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "implementation", WorktreePath: worktree, ArtifactDir: artifactDir, BeadID: "kb-1"})
	if ok {
		t.Fatal("an empty file must not satisfy the decision_record gate")
	}
	if !strings.HasPrefix(reason, "decision_record_invalid_json: ") {
		t.Errorf("reason must use the decision_record_invalid_json family, got %q", reason)
	}
}

// TestEvaluateExitGate_DecisionRecord_EmptyDecisionsArrayFails proves valid
// JSON with no decisions in it is still rejected, precisely - never silently
// treated as "nothing to record".
func TestEvaluateExitGate_DecisionRecord_EmptyDecisionsArrayFails(t *testing.T) {
	worktree := t.TempDir()
	artifactDir := t.TempDir()
	path := filepath.Join(artifactDir, "decision-record.json")
	if err := os.WriteFile(path, []byte(`{"decisions":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	wf := decisionRecordGateWorkflow()
	ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "implementation", WorktreePath: worktree, ArtifactDir: artifactDir, BeadID: "kb-1"})
	if ok {
		t.Fatal("an empty decisions array must not satisfy the decision_record gate")
	}
	if reason != `decision_record_empty: the "decisions" array must contain at least one entry` {
		t.Errorf("unexpected reason: %q", reason)
	}
}

// TestEvaluateExitGate_DecisionRecord_MissingEachField proves the failure
// reason names precisely which field, at which array index, is absent or
// blank - an agent told "invalid" cannot act on it, one told
// "decisions[0].tradeOffs" can. This replaces the old
// decision_record_missing_sections family: the artifact is JSON now, so the
// reason names JSON fields, not markdown section keys.
func TestEvaluateExitGate_DecisionRecord_MissingEachField(t *testing.T) {
	base := map[string]string{
		"decision":          "d",
		"optionsConsidered": "o",
		"tradeOffs":         "t",
		"rationale":         "r",
	}

	for _, missingField := range []string{"decision", "optionsConsidered", "tradeOffs", "rationale"} {
		t.Run(missingField, func(t *testing.T) {
			var b strings.Builder
			b.WriteString(`{"decisions":[{`)
			first := true
			for field, value := range base {
				if field == missingField {
					continue
				}
				if !first {
					b.WriteString(",")
				}
				first = false
				b.WriteString(`"` + field + `":"` + value + `"`)
			}
			b.WriteString(`}]}`)

			worktree := t.TempDir()
			artifactDir := t.TempDir()
			path := filepath.Join(artifactDir, "decision-record.json")
			if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
				t.Fatal(err)
			}

			wf := decisionRecordGateWorkflow()
			ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "implementation", WorktreePath: worktree, ArtifactDir: artifactDir, BeadID: "kb-1"})
			if ok {
				t.Fatalf("a record missing %q must not pass", missingField)
			}
			want := "decision_record_missing_fields: decisions[0]." + missingField
			if reason != want {
				t.Errorf("reason = %q, want %q", reason, want)
			}
		})
	}
}

// TestEvaluateExitGate_DecisionRecord_BlankFieldFails proves a field present
// but blank (whitespace-only) is treated the same as absent - an agent
// cannot satisfy the gate by adding an empty string for a field it has
// nothing to say.
func TestEvaluateExitGate_DecisionRecord_BlankFieldFails(t *testing.T) {
	worktree := t.TempDir()
	artifactDir := t.TempDir()
	path := filepath.Join(artifactDir, "decision-record.json")
	content := `{"decisions":[{"decision":"d","optionsConsidered":"o","tradeOffs":"   ","rationale":"r"}]}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	wf := decisionRecordGateWorkflow()
	ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "implementation", WorktreePath: worktree, ArtifactDir: artifactDir, BeadID: "kb-1"})
	if ok {
		t.Fatal("a whitespace-only field must not satisfy the decision_record gate")
	}
	if reason != "decision_record_missing_fields: decisions[0].tradeOffs" {
		t.Errorf("unexpected reason: %q", reason)
	}
}

// TestEvaluateExitGate_DecisionRecord_MalformedJSONFails proves invalid JSON
// is rejected with the syntax-error family, naming the underlying JSON
// error rather than a generic "invalid".
func TestEvaluateExitGate_DecisionRecord_MalformedJSONFails(t *testing.T) {
	worktree := t.TempDir()
	artifactDir := t.TempDir()
	path := filepath.Join(artifactDir, "decision-record.json")
	if err := os.WriteFile(path, []byte(`{"decisions": [not valid json`), 0o644); err != nil {
		t.Fatal(err)
	}

	wf := decisionRecordGateWorkflow()
	ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "implementation", WorktreePath: worktree, ArtifactDir: artifactDir, BeadID: "kb-1"})
	if ok {
		t.Fatal("malformed JSON must not satisfy the decision_record gate")
	}
	if !strings.HasPrefix(reason, "decision_record_invalid_json: ") {
		t.Errorf("reason must use the decision_record_invalid_json family, got %q", reason)
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
	if err := os.WriteFile(filepath.Join(worktree, "decision-record.json"), []byte(oneDecisionRecord()), 0o644); err != nil {
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
	gates := withPR.ExitGates["implementation"]
	if len(gates) != 1 || gates[0].Type != "decision_record" || gates[0].Path != "<artifact_dir>/decision-record.json" {
		t.Errorf("autopilot_with_pr implementation gates = %+v; want exactly one decision_record/<artifact_dir>/decision-record.json", gates)
	}

	stage, ok := withPR.Stages["implementation"]
	if !ok || stage.DecisionRecord.Path != "<artifact_dir>/decision-record.json" {
		t.Errorf("autopilot_with_pr implementation stage DecisionRecord = %+v, ok=%v; want <artifact_dir>/decision-record.json", stage.DecisionRecord, ok)
	}

	autopilot := BuiltinProfileDescriptor("autopilot")
	if len(autopilot.ExitGates) != 0 {
		t.Errorf("autopilot must carry no exit gates, got %+v", autopilot.ExitGates)
	}
}
