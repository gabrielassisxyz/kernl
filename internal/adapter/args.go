package adapter

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	ClaudeApprovalMCPServer  = "kernl_approval"
	ClaudeApprovalPromptTool = "mcp__kernl_approval__ask"
)

// ClaudeApprovalBridgeMCPConfig wires kernl itself in as the permission-prompt
// server. The bridge is a kernl subcommand rather than a shipped script because
// kernl is a single binary: a script would be a second artifact to install,
// keep in step with the store format, and find at dispatch time.
func ClaudeApprovalBridgeMCPConfig(kernlPath string) string {
	cfg := map[string]any{
		"mcpServers": map[string]any{
			ClaudeApprovalMCPServer: map[string]any{
				"command": kernlPath,
				"args":    ApprovalBridgeArgs("claude", true),
			},
		},
	}
	data, _ := json.Marshal(cfg)
	return string(data)
}

// ApprovalBridgeArgs is the argv that turns the kernl binary into the bridge.
func ApprovalBridgeArgs(adapterName string, mcp bool) []string {
	args := []string{"approval", "bridge", "--adapter", adapterName}
	if mcp {
		args = append(args, "--mcp")
	}
	return args
}

// SupportsApprovalPrompt reports whether a dialect can raise a judgment gate.
//
// Only the two measured against a real CLI are listed. Every other dialect is
// dispatched with its permission checks bypassed, so answering "yes" for one
// would produce a run that silently never asks - the failure this gate exists
// to make impossible.
func SupportsApprovalPrompt(dialect AgentDialect) bool {
	return dialect == DialectClaude || dialect == DialectPi
}

func appendClaudePermissionArgs(args []string, agent AgentTarget) []string {
	if ShouldBypassPermissions(agent) {
		return append(args, "--dangerously-skip-permissions")
	}
	args = append(args,
		"--permission-mode", "default",
		"--setting-sources", "project",
		"--strict-mcp-config",
		"--mcp-config", ClaudeApprovalBridgeMCPConfig(agent.ApprovalBridgePath),
		"--permission-prompt-tool", ClaudeApprovalPromptTool,
	)
	return args
}

func BuildClaudeInteractiveArgs(agent AgentTarget) PromptModeArgs {
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
	args = appendClaudePermissionArgs(args, agent)
	if agent.Model != "" {
		args = append(args, "--model", agent.Model)
	}
	return PromptModeArgs{Command: cmd, Args: args}
}

func BuildClaudePromptModeArgs(agent AgentTarget, prompt string) PromptModeArgs {
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
	args = appendClaudePermissionArgs(args, agent)
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
	case DialectPi:
		// pi takes `[options] [messages...]`, so the prompt is the trailing
		// positional. --mode json is what makes it emit the NDJSON stream
		// this repository's session runtime reads; without it pi prints
		// prose and every event-derived signal goes dark.
		args := []string{"-p", "--mode", "json"}
		// pi has no permission-prompt flag: a tool call is gated from inside
		// an extension's tool_call hook, so the gate is an -e file rather than
		// a flag. Without one, pi runs every tool unasked.
		if !ShouldBypassPermissions(agent) && agent.ApprovalExtensionPath != "" {
			args = append(args, "-e", agent.ApprovalExtensionPath)
		}
		if agent.Model != "" {
			args = append(args, "--model", agent.Model)
		}
		args = append(args, prompt)
		return PromptModeArgs{Command: cmd, Args: args}
	case DialectAgy:
		// agy has no JSON output mode: -p prints the answer as plain text,
		// which the runtime forwards as raw stdout. See agyOneShotCapabilities
		// for what that costs (no turn events, no token usage).
		args := []string{"-p", prompt, "--dangerously-skip-permissions"}
		if agent.Model != "" {
			args = append(args, "--model", agent.Model)
		}
		return PromptModeArgs{Command: cmd, Args: args}
	default:
		return BuildClaudePromptModeArgs(agent, prompt)
	}
}

// ShouldBypassPermissions reports whether this agent runs unasked. It is named
// for the decision rather than for claude because pi reads it too: the flag is
// the operator's, and only the mechanism differs per dialect.
func ShouldBypassPermissions(agent AgentTarget) bool {
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

// The approval bridge inherits its context through the environment, because it
// runs as a grandchild of the dispatch: kernl spawns the agent, the agent
// spawns the bridge, and only the environment crosses that second boundary
// without the agent having to be taught to forward anything.
//
// These replaced a base URL and a bearer token. The store is a directory, not a
// server, so there is no port for the bridge to discover and no endpoint to
// authenticate against - which is also why a gate raised under `kernl bead run`
// is answerable at all, since that verb stands up no HTTP listener.
const (
	EnvTerminalSessionID = "KERNL_TERMINAL_SESSION_ID"
	EnvApprovalDir       = "KERNL_APPROVAL_DIR"
	EnvApprovalBeadID    = "KERNL_APPROVAL_BEAD_ID"
	EnvApprovalRepoPath  = "KERNL_APPROVAL_REPO_PATH"
	EnvApprovalAgentName = "KERNL_APPROVAL_AGENT_NAME"
	EnvApprovalTimeout   = "KERNL_APPROVAL_TIMEOUT"
)

// ApprovalBridgeContext is what a dispatch knows and the bridge cannot derive.
type ApprovalBridgeContext struct {
	SessionID string
	StoreDir  string
	BeadID    string
	RepoPath  string
	AgentName string
	Timeout   time.Duration
}

// ApprovalBridgeEnvVars renders the bridge context into environment variables.
// Empty fields are omitted rather than exported blank, so the bridge's own
// defaults stay reachable.
func ApprovalBridgeEnvVars(ctx ApprovalBridgeContext) map[string]string {
	env := map[string]string{}
	for name, value := range map[string]string{
		EnvTerminalSessionID: ctx.SessionID,
		EnvApprovalDir:       ctx.StoreDir,
		EnvApprovalBeadID:    ctx.BeadID,
		EnvApprovalRepoPath:  ctx.RepoPath,
		EnvApprovalAgentName: ctx.AgentName,
	} {
		if value != "" {
			env[name] = value
		}
	}
	if ctx.Timeout > 0 {
		env[EnvApprovalTimeout] = ctx.Timeout.String()
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
