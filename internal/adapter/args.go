package adapter

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	ClaudeApprovalMCPServer  = "kernl_approval"
	ClaudeApprovalPromptTool = "mcp__kernl_approval__ask"
)

func ClaudeApprovalBridgeMCPConfig(bridgeScriptPath string) string {
	cfg := map[string]any{
		"mcpServers": map[string]any{
			ClaudeApprovalMCPServer: map[string]any{
				"command": bridgeScriptPath,
			},
		},
	}
	data, _ := json.Marshal(cfg)
	return string(data)
}

func appendClaudePermissionArgs(args []string, agent AgentTarget, bridgeScriptPath string) []string {
	if ShouldBypassClaudePermissions(agent) {
		return append(args, "--dangerously-skip-permissions")
	}
	args = append(args,
		"--permission-mode", "default",
		"--setting-sources", "project",
		"--strict-mcp-config",
		"--mcp-config", ClaudeApprovalBridgeMCPConfig(bridgeScriptPath),
		"--permission-prompt-tool", ClaudeApprovalPromptTool,
	)
	return args
}

func BuildClaudeInteractiveArgs(agent AgentTarget) PromptModeArgs {
	return BuildClaudeInteractiveArgsWithBridge(agent, "")
}

func BuildClaudeInteractiveArgsWithBridge(agent AgentTarget, bridgeScriptPath string) PromptModeArgs {
	cmd := agent.Command
	if cmd == "" {
		cmd = "claude"
	}
	args := []string{
		"-p",
		"--input-format", "stream-json",
		"--verbose",
		"--output-format", "stream-json",
	}
	args = appendClaudePermissionArgs(args, agent, bridgeScriptPath)
	if agent.Model != "" {
		args = append(args, "--model", agent.Model)
	}
	return PromptModeArgs{Command: cmd, Args: args}
}

func BuildClaudePromptModeArgs(agent AgentTarget, prompt string) PromptModeArgs {
	return BuildClaudePromptModeArgsWithBridge(agent, prompt, "")
}

func BuildClaudePromptModeArgsWithBridge(agent AgentTarget, prompt string, bridgeScriptPath string) PromptModeArgs {
	cmd := agent.Command
	if cmd == "" {
		cmd = "claude"
	}
	args := []string{
		"-p", prompt,
		"--input-format", "text",
		"--output-format", "stream-json",
		"--include-partial-messages",
		"--verbose",
	}
	args = appendClaudePermissionArgs(args, agent, bridgeScriptPath)
	if agent.Model != "" {
		args = append(args, "--model", agent.Model)
	}
	return PromptModeArgs{Command: cmd, Args: args}
}

func BuildCodexInteractiveArgs(agent AgentTarget) PromptModeArgs {
	cmd := agent.Command
	if cmd == "" {
		cmd = "codex"
	}
	args := []string{"app-server"}
	if agent.ApprovalMode == "prompt" {
		args = append(args,
			"-c", "approval_policy=\"untrusted\"",
			"-c", "sandbox_mode=\"read-only\"",
		)
	}
	args = append(args, "--listen", "stdio://")
	if agent.Model != "" {
		args = append(args, "-c", fmt.Sprintf("model=\"%s\"", agent.Model))
	}
	return PromptModeArgs{Command: cmd, Args: args}
}

func BuildCopilotInteractiveArgs(agent AgentTarget) PromptModeArgs {
	cmd := agent.Command
	if cmd == "" {
		cmd = "copilot"
	}
	args := []string{
		"--session",
		"--output-format", "json",
		"--stream", "on",
		"--allow-all",
	}
	if agent.Model != "" {
		args = append(args, "--model", agent.Model)
	}
	return PromptModeArgs{Command: cmd, Args: args}
}

func BuildOpenCodeInteractiveArgs(agent AgentTarget) PromptModeArgs {
	cmd := agent.Command
	if cmd == "" {
		cmd = "opencode"
	}
	args := []string{"serve", "--port", "0", "--print-logs"}
	return PromptModeArgs{Command: cmd, Args: args}
}

func BuildGeminiInteractiveArgs(agent AgentTarget) PromptModeArgs {
	cmd := agent.Command
	if cmd == "" {
		cmd = "gemini"
	}
	args := []string{"--acp", "-y"}
	if agent.Model != "" {
		args = append(args, "-m", agent.Model)
	}
	return PromptModeArgs{Command: cmd, Args: args}
}

func BuildPromptModeArgs(agent AgentTarget, prompt string) PromptModeArgs {
	cmd := agent.Command
	if cmd == "" {
		cmd = "claude"
	}
	dialect := ResolveDialect(cmd)

	switch dialect {
	case DialectGemini:
		args := []string{"-p", prompt, "-o", "stream-json", "-y"}
		if agent.Model != "" {
			args = append(args, "-m", agent.Model)
		}
		return PromptModeArgs{Command: cmd, Args: args}
	case DialectOpenCode:
		args := []string{"run", "--format", "json"}
		if agent.Model != "" {
			args = append(args, "-m", agent.Model)
		}
		args = append(args, prompt)
		return PromptModeArgs{Command: cmd, Args: args}
	case DialectCopilot:
		args := []string{
			"-p", prompt,
			"--output-format", "json",
			"--stream", "on",
			"--allow-all",
			"--no-ask-user",
		}
		if agent.Model != "" {
			args = append(args, "--model", agent.Model)
		}
		return PromptModeArgs{Command: cmd, Args: args}
	case DialectCodex:
		args := []string{
			"exec", prompt,
			"--json",
			"--dangerously-bypass-approvals-and-sandbox",
		}
		if agent.Model != "" {
			args = append(args, "-m", agent.Model)
		}
		return PromptModeArgs{Command: cmd, Args: args}
	default:
		return BuildClaudePromptModeArgs(agent, prompt)
	}
}

func ShouldBypassClaudePermissions(agent AgentTarget) bool {
	return agent.ApprovalMode != "prompt"
}

func BuildInteractiveArgs(agent AgentTarget) PromptModeArgs {
	dialect := ResolveDialect(agent.Command)
	switch dialect {
	case DialectCodex:
		return BuildCodexInteractiveArgs(agent)
	case DialectCopilot:
		return BuildCopilotInteractiveArgs(agent)
	case DialectOpenCode:
		return BuildOpenCodeInteractiveArgs(agent)
	case DialectGemini:
		return BuildGeminiInteractiveArgs(agent)
	default:
		return BuildClaudeInteractiveArgs(agent)
	}
}

const (
	EnvTerminalSessionID      = "KERNL_TERMINAL_SESSION_ID"
	EnvApprovalBridgeBaseURL  = "KERNL_APPROVAL_BRIDGE_BASE_URL"
	EnvApprovalBridgeToken    = "KERNL_APPROVAL_BRIDGE_TOKEN"
)

func ApprovalBridgeEnvVars(sessionID, baseURL, token string) map[string]string {
	env := map[string]string{
		EnvTerminalSessionID:     sessionID,
		EnvApprovalBridgeBaseURL: baseURL,
	}
	if token != "" {
		env[EnvApprovalBridgeToken] = token
	}
	return env
}

func BuildAgentArgs(agent AgentTarget, dialect AgentDialect, dispatchKind TerminalDispatchKind, isInteractive, isJSONRPC, isHTTPServer, isACP bool) (string, []string, error) {
	if isJSONRPC {
		built := BuildCodexInteractiveArgs(agent)
		return built.Command, built.Args, nil
	}
	if isHTTPServer {
		built := BuildOpenCodeInteractiveArgs(agent)
		return built.Command, built.Args, nil
	}
	if isACP {
		built := BuildGeminiInteractiveArgs(agent)
		return built.Command, built.Args, nil
	}
	if isInteractive && dialect == DialectCopilot {
		built := BuildCopilotInteractiveArgs(agent)
		return built.Command, built.Args, nil
	}
	if isInteractive {
		built := BuildClaudeInteractiveArgs(agent)
		return built.Command, built.Args, nil
	}
	return "", nil, fmt.Errorf("%s", FormatTakeSceneOneShotFailure(dialect, dispatchKind, "cli-arg"))
}

func BuildSpawnArgs(agent AgentTarget, dialect AgentDialect, dispatchKind TerminalDispatchKind, isInteractive, isJSONRPC, isHTTPServer, isACP bool) (string, []string, error) {
	if isJSONRPC {
		built := BuildCodexInteractiveArgs(agent)
		return built.Command, built.Args, nil
	}
	if isHTTPServer {
		built := BuildOpenCodeInteractiveArgs(agent)
		return built.Command, built.Args, nil
	}
	if isACP {
		built := BuildGeminiInteractiveArgs(agent)
		return built.Command, built.Args, nil
	}
	if isInteractive && dialect == DialectCopilot {
		built := BuildCopilotInteractiveArgs(agent)
		return built.Command, built.Args, nil
	}
	if isInteractive {
		built := BuildClaudeInteractiveArgs(agent)
		return built.Command, built.Args, nil
	}
	return "", nil, fmt.Errorf("%s", FormatTakeSceneOneShotFailure(dialect, dispatchKind, "cli-arg"))
}

func parseModelSelection(model string) (providerID, modelID string, err error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return "", "", fmt.Errorf("KERNL DISPATCH FAILURE: expected <providerID>/<modelID>, got empty string")
	}
	idx := strings.Index(model, "/")
	if idx < 0 {
		return "", "", fmt.Errorf("KERNL DISPATCH FAILURE: expected <providerID>/<modelID>, got %q (missing provider slash)", model)
	}
	return model[:idx], model[idx+1:], nil
}