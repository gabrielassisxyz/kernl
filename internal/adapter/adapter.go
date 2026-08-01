package adapter

import (
	"fmt"
	"sort"
	"strings"
)

type AgentDialect string

const (
	DialectClaude   AgentDialect = "claude"
	DialectCodex    AgentDialect = "codex"
	DialectCopilot  AgentDialect = "copilot"
	DialectOpenCode AgentDialect = "opencode"
	DialectGemini   AgentDialect = "gemini"
	DialectPi       AgentDialect = "pi"
	DialectAgy      AgentDialect = "agy"

	// DialectUnknown is what a configured command matching no dialect
	// resolves to. It exists so the mismatch has a name: before it, an
	// unrecognised command silently became DialectClaude and was dispatched
	// with claude's argv, so `command: pi` spawned pi with
	// `--input-format stream-json` and failed for a reason that named
	// neither the command nor the configuration that chose it.
	//
	// Nothing dispatches it: ResolveDialectStrict turns it into a
	// KERNL DISPATCH FAILURE when an agent is resolved from a pool, which is
	// before any stage has started.
	DialectUnknown AgentDialect = "unknown"
)

// exactCommandDialects maps an executable's exact base name to its dialect.
//
// Exact, not substring, and checked before the substring rules in
// ResolveDialect: "pi" is a substring of "copilot", so a contains-rule for it
// would silently claim copilot's dispatch. A short name has to match whole.
var exactCommandDialects = map[string]AgentDialect{
	"pi":  DialectPi,
	"agy": DialectAgy,
}

type PromptModeArgs struct {
	Command string
	Args    []string
}

// ResolveDialect names the CLI dialect a command speaks, or DialectUnknown.
//
// An empty command is deliberately NOT unknown: every argv builder in this
// package answers an empty command with claude (see
// BuildClaudePromptModeArgs), so "unset" and "wrong" stay different answers.
func ResolveDialect(command string) AgentDialect {
	base := command
	if idx := strings.LastIndex(command, "/"); idx >= 0 {
		base = command[idx+1:]
	}
	lower := strings.ToLower(base)
	if lower == "" {
		return DialectClaude
	}
	if dialect, ok := exactCommandDialects[lower]; ok {
		return dialect
	}
	if strings.Contains(lower, "gemini") {
		return DialectGemini
	}
	if strings.Contains(lower, "copilot") {
		return DialectCopilot
	}
	if strings.Contains(lower, "opencode") {
		return DialectOpenCode
	}
	if strings.Contains(lower, "codex") || strings.Contains(lower, "chatgpt") {
		return DialectCodex
	}
	if strings.Contains(lower, "claude") {
		return DialectClaude
	}
	return DialectUnknown
}

// ResolveDialectStrict is ResolveDialect for the dispatch path: a command
// this package cannot speak is an error, not a claude-shaped guess.
func ResolveDialectStrict(command string) (AgentDialect, error) {
	dialect := ResolveDialect(command)
	if dialect != DialectUnknown {
		return dialect, nil
	}
	return DialectUnknown, fmt.Errorf(
		"KERNL DISPATCH FAILURE: agent command %q speaks no dialect this build knows - Fix: set settings.agents.<id>.command to a CLI whose name contains one of %s, or add a dialect for it in internal/adapter",
		command, strings.Join(KnownDialectNames(), ", "))
}

// KnownDialectNames lists every dispatchable dialect, so an error can tell the
// operator what the valid answers are and not only what was wrong.
func KnownDialectNames() []string {
	names := []string{
		string(DialectClaude), string(DialectCodex), string(DialectCopilot),
		string(DialectOpenCode), string(DialectGemini), string(DialectPi),
		string(DialectAgy),
	}
	sort.Strings(names)
	return names
}
