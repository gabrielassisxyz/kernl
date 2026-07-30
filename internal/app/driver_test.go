package app

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/adapter"
	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/session"
)

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
	d := NewSessionDriver(DriverDeps{Backend: be, Spawn: spawn.Spawn, SCM: newTestSCM(), NudgeRegistry: nudges})

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
// produce a TakeLoopContext whose capabilities refuse a follow-up loudly,
// AND that refusal must land on the pump where RunBead can find it after the
// run. TestDriverReportsFailureWhenOneShotClaudeCannotBeNudged below is the
// end-to-end proof through the real spawn/drain/RunBead path; this one is
// kept alongside it because it isolates the gate itself from the drain
// timing.
func TestSessionPumpRefusesFollowUpForOneShotClaude(t *testing.T) {
	be := &fakeBackend{state: map[string]string{"kb-1": "implementation"}}
	scm := newTestSCM()
	scm.Connect("kb-1-claude")

	dialect := adapter.ResolveDialect("claude")
	runtime := session.NewSessionRuntimeWithCapabilities("kb-1", "/repo", string(dialect), false)

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

// TestDriverReportsFailureWhenOneShotClaudeCannotBeNudged drives the full
// RunBead path: real spawn (fakeSpawner), real SessionRuntime.Start/
// WaitDrained/Dispose, real sessionPump. Claude emits "result" (its
// turn-ending event) and exits zero while the bead is left in an active,
// non-terminal state, so the take-loop tries to nudge it and finds the
// dialect has no follow-up path. RunBead must report that as a failure -
// an exit-zero agent process is not the same thing as a stage that actually
// finished, and DriveBeadToTerminal only halts on a non-nil error or
// Success=false (internal/app/drive_bead.go), never on a log line.
//
// This exercises the WaitDrained fix for the drain race directly: before
// that fix, proc.Wait() (fakeProcess.Wait() here returns immediately) raced
// the goroutine still reading the "result" line, and cancellation from
// Dispose() usually won - see the inversion check in the PR description for
// the failure mode this reproduces.
func TestDriverReportsFailureWhenOneShotClaudeCannotBeNudged(t *testing.T) {
	be := &fakeBackend{state: map[string]string{"kb-1": "implementation"}}
	spawn := &fakeSpawner{script: "{\"type\":\"result\"}\n"}
	scm := newTestSCM()
	d := NewSessionDriver(DriverDeps{Backend: be, Spawn: spawn.Spawn, SCM: scm})

	res, err := d.RunBead(context.Background(), RunBeadInput{
		BeadID: "kb-1", RepoPath: t.TempDir(), Command: "claude", AgentName: "claude",
	})

	if err == nil {
		t.Fatal("expected RunBead to return an error when the dialect cannot be nudged")
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") || !strings.Contains(err.Error(), "claude") {
		t.Errorf("expected the error to carry KERNL DISPATCH FAILURE and name claude, got: %v", err)
	}
	if res.Success {
		t.Error("expected Success=false - an exit-zero agent that could not be nudged did not finish the stage")
	}
}

func TestDriverRunBeadAdvancesViaTakeLoop(t *testing.T) {
	be := &fakeBackend{state: map[string]string{"kb-1": "ready_for_implementation"}}
	spawn := &fakeSpawner{
		script: "{\"type\":\"output\",\"content\":\"ok\"}\n{\"type\":\"session_idle\"}\n",
		onExit: func() { be.state["kb-1"] = "done" },
	}
	d := NewSessionDriver(DriverDeps{Backend: be, Spawn: spawn.Spawn, SCM: newTestSCM()})
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
