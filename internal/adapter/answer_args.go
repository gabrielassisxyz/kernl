package adapter

import "fmt"

// BuildAnswerModeArgs builds the argv for asking a CLI one question and
// reading one plain-text answer off stdout.
//
// This is not BuildPromptModeArgs with different flags, it is a different
// job. A stage dispatch wants an agent with tools, a working tree, and an
// event stream to follow; this wants a sentence back and nothing touched. The
// two dialects below are the ones that can be asked for exactly that and were
// measured doing it - their stdout is the answer, with no framing to parse
// off it:
//
//	claude  `-p --tools "" <prompt>`     -> the answer, verbatim
//	pi      `-p --no-tools <prompt>`     -> the answer, verbatim
//
// Every other dialect is refused rather than guessed at, and the two reasons
// are different. codex `exec` prints its own framing around the answer
// ("Reading additional input from stdin...") and refuses to run outside a
// trusted git directory at all, so its stdout is not an answer. agy has no
// way to disable its tools: `-p` does return the answer verbatim, but a
// composer that only needs a paragraph should not be able to edit files, and
// there is no flag to promise that.
//
// Both supported forms deliberately omit any permission-bypass flag. Asking
// for a paragraph is not work that needs write access, and a dialect that has
// to ask before touching anything is a dialect that will not touch anything
// when nobody is there to answer.
func BuildAnswerModeArgs(agent AgentTarget, prompt string) (PromptModeArgs, error) {
	cmd := agent.Command
	if cmd == "" {
		cmd = "claude"
	}

	switch ResolveDialect(cmd) {
	case DialectClaude:
		// --tools "" is claude 2.1.220's own documented way to turn every
		// built-in tool off (`claude --help`: "Use \"\" to disable all
		// tools"), measured against --allowed-tools/--disallowed-tools,
		// which restrict WHICH tools are reachable rather than disabling
		// tool use altogether. Omitting a tool flag - what this used to do -
		// left the CLI's own default tool set enabled, which made the
		// tool-less claim made about this actor elsewhere false for claude.
		//
		// The prompt comes FIRST here, before the flags, and that ordering is
		// load-bearing rather than stylistic: claude spells this flag
		// `--tools <tools...>`, which is variadic, so a prompt trailing it is
		// consumed as one more tool name and the CLI then exits with "Input
		// must be provided either through stdin or as a prompt argument when
		// using --print". Measured against claude 2.1.220:
		//
		//	claude -p --tools "" "<prompt>"   -> Error: Input must be provided...
		//	claude -p "<prompt>" --tools ""   -> the answer
		//
		// Every other dialect here takes its prompt as a trailing positional,
		// which is why this one reads differently from the rest of the file.
		args := []string{"-p", prompt, "--tools", ""}
		if agent.Model != "" {
			args = append(args, "--model", agent.Model)
		}
		return PromptModeArgs{Command: cmd, Args: args}, nil
	case DialectPi:
		args := []string{"-p", "--no-tools"}
		if agent.Model != "" {
			args = append(args, "--model", agent.Model)
		}
		args = append(args, prompt)
		return PromptModeArgs{Command: cmd, Args: args}, nil
	default:
		return PromptModeArgs{}, fmt.Errorf(
			"KERNL DISPATCH FAILURE: agent command %q cannot be asked a plain question - its one-shot output carries framing around the answer, or it offers no way to run without tools - Fix: point llm.agent at an agent whose command is claude or pi",
			cmd)
	}
}
