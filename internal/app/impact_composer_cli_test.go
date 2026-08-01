package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/config"
)

// recordingRunner is a named fake standing in for a spawned CLI: it captures
// what would have been executed and returns a canned answer, so every test
// here stays hermetic (no subprocess, no PATH lookup, no network).
type recordingRunner struct {
	command string
	args    []string
	workDir string
	answer  string
	err     error
	calls   int
}

func (r *recordingRunner) run(_ context.Context, command string, args []string, workDir string) (string, error) {
	r.calls++
	r.command, r.args, r.workDir = command, args, workDir
	return r.answer, r.err
}

func impactInput() DecisionImpact {
	return DecisionImpact{
		DecisionTitle:     "cache the robots.txt fetch",
		DecisionContext:   "every URL refetched it",
		OptionsConsidered: "per-run cache; per-host cache",
		TradeOffs:         "staleness against request volume",
		Outcome:           "per-host cache",
		RepoPath:          "/repos/archeion",
		BeadTitle:         "a sitemap run refetches robots.txt once per URL",
	}
}

func TestCLIImpactComposer_AsksTheAgentAndReturnsItsAnswer(t *testing.T) {
	runner := &recordingRunner{answer: "Crawls of large sitemaps stop hammering each host.\n"}
	composer := CLIImpactComposer{
		AgentID: "claude-opus",
		Agent:   config.AgentConfig{Command: "claude", Model: "opus"},
		Run:     runner.run,
	}

	got, err := composer.ComposeImpact(context.Background(), impactInput())
	if err != nil {
		t.Fatalf("ComposeImpact: %v", err)
	}
	if got != "Crawls of large sitemaps stop hammering each host." {
		t.Errorf("answer = %q, want it trimmed and verbatim", got)
	}
	if runner.command != "claude" {
		t.Errorf("command = %q, want claude", runner.command)
	}
	if !contains(runner.args, "--model") || !contains(runner.args, "opus") {
		t.Errorf("args = %q, want the configured model passed through", runner.args)
	}
	// No permission bypass: the mayor writes a paragraph, and a dialect that
	// has to ask before touching anything will touch nothing unattended.
	for _, forbidden := range []string{"--dangerously-skip-permissions", "--dangerously-bypass-approvals-and-sandbox"} {
		if contains(runner.args, forbidden) {
			t.Errorf("args = %q, want no %s", runner.args, forbidden)
		}
	}
}

// The composer must not be able to reach a repository: it is asked a question
// about work already done, not asked to look at it.
func TestCLIImpactComposer_RunsOutsideTheRepository(t *testing.T) {
	runner := &recordingRunner{answer: "something"}
	composer := CLIImpactComposer{
		AgentID: "claude-opus",
		Agent:   config.AgentConfig{Command: "claude"},
		Run:     runner.run,
	}

	if _, err := composer.ComposeImpact(context.Background(), impactInput()); err != nil {
		t.Fatalf("ComposeImpact: %v", err)
	}
	if runner.workDir == "" {
		t.Fatal("workDir is empty, want a scratch directory")
	}
	if strings.Contains(runner.workDir, "archeion") {
		t.Errorf("workDir = %q, want somewhere that is not the repository under work", runner.workDir)
	}
}

func TestCLIImpactComposer_WhitespaceAnswerIsAFailure(t *testing.T) {
	runner := &recordingRunner{answer: "   \n\t\n"}
	composer := CLIImpactComposer{
		AgentID: "claude-opus",
		Agent:   config.AgentConfig{Command: "claude"},
		Run:     runner.run,
	}

	// Persisting "" would tell a later reader the mayor looked at this
	// decision and had nothing to say. A truncated or refused answer is not
	// that judgment, and the API-backed composer already treats it this way.
	if _, err := composer.ComposeImpact(context.Background(), impactInput()); err == nil {
		t.Fatal("a whitespace-only answer returned no error, want a failure")
	}
}

func TestCLIImpactComposer_SurfacesTheAgentFailure(t *testing.T) {
	runner := &recordingRunner{err: errors.New("exit status 1: model not found")}
	composer := CLIImpactComposer{
		AgentID: "claude-opus",
		Agent:   config.AgentConfig{Command: "claude"},
		Run:     runner.run,
	}

	_, err := composer.ComposeImpact(context.Background(), impactInput())
	if err == nil {
		t.Fatal("expected the agent's failure to surface")
	}
	for _, want := range []string{"KERNL DISPATCH FAILURE", "claude-opus", "model not found"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestCLIImpactComposer_RefusesADialectThatCannotAnswerPlainly(t *testing.T) {
	runner := &recordingRunner{answer: "irrelevant"}
	composer := CLIImpactComposer{
		AgentID: "the-codex",
		Agent:   config.AgentConfig{Command: "codex"},
		Run:     runner.run,
	}

	_, err := composer.ComposeImpact(context.Background(), impactInput())
	if err == nil {
		t.Fatal("codex was accepted, want a refusal - its one-shot output frames the answer")
	}
	if runner.calls != 0 {
		t.Errorf("the agent ran %d times, want 0 - the refusal must come before any spawn", runner.calls)
	}
	if !strings.Contains(err.Error(), "the-codex") {
		t.Errorf("error %q does not name the agent id the operator configured", err)
	}
}

func TestNewImpactComposer_PrefersTheAgentOverTheProviderAPI(t *testing.T) {
	cfg := &config.Config{}
	cfg.LLM = config.LLMConfig{Provider: "openai", Endpoint: "http://localhost:4000", Model: "some-model", Agent: "claude-opus"}
	cfg.Settings.Agents = map[string]config.AgentConfig{
		"claude-opus": {Command: "claude", Model: "opus"},
	}

	composer, err := NewImpactComposer(cfg)
	if err != nil {
		t.Fatalf("NewImpactComposer: %v", err)
	}
	cli, ok := composer.(CLIImpactComposer)
	if !ok {
		t.Fatalf("composer is %T, want CLIImpactComposer - llm.agent must win over a configured provider", composer)
	}
	if cli.Agent.Command != "claude" || cli.Agent.Model != "opus" {
		t.Errorf("agent = %+v, want the settings.agents entry llm.agent names", cli.Agent)
	}
}

func TestNewImpactComposer_FallsBackToTheProviderAPI(t *testing.T) {
	cfg := &config.Config{}
	cfg.LLM = config.LLMConfig{Provider: "openai", Endpoint: "http://localhost:4000", Model: "some-model"}

	composer, err := NewImpactComposer(cfg)
	if err != nil {
		t.Fatalf("NewImpactComposer: %v", err)
	}
	if _, ok := composer.(LLMImpactComposer); !ok {
		t.Fatalf("composer is %T, want LLMImpactComposer", composer)
	}
}

// An unconfigured llm has never stopped a run, and must not start now:
// ComposeRunReport writes a report without field 4 rather than failing.
func TestNewImpactComposer_NothingConfiguredIsNotAnError(t *testing.T) {
	composer, err := NewImpactComposer(&config.Config{})
	if err != nil {
		t.Fatalf("NewImpactComposer: %v", err)
	}
	if composer != nil {
		t.Errorf("composer = %v, want nil when neither llm.agent nor llm.provider is set", composer)
	}
}

// A configured-but-broken llm.agent is the opposite case: it is an operator
// error, and finding it after 30 minutes of work is finding it too late.
func TestNewImpactComposer_UndeclaredAgentFailsLoud(t *testing.T) {
	cfg := &config.Config{}
	cfg.LLM = config.LLMConfig{Agent: "claude-opus"}
	cfg.Settings.Agents = map[string]config.AgentConfig{"codex": {Command: "codex"}}

	_, err := NewImpactComposer(cfg)
	if err == nil {
		t.Fatal("an llm.agent naming no declared agent returned no error")
	}
	for _, want := range []string{"KERNL DISPATCH FAILURE", "claude-opus", "settings.agents", "Fix:"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}
