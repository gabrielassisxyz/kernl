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
	if !strings.Contains(out, "revert/fix-up rate is not reported") {
		t.Errorf("table must carry the quality-column disclaimer, got: %q", out)
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
