package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/adapter"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/prompt"
)

// AnswerRunner runs a CLI to completion and returns everything it wrote to
// stdout. It exists so CLIImpactComposer can be tested without a subprocess:
// the production implementation is RunAnswerCommand, and a test supplies its
// own.
type AnswerRunner func(ctx context.Context, command string, args []string, workDir string) (string, error)

// CLIImpactComposer is the ImpactComposer that asks an agent CLI instead of a
// provider API.
//
// The reason to have both: the orchestrator already reaches its models
// through CLI agents, where a coding plan's best models live, while
// LLMImpactComposer reaches an openai-compatible endpoint that has to serve
// that model itself. Pointing llm.agent at a configured agent lets the one
// paragraph the mayor writes come from the same place the implementers come
// from, without a second billing route existing only for it.
//
// It is deliberately NOT a stage dispatch. The agent is asked one question in
// a scratch directory with no tools (see adapter.BuildAnswerModeArgs), so it
// cannot reach a repository, and nothing about its run is recorded in the
// stage-attempt ledger - it did not attempt a stage.
type CLIImpactComposer struct {
	// AgentID is the settings.agents key, carried for error messages: an
	// operator debugging a failed compose needs the name they wrote in
	// kernl.yaml, not the command it expanded to.
	AgentID string
	Agent   config.AgentConfig
	// WorkDir is where the CLI is spawned. Empty means a fresh temp
	// directory per call, which is the safe default: the composer has no
	// business in a repository, and a CLI started inside one will read it.
	WorkDir string
	// Run defaults to RunAnswerCommand when nil.
	Run AnswerRunner
}

// ComposeImpact implements ImpactComposer.
//
// The emptiness rule matches LLMImpactComposer's exactly, and for the same
// reason: an answer that is only whitespace is a truncation or a refusal, not
// the deliberate judgment that this decision has nothing worth saying about
// it. See nonEmptyCompletion.
func (c CLIImpactComposer) ComposeImpact(ctx context.Context, in DecisionImpact) (string, error) {
	if strings.TrimSpace(c.Agent.Command) == "" {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: llm.agent %q has no command - Fix: set settings.agents.%s.command in kernl.yaml", c.AgentID, c.AgentID)
	}

	built, err := adapter.BuildAnswerModeArgs(
		adapter.AgentTarget{Command: c.Agent.Command, Model: c.Agent.Model},
		prompt.RenderImpactOnUse(prompt.ImpactOnUseInput{
			DecisionTitle:     in.DecisionTitle,
			DecisionContext:   in.DecisionContext,
			OptionsConsidered: in.OptionsConsidered,
			TradeOffs:         in.TradeOffs,
			Outcome:           in.Outcome,
			RepoPath:          in.RepoPath,
			BeadTitle:         in.BeadTitle,
		}))
	if err != nil {
		return "", fmt.Errorf("llm.agent %q: %w", c.AgentID, err)
	}

	workDir, cleanup, err := c.resolveWorkDir()
	if err != nil {
		return "", err
	}
	defer cleanup()

	run := c.Run
	if run == nil {
		run = RunAnswerCommand
	}
	out, err := run(ctx, built.Command, built.Args, workDir)
	if err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: llm.agent %q (%s) could not answer the impact question: %w", c.AgentID, built.Command, err)
	}
	return nonEmptyCompletion(out)
}

// resolveWorkDir returns the directory to spawn in and a cleanup for it. A
// configured WorkDir is used as given and never removed; the default is a
// throwaway the composer owns.
func (c CLIImpactComposer) resolveWorkDir() (string, func(), error) {
	if c.WorkDir != "" {
		return c.WorkDir, func() {}, nil
	}
	dir, err := os.MkdirTemp("", "kernl-impact-*")
	if err != nil {
		return "", nil, fmt.Errorf("KERNL DISPATCH FAILURE: llm.agent %q needs a scratch directory to run in and none could be created: %w", c.AgentID, err)
	}
	return dir, func() { _ = os.RemoveAll(dir) }, nil
}

// RunAnswerCommand is the production AnswerRunner.
//
// stderr is captured separately and folded into the error rather than into
// the answer: agent CLIs write progress and warnings there, and letting that
// reach the caller would persist a log line as a decision's impact.
func RunAnswerCommand(ctx context.Context, command string, args []string, workDir string) (string, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = workDir
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("%w: %s", err, msg)
		}
		return "", err
	}
	return string(out), nil
}

// NewImpactComposer picks the mayor from configuration: the agent CLI named
// by llm.agent when there is one, the provider API at llm.endpoint otherwise,
// and nothing at all when neither is configured.
//
// A nil composer is a supported outcome, not a failure - ComposeRunReport
// writes a report without field 4 rather than failing the close, which is why
// an unconfigured llm has never stopped a run.
//
// A CONFIGURED but broken llm.agent is the opposite, and it is an error here
// so the run refuses to start. The alternative is discovering it after the
// work is done, at the moment the report is written, which is the same shape
// as a run finding out at shipment that it has nowhere it is allowed to
// publish - the reason that check moved to startup too.
func NewImpactComposer(cfg *config.Config) (ImpactComposer, error) {
	if cfg == nil {
		return nil, nil
	}
	agentID := strings.TrimSpace(cfg.LLM.Agent)
	if agentID == "" {
		if cfg.LLM.IsSet() {
			return LLMImpactComposer{LLM: cfg.LLM}, nil
		}
		return nil, nil
	}
	agentCfg, ok := cfg.Settings.Agents[agentID]
	if !ok {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: llm.agent %q is not declared in settings.agents - Fix: add settings.agents.%s with a command, or clear llm.agent to ask the provider API at llm.endpoint instead", agentID, agentID)
	}
	return CLIImpactComposer{AgentID: agentID, Agent: agentCfg}, nil
}
