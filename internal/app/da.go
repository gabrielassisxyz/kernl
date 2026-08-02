package app

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/adapter"
	"github.com/gabrielassisxyz/kernl/internal/config"
)

// DA is the actor an implementer hands a fork over to when it meets a choice
// the bead, the docs and existing precedent do not determine (see
// fork_handover.go). It is NOT the Oracle: the Oracle is tool-less and
// context-less on purpose, because it only ever writes about a decision
// already taken (see CLIImpactComposer's own doc comment). The DA is the
// opposite on purpose - it has tools and a working directory because
// choosing on the operator's behalf is worthless without knowing what the
// operator has already decided, written down, or would want. Neither actor's
// rule is a precedent for the other's.
type DA interface {
	// Consult asks the DA one question and returns its trimmed answer.
	Consult(ctx context.Context, question string) (string, error)
}

// daConsultTimeout bounds one DA consultation. It is longer than
// reversibilityJudgeTimeout (90s) for the reason the two actors differ at
// all: the Oracle answers from text handed to it, while the DA reads files
// in a real working directory before it answers, and that costs real wall
// clock a tool-less actor never spends. Three minutes is generous for
// reading a handful of notes and recorded preferences without leaving a
// stage hung for the rest of a run over one slow consultation.
const daConsultTimeout = 3 * time.Minute

// CLIDA is the production DA: it spawns an agent CLI with its working
// directory set to the operator's own system repository and asks it one
// question, with tools restricted to read-only access (see
// adapter.BuildConsultModeArgs).
type CLIDA struct {
	// AgentID is the settings.agents key this DA was resolved from, carried
	// for error messages: an operator debugging a failed consult needs the
	// name they wrote under da.agent, not the command it expanded to.
	AgentID string
	Agent   config.AgentConfig
	// WorkDir is the operator's own system repository. Unlike
	// CLIImpactComposer.WorkDir, this is never empty and never a scratch
	// directory: NewDA refuses to construct a CLIDA at all when da.workDir
	// does not resolve to a real directory, so by the time Consult runs,
	// WorkDir is already known good.
	WorkDir string
	// Run reuses AnswerRunner and RunAnswerCommand (impact_composer_cli.go)
	// rather than declaring a second runner type: spawning a CLI and
	// reading its stdout is the same job for both actors, only the argv and
	// the working directory differ. Defaults to RunAnswerCommand when nil.
	Run AnswerRunner
}

var _ DA = CLIDA{}

// Consult implements DA.
func (d CLIDA) Consult(ctx context.Context, question string) (string, error) {
	if strings.TrimSpace(d.Agent.Command) == "" {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: da.agent %q has no command - Fix: set settings.agents.%s.command in kernl.yaml", d.AgentID, d.AgentID)
	}

	built, err := adapter.BuildConsultModeArgs(
		adapter.AgentTarget{Command: d.Agent.Command, Model: d.Agent.Model},
		question)
	if err != nil {
		return "", fmt.Errorf("da.agent %q: %w", d.AgentID, err)
	}

	run := d.Run
	if run == nil {
		run = RunAnswerCommand
	}
	askCtx, cancel := context.WithTimeout(ctx, daConsultTimeout)
	defer cancel()
	out, err := run(askCtx, built.Command, built.Args, d.WorkDir)
	if err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: da.agent %q (%s) could not answer the fork question it was asked: %w", d.AgentID, built.Command, err)
	}
	return nonEmptyCompletion(out)
}

// NewDA resolves the DA from configuration, or reports nil, nil when the
// fork gate is off - config.Load already guarantees da.agent and da.workDir
// are either both set or both empty, so the only thing left to check here is
// what Load could not: whether da.agent names a real settings.agents entry,
// and whether da.workDir is a real directory.
//
// A nil DA with a nil error means "not configured" - the caller (
// DecideForkAction) treats that as its own escalation cause, exactly like
// NewOracle's nil Oracle already means "compose the report without field 4"
// to its own callers. This MUST return a nil interface, never a typed nil:
// see newOracle's own doc comment on why "var d DA = CLIDA{}" wrapped around
// a zero value would make every "is a DA configured" check downstream
// silently answer yes.
func NewDA(cfg *config.Config) (DA, error) {
	if cfg == nil {
		return nil, nil
	}
	agentID := strings.TrimSpace(cfg.DA.Agent)
	workDir := strings.TrimSpace(cfg.DA.WorkDir)
	if agentID == "" && workDir == "" {
		return nil, nil
	}

	agentCfg, ok := cfg.Settings.Agents[agentID]
	if !ok {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: da.agent %q is not declared in settings.agents - Fix: add settings.agents.%s with a command, or clear da.agent (and da.workDir) to leave the fork gate off", agentID, agentID)
	}

	// A configured-but-wrong directory is an operator mistake discovered at
	// startup, not at the moment a fork is handed over mid-run - the same
	// reason NewOracle/NewImpactComposer refuse a broken llm.agent up front
	// rather than at report-close time.
	//
	// §3.3 of the plan this unit implements considered falling back to
	// --add-dir when the working directory "cannot be set", and this
	// deliberately does not build that: in Go, cmd.Dir (RunAnswerCommand) is
	// always settable on any *exec.Cmd before it runs - the only real
	// failure is a directory that does not exist, and that is the loud
	// error below, not a fallback for a case that cannot occur (AGENTS.md
	// §2 forbids exactly that shape of fallback for a configured resource).
	info, err := os.Stat(workDir)
	if err != nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: da.workDir %q does not exist: %w - Fix: point da.workDir at the operator's own system repository, or clear it (and da.agent) to leave the fork gate off", workDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: da.workDir %q is not a directory - Fix: point da.workDir at the operator's own system repository", workDir)
	}

	// A DA whose command BuildConsultModeArgs refuses can never be consulted
	// at all - discovered here, at startup, rather than at the moment an
	// implementer's first fork is handed over mid-run, which would convert a
	// pure configuration defect into a da_unavailable escalation that blocks
	// the bead and wakes the operator (see ForkCauseDAUnavailable). The probe
	// prompt and the argv BuildConsultModeArgs would build are both
	// discarded - only whether it errors matters - so this reuses the same
	// dialect check Consult itself makes rather than hardcoding a second
	// "claude, pi" allowlist here that could drift from adapter's own.
	if _, err := adapter.BuildConsultModeArgs(adapter.AgentTarget{Command: agentCfg.Command, Model: agentCfg.Model}, "kernl startup dialect probe"); err != nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: da.agent %q (command %q) cannot be consulted - the fork gate's read-only consult mode is only supported for the claude and pi commands, and this agent's command is neither - Fix: point da.agent at a settings.agents entry whose command is claude or pi, or clear da.agent (and da.workDir) to leave the fork gate off", agentID, agentCfg.Command)
	}

	return CLIDA{AgentID: agentID, Agent: agentCfg, WorkDir: workDir}, nil
}
