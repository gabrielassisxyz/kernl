package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/app"
)

// orchestratorTestHome points app.DefaultStateDir() (HOME/.kernl) at a fresh
// temp directory, the same seam TestEpicMergeAndEpicAbortAgreeOnStateDir uses
// - runOrchestratorStats resolves its own StateDir this way rather than
// taking one as a parameter, so a hermetic test controls it through HOME.
func orchestratorTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func writeOrchestratorFixture(t *testing.T, home, epic string, in app.StageAttemptInput) {
	t.Helper()
	if in.DiffStats == nil {
		in.DiffStats = fixedDiffStatter{}
	}
	rec := app.BuildStageAttemptRecord(in)
	if err := app.AppendStageAttempt(home+"/.kernl", epic, rec); err != nil {
		t.Fatalf("writeOrchestratorFixture: %v", err)
	}
}

type fixedDiffStatter struct{ added, removed *int }

func (f fixedDiffStatter) DiffStat(worktree, baseSHA, commitSHA string) (added, removed *int) {
	return f.added, f.removed
}

func TestOrchestratorStatsNoLedgerPrintsMessageNotError(t *testing.T) {
	orchestratorTestHome(t)

	var buf bytes.Buffer
	if err := runOrchestratorStats(&buf, nil); err != nil {
		t.Fatalf("expected no error with no ledger, got: %v", err)
	}
	if !strings.Contains(buf.String(), "no attempts recorded yet") {
		t.Errorf("expected the no-attempts message, got: %q", buf.String())
	}
}

func TestOrchestratorStatsNoLedgerJSON(t *testing.T) {
	orchestratorTestHome(t)

	var buf bytes.Buffer
	if err := runOrchestratorStats(&buf, []string{"--json"}); err != nil {
		t.Fatalf("runOrchestratorStats --json: %v", err)
	}
	var out orchestratorStatsOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("not valid JSON: %v (%s)", err, buf.String())
	}
	if out.Message == "" {
		t.Error("expected a message explaining no attempts were recorded")
	}
	if out.Agents == nil {
		t.Error("agents must be an empty array, not null/omitted")
	}
}

func TestOrchestratorStatsAggregatesAndFormatsTable(t *testing.T) {
	home := orchestratorTestHome(t)
	now := time.Now()

	writeOrchestratorFixture(t, home, "epic-a", app.StageAttemptInput{
		AgentID: "claude", BeadID: "bead-1", Stage: "implementation",
		StartedAt: now, Duration: 30 * time.Second, GatePassed: true,
	})
	writeOrchestratorFixture(t, home, "epic-a", app.StageAttemptInput{
		AgentID: "claude", BeadID: "bead-2", Stage: "implementation",
		StartedAt: now, Duration: 50 * time.Second, GatePassed: false,
	})

	var buf bytes.Buffer
	if err := runOrchestratorStats(&buf, nil); err != nil {
		t.Fatalf("runOrchestratorStats: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "claude") {
		t.Errorf("table must list the claude agent, got: %q", out)
	}
	if !strings.Contains(out, "revert rate is not part of this report") {
		t.Errorf("table must carry the quality-column disclaimer, got: %q", out)
	}
}

// TestQualityColumnNoteDoesNotReferToPosition guards against the note text
// regressing to a phrasing like "the numbers below": the JSON surface has no
// "below" for such a phrase to point at, so the note must name what it
// qualifies instead of relying on where it happens to be printed.
func TestQualityColumnNoteDoesNotReferToPosition(t *testing.T) {
	lower := strings.ToLower(qualityColumnNote)
	for _, positional := range []string{"below", "above"} {
		if strings.Contains(lower, positional) {
			t.Errorf("qualityColumnNote must not rely on print position, found %q in: %s", positional, qualityColumnNote)
		}
	}
}

// TestOrchestratorStatsNoteMatchesInTableAndJSON asserts the same note text
// - byte for byte - reaches both surfaces: the human table (printed as
// plain text) and --json (carried as the qualityColumnNote field). A note
// that reads correctly in one surface but drifts in the other is exactly
// the defect this bead fixes.
func TestOrchestratorStatsNoteMatchesInTableAndJSON(t *testing.T) {
	home := orchestratorTestHome(t)
	now := time.Now()
	writeOrchestratorFixture(t, home, "epic-a", app.StageAttemptInput{
		AgentID: "claude", BeadID: "bead-1", Stage: "implementation",
		StartedAt: now, Duration: 30 * time.Second, GatePassed: true,
	})

	var tableBuf bytes.Buffer
	if err := runOrchestratorStats(&tableBuf, nil); err != nil {
		t.Fatalf("runOrchestratorStats: %v", err)
	}
	if !strings.Contains(tableBuf.String(), qualityColumnNote) {
		t.Errorf("human table must carry qualityColumnNote verbatim, got: %q", tableBuf.String())
	}

	var jsonBuf bytes.Buffer
	if err := runOrchestratorStats(&jsonBuf, []string{"--json"}); err != nil {
		t.Fatalf("runOrchestratorStats --json: %v", err)
	}
	var out orchestratorStatsOutput
	if err := json.Unmarshal(jsonBuf.Bytes(), &out); err != nil {
		t.Fatalf("not valid JSON: %v (%s)", err, jsonBuf.String())
	}
	if out.Note != qualityColumnNote {
		t.Errorf("--json qualityColumnNote field must match the table's note, got: %q", out.Note)
	}
}

func TestOrchestratorStatsStageFlagFilters(t *testing.T) {
	home := orchestratorTestHome(t)
	now := time.Now()

	writeOrchestratorFixture(t, home, "epic-a", app.StageAttemptInput{
		AgentID: "claude", BeadID: "bead-1", Stage: "implementation", StartedAt: now, GatePassed: true,
	})
	writeOrchestratorFixture(t, home, "epic-a", app.StageAttemptInput{
		AgentID: "codex", BeadID: "bead-1", Stage: "review", StartedAt: now, GatePassed: true,
	})

	var buf bytes.Buffer
	if err := runOrchestratorStats(&buf, []string{"--stage", "implementation", "--json"}); err != nil {
		t.Fatalf("runOrchestratorStats: %v", err)
	}
	var out orchestratorStatsOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	if len(out.Agents) != 1 || out.Agents[0].AgentID != "claude" {
		t.Errorf("--stage implementation must exclude the review-stage row, got: %+v", out.Agents)
	}
}

func TestOrchestratorStatsSinceAcceptsGoDurationAndDayShorthand(t *testing.T) {
	home := orchestratorTestHome(t)
	now := time.Now()

	writeOrchestratorFixture(t, home, "epic-a", app.StageAttemptInput{
		AgentID: "old-agent", BeadID: "bead-old", Stage: "implementation",
		StartedAt: now.Add(-60 * 24 * time.Hour), GatePassed: true,
	})
	writeOrchestratorFixture(t, home, "epic-a", app.StageAttemptInput{
		AgentID: "new-agent", BeadID: "bead-new", Stage: "implementation",
		StartedAt: now.Add(-1 * time.Hour), GatePassed: true,
	})

	for _, since := range []string{"720h", "30d"} { // 720h == 30d
		t.Run(since, func(t *testing.T) {
			var buf bytes.Buffer
			if err := runOrchestratorStats(&buf, []string{"--since", since, "--json"}); err != nil {
				t.Fatalf("runOrchestratorStats --since %s: %v", since, err)
			}
			var out orchestratorStatsOutput
			if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
				t.Fatalf("not valid JSON: %v", err)
			}
			if len(out.Agents) != 1 || out.Agents[0].AgentID != "new-agent" {
				t.Errorf("--since %s must keep only the recent row, got: %+v", since, out.Agents)
			}
		})
	}
}

func TestOrchestratorStatsRejectsMalformedSince(t *testing.T) {
	orchestratorTestHome(t)

	var buf bytes.Buffer
	err := runOrchestratorStats(&buf, []string{"--since", "not-a-duration"})
	if err == nil {
		t.Fatal("expected a malformed --since value to be refused")
	}
	if exitCode(err) != 2 {
		t.Errorf("a bad --since must be a usage error (exit 2), got exit %d: %v", exitCode(err), err)
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("error must carry the marker, got: %v", err)
	}
}

func TestOrchestratorStatsRejectsNonFiniteAndNegativeDayShorthand(t *testing.T) {
	orchestratorTestHome(t)

	for _, since := range []string{"NaNd", "Infd", "-Infd", "-30d"} {
		t.Run(since, func(t *testing.T) {
			var buf bytes.Buffer
			err := runOrchestratorStats(&buf, []string{"--since", since})
			if err == nil {
				t.Fatalf("expected --since %q to be refused", since)
			}
			if exitCode(err) != 2 {
				t.Errorf("--since %q must be a usage error (exit 2), got exit %d: %v", since, exitCode(err), err)
			}
		})
	}
}

func TestOrchestratorStatsUnknownFlagRejected(t *testing.T) {
	orchestratorTestHome(t)

	var buf bytes.Buffer
	err := runOrchestratorStats(&buf, []string{"--bogus"})
	if err == nil || exitCode(err) != 2 {
		t.Fatalf("expected a usage error for an unknown flag, got: %v", err)
	}
}

func TestOrchestratorStatsRejectsPositionalArgs(t *testing.T) {
	orchestratorTestHome(t)

	var buf bytes.Buffer
	err := runOrchestratorStats(&buf, []string{"extra"})
	if err == nil || exitCode(err) != 2 {
		t.Fatalf("expected a usage error for a stray positional argument, got: %v", err)
	}
}

func TestOrchestratorStatsUnmeasuredGroupPrintsDashNotZero(t *testing.T) {
	home := orchestratorTestHome(t)
	now := time.Now()

	// No DiffStats configured beyond the fixedDiffStatter zero value, so
	// DiffLinesAdded/Removed stay nil - nothing to compute a median diff
	// size from.
	writeOrchestratorFixture(t, home, "epic-a", app.StageAttemptInput{
		AgentID: "claude", BeadID: "bead-1", Stage: "implementation", StartedAt: now, GatePassed: true,
	})

	var buf bytes.Buffer
	if err := runOrchestratorStats(&buf, nil); err != nil {
		t.Fatalf("runOrchestratorStats: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "-  (n=0)") {
		t.Errorf("an unmeasured median must render as \"-\", not 0 - got: %q", out)
	}
	if strings.Contains(out, "0.0  (n=0)") {
		t.Errorf("an unmeasured median must never render as a numeric zero, got: %q", out)
	}
}

func TestOrchestratorStatsSurfacesDanglingLedgerInTableAndJSON(t *testing.T) {
	home := orchestratorTestHome(t)
	now := time.Now()

	writeOrchestratorFixture(t, home, "epic-a", app.StageAttemptInput{
		AgentID: "claude", BeadID: "bead-1", Stage: "implementation", StartedAt: now, GatePassed: true,
	})
	ledgerPath := home + "/.kernl/run/epic-a/attempts.jsonl"
	f, err := os.OpenFile(ledgerPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"agentId":"claude","beadId":"bead-2"}`); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	var tableBuf bytes.Buffer
	if err := runOrchestratorStats(&tableBuf, nil); err != nil {
		t.Fatalf("runOrchestratorStats: %v", err)
	}
	if !strings.Contains(tableBuf.String(), "incomplete trailing row") {
		t.Errorf("table output must warn about the dangling ledger, got: %q", tableBuf.String())
	}
	if !strings.Contains(tableBuf.String(), ledgerPath) {
		t.Errorf("table warning must name the ledger file, got: %q", tableBuf.String())
	}

	var jsonBuf bytes.Buffer
	if err := runOrchestratorStats(&jsonBuf, []string{"--json"}); err != nil {
		t.Fatalf("runOrchestratorStats --json: %v", err)
	}
	var out orchestratorStatsOutput
	if err := json.Unmarshal(jsonBuf.Bytes(), &out); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	if len(out.DanglingLedgers) != 1 || out.DanglingLedgers[0] != ledgerPath {
		t.Errorf("expected danglingLedgers to name %q, got %v", ledgerPath, out.DanglingLedgers)
	}
}

func TestOrchestratorStatsUnreadableRunDirIsAnErrorNotEmptyResult(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	home := orchestratorTestHome(t)
	now := time.Now()

	writeOrchestratorFixture(t, home, "epic-a", app.StageAttemptInput{
		AgentID: "claude", BeadID: "bead-1", Stage: "implementation", StartedAt: now, GatePassed: true,
	})
	runDir := home + "/.kernl/run"
	if err := os.Chmod(runDir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(runDir, 0o755); err != nil {
			t.Errorf("restoring run dir permissions: %v", err)
		}
	})

	var buf bytes.Buffer
	err := runOrchestratorStats(&buf, nil)
	if err == nil {
		t.Fatal("expected an unreadable run directory to fail loudly")
	}
	if strings.Contains(buf.String(), "no attempts recorded yet") {
		t.Errorf("an unreadable run directory must not be reported as \"nothing recorded\", got: %q", buf.String())
	}
}

func TestOrchestratorDispatchesToStats(t *testing.T) {
	orchestratorTestHome(t)

	var buf bytes.Buffer
	if err := runOrchestrator(&buf, []string{"stats"}); err != nil {
		t.Fatalf("runOrchestrator stats: %v", err)
	}
	if !strings.Contains(buf.String(), "no attempts recorded yet") {
		t.Errorf("expected the stats subcommand to run, got: %q", buf.String())
	}
}

func TestOrchestratorRequiresSubcommand(t *testing.T) {
	var buf bytes.Buffer
	err := runOrchestrator(&buf, nil)
	if err == nil || exitCode(err) != 2 {
		t.Fatalf("expected a usage error with no subcommand, got: %v", err)
	}
}

func TestOrchestratorCommandIsInDispatchTable(t *testing.T) {
	if findCommand(commandTable, "orchestrator") == nil {
		t.Fatal(`"orchestrator" is missing from commandTable - help/capabilities/robot-docs would not see it`)
	}
}

// --- rework table ---

func TestOrchestratorStatsReworkTableChargesTheAgentThatRedidTheWork(t *testing.T) {
	home := orchestratorTestHome(t)
	now := time.Now()
	verdict := "REJECT"

	writeOrchestratorFixture(t, home, "epic-a", app.StageAttemptInput{
		AgentID: "claude", BeadID: "bead-1", Stage: "implementation",
		StartedAt: now, Duration: 10 * time.Second, GatePassed: true,
	})
	writeOrchestratorFixture(t, home, "epic-a", app.StageAttemptInput{
		AgentID: "codex", BeadID: "bead-1", Stage: "implementation_review",
		StartedAt: now.Add(time.Minute), Duration: time.Second, GatePassed: false,
		GateFailureReason: "verdict_reject: /artifacts/implementation-review.md",
		ReviewVerdict:     &verdict,
	})
	writeOrchestratorFixture(t, home, "epic-a", app.StageAttemptInput{
		AgentID: "claude", BeadID: "bead-1", Stage: "implementation",
		StartedAt: now.Add(2 * time.Minute), Duration: 30 * time.Second, GatePassed: true,
	})

	var buf bytes.Buffer
	if err := runOrchestratorStats(&buf, nil); err != nil {
		t.Fatalf("runOrchestratorStats: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "REWORK") {
		t.Errorf("the rework table must be printed when there is rework, got: %q", out)
	}
	if !strings.Contains(out, "30s") {
		t.Errorf("the rework row must carry the time the redo cost, got: %q", out)
	}
	// codex only rejected; it redid nothing, so it has no row here even
	// though it appears in the table above.
	reworkSection := out[strings.Index(out, "REWORK"):]
	if strings.Contains(reworkSection, "codex") {
		t.Errorf("an agent that only reviewed must not appear in the rework table, got: %q", reworkSection)
	}
}

// A window with no rework says so. An empty table would look identical to a
// window with no attempts at all, and only one of the two is good news.
func TestOrchestratorStatsSaysWhenThereWasNoRework(t *testing.T) {
	home := orchestratorTestHome(t)

	writeOrchestratorFixture(t, home, "epic-a", app.StageAttemptInput{
		AgentID: "claude", BeadID: "bead-1", Stage: "implementation",
		StartedAt: time.Now(), Duration: time.Second, GatePassed: true,
	})

	var buf bytes.Buffer
	if err := runOrchestratorStats(&buf, nil); err != nil {
		t.Fatalf("runOrchestratorStats: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "no attempt in this window redid work a review rejected") {
		t.Errorf("a clean window must say there was no rework, got: %q", out)
	}
	if strings.Contains(out, "REWORK\t") {
		t.Errorf("an empty rework table must not be printed at all, got: %q", out)
	}
}

func TestOrchestratorStatsReworkReachesJSON(t *testing.T) {
	home := orchestratorTestHome(t)
	now := time.Now()
	verdict := "REJECT"

	writeOrchestratorFixture(t, home, "epic-a", app.StageAttemptInput{
		AgentID: "claude", BeadID: "bead-1", Stage: "implementation",
		StartedAt: now, Duration: time.Second, GatePassed: true,
	})
	writeOrchestratorFixture(t, home, "epic-a", app.StageAttemptInput{
		AgentID: "claude", BeadID: "bead-1", Stage: "implementation_review",
		StartedAt: now.Add(time.Minute), Duration: time.Second, GatePassed: false,
		GateFailureReason: "verdict_reject: /artifacts/implementation-review.md",
		ReviewVerdict:     &verdict,
	})
	writeOrchestratorFixture(t, home, "epic-a", app.StageAttemptInput{
		AgentID: "claude", BeadID: "bead-1", Stage: "implementation",
		StartedAt: now.Add(2 * time.Minute), Duration: time.Second, GatePassed: true,
	})

	var buf bytes.Buffer
	if err := runOrchestratorStats(&buf, []string{"--json"}); err != nil {
		t.Fatalf("runOrchestratorStats --json: %v", err)
	}
	var out orchestratorStatsOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Agents) != 1 {
		t.Fatalf("expected one agent, got %+v", out.Agents)
	}
	if out.Agents[0].ReworkAttempts != 1 {
		t.Errorf("reworkAttempts = %d, want 1", out.Agents[0].ReworkAttempts)
	}
	if out.Agents[0].ReworkCostUSD != nil {
		t.Errorf("reworkCostUSD must be null when no dialect reported one, got %v", *out.Agents[0].ReworkCostUSD)
	}
	if !strings.Contains(buf.String(), "\"reworkRate\"") {
		t.Errorf("the JSON surface must carry reworkRate, got: %q", buf.String())
	}
}

// --- backfill-rework ---

// writeRawOrchestratorLedger plants rows the current writer cannot produce:
// the ones an older build wrote while causedBy matched the wrong gate reason.
func writeRawOrchestratorLedger(t *testing.T, home, epic string, lines ...string) string {
	t.Helper()
	dir := home + "/.kernl/run/" + epic
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := dir + "/attempts.jsonl"
	body := ""
	for _, l := range lines {
		body += l + "\n"
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

const (
	legacyImpl     = `{"agentId":"claude","beadId":"b1","stage":"implementation","gatePassed":true,"causedBy":null}`
	legacyRejected = `{"agentId":"codex","beadId":"b1","stage":"implementation_review","gatePassed":false,"gateFailureReason":"verdict_reject: /artifacts/implementation-review.md","causedBy":null}`
	legacyRedo     = `{"agentId":"claude","beadId":"b1","stage":"implementation","gatePassed":true,"causedBy":null}`
)

func TestOrchestratorBackfillReworkIsADryRunByDefault(t *testing.T) {
	home := orchestratorTestHome(t)
	path := writeRawOrchestratorLedger(t, home, "epic-a", legacyImpl, legacyRejected, legacyRedo)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := runOrchestrator(&buf, []string{"backfill-rework"}); err != nil {
		t.Fatalf("runOrchestrator backfill-rework: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "nothing was written") {
		t.Errorf("a run with no --yes must say it wrote nothing, got: %q", out)
	}
	if !strings.Contains(out, "1 row(s) to mark as rework") {
		t.Errorf("the dry run must report the work, got: %q", out)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("a default run must not rewrite the ledger")
	}
}

func TestOrchestratorBackfillReworkAppliesWithYes(t *testing.T) {
	home := orchestratorTestHome(t)
	path := writeRawOrchestratorLedger(t, home, "epic-a", legacyImpl, legacyRejected, legacyRedo)

	var buf bytes.Buffer
	if err := runOrchestrator(&buf, []string{"backfill-rework", "--yes"}); err != nil {
		t.Fatalf("runOrchestrator backfill-rework --yes: %v", err)
	}
	if strings.Contains(buf.String(), "nothing was written") {
		t.Errorf("a confirmed run must not claim it wrote nothing, got: %q", buf.String())
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"causedBy":"/artifacts/implementation-review.md"`) {
		t.Errorf("the redo must now name the rejecting artifact, got: %s", data)
	}

	// And the numbers it feeds must move with it.
	var statsBuf bytes.Buffer
	if err := runOrchestratorStats(&statsBuf, []string{"--json"}); err != nil {
		t.Fatalf("runOrchestratorStats: %v", err)
	}
	var stats orchestratorStatsOutput
	if err := json.Unmarshal(statsBuf.Bytes(), &stats); err != nil {
		t.Fatal(err)
	}
	for _, a := range stats.Agents {
		if a.AgentID == "claude" && a.ReworkAttempts != 1 {
			t.Errorf("after the backfill, claude must show 1 rework attempt, got %d", a.ReworkAttempts)
		}
	}
}

// --dry-run and --yes ask for opposite things. Choosing one would mean
// guessing, on a command that rewrites the only record of what ran.
func TestOrchestratorBackfillReworkRefusesDryRunWithYes(t *testing.T) {
	orchestratorTestHome(t)

	var buf bytes.Buffer
	err := runOrchestrator(&buf, []string{"backfill-rework", "--dry-run", "--yes"})
	if err == nil {
		t.Fatal("expected --dry-run --yes together to be refused")
	}
	if exitCode(err) != 2 {
		t.Errorf("a contradictory flag pair must be a usage error (exit 2), got exit %d: %v", exitCode(err), err)
	}
}

func TestOrchestratorBackfillReworkCleanLedgerSaysNothingToDo(t *testing.T) {
	home := orchestratorTestHome(t)
	writeRawOrchestratorLedger(t, home, "epic-a", legacyImpl)

	var buf bytes.Buffer
	if err := runOrchestrator(&buf, []string{"backfill-rework"}); err != nil {
		t.Fatalf("runOrchestrator backfill-rework: %v", err)
	}
	if !strings.Contains(buf.String(), "nothing to do") {
		t.Errorf("a ledger already consistent must say so, got: %q", buf.String())
	}
}

func TestOrchestratorBackfillReworkJSON(t *testing.T) {
	home := orchestratorTestHome(t)
	writeRawOrchestratorLedger(t, home, "epic-a", legacyImpl, legacyRejected, legacyRedo)

	var buf bytes.Buffer
	if err := runOrchestrator(&buf, []string{"backfill-rework", "--json"}); err != nil {
		t.Fatalf("runOrchestrator backfill-rework --json: %v", err)
	}
	var out orchestratorBackfillOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("not valid JSON: %v (%s)", err, buf.String())
	}
	if out.Applied {
		t.Error("applied must be false without --yes")
	}
	if out.RowsScanned != 3 || len(out.Ledgers) != 1 || out.Ledgers[0].Marked != 1 {
		t.Errorf("unexpected document: %+v", out)
	}
}
