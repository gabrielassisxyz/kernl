package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/adapter"
	"github.com/gabrielassisxyz/kernl/internal/config"
)

// --- CLIDA.Consult ---

func TestCLIDA_ConsultAsksTheAgentInsideItsWorkDir(t *testing.T) {
	runner := &recordingRunner{answer: "FORK: DECIDE\nCHOSEN: relevance-first\nnothing outside this bead has to agree.\n"}
	da := CLIDA{
		AgentID: "claude-opus",
		Agent:   config.AgentConfig{Command: "claude", Model: "opus"},
		WorkDir: "/home/operator/system-repo",
		Run:     runner.run,
	}

	got, err := da.Consult(context.Background(), "which option should win")
	if err != nil {
		t.Fatalf("Consult: %v", err)
	}
	if got != strings.TrimSpace("FORK: DECIDE\nCHOSEN: relevance-first\nnothing outside this bead has to agree.\n") {
		t.Errorf("answer = %q, want it trimmed and verbatim", got)
	}
	if runner.workDir != "/home/operator/system-repo" {
		t.Errorf("workDir = %q, want the operator's own system repository, not a scratch directory", runner.workDir)
	}
	if runner.command != "claude" {
		t.Errorf("command = %q, want claude", runner.command)
	}
	if !contains(runner.args, "--tools") || !contains(runner.args, "Read,Grep,Glob") {
		t.Errorf("args = %q, want tools restricted to read-only access", runner.args)
	}
	if !contains(runner.args, "which option should win") {
		t.Errorf("args = %q, want the question passed through", runner.args)
	}
}

func TestCLIDA_ConsultMissingCommandFailsLoud(t *testing.T) {
	da := CLIDA{AgentID: "stub", WorkDir: "/home/operator/system-repo"}
	_, err := da.Consult(context.Background(), "q")
	if err == nil || !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Fatalf("Consult() = %v, want a KERNL DISPATCH FAILURE naming the missing command", err)
	}
	if !strings.Contains(err.Error(), "da.agent") {
		t.Errorf("error %q should name da.agent, not llm.agent", err)
	}
}

func TestCLIDA_ConsultWhitespaceAnswerIsAFailure(t *testing.T) {
	runner := &recordingRunner{answer: "   \n"}
	da := CLIDA{
		AgentID: "stub",
		Agent:   config.AgentConfig{Command: "claude"},
		WorkDir: "/home/operator/system-repo",
		Run:     runner.run,
	}
	if _, err := da.Consult(context.Background(), "q"); err == nil {
		t.Fatal("expected an error for a whitespace-only answer")
	}
}

func TestCLIDA_ConsultRunnerFailureIsDispatchFailure(t *testing.T) {
	runner := &recordingRunner{err: errors.New("exit status 1")}
	da := CLIDA{
		AgentID: "stub",
		Agent:   config.AgentConfig{Command: "claude"},
		WorkDir: "/home/operator/system-repo",
		Run:     runner.run,
	}
	_, err := da.Consult(context.Background(), "q")
	if err == nil || !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Fatalf("Consult() = %v, want a KERNL DISPATCH FAILURE", err)
	}
}

// --- NewDA ---

func TestNewDA_NilConfigIsNotConfigured(t *testing.T) {
	da, err := NewDA(nil)
	if err != nil || da != nil {
		t.Fatalf("NewDA(nil) = %v, %v, want nil, nil", da, err)
	}
}

func TestNewDA_BothUnsetIsNotConfigured(t *testing.T) {
	cfg := &config.Config{Settings: config.Settings{Agents: map[string]config.AgentConfig{"stub": {Command: "claude"}}}}
	da, err := NewDA(cfg)
	if err != nil {
		t.Fatalf("NewDA: %v", err)
	}
	// Must be a genuinely nil interface (AGENTS.md: never a typed nil) - a
	// direct comparison catches the trap a nil *CLIDA wrapped in a DA would
	// not.
	if da != nil {
		t.Fatalf("NewDA returned a non-nil DA for an unconfigured fork gate: %#v", da)
	}
}

func TestNewDA_AgentNotDeclaredFailsLoud(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		DA:       config.DAConfig{Agent: "missing-agent", WorkDir: dir},
		Settings: config.Settings{Agents: map[string]config.AgentConfig{}},
	}
	_, err := NewDA(cfg)
	if err == nil || !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Fatalf("NewDA() = %v, want a KERNL DISPATCH FAILURE naming the missing agent", err)
	}
	if !strings.Contains(err.Error(), "missing-agent") {
		t.Errorf("error %q should name the missing agent", err)
	}
}

func TestNewDA_WorkDirDoesNotExistFailsLoud(t *testing.T) {
	cfg := &config.Config{
		DA:       config.DAConfig{Agent: "stub", WorkDir: filepath.Join(t.TempDir(), "does-not-exist")},
		Settings: config.Settings{Agents: map[string]config.AgentConfig{"stub": {Command: "claude"}}},
	}
	_, err := NewDA(cfg)
	if err == nil || !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Fatalf("NewDA() = %v, want a KERNL DISPATCH FAILURE naming the bad workDir", err)
	}
}

func TestNewDA_WorkDirIsAFileFailsLoud(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(filePath, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		DA:       config.DAConfig{Agent: "stub", WorkDir: filePath},
		Settings: config.Settings{Agents: map[string]config.AgentConfig{"stub": {Command: "claude"}}},
	}
	_, err := NewDA(cfg)
	if err == nil || !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Fatalf("NewDA() = %v, want a KERNL DISPATCH FAILURE naming that workDir is not a directory", err)
	}
}

// TestNewDA_UnsupportedDialectFailsLoud proves finding 2 of the
// fork/decision-gate hardening pass: a da.agent whose command
// BuildConsultModeArgs refuses (everything but claude and pi) must fail at
// NewDA, not at the first fork handover mid-run.
func TestNewDA_UnsupportedDialectFailsLoud(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		DA:       config.DAConfig{Agent: "opencode", WorkDir: dir},
		Settings: config.Settings{Agents: map[string]config.AgentConfig{"opencode": {Command: "opencode"}}},
	}
	_, err := NewDA(cfg)
	if err == nil || !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Fatalf("NewDA() = %v, want a KERNL DISPATCH FAILURE naming the unsupported dialect", err)
	}
	if !strings.Contains(err.Error(), "opencode") {
		t.Errorf("error %q should name da.agent and its command (opencode)", err)
	}
}

// daAgentExampleLine matches kernl.yaml.example's own commented `agent:`
// line inside its `# da:` block (there is exactly one `agent:` line in that
// file - grepped and confirmed before writing this test).
var daAgentExampleLine = regexp.MustCompile(`(?m)^#\s*agent:\s*(\S+)\s*$`)

// TestExampleConfig_DAAgentIsASupportedDialect proves kernl.yaml.example's
// own `da: agent:` example can never again drift out of the set
// adapter.BuildConsultModeArgs actually supports (finding 2 of the
// fork/decision-gate hardening pass). The example used to publish `agent:
// opencode`, a command BuildConsultModeArgs refuses by name - so anyone who
// copied it got a fork gate that started fine and only failed the first time
// an implementer actually handed a fork over, mid-run.
//
// This follows the settings.agents convention every entry in this file
// already uses (the settings.agents KEY is also the CLI COMMAND, e.g.
// `opencode: {command: opencode}`), so the example agent name is checked
// directly as a command.
func TestExampleConfig_DAAgentIsASupportedDialect(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file path")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	data, err := os.ReadFile(filepath.Join(repoRoot, "kernl.yaml.example"))
	if err != nil {
		t.Fatalf("read kernl.yaml.example: %v", err)
	}

	match := daAgentExampleLine.FindSubmatch(data)
	if match == nil {
		t.Fatal("kernl.yaml.example no longer documents a `da: agent:` example line - update this test's regexp to match its new shape")
	}
	exampleCommand := string(match[1])

	if _, err := adapter.BuildConsultModeArgs(adapter.AgentTarget{Command: exampleCommand}, "probe"); err != nil {
		t.Errorf("kernl.yaml.example's own `da: agent: %s` example does not resolve to a dialect BuildConsultModeArgs supports: %v", exampleCommand, err)
	}
}

func TestNewDA_BothSetAndValidReturnsAWorkingCLIDA(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		DA:       config.DAConfig{Agent: "stub", WorkDir: dir},
		Settings: config.Settings{Agents: map[string]config.AgentConfig{"stub": {Command: "claude", Model: "opus"}}},
	}
	da, err := NewDA(cfg)
	if err != nil {
		t.Fatalf("NewDA: %v", err)
	}
	got, ok := da.(CLIDA)
	if !ok {
		t.Fatalf("NewDA returned %T, want a CLIDA", da)
	}
	if got.AgentID != "stub" || got.Agent.Command != "claude" || got.WorkDir != dir {
		t.Errorf("got %+v, want it built from the resolved settings.agents entry and da.workDir", got)
	}
}
