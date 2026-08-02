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
//	claude  `-p <prompt> --tools ""`     -> the answer, verbatim
//	pi      `-p --no-tools <prompt>`     -> the answer, verbatim
//
// claude's prompt comes BEFORE --tools, unlike every other flag order in this
// package, and that is load-bearing, not a style choice: `claude --help`
// documents `--tools <tools...>` as VARIADIC, so a bare positional prompt
// trailing it is consumed as one more tool name rather than as the prompt.
// Measured against claude 2.1.220 - `claude -p --tools "" "Reply with
// exactly: OK"` (prompt trailing, the shape this function used to emit)
// fails with "Error: Input must be provided either through stdin or as a
// prompt argument when using --print", while `claude -p "Reply with exactly:
// OK" --tools ""` (prompt immediately after -p) answers correctly. pi's
// `--tools, -t <tools>` takes a single value, not a variadic list, and
// `--no-tools` is a boolean, so pi has no such trap and keeps its prompt
// trailing.
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
		// prompt is placed right after -p, before --tools/--model: see this
		// function's own doc comment on why --tools being variadic makes a
		// trailing prompt get swallowed as another tool name instead.
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

// BuildConsultModeArgs builds the argv for asking a CLI one question with its
// BUILT-IN tools restricted to a read-only set, inside a working directory
// the agent is free to explore.
//
// "Restricted to read-only" describes the built-in tool set ONLY, and that
// distinction is deliberate, not incidental - see this function's own
// "Also measured" paragraph below: any MCP server already configured for
// that working directory keeps every one of its own tools reachable,
// write-capable ones included, because this flag never reaches MCP tools at
// all. That is what makes the plan's §3.5 true - the DA can reach memory
// kernl itself never reads - and it is also the thing an operator relying on
// this flag as a write guard would get wrong; see below for exactly what is
// and is not enforced.
//
// It is the deliberate mirror image of BuildAnswerModeArgs, and for the
// opposite reason. That function serves an actor (the Oracle) that must not
// reach anything, so it disables every tool. This one serves the DA, whose
// entire value is reaching what the operator has already written down in
// their own system repository - so built-in tools stay ON, restricted to
// ones that can only read.
//
// Same dialect allowlist as BuildAnswerModeArgs, and the same measured
// reason: claude and pi are the two dialects whose one-shot stdout is the
// answer with no framing around it. Everything else is refused BY NAME, and
// the Fix names da.agent - not llm.agent - because that is the config key
// this actor is resolved from.
//
//	claude  `-p <prompt> --tools "Read,Grep,Glob"`
//
// Measured against claude 2.1.220's own --help: "--tools <tools...>
// Specify the list of available tools from the built-in set. Use \"\" to
// disable all tools, \"default\" to use all tools, or specify tool names
// (e.g. \"Bash,Edit,Read\")." Unlike pi below, claude's own --help does not
// enumerate the complete built-in set - only that example. Read, Grep and
// Glob are this package's own choice of claude's stable, publicly
// documented read-only tool names (the ones that inspect a repository
// without ever writing to it), not a list --help itself spells out
// exhaustively - recorded here rather than left implicit, since the
// difference from pi's measurement below matters.
//
// The prompt goes right after -p, before --tools/--model, for the exact
// reason BuildAnswerModeArgs's own doc comment records: --tools is
// VARIADIC, so a prompt trailing it is consumed as one more tool name and
// claude then refuses to run at all ("Input must be provided either through
// stdin or as a prompt argument when using --print"). Both orders were
// measured end to end: `claude -p <prompt> --tools "" --model <m>` answers,
// `claude -p --tools "" <prompt> --model <m>` does not.
//
// Also measured, inside a real operator system repository: `--tools
// "Read,Grep,Glob"` restricts the BUILT-IN tool set only. Every
// `mcp__<server>__*` tool configured for that directory - both the ones that
// only read and any that write - stayed loaded and reachable; this flag does
// not touch MCP tools at all, restrict or otherwise. This is what makes the
// plan's §3.5 true: kernl deliberately does not read the operator's own
// standing-memory store itself, and the DA reaching it is only possible
// BECAUSE restricting built-ins does not also disable MCP servers already
// configured in that directory. Which MCP servers (if any) are configured
// there, and whether any of them write, is the operator's own choice made
// outside kernl entirely - kernl neither names nor restricts a specific one.
//
//	pi      `-p --tools read,grep,find,ls <prompt>`
//
// Measured against pi 0.83.0's own --help, which documents this exact
// read-only set verbatim, unlike claude's: "read - Read file contents",
// "grep - Search file contents (read-only, off by default)", "find - Find
// files by glob pattern (read-only, off by default)", "ls - List directory
// contents (read-only, off by default)" - and gives this as its own worked
// example: `pi --tools read,grep,find,ls -p "Review the code in src/"`. pi's
// `--tools, -t <tools>` takes a single value, not a variadic list, so it has
// no such trap and keeps its prompt trailing, exactly like
// BuildAnswerModeArgs's pi branch.
//
// No permission-bypass flag, matching BuildAnswerModeArgs: the DA only needs
// to read, and a dialect that has to ask before touching anything is a
// dialect that will not touch anything when nobody is there to answer.
func BuildConsultModeArgs(agent AgentTarget, prompt string) (PromptModeArgs, error) {
	cmd := agent.Command
	if cmd == "" {
		cmd = "claude"
	}

	switch ResolveDialect(cmd) {
	case DialectClaude:
		args := []string{"-p", prompt, "--tools", "Read,Grep,Glob"}
		if agent.Model != "" {
			args = append(args, "--model", agent.Model)
		}
		return PromptModeArgs{Command: cmd, Args: args}, nil
	case DialectPi:
		args := []string{"-p", "--tools", "read,grep,find,ls"}
		if agent.Model != "" {
			args = append(args, "--model", agent.Model)
		}
		args = append(args, prompt)
		return PromptModeArgs{Command: cmd, Args: args}, nil
	default:
		return PromptModeArgs{}, fmt.Errorf(
			"KERNL DISPATCH FAILURE: agent command %q cannot be asked a plain question with read-only tools - its one-shot output carries framing around the answer, or it offers no way to restrict it to read-only tools - Fix: point da.agent at an agent whose command is claude or pi",
			cmd)
	}
}
