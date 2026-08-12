package app

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/adapter"
	"github.com/gabrielassisxyz/kernl/internal/approvals"
	"github.com/gabrielassisxyz/kernl/internal/config"
)

func gateInput(t *testing.T) ApprovalBridgeInput {
	t.Helper()
	return ApprovalBridgeInput{
		StateDir:  t.TempDir(),
		SessionID: "kernl-1-claude",
		BeadID:    "kernl-1",
		RepoPath:  "/repo",
		AgentName: "claude",
		Timeout:   30 * time.Minute,
	}
}

// The default path must be untouched: an agent the operator did not put behind
// a gate is dispatched exactly as before.
func TestApprovalModeAutoArmsNothing(t *testing.T) {
	target := adapter.AgentTarget{Command: "claude", ApprovalMode: "auto"}
	env := map[string]string{}

	if err := applyApprovalBridge(&target, env, gateInput(t)); err != nil {
		t.Fatalf("applyApprovalBridge: %v", err)
	}
	if target.ApprovalBridgePath != "" || len(env) != 0 {
		t.Errorf("an ungated dispatch must stay ungated: path=%q env=%v", target.ApprovalBridgePath, env)
	}

	_, args := BuildStageArgs(target, nil, "kernl-1", "/wt", "", "do it")
	if !containsArg(args, "--dangerously-skip-permissions") {
		t.Errorf("an ungated claude still bypasses permissions, got %q", args)
	}
}

func TestClaudeGateWiresTheBridgeIntoItsCommandLine(t *testing.T) {
	target := adapter.AgentTarget{Command: "claude", ApprovalMode: "prompt"}
	env := map[string]string{}
	in := gateInput(t)

	if err := applyApprovalBridge(&target, env, in); err != nil {
		t.Fatalf("applyApprovalBridge: %v", err)
	}
	if target.ApprovalBridgePath == "" {
		t.Fatal("the gate must name the binary the agent spawns")
	}

	_, args := BuildStageArgs(target, nil, "kernl-1", "/wt", "", "do it")
	if containsArg(args, "--dangerously-skip-permissions") {
		t.Fatal("a gated claude must not also be told to skip permissions")
	}
	for _, want := range []string{"--permission-prompt-tool", adapter.ClaudeApprovalPromptTool, "--mcp-config", "--strict-mcp-config"} {
		if !containsArg(args, want) {
			t.Errorf("missing %q in %q", want, args)
		}
	}

	// The MCP config must point at kernl's own bridge subcommand; a config
	// naming a command that does not exist produces an agent that cannot ask.
	var cfg struct {
		MCPServers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(argAfter(args, "--mcp-config")), &cfg); err != nil {
		t.Fatalf("the mcp config is not JSON: %v", err)
	}
	server, ok := cfg.MCPServers[adapter.ClaudeApprovalMCPServer]
	if !ok {
		t.Fatalf("no %q server in the mcp config", adapter.ClaudeApprovalMCPServer)
	}
	if server.Command != target.ApprovalBridgePath {
		t.Errorf("the server command must be the kernl binary, got %q", server.Command)
	}
	if strings.Join(server.Args, " ") != "approval bridge --adapter claude --mcp" {
		t.Errorf("the bridge argv is wrong: %q", server.Args)
	}
}

func TestPiGateShipsTheExtensionAndPointsAtTheBridge(t *testing.T) {
	target := adapter.AgentTarget{Command: "pi", ApprovalMode: "prompt"}
	env := map[string]string{}
	in := gateInput(t)
	in.AgentName = "pi-kimi"

	if err := applyApprovalBridge(&target, env, in); err != nil {
		t.Fatalf("applyApprovalBridge: %v", err)
	}
	if target.ApprovalExtensionPath == "" {
		t.Fatal("pi is gated by an extension file, which must exist on disk")
	}
	body, err := os.ReadFile(target.ApprovalExtensionPath)
	if err != nil {
		t.Fatalf("the extension pi is told to load must exist: %v", err)
	}
	if !strings.Contains(string(body), "tool_call") {
		t.Error("the extension must register the hook that gates a tool call")
	}
	if env[EnvApprovalBridgeBin] == "" {
		t.Error("the extension resolves the bridge from the environment, so it must be set")
	}

	_, args := BuildStageArgs(target, nil, "kernl-1", "/wt", "", "do it")
	if argAfter(args, "-e") != target.ApprovalExtensionPath {
		t.Errorf("pi must be told to load the extension, got %q", args)
	}
	if args[len(args)-1] != "do it" {
		t.Errorf("the prompt must stay the trailing positional, got %q", args)
	}
}

func TestGateCarriesTheRunContextToTheBridge(t *testing.T) {
	target := adapter.AgentTarget{Command: "claude", ApprovalMode: "prompt"}
	env := map[string]string{}
	in := gateInput(t)

	if err := applyApprovalBridge(&target, env, in); err != nil {
		t.Fatalf("applyApprovalBridge: %v", err)
	}
	for name, want := range map[string]string{
		adapter.EnvTerminalSessionID: "kernl-1-claude",
		adapter.EnvApprovalDir:       ApprovalStoreDir(in.StateDir),
		adapter.EnvApprovalBeadID:    "kernl-1",
		adapter.EnvApprovalRepoPath:  "/repo",
		adapter.EnvApprovalAgentName: "claude",
		adapter.EnvApprovalTimeout:   "30m0s",
	} {
		if env[name] != want {
			t.Errorf("%s = %q, want %q", name, env[name], want)
		}
	}
}

// The gate must refuse rather than dispatch an agent it cannot gate. Silently
// falling back to the bypass would leave the operator believing every tool call
// is being reviewed while none of them is.
func TestUnsupportedDialectUnderAGateIsRefused(t *testing.T) {
	for _, command := range []string{"codex", "opencode", "gemini", "copilot", "agy"} {
		t.Run(command, func(t *testing.T) {
			target := adapter.AgentTarget{Command: command, ApprovalMode: "prompt"}
			err := applyApprovalBridge(&target, map[string]string{}, gateInput(t))
			if err == nil {
				t.Fatalf("%s cannot raise a gate and must be refused", command)
			}
			for _, want := range []string{"KERNL DISPATCH FAILURE", command, "approvalMode"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("the error must mention %q, got %v", want, err)
				}
			}
		})
	}
}

func TestApprovalTimeoutReadsTheOperatorsDeadline(t *testing.T) {
	got, err := ApprovalTimeout(nil)
	if err != nil {
		t.Fatalf("nil config: %v", err)
	}
	if got != approvals.DefaultTimeout {
		t.Errorf("an unset deadline must default to %s, got %s", approvals.DefaultTimeout, got)
	}

	cfg := &config.Config{}
	cfg.Orchestrator.ApprovalTimeout = "2h"
	if got, err = ApprovalTimeout(cfg); err != nil || got != 2*time.Hour {
		t.Errorf("want 2h, got %s (%v)", got, err)
	}

	// "30" reads as 30 nanoseconds to a naive parser and as half an hour to
	// the operator who wrote it. Refusing is the only reading that cannot
	// silently expire every gate.
	for _, bad := range []string{"30", "half an hour", "0", "-5m"} {
		cfg.Orchestrator.ApprovalTimeout = bad
		if _, err := ApprovalTimeout(cfg); err == nil {
			t.Errorf("approvalTimeout %q must be refused", bad)
		}
	}
}

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func argAfter(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}
