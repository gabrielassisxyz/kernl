package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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

// --- ParseDecisionRecordDocument: the two silent-drop paths a plain
// json.Unmarshal leaves open (FIX 1). Every case below is written as a
// literal JSON string, the way an agent actually writes one to disk - never
// by marshalling DecisionRecordEntry, which would only prove this package
// agrees with itself about field names, not that it catches a real typo or
// a real duplicate key. ---

// TestParseDecisionRecordDocument_RejectsUnknownField is the review's exact
// example: a typo'd field name ("titel" for "title") that json.Unmarshal
// would otherwise ignore outright, leaving the agent's intended title
// silently replaced by the derived fallback with no error at all.
func TestParseDecisionRecordDocument_RejectsUnknownField(t *testing.T) {
	content := `{"decisions":[{"titel":"My title","decision":"d","optionsConsidered":"o","tradeOffs":"t","rationale":"r"}]}`
	_, err := ParseDecisionRecordDocument(content)
	if err == nil {
		t.Fatal("expected an error for an unrecognized field name")
	}
	if !strings.HasPrefix(err.Error(), "decision_record_invalid_json: ") {
		t.Errorf("reason must use the decision_record_invalid_json family, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "titel") {
		t.Errorf("error must name the offending field, got %q", err.Error())
	}
}

// TestParseDecisionRecordDocument_UnknownFieldInsideEntryRejected proves the
// same guard reaches every array entry, not just the document's top level -
// DisallowUnknownFields applies recursively through the nested struct, and
// this pins that it actually does for THIS schema instead of assuming it.
func TestParseDecisionRecordDocument_UnknownFieldInsideEntryRejected(t *testing.T) {
	content := `{"decisions":[{"decision":"d","optionsConsidered":"o","tradeOffs":"t","rationale":"r","subject":"extra"}]}`
	_, err := ParseDecisionRecordDocument(content)
	if err == nil {
		t.Fatal("expected an error for an unrecognized field inside a decisions[] entry")
	}
	if !strings.HasPrefix(err.Error(), "decision_record_invalid_json: ") {
		t.Errorf("reason must use the decision_record_invalid_json family, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "subject") {
		t.Errorf("error must name the offending field, got %q", err.Error())
	}
}

// TestParseDecisionRecordDocument_UnknownTopLevelFieldRejected proves an
// extra field on the ENVELOPE object (sibling to "decisions"), not just
// inside an entry, is caught too.
func TestParseDecisionRecordDocument_UnknownTopLevelFieldRejected(t *testing.T) {
	content := `{"decisions":[{"decision":"d","optionsConsidered":"o","tradeOffs":"t","rationale":"r"}],"summary":"extra"}`
	_, err := ParseDecisionRecordDocument(content)
	if err == nil {
		t.Fatal("expected an error for an unrecognized top-level field")
	}
	if !strings.Contains(err.Error(), "summary") {
		t.Errorf("error must name the offending field, got %q", err.Error())
	}
}

// TestParseDecisionRecordDocument_CaseVariantFieldNameStillAccepted documents
// a boundary of the unknown-field guard rather than asserting a rejection:
// encoding/json's own field-matching falls back to a case-INSENSITIVE match
// on the struct's tag/name before DisallowUnknownFields ever gets to call
// anything "unknown", so a field spelled with different casing than the
// wire name ("tradeoffs" for "tradeOffs") is not a typo the decoder can
// tell apart from a deliberate rename - it resolves to the same field,
// correctly, with no data lost. This is Go's own behavior, not a gap this
// gate leaves open: the value the agent wrote is not silently dropped, it
// is captured under the field it was clearly meant for.
func TestParseDecisionRecordDocument_CaseVariantFieldNameStillAccepted(t *testing.T) {
	content := `{"decisions":[{"decision":"d","optionsConsidered":"o","tradeoffs":"the real trade-off","rationale":"r"}]}`
	entries, err := ParseDecisionRecordDocument(content)
	if err != nil {
		t.Fatalf("ParseDecisionRecordDocument: %v", err)
	}
	if entries[0].TradeOffs != "the real trade-off" {
		t.Errorf("TradeOffs = %q, want the value written under the differently-cased key, not dropped", entries[0].TradeOffs)
	}
}

// TestParseDecisionRecordDocument_RejectsDuplicateKeyWithinEntry is the
// review's other example, reproduced exactly: two "decision" keys in the
// same object. encoding/json.Unmarshal silently keeps only the last one -
// this is the rejected-prefix-match failure mode ("later data silently
// overrides earlier data, and the gate says nothing") reproduced one layer
// down, at the JSON layer instead of the markdown layer.
func TestParseDecisionRecordDocument_RejectsDuplicateKeyWithinEntry(t *testing.T) {
	content := `{"decisions":[{"decision":"Use SQLite","decision":"Use Postgres","optionsConsidered":"o","tradeOffs":"t","rationale":"r"}]}`
	_, err := ParseDecisionRecordDocument(content)
	if err == nil {
		t.Fatal("expected an error for a duplicated object key")
	}
	if !strings.HasPrefix(err.Error(), "decision_record_duplicate_key: ") {
		t.Errorf("reason must use the decision_record_duplicate_key family, got %q", err.Error())
	}
	for _, want := range []string{"decision", "$.decisions[0]"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error must name %q, got %q", want, err.Error())
		}
	}
}

// TestParseDecisionRecordDocument_RejectsDuplicateTopLevelKey proves the
// same guard reaches the envelope object itself, not only an entry -
// {"decisions":[...],"decisions":[...]} is caught with the root path "$".
func TestParseDecisionRecordDocument_RejectsDuplicateTopLevelKey(t *testing.T) {
	content := `{"decisions":[{"decision":"d","optionsConsidered":"o","tradeOffs":"t","rationale":"r"}],"decisions":[{"decision":"d2","optionsConsidered":"o2","tradeOffs":"t2","rationale":"r2"}]}`
	_, err := ParseDecisionRecordDocument(content)
	if err == nil {
		t.Fatal("expected an error for a duplicated top-level key")
	}
	if !strings.HasPrefix(err.Error(), "decision_record_duplicate_key: ") {
		t.Errorf("reason must use the decision_record_duplicate_key family, got %q", err.Error())
	}
	for _, want := range []string{"decisions", "$"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error must name %q, got %q", want, err.Error())
		}
	}
}

// TestParseDecisionRecordDocument_RejectsDuplicateKeyInSecondEntry proves
// the walk does not stop at the first array element - a duplicate later in
// the array must still be caught and named at its own index.
func TestParseDecisionRecordDocument_RejectsDuplicateKeyInSecondEntry(t *testing.T) {
	content := `{"decisions":[` +
		`{"decision":"d1","optionsConsidered":"o1","tradeOffs":"t1","rationale":"r1"},` +
		`{"decision":"d2","optionsConsidered":"o2","tradeOffs":"a","tradeOffs":"b","rationale":"r2"}` +
		`]}`
	_, err := ParseDecisionRecordDocument(content)
	if err == nil {
		t.Fatal("expected an error for a duplicate key in the second entry")
	}
	for _, want := range []string{"tradeOffs", "$.decisions[1]"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error must name %q, got %q", want, err.Error())
		}
	}
}

// TestParseDecisionRecordDocument_DuplicateKeyDetectionDoesNotFlagDistinctEntries
// guards against a broken implementation that tracks seen keys globally
// instead of per-object: two DIFFERENT entries each legitimately using the
// key "decision" once is not a duplicate, and a record with several
// well-formed decisions (the exact shape this whole gate exists to accept)
// must keep passing.
func TestParseDecisionRecordDocument_DuplicateKeyDetectionDoesNotFlagDistinctEntries(t *testing.T) {
	if _, err := ParseDecisionRecordDocument(threeDecisionsRecord()); err != nil {
		t.Fatalf("ParseDecisionRecordDocument: %v", err)
	}
}

// --- ParseDecisionRecordDocument: bounded entry count and field/document
// size (FIX 4). ---

// nDecisionsRecord builds a literal decision_record JSON document with n
// well-formed, distinct entries - a hand-built string, not a marshalled
// struct, for the same reason every other fixture in this file is.
func nDecisionsRecord(n int) string {
	var b strings.Builder
	b.WriteString(`{"decisions":[`)
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"decision":"decision %d","optionsConsidered":"o","tradeOffs":"t","rationale":"r"}`, i)
	}
	b.WriteString(`]}`)
	return b.String()
}

// TestParseDecisionRecordDocument_AcceptsMaxEntries proves the limit is
// inclusive - exactly maxDecisionRecordEntries must still pass, not just
// one fewer.
func TestParseDecisionRecordDocument_AcceptsMaxEntries(t *testing.T) {
	entries, err := ParseDecisionRecordDocument(nDecisionsRecord(maxDecisionRecordEntries))
	if err != nil {
		t.Fatalf("ParseDecisionRecordDocument: %v", err)
	}
	if len(entries) != maxDecisionRecordEntries {
		t.Fatalf("got %d entries, want %d", len(entries), maxDecisionRecordEntries)
	}
}

// TestParseDecisionRecordDocument_RejectsTooManyEntries is the review's
// runaway-record example: one entry over the limit must be rejected, naming
// the count and the limit, not silently accepted and left for
// WriteDecisionRecordNode / ComposeRunReport to choke on later.
func TestParseDecisionRecordDocument_RejectsTooManyEntries(t *testing.T) {
	_, err := ParseDecisionRecordDocument(nDecisionsRecord(maxDecisionRecordEntries + 1))
	if err == nil {
		t.Fatal("expected an error for a decisions array over the entry limit")
	}
	if !strings.HasPrefix(err.Error(), "decision_record_too_many_entries: ") {
		t.Errorf("reason must use the decision_record_too_many_entries family, got %q", err.Error())
	}
	want := strconv.Itoa(maxDecisionRecordEntries + 1)
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error must name the actual entry count %q, got %q", want, err.Error())
	}
}

// TestParseDecisionRecordDocument_RejectsOversizedField proves a single
// field far past any real decision's length (an agent pasting a log or a
// whole source file into one field, say) is rejected by name, not
// silently accepted into the graph and the composer's prompt.
func TestParseDecisionRecordDocument_RejectsOversizedField(t *testing.T) {
	huge := strings.Repeat("x", maxDecisionRecordFieldBytes+1)
	content := fmt.Sprintf(`{"decisions":[{"decision":%q,"optionsConsidered":"o","tradeOffs":"t","rationale":"r"}]}`, huge)
	_, err := ParseDecisionRecordDocument(content)
	if err == nil {
		t.Fatal("expected an error for a field over the byte limit")
	}
	if !strings.HasPrefix(err.Error(), "decision_record_field_too_large: ") {
		t.Errorf("reason must use the decision_record_field_too_large family, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "decisions[0].decision") {
		t.Errorf("error must name the offending field, got %q", err.Error())
	}
}

// TestParseDecisionRecordDocument_RejectsOversizedDocument proves the
// whole-file size gate fires before the document is even unmarshalled -
// checked directly against len(content), independent of the per-field
// check above.
func TestParseDecisionRecordDocument_RejectsOversizedDocument(t *testing.T) {
	content := nDecisionsRecord(1) + strings.Repeat(" ", maxDecisionRecordDocumentBytes)
	_, err := ParseDecisionRecordDocument(content)
	if err == nil {
		t.Fatal("expected an error for a document over the total byte limit")
	}
	if !strings.HasPrefix(err.Error(), "decision_record_too_large: ") {
		t.Errorf("reason must use the decision_record_too_large family, got %q", err.Error())
	}
}

// TestEvaluateExitGate_DecisionRecord_RejectsUnknownFieldEndToEnd and
// TestEvaluateExitGate_DecisionRecord_RejectsDuplicateKeyEndToEnd run FIX 1's
// two new rejections through the real gate entry point, not just the parser
// helper - the same discipline every other decision_record gate test in
// this file follows.
func TestEvaluateExitGate_DecisionRecord_RejectsUnknownFieldEndToEnd(t *testing.T) {
	worktree := t.TempDir()
	artifactDir := t.TempDir()
	path := filepath.Join(artifactDir, "decision-record.json")
	content := `{"decisions":[{"titel":"t","decision":"d","optionsConsidered":"o","tradeOffs":"t","rationale":"r"}]}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	wf := decisionRecordGateWorkflow()
	ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "implementation", WorktreePath: worktree, ArtifactDir: artifactDir, BeadID: "kb-1"})
	if ok {
		t.Fatal("a record with an unrecognized field must not satisfy the decision_record gate")
	}
	if !strings.HasPrefix(reason, "decision_record_invalid_json: ") {
		t.Errorf("reason must use the decision_record_invalid_json family, got %q", reason)
	}
}

func TestEvaluateExitGate_DecisionRecord_RejectsDuplicateKeyEndToEnd(t *testing.T) {
	worktree := t.TempDir()
	artifactDir := t.TempDir()
	path := filepath.Join(artifactDir, "decision-record.json")
	content := `{"decisions":[{"decision":"a","decision":"b","optionsConsidered":"o","tradeOffs":"t","rationale":"r"}]}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	wf := decisionRecordGateWorkflow()
	ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "implementation", WorktreePath: worktree, ArtifactDir: artifactDir, BeadID: "kb-1"})
	if ok {
		t.Fatal("a record with a duplicated object key must not satisfy the decision_record gate")
	}
	if !strings.HasPrefix(reason, "decision_record_duplicate_key: ") {
		t.Errorf("reason must use the decision_record_duplicate_key family, got %q", reason)
	}
}
