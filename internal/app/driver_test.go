package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/adapter"
	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/session"
)

// TestExitCodeFromErr_DistinguishesCleanSignalAndNoProcess proves finding
// 3's fix: exitCodeFromErr never fabricates a code. A nil error means a
// real, observed clean exit (0). Any other error that is not the child
// process's own *exec.ExitError means no exit status was ever observed at
// all, and must report that as nil, not a made-up 1.
func TestExitCodeFromErr_DistinguishesCleanSignalAndNoProcess(t *testing.T) {
	if got := exitCodeFromErr(nil); got == nil || *got != 0 {
		t.Errorf("nil error -> ExitCode = %v, want 0", got)
	}

	// A non-*exec.ExitError failure (e.g. proc.Wait() itself erroring)
	// means no exit status was ever observed.
	if got := exitCodeFromErr(errors.New("wait: some other failure")); got != nil {
		t.Errorf("non-ExitError -> ExitCode = %v, want nil (no process exit observed)", *got)
	}
}

type fakeBackend struct {
	mu    sync.Mutex
	state map[string]string
}

func (b *fakeBackend) ListWorkflows(repoPath string) ([]backend.WorkflowDescriptor, error) {
	return nil, nil
}
func (b *fakeBackend) List(filters *backend.BeadListFilters, repoPath string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *fakeBackend) ListReady(filters *backend.BeadListFilters, repoPath string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *fakeBackend) Get(id string, repoPath string) (*backend.Bead, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	s, ok := b.state[id]
	if !ok {
		return nil, nil
	}
	return &backend.Bead{ID: id, State: s}, nil
}
func (b *fakeBackend) Create(input backend.CreateBeadInput, repoPath string) (*backend.Bead, error) {
	return nil, nil
}
func (b *fakeBackend) Update(id string, input backend.UpdateBeadInput, repoPath string) error {
	return nil
}
func (b *fakeBackend) Delete(id string, repoPath string) error { return nil }
func (b *fakeBackend) Close(id string, reason string, repoPath string) (*backend.TerminalState, error) {
	return nil, nil
}
func (b *fakeBackend) MarkTerminal(id string, targetState string, reason string, repoPath string) error {
	return nil
}
func (b *fakeBackend) Reopen(id string, reason string, repoPath string) error { return nil }
func (b *fakeBackend) Rewind(id string, targetState string, reason string, repoPath string) error {
	return nil
}
func (b *fakeBackend) Search(query string, filters *backend.BeadListFilters, repoPath string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *fakeBackend) Query(expression string, options *backend.BeadQueryOptions, repoPath string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *fakeBackend) AddDependency(blockerID string, blockedID string, repoPath string) error {
	return nil
}
func (b *fakeBackend) RemoveDependency(blockerID string, blockedID string, repoPath string) error {
	return nil
}
func (b *fakeBackend) ListDependencies(id string, repoPath string, options *backend.DependencyListOptions) ([]backend.BeadDependency, error) {
	return nil, nil
}
func (b *fakeBackend) BuildTakePrompt(beadID string, options *backend.TakePromptOptions, repoPath string) (*backend.TakePromptResult, error) {
	return nil, nil
}
func (b *fakeBackend) BuildPollPrompt(options *backend.PollPromptOptions, repoPath string) (*backend.PollPromptResult, error) {
	return nil, nil
}
func (b *fakeBackend) Comment(id string, body string, repoPath string) error { return nil }
func (b *fakeBackend) Capabilities() backend.BackendCapabilities {
	return backend.BackendCapabilities{}
}

type fakeProcess struct {
	exitErr  error
	onExit   func()
	exitOnce sync.Once
}

func (p *fakeProcess) Wait() error {
	p.exitOnce.Do(func() {
		if p.onExit != nil {
			p.onExit()
		}
	})
	return p.exitErr
}

func (p *fakeProcess) Kill() error { return nil }

type fakeSpawner struct {
	script  string
	onExit  func()
	spawned bool
}

func (s *fakeSpawner) Spawn(ctx context.Context, cmd string, args []string, cwd string, env []string) (Process, io.Reader, io.Reader, error) {
	s.spawned = true
	return &fakeProcess{onExit: s.onExit}, strings.NewReader(s.script), strings.NewReader(""), nil
}

type stubProvider struct{}

func (p *stubProvider) GetSessionEntry(id string) (session.SessionInfo, bool) {
	return session.SessionInfo{}, false
}
func (p *stubProvider) ListSessionIDs() []session.SessionInfo          { return nil }
func (p *stubProvider) PushEvent(id string, evt session.TerminalEvent) {}

func newTestSCM() *session.SessionConnectionManager {
	return session.NewSessionConnectionManager(&stubProvider{}, nil)
}

// A follow-up resumes the same conversation and must resume it under the same
// allowlist. The nudge path used to re-derive one with no stage, which drops
// the stage contract's deny rules, so the record has to carry the path the
// dispatch actually ran under.
func TestDriverRecordsTheAllowlistTheNudgeMustReuse(t *testing.T) {
	be := &fakeBackend{state: map[string]string{"kb-1": "ready_for_implementation"}}
	spawn := &fakeSpawner{
		script: "{\"type\":\"session_idle\"}\n",
		onExit: func() { be.state["kb-1"] = "done" },
	}
	nudges := session.NewNudgeRegistry()
	d := NewSessionDriver(DriverDeps{Backend: be, Spawn: spawn.Spawn, SCM: newTestSCM(), NudgeRegistry: nudges, LogDir: t.TempDir()})

	stageAllowlist := filepath.Join(t.TempDir(), "opencode-kb-1-implementation.json")
	if _, err := d.RunBead(context.Background(), RunBeadInput{
		BeadID: "kb-1", RepoPath: t.TempDir(), Command: "opencode", AgentName: "opencode",
		Env: map[string]string{"OPENCODE_CONFIG": stageAllowlist},
	}); err != nil {
		t.Fatalf("RunBead: %v", err)
	}

	rec, ok := nudges.Get("kb-1-opencode")
	if !ok {
		t.Fatal("driver must register the spawn context for the nudge path")
	}
	if rec.OpencodeConfigPath != stageAllowlist {
		t.Errorf("OpencodeConfigPath = %q, want the allowlist the dispatch ran under %q", rec.OpencodeConfigPath, stageAllowlist)
	}
}

// TestSessionPumpRefusesFollowUpForOneShotClaude is a narrow unit test on
// sessionPump.handleTurnEnded in isolation: a SessionRuntime built the way
// RunBead always builds one - dialect resolved from the command,
// interactive=false because RunBead only ever dispatches one-shot - must
// produce a TakeLoopContext whose capabilities refuse a follow-up loudly
// when the turn genuinely didn't finish (is_error:true on claude's result),
// AND that refusal must land on the pump where RunBead can find it after the
// run. The runtime is driven through its real Start/WaitDrained event
// pipeline (not a hand-set field) so this exercises the same is_error
// parsing production code path uses - see TakeLoopContext.TurnFailed.
// TestDriverReportsFailureWhenOneShotClaudeTurnFailsAndCannotBeNudged below
// is the end-to-end proof through the real spawn/drain/RunBead path; this
// one is kept alongside it because it isolates the gate itself from the
// spawn/drain timing.
func TestSessionPumpRefusesFollowUpForOneShotClaude(t *testing.T) {
	be := &fakeBackend{state: map[string]string{"kb-1": "implementation"}}
	scm := newTestSCM()
	scm.Connect("kb-1-claude")

	dialect := adapter.ResolveDialect("claude")
	runtime := session.NewSessionRuntimeWithCapabilities("kb-1", "/repo", string(dialect), false)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stdout := strings.NewReader(`{"type":"result","is_error":true,"result":"error_max_turns"}` + "\n")
	stderr := strings.NewReader("")
	runtime.Start(ctx, stdout, stderr)
	runtime.WaitDrained()

	pump := &sessionPump{
		scm:       scm,
		runtime:   runtime,
		sessionID: "kb-1-claude",
		beadID:    "kb-1",
		repoPath:  "/repo",
		backend:   be,
	}

	if proceed := pump.handleTurnEnded("turn_ended"); proceed {
		t.Error("expected handleTurnEnded to return false when the dialect has no follow-up path")
	}

	err := pump.followUpError()
	if err == nil {
		t.Fatal("expected the refusal to be recorded on the pump so RunBead can report it as a failure")
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") || !strings.Contains(err.Error(), "claude") {
		t.Errorf("expected the recorded error to carry KERNL DISPATCH FAILURE and name claude, got: %v", err)
	}

	buffer := scm.GetBuffer("kb-1-claude")
	found := false
	for _, evt := range buffer {
		if evt.Type == "stderr" && strings.Contains(evt.Data, "KERNL DISPATCH FAILURE") && strings.Contains(evt.Data, "claude") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a KERNL DISPATCH FAILURE banner naming claude in the event buffer, got: %+v", buffer)
	}
}

// TestSessionPumpAllowsCleanTurnEndForOneShotClaude is
// TestSessionPumpRefusesFollowUpForOneShotClaude's mirror: the same one-shot,
// no-follow-up-path claude runtime, but the result event it observes reports
// is_error:false - a clean completion. HandleTakeLoopTurnEnded must step
// aside quietly (no proceed, no recorded error) rather than refuse a
// follow-up nobody asked for.
func TestSessionPumpAllowsCleanTurnEndForOneShotClaude(t *testing.T) {
	be := &fakeBackend{state: map[string]string{"kb-1": "implementation"}}
	scm := newTestSCM()
	scm.Connect("kb-1-claude")

	dialect := adapter.ResolveDialect("claude")
	runtime := session.NewSessionRuntimeWithCapabilities("kb-1", "/repo", string(dialect), false)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stdout := strings.NewReader(`{"type":"result","is_error":false,"result":"done"}` + "\n")
	stderr := strings.NewReader("")
	runtime.Start(ctx, stdout, stderr)
	runtime.WaitDrained()

	pump := &sessionPump{
		scm:       scm,
		runtime:   runtime,
		sessionID: "kb-1-claude",
		beadID:    "kb-1",
		repoPath:  "/repo",
		backend:   be,
	}

	if proceed := pump.handleTurnEnded("turn_ended"); proceed {
		t.Error("expected handleTurnEnded to return false: nothing to keep stdin open for on a clean turn end")
	}
	if err := pump.followUpError(); err != nil {
		t.Errorf("expected no follow-up error recorded for a clean turn end, got: %v", err)
	}
}

// TestSessionPumpSendsFollowUpForInteractiveDialectWhenTurnFails is
// criterion 3 driven through the real event pipeline: an interactive
// dialect (SupportsFollowUp=true) whose turn genuinely failed still gets
// nudged - the capability-gated halt introduced by this fix is scoped to
// dialects that cannot be nudged at all, not to every failed turn.
func TestSessionPumpSendsFollowUpForInteractiveDialectWhenTurnFails(t *testing.T) {
	be := &fakeBackend{state: map[string]string{"kb-1": "implementation"}}
	scm := newTestSCM()
	scm.Connect("kb-1-opencode")

	dialect := adapter.ResolveDialect("opencode")
	runtime := session.NewSessionRuntimeWithCapabilities("kb-1", "/repo", string(dialect), true)
	var stdin bytes.Buffer
	runtime.SetStdin(&stdin)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stdout := strings.NewReader(`{"type":"session_error"}` + "\n")
	stderr := strings.NewReader("")
	runtime.Start(ctx, stdout, stderr)
	runtime.WaitDrained()

	pump := &sessionPump{
		scm:       scm,
		runtime:   runtime,
		sessionID: "kb-1-opencode",
		beadID:    "kb-1",
		repoPath:  "/repo",
		backend:   be,
	}

	if proceed := pump.handleTurnEnded("turn_ended"); !proceed {
		t.Error("expected handleTurnEnded to return true: opencode supports follow-up and the turn failed")
	}
	if err := pump.followUpError(); err != nil {
		t.Errorf("expected no error - opencode can be nudged, got: %v", err)
	}
	if stdin.Len() == 0 {
		t.Error("expected a follow-up prompt written to stdin")
	}
	count, nudged := pump.followUpStats()
	if !nudged || count != 1 {
		t.Errorf("expected nudged=true count=1, got nudged=%v count=%d", nudged, count)
	}
}

// TestDriverReportsFailureWhenOneShotClaudeTurnFailsAndCannotBeNudged drives
// the full RunBead path: real spawn (fakeSpawner), real SessionRuntime.Start/
// WaitDrained/Dispose, real sessionPump. Claude's "result" (its turn-ending
// event) reports is_error:true - the turn genuinely didn't finish - while
// the bead is left in an active, non-terminal state, so the take-loop tries
// to nudge it and finds the dialect has no follow-up path. RunBead must
// report that as a failure: a turn that ended without finishing, on a
// dialect that cannot be nudged, is exactly the case PR #158 introduced this
// halt for. DriveBeadToTerminal only halts on a non-nil error or
// Success=false (internal/app/drive_bead.go), never on a log line.
//
// This exercises the WaitDrained fix for the drain race directly: before
// that fix, proc.Wait() (fakeProcess.Wait() here returns immediately) raced
// the goroutine still reading the "result" line, and cancellation from
// Dispose() usually won - see the inversion check in the PR description for
// the failure mode this reproduces.
func TestDriverReportsFailureWhenOneShotClaudeTurnFailsAndCannotBeNudged(t *testing.T) {
	be := &fakeBackend{state: map[string]string{"kb-1": "implementation"}}
	spawn := &fakeSpawner{script: "{\"type\":\"result\",\"is_error\":true,\"result\":\"error_max_turns\"}\n"}
	scm := newTestSCM()
	d := NewSessionDriver(DriverDeps{Backend: be, Spawn: spawn.Spawn, SCM: scm, LogDir: t.TempDir()})

	res, err := d.RunBead(context.Background(), RunBeadInput{
		BeadID: "kb-1", RepoPath: t.TempDir(), Command: "claude", AgentName: "claude",
	})

	if err == nil {
		t.Fatal("expected RunBead to return an error when a genuinely failed turn cannot be nudged")
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") || !strings.Contains(err.Error(), "claude") {
		t.Errorf("expected the error to carry KERNL DISPATCH FAILURE and name claude, got: %v", err)
	}
	if res.Success {
		t.Error("expected Success=false - a turn that ended without finishing and could not be nudged did not finish the stage")
	}
}

// TestDriverRunsToCompletionWhenOneShotClaudeFinishesCleanly is the
// regression this fix closes. A real orchestrator run against a real target
// repository did everything right - correct commit, clean worktree, a
// decision record the strict parser accepts, verification through the
// target repo's own bin/ci - and terminated normally: claude's "result"
// event with the process exiting zero. The take-loop still refused a
// follow-up for claude (SupportsFollowUp=false, no resume transport wired)
// and reported the whole stage as KERNL DISPATCH FAILURE, because the bead
// was still in its active state at turn-end - true on every turn end,
// success or failure, since the exit gate only runs after RunBead returns
// (see internal/app/drive_bead.go). This drives the same real
// spawn/drain/RunBead path as
// TestDriverReportsFailureWhenOneShotClaudeTurnFailsAndCannotBeNudged above,
// changing only is_error to false, and must run to completion: no error, no
// halt.
func TestDriverRunsToCompletionWhenOneShotClaudeFinishesCleanly(t *testing.T) {
	be := &fakeBackend{state: map[string]string{"kb-1": "implementation"}}
	spawn := &fakeSpawner{
		script: "{\"type\":\"result\",\"is_error\":false,\"result\":\"done\"}\n",
		onExit: func() { be.state["kb-1"] = "ready_for_review" },
	}
	scm := newTestSCM()
	d := NewSessionDriver(DriverDeps{Backend: be, Spawn: spawn.Spawn, SCM: scm, LogDir: t.TempDir()})

	res, err := d.RunBead(context.Background(), RunBeadInput{
		BeadID: "kb-1", RepoPath: t.TempDir(), Command: "claude", AgentName: "claude",
	})

	if err != nil {
		t.Fatalf("RunBead: expected the stage to run to completion, got: %v", err)
	}
	if !res.Success {
		t.Error("expected Success=true for a clean, exit-zero completion")
	}
}

func TestDriverReportsFailureWhenPiStreamContainsErrorDespiteExitZero(t *testing.T) {
	be := &fakeBackend{state: map[string]string{"kb-1": "implementation"}}
	spawn := &fakeSpawner{
		script: `{"type":"session","version":3,"id":"019fc4ff-74ee-79d1-910b-734b253f6668"}` + "\n" +
			`{"type":"message","id":"9d37a587","message":{"role":"assistant","stopReason":"error","errorMessage":"429: litellm.RateLimitError: Model rate limit exceeded"}}` + "\n" +
			`{"type":"agent_settled"}` + "\n",
	}
	scm := newTestSCM()
	d := NewSessionDriver(DriverDeps{Backend: be, Spawn: spawn.Spawn, SCM: scm, LogDir: t.TempDir()})

	res, err := d.RunBead(context.Background(), RunBeadInput{
		BeadID: "kb-1", RepoPath: t.TempDir(), Command: "pi", AgentName: "pi-kimi",
	})

	if err == nil {
		t.Fatal("expected RunBead to return an error when pi stream reports a RateLimitError")
	}
	if res.Success {
		t.Error("expected Success=false when pi stream reports a RateLimitError")
	}
}

// An empty LogDir used to fall back to deriving one from os.Getenv("HOME")
// inside openStageLogs, which made agent stdout/stderr logs land in the
// operator's real ~/.kernl/logs from any caller (test or otherwise) that
// forgot to set it. It must fail loud instead, naming the field that fixes
// it, rather than spawn the agent under an unconfigured log path.
func TestDriverRunBeadFailsLoudWithoutLogDir(t *testing.T) {
	be := &fakeBackend{state: map[string]string{"kb-1": "ready_for_implementation"}}
	spawn := &fakeSpawner{script: "{\"type\":\"session_idle\"}\n"}
	d := NewSessionDriver(DriverDeps{Backend: be, Spawn: spawn.Spawn, SCM: newTestSCM()})

	_, err := d.RunBead(context.Background(), RunBeadInput{BeadID: "kb-1", RepoPath: t.TempDir(), Command: "opencode", AgentName: "opencode"})
	if err == nil {
		t.Fatal("expected a loud refusal rather than a run with no log directory")
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("error must carry the KERNL DISPATCH FAILURE marker, got: %v", err)
	}
	if !strings.Contains(err.Error(), "DriverDeps.LogDir") {
		t.Errorf("error must name the field that fixes it (DriverDeps.LogDir), got: %v", err)
	}
	if spawn.spawned {
		t.Error("driver must refuse before spawning the agent process")
	}
}

func TestDriverRunBeadAdvancesViaTakeLoop(t *testing.T) {
	be := &fakeBackend{state: map[string]string{"kb-1": "ready_for_implementation"}}
	spawn := &fakeSpawner{
		script: "{\"type\":\"output\",\"content\":\"ok\"}\n{\"type\":\"session_idle\"}\n",
		onExit: func() { be.state["kb-1"] = "done" },
	}
	d := NewSessionDriver(DriverDeps{Backend: be, Spawn: spawn.Spawn, SCM: newTestSCM(), LogDir: t.TempDir()})
	res, err := d.RunBead(context.Background(), RunBeadInput{BeadID: "kb-1", RepoPath: t.TempDir(), Command: "opencode", AgentName: "opencode"})
	if err != nil {
		t.Fatalf("RunBead: %v", err)
	}
	if res.FinalState != "done" {
		t.Errorf("FinalState = %q, want done", res.FinalState)
	}
	if !spawn.spawned {
		t.Error("driver must spawn the agent process")
	}
}
