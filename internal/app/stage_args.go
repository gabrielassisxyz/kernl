package app

import (
	"github.com/gabrielassisxyz/kernl/internal/adapter"
)

// BuildStageArgs produces the full argv that carries one stage's prompt to one
// agent CLI.
//
// The shape is per-dialect and comes from adapter.BuildPromptModeArgs, which
// already knows that claude wants `-p <prompt>`, codex wants
// `exec <prompt> --json`, and opencode wants the prompt as a trailing
// positional. Dispatching every agent with opencode's shape is why only
// opencode could ever be dispatched: claude has no --dir/-s/--title and codex
// takes -m rather than --model, so both died on unknown flags before reading
// their prompt.
//
// extra carries the operator's own flags from settings.agents.<id>.args. They
// are appended after the built args, and for opencode inserted before the
// trailing positional prompt, so a configured flag never displaces the message.
//
// opencode alone still needs --dir: it resolves "the project" by git discovery
// from the launch directory rather than from the process working directory, so
// the worktree has to be named explicitly. Every other dialect follows Cwd.
func BuildStageArgs(agent adapter.AgentTarget, extra []string, beadID, worktree, sessionID, prompt string) (string, []string) {
	built := adapter.BuildPromptModeArgs(agent, prompt)

	if adapter.ResolveDialect(built.Command) != adapter.DialectOpenCode {
		return built.Command, append(append([]string(nil), built.Args...), extra...)
	}

	// The opencode builder puts the prompt last; peel it off so the stage
	// flags land before it and the positional stays last.
	base := built.Args
	if len(base) > 0 {
		base = base[:len(base)-1]
	}
	args := append(append([]string(nil), base...), extra...)
	return built.Command, appendOpencodeStageFlags(args, beadID, worktree, sessionID, prompt)
}
