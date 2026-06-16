package adapter

import (
	"strings"
)

type AgentDialect string

const (
	DialectClaude   AgentDialect = "claude"
	DialectCodex    AgentDialect = "codex"
	DialectCopilot  AgentDialect = "copilot"
	DialectOpenCode AgentDialect = "opencode"
	DialectGemini   AgentDialect = "gemini"
)

type PromptModeArgs struct {
	Command string
	Args    []string
}

func ResolveDialect(command string) AgentDialect {
	base := command
	if idx := strings.LastIndex(command, "/"); idx >= 0 {
		base = command[idx+1:]
	}
	lower := strings.ToLower(base)
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
	return DialectClaude
}
