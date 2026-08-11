package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/session"
)

// measuredRateLimitEnvelope is the provider payload behind six of the ten
// commit_marker_missing rows in the ledger, copied verbatim from the pi
// transcripts (see internal/session/turn_failure_test.go for the full
// inventory).
const measuredRateLimitEnvelope = `429: {"message":"litellm.RateLimitError: Model rate limit exceeded. RPM limit=15, current usage=23. Received Model Group=kimi-k2.7\nAvailable Model Group Fallbacks=None","type":"throttling_error","param":null,"code":"429"}`

func piErrorScript(t *testing.T, envelope string) string {
	t.Helper()
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("encoding the fixture: %v", err)
	}
	return `{"type":"session","version":3,"id":"019fc4ff-74ee-79d1-910b-734b253f6668"}` + "\n" +
		`{"type":"message","id":"m1","message":{"role":"assistant","stopReason":"error","errorMessage":` + string(encoded) + `}}` + "\n" +
		`{"type":"agent_settled"}` + "\n"
}

// TestRunBeadCarriesTheClassifiedFailureOnTheErrorReturn is the production
// path all six measured rate limits took: pi has no follow-up channel, so a
// failed turn ends the dispatch through the follow-up refusal, which returns
// an error. A classification attached only to the ordinary return would be
// missing on exactly the failures a retry policy exists for.
func TestRunBeadCarriesTheClassifiedFailureOnTheErrorReturn(t *testing.T) {
	be := &fakeBackend{state: map[string]string{"kb-1": "implementation"}}
	spawn := &fakeSpawner{script: piErrorScript(t, measuredRateLimitEnvelope)}
	d := NewSessionDriver(DriverDeps{Backend: be, Spawn: spawn.Spawn, SCM: newTestSCM(), LogDir: t.TempDir()})

	res, err := d.RunBead(context.Background(), RunBeadInput{
		BeadID: "kb-1", RepoPath: t.TempDir(), Command: "pi", AgentName: "pi-kimi",
	})
	if err == nil {
		t.Fatal("precondition: a pi turn that failed with no follow-up channel must return an error")
	}
	if res.Failure == nil {
		t.Fatal("the result must carry the failure even when RunBead also returns an error")
	}
	if res.Failure.Kind != session.FailureRateLimited {
		t.Errorf("Kind = %q, want the measured 429 recognised", res.Failure.Kind)
	}
	if res.Failure.Raw != measuredRateLimitEnvelope {
		t.Errorf("Raw = %q, want the provider's message intact", res.Failure.Raw)
	}
}

// TestRunBeadLeavesAnUnmeasuredDialectUnclassified is the other half: the
// failure still travels, and still carries what happened, without a kind
// nobody has evidence for.
func TestRunBeadLeavesAnUnmeasuredDialectUnclassified(t *testing.T) {
	be := &fakeBackend{state: map[string]string{"kb-1": "implementation"}}
	spawn := &fakeSpawner{script: `{"type":"turn.failed","error":{"message":"429 rate limit","code":"429"}}` + "\n"}
	d := NewSessionDriver(DriverDeps{Backend: be, Spawn: spawn.Spawn, SCM: newTestSCM(), LogDir: t.TempDir()})

	res, _ := d.RunBead(context.Background(), RunBeadInput{
		BeadID: "kb-1", RepoPath: t.TempDir(), Command: "codex", AgentName: "codex-max",
	})
	if res.Success {
		t.Fatal("precondition: a codex turn.failed must not report success")
	}
	if res.Failure == nil || res.Failure.Kind != session.FailureUnknown {
		t.Fatalf("got %+v, want an unclassified failure for a dialect with no measured envelope", res.Failure)
	}
	if !strings.Contains(res.Failure.Raw, "429 rate limit") {
		t.Errorf("Raw = %q, want the message preserved", res.Failure.Raw)
	}
}

// TestRunBeadReportsNoFailureForACleanDispatch keeps the field from becoming
// decoration: a dispatch that ended well has nothing to classify, and a
// stale failure from a turn the agent recovered from must not be reported as
// this dispatch's outcome.
func TestRunBeadReportsNoFailureForACleanDispatch(t *testing.T) {
	be := &fakeBackend{state: map[string]string{"kb-1": "implementation"}}
	spawn := &fakeSpawner{script: `{"type":"session","version":3,"id":"s1"}` + "\n" +
		`{"type":"message","id":"m1","message":{"role":"assistant","stopReason":"error","errorMessage":` +
		`"Connection error."}}` + "\n" +
		`{"type":"message","id":"m2","message":{"role":"assistant","stopReason":"stop"}}` + "\n" +
		`{"type":"agent_settled"}` + "\n"}
	d := NewSessionDriver(DriverDeps{Backend: be, Spawn: spawn.Spawn, SCM: newTestSCM(), LogDir: t.TempDir()})

	res, err := d.RunBead(context.Background(), RunBeadInput{
		BeadID: "kb-1", RepoPath: t.TempDir(), Command: "pi", AgentName: "pi-kimi",
	})
	if err != nil {
		t.Fatalf("a recovered dispatch must not fail: %v", err)
	}
	if res.Failure != nil {
		t.Fatalf("got %+v, want no failure reported for a dispatch that recovered", res.Failure)
	}
}

// TestStageAttemptRecordCarriesTheFailureBesideTheGateReason pins the ledger
// contract: the two fields answer different questions and neither replaces
// the other.
func TestStageAttemptRecordCarriesTheFailureBesideTheGateReason(t *testing.T) {
	failure := session.ClassifyPiTurnFailure(measuredRateLimitEnvelope)
	rec := BuildStageAttemptRecord(StageAttemptInput{
		BeadID: "kb-1", Stage: "implementation", AgentID: "pi-kimi", Dialect: "pi",
		GatePassed: false, GateFailureReason: "pi agent_error: " + measuredRateLimitEnvelope,
		Failure: failure,
	})
	if rec.Failure == nil || rec.Failure.Kind != session.FailureRateLimited {
		t.Fatalf("Failure = %+v, want the classification persisted", rec.Failure)
	}
	if rec.GateFailureReason == nil || !strings.Contains(*rec.GateFailureReason, "RateLimitError") {
		t.Errorf("GateFailureReason = %v, want it unchanged alongside the classification", rec.GateFailureReason)
	}

	encoded, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(encoded), `"failure":{"kind":"rate_limited"`) {
		t.Errorf("the row must carry a camelCase failure object; got %s", encoded)
	}
}

// TestStageAttemptRecordWithoutAFailureOmitsIt covers the rows written before
// this field existed: they decode with no failure rather than with an empty
// one, and a row for a gate failure - which is not a turn failure at all -
// writes none.
func TestStageAttemptRecordWithoutAFailureOmitsIt(t *testing.T) {
	rec := BuildStageAttemptRecord(StageAttemptInput{
		BeadID: "kb-1", Stage: "implementation", GatePassed: false,
		GateFailureReason: "commit_marker_missing: implementation",
	})
	encoded, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(encoded), `"failure"`) {
		t.Errorf("a gate failure must not fabricate a turn failure; got %s", encoded)
	}

	var back StageAttemptRecord
	if err := json.Unmarshal(encoded, &back); err != nil {
		t.Fatalf("a row with no failure must still decode: %v", err)
	}
	if back.Failure != nil {
		t.Errorf("Failure = %+v, want nil", back.Failure)
	}
}
