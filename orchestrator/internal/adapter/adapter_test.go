package adapter

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestResolveDialect(t *testing.T) {
	tests := []struct {
		command string
		want    AgentDialect
	}{
		{"claude", DialectClaude},
		{"claude-code", DialectClaude},
		{"/usr/local/bin/claude", DialectClaude},
		{"codex", DialectCodex},
		{"chatgpt", DialectCodex},
		{"/opt/openai/bin/codex", DialectCodex},
		{"copilot", DialectCopilot},
		{"github-copilot", DialectCopilot},
		{"opencode", DialectOpenCode},
		{"/home/user/.local/bin/opencode", DialectOpenCode},
		{"gemini", DialectGemini},
		{"gemini-cli", DialectGemini},
		{"unknown-binary", DialectClaude},
		{"", DialectClaude},
	}
	for _, tt := range tests {
		got := ResolveDialect(tt.command)
		if got != tt.want {
			t.Errorf("ResolveDialect(%q) = %q, want %q", tt.command, got, tt.want)
		}
	}
}

func TestClaudeNormalizer_Passthrough(t *testing.T) {
	n := CreateLineNormalizer(DialectClaude)
	input := `{"type":"result","result":"hello"}`
	result, ok := n(json.RawMessage(input))
	if !ok {
		t.Fatalf("expected ok for valid JSON object")
	}
	if result["type"] != "result" {
		t.Errorf("expected type=result, got %v", result["type"])
	}
}

func TestClaudeNormalizer_InvalidInput(t *testing.T) {
	n := CreateLineNormalizer(DialectClaude)
	_, ok := n(json.RawMessage(`"just a string"`))
	if ok {
		t.Error("expected non-object to be rejected")
	}
	_, ok = n(json.RawMessage(`null`))
	if ok {
		t.Error("expected null to be rejected")
	}
}

func TestClaudeNormalizer_ValidObjectPassesThrough(t *testing.T) {
	n := CreateLineNormalizer(DialectClaude)
	raw := `{"type":"assistant","message":{"content":[{"type":"text","text":"hi"}]}}`
	result, ok := n(json.RawMessage(raw))
	if !ok {
		t.Fatalf("expected ok")
	}
	if result["type"] != "assistant" {
		t.Errorf("expected type=assistant, got %v", result["type"])
	}
}

func TestGeminiNormalizer_SkipsInit(t *testing.T) {
	n := CreateLineNormalizer(DialectGemini)
	_, ok := n(json.RawMessage(`{"type":"init"}`))
	if ok {
		t.Error("init should be skipped")
	}
}

func TestGeminiNormalizer_SkipsUserMessage(t *testing.T) {
	n := CreateLineNormalizer(DialectGemini)
	_, ok := n(json.RawMessage(`{"type":"message","role":"user","content":"hello"}`))
	if ok {
		t.Error("user message should be skipped")
	}
}

func TestGeminiNormalizer_AssistantMessage(t *testing.T) {
	n := CreateLineNormalizer(DialectGemini)
	result, ok := n(json.RawMessage(`{"type":"message","role":"assistant","content":"hello world"}`))
	if !ok {
		t.Fatalf("expected ok")
	}
	if result["type"] != "assistant" {
		t.Errorf("expected type=assistant, got %v", result["type"])
	}
}

func TestGeminiNormalizer_ResultSuccess(t *testing.T) {
	n := CreateLineNormalizer(DialectGemini)
	n(json.RawMessage(`{"type":"message","role":"assistant","content":"accumulated"}`))
	result, ok := n(json.RawMessage(`{"type":"result","status":"success"}`))
	if !ok {
		t.Fatalf("expected ok")
	}
	if result["is_error"] != false {
		t.Errorf("expected is_error=false")
	}
	if result["result"] != "accumulated" {
		t.Errorf("expected accumulated text, got %v", result["result"])
	}
}

func TestGeminiNormalizer_ResultError(t *testing.T) {
	n := CreateLineNormalizer(DialectGemini)
	result, ok := n(json.RawMessage(`{"type":"result","status":"error"}`))
	if !ok {
		t.Fatalf("expected ok")
	}
	if result["is_error"] != true {
		t.Errorf("expected is_error=true")
	}
}

func TestOpenCodeNormalizer_StepStartSkipped(t *testing.T) {
	n := CreateLineNormalizer(DialectOpenCode)
	_, ok := n(json.RawMessage(`{"type":"step_start"}`))
	if ok {
		t.Error("step_start should be skipped")
	}
}

func TestOpenCodeNormalizer_TextEvent(t *testing.T) {
	n := CreateLineNormalizer(DialectOpenCode)
	result, ok := n(json.RawMessage(`{"type":"text","part":{"text":"hello"}}`))
	if !ok {
		t.Fatalf("expected ok")
	}
	if result["type"] != "assistant" {
		t.Errorf("expected type=assistant, got %v", result["type"])
	}
}

func TestOpenCodeNormalizer_StepFinishStop_Siltersskipped(t *testing.T) {
	n := CreateLineNormalizer(DialectOpenCode)
	_, ok := n(json.RawMessage(`{"type":"step_finish","part":{"reason":"stop"}}`))
	if ok {
		t.Error("step_finish with reason=stop should be skipped")
	}
}

func TestOpenCodeNormalizer_StepFinishError(t *testing.T) {
	n := CreateLineNormalizer(DialectOpenCode)
	result, ok := n(json.RawMessage(`{"type":"step_finish","part":{"reason":"error"}}`))
	if !ok {
		t.Fatalf("expected ok")
	}
	if result["is_error"] != true {
		t.Errorf("expected is_error=true")
	}
}

func TestOpenCodeNormalizer_SessionIdle(t *testing.T) {
	n := CreateLineNormalizer(DialectOpenCode)
	n(json.RawMessage(`{"type":"text","part":{"text":"hello"}}`))
	result, ok := n(json.RawMessage(`{"type":"session_idle"}`))
	if !ok {
		t.Fatalf("expected ok")
	}
	if result["type"] != "result" {
		t.Errorf("expected type=result, got %v", result["type"])
	}
	if result["is_error"] != false {
		t.Errorf("expected is_error=false")
	}
}

func TestOpenCodeNormalizer_SessionIdleResetsAccumulator(t *testing.T) {
	n := CreateLineNormalizer(DialectOpenCode)
	n(json.RawMessage(`{"type":"text","part":{"text":"first"}}`))
	result1, _ := n(json.RawMessage(`{"type":"session_idle"}`))
	if result1["result"] != "first" {
		t.Errorf("expected 'first', got %v", result1["result"])
	}
	n(json.RawMessage(`{"type":"text","part":{"text":"second"}}`))
	result2, _ := n(json.RawMessage(`{"type":"session_idle"}`))
	if result2["result"] != "second" {
		t.Errorf("expected 'second', got %v", result2["result"])
	}
}

func TestOpenCodeNormalizer_ToolUse(t *testing.T) {
	n := CreateLineNormalizer(DialectOpenCode)
	result, ok := n(json.RawMessage(`{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"}}`))
	if !ok {
		t.Fatalf("expected ok")
	}
	if result["type"] != "assistant" {
		t.Errorf("expected type=assistant")
	}
}

func TestOpenCodeNormalizer_SessionError(t *testing.T) {
	n := CreateLineNormalizer(DialectOpenCode)
	result, ok := n(json.RawMessage(`{"type":"session_error","message":"boom"}`))
	if !ok {
		t.Fatalf("expected ok")
	}
	if result["is_error"] != true {
		t.Errorf("expected is_error=true")
	}
}

func TestOpenCodeNormalizer_ReasoningNotEmpty(t *testing.T) {
	n := CreateLineNormalizer(DialectOpenCode)
	result, ok := n(json.RawMessage(`{"type":"reasoning","text":"thinking..."}`))
	if !ok {
		t.Fatalf("expected ok")
	}
	if result["type"] != "stream_event" {
		t.Errorf("expected type=stream_event, got %v", result["type"])
	}
}

func TestOpenCodeNormalizer_ReasoningEmptySkipped(t *testing.T) {
	n := CreateLineNormalizer(DialectOpenCode)
	_, ok := n(json.RawMessage(`{"type":"reasoning","text":""}`))
	if ok {
		t.Error("empty reasoning should be skipped")
	}
}

func TestCopilotNormalizer_MessageDelta(t *testing.T) {
	n := CreateLineNormalizer(DialectCopilot)
	result, ok := n(json.RawMessage(`{"type":"assistant.message_delta","data":{"messageId":"m1","deltaContent":"hello"}}`))
	if !ok {
		t.Fatalf("expected ok")
	}
	if result["type"] != "stream_event" {
		t.Errorf("expected type=stream_event, got %v", result["type"])
	}
}

func TestCopilotNormalizer_TaskComplete(t *testing.T) {
	n := CreateLineNormalizer(DialectCopilot)
	n(json.RawMessage(`{"type":"assistant.message_delta","data":{"deltaContent":"hello"}}`))
	result, ok := n(json.RawMessage(`{"type":"session.task_complete","data":{"summary":"done","success":true}}`))
	if !ok {
		t.Fatalf("expected ok")
	}
	if result["is_error"] != false {
		t.Errorf("expected is_error=false")
	}
}

func TestCopilotNormalizer_SessionError(t *testing.T) {
	n := CreateLineNormalizer(DialectCopilot)
	result, ok := n(json.RawMessage(`{"type":"session.error","data":{"message":"crash"}}`))
	if !ok {
		t.Fatalf("expected ok")
	}
	if result["is_error"] != true {
		t.Errorf("expected is_error=true")
	}
	if result["result"] != "crash" {
		t.Errorf("expected result=crash, got %v", result["result"])
	}
}

func TestCopilotNormalizer_UserInputRequested(t *testing.T) {
	n := CreateLineNormalizer(DialectCopilot)
	result, ok := n(json.RawMessage(`{"type":"user_input.requested","data":{"question":"Continue?","choices":["yes","no"],"toolCallId":"tc1"}}`))
	if !ok {
		t.Fatalf("expected ok")
	}
	if result["type"] != "assistant" {
		t.Errorf("expected type=assistant, got %v", result["type"])
	}
}

func TestCopilotNormalizer_EmptyDeltaSkipped(t *testing.T) {
	n := CreateLineNormalizer(DialectCopilot)
	_, ok := n(json.RawMessage(`{"type":"assistant.message_delta","data":{"deltaContent":""}}`))
	if ok {
		t.Error("empty delta should be skipped")
	}
}

func TestCodexNormalizer_SkipsThreadTurnStarted(t *testing.T) {
	n := CreateLineNormalizer(DialectCodex)
	for _, typ := range []string{"thread.started", "turn.started"} {
		_, ok := n(json.RawMessage(`{"type":"` + typ + `"}`))
		if ok {
			t.Errorf("%s should be skipped", typ)
		}
	}
}

func TestCodexNormalizer_ItemCompletedAgentMessage(t *testing.T) {
	n := CreateLineNormalizer(DialectCodex)
	result, ok := n(json.RawMessage(`{"type":"item.completed","item":{"type":"agent_message","text":"hello"}}`))
	if !ok {
		t.Fatalf("expected ok")
	}
	if result["type"] != "assistant" {
		t.Errorf("expected type=assistant, got %v", result["type"])
	}
}

func TestCodexNormalizer_ItemStartedCommandExecution(t *testing.T) {
	n := CreateLineNormalizer(DialectCodex)
	result, ok := n(json.RawMessage(`{"type":"item.started","item":{"type":"command_execution","command":"ls -la"}}`))
	if !ok {
		t.Fatalf("expected ok")
	}
	if result["type"] != "assistant" {
		t.Errorf("expected type=assistant, got %v", result["type"])
	}
}

func TestCodexNormalizer_TurnCompleted(t *testing.T) {
	n := CreateLineNormalizer(DialectCodex)
	n(json.RawMessage(`{"type":"item.completed","item":{"type":"agent_message","text":"accumulated"}}`))
	result, ok := n(json.RawMessage(`{"type":"turn.completed","status":"completed"}`))
	if !ok {
		t.Fatalf("expected ok")
	}
	if result["type"] != "result" {
		t.Errorf("expected type=result, got %v", result["type"])
	}
	if result["is_error"] != false {
		t.Errorf("expected is_error=false")
	}
}

func TestCodexNormalizer_TurnFailed(t *testing.T) {
	n := CreateLineNormalizer(DialectCodex)
	result, ok := n(json.RawMessage(`{"type":"turn.failed","error":{"message":"bad"}}`))
	if !ok {
		t.Fatalf("expected ok")
	}
	if result["is_error"] != true {
		t.Errorf("expected is_error=true")
	}
	if result["result"] != "bad" {
		t.Errorf("expected result=bad, got %v", result["result"])
	}
}

func TestCodexNormalizer_TurnCompletedFailed(t *testing.T) {
	n := CreateLineNormalizer(DialectCodex)
	result, ok := n(json.RawMessage(`{"type":"turn.completed","status":"failed","error":{"message":"oops"}}`))
	if !ok {
		t.Fatalf("expected ok")
	}
	if result["is_error"] != true {
		t.Errorf("expected is_error=true")
	}
}

func TestCodexNormalizer_Error(t *testing.T) {
	n := CreateLineNormalizer(DialectCodex)
	result, ok := n(json.RawMessage(`{"type":"error","message":"something broke"}`))
	if !ok {
		t.Fatalf("expected ok")
	}
	if result["is_error"] != true {
		t.Errorf("expected is_error=true")
	}
}

func TestCodexNormalizer_Reasoning(t *testing.T) {
	n := CreateLineNormalizer(DialectCodex)
	result, ok := n(json.RawMessage(`{"type":"item.completed","item":{"type":"reasoning","text":"thinking"}}`))
	if !ok {
		t.Fatalf("expected ok")
	}
	if result["type"] != "stream_event" {
		t.Errorf("expected type=stream_event, got %v", result["type"])
	}
}

func TestCodexNormalizer_CommandExecutionCompleted(t *testing.T) {
	n := CreateLineNormalizer(DialectCodex)
	result, ok := n(json.RawMessage(`{"type":"item.completed","item":{"type":"command_execution","aggregated_output":"file1\nfile2"}}`))
	if !ok {
		t.Fatalf("expected ok")
	}
	if result["type"] != "user" {
		t.Errorf("expected type=user, got %v", result["type"])
	}
}

func TestCodexNormalizer_UnknownTypeSkipped(t *testing.T) {
	n := CreateLineNormalizer(DialectCodex)
	_, ok := n(json.RawMessage(`{"type":"mcpServer/startupStatus/updated"}`))
	if ok {
		t.Error("unknown Codex type should be skipped")
	}
}

func TestBuildPromptModeArgs_Claude(t *testing.T) {
	agent := AgentTarget{Command: "claude", ApprovalMode: "bypass"}
	args := BuildPromptModeArgs(agent, "do something")
	if args.Command != "claude" {
		t.Errorf("expected command=claude, got %s", args.Command)
	}
	found := false
	for _, a := range args.Args {
		if a == "--dangerously-skip-permissions" {
			found = true
		}
	}
	if !found {
		t.Error("expected --dangerously-skip-permissions in args")
	}
}

func TestBuildPromptModeArgs_Codex(t *testing.T) {
	agent := AgentTarget{Command: "codex"}
	args := BuildPromptModeArgs(agent, "test prompt")
	if args.Command != "codex" {
		t.Errorf("expected command=codex, got %s", args.Command)
	}
	hasExec := false
	for _, a := range args.Args {
		if a == "exec" {
			hasExec = true
		}
	}
	if !hasExec {
		t.Error("expected 'exec' in args")
	}
}

func TestBuildPromptModeArgs_OpenCode(t *testing.T) {
	agent := AgentTarget{Command: "opencode"}
	args := BuildPromptModeArgs(agent, "test prompt")
	if args.Command != "opencode" {
		t.Errorf("expected command=opencode")
	}
	hasRun := false
	for _, a := range args.Args {
		if a == "run" {
			hasRun = true
		}
	}
	if !hasRun {
		t.Error("expected 'run' in args")
	}
}

func TestBuildInteractiveArgs_Codex(t *testing.T) {
	agent := AgentTarget{Command: "codex"}
	args := BuildInteractiveArgs(agent)
	hasAppServer := false
	for _, a := range args.Args {
		if a == "app-server" {
			hasAppServer = true
		}
	}
	if !hasAppServer {
		t.Error("expected 'app-server' in interactive codex args")
	}
}

func TestBuildInteractiveArgs_Copilot(t *testing.T) {
	agent := AgentTarget{Command: "copilot"}
	args := BuildInteractiveArgs(agent)
	hasSession := false
	for _, a := range args.Args {
		if a == "--session" {
			hasSession = true
		}
	}
	if !hasSession {
		t.Error("expected '--session' in interactive copilot args")
	}
}

func TestBuildInteractiveArgs_OpenCode(t *testing.T) {
	agent := AgentTarget{Command: "opencode"}
	args := BuildInteractiveArgs(agent)
	hasServe := false
	for _, a := range args.Args {
		if a == "serve" {
			hasServe = true
		}
	}
	if !hasServe {
		t.Error("expected 'serve' in interactive opencode args")
	}
}

func TestBuildInteractiveArgs_Gemini(t *testing.T) {
	agent := AgentTarget{Command: "gemini"}
	args := BuildInteractiveArgs(agent)
	hasACP := false
	for _, a := range args.Args {
		if a == "--acp" {
			hasACP = true
		}
	}
	if !hasACP {
		t.Error("expected '--acp' in interactive gemini args")
	}
}

func TestShouldBypassClaudePermissions(t *testing.T) {
	if !ShouldBypassClaudePermissions(AgentTarget{ApprovalMode: "bypass"}) {
		t.Error("expected bypass for non-prompt mode")
	}
	if ShouldBypassClaudePermissions(AgentTarget{ApprovalMode: "prompt"}) {
		t.Error("expected no bypass for prompt mode")
	}
}

func TestParseModelSelection(t *testing.T) {
	provider, model, err := parseModelSelection("openrouter/z-ai/glm-5.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider != "openrouter" {
		t.Errorf("expected provider=openrouter, got %s", provider)
	}
	if model != "z-ai/glm-5.1" {
		t.Errorf("expected model=z-ai/glm-5.1, got %s", model)
	}
}

func TestParseModelSelection_Empty(t *testing.T) {
	_, _, err := parseModelSelection("")
	if err == nil {
		t.Error("expected error for empty string")
	}
}

func TestParseModelSelection_NoSlash(t *testing.T) {
	_, _, err := parseModelSelection("just-a-model")
	if err == nil {
		t.Error("expected error for missing provider slash")
	}
}

func TestClaudeInteractiveArgs_BypassByDefault(t *testing.T) {
	agent := AgentTarget{Command: "claude", Model: "sonnet"}
	args := BuildClaudeInteractiveArgs(agent)
	foundBypass := false
	for _, a := range args.Args {
		if a == "--dangerously-skip-permissions" {
			foundBypass = true
		}
	}
	if !foundBypass {
		t.Error("expected --dangerously-skip-permissions for default approval mode")
	}
	foundModel := false
	for i, a := range args.Args {
		if a == "--model" && i+1 < len(args.Args) && args.Args[i+1] == "sonnet" {
			foundModel = true
		}
	}
	if !foundModel {
		t.Error("expected --model sonnet")
	}
}

func TestClaudeInteractiveArgs_PromptMode(t *testing.T) {
	agent := AgentTarget{Command: "claude", Model: "sonnet", ApprovalMode: "prompt"}
	args := BuildClaudeInteractiveArgsWithBridge(agent, "/path/to/bridge.mjs")
	for _, a := range args.Args {
		if a == "--dangerously-skip-permissions" {
			t.Error("should not have --dangerously-skip-permissions in prompt mode")
		}
	}
	assertHasPair(t, args.Args, "--permission-mode", "default")
	assertHasPair(t, args.Args, "--setting-sources", "project")
	assertHasPair(t, args.Args, "--strict-mcp-config", "")
	assertHasPair(t, args.Args, "--permission-prompt-tool", ClaudeApprovalPromptTool)
	foundMCPConfig := false
	for i, a := range args.Args {
		if a == "--mcp-config" && i+1 < len(args.Args) {
			foundMCPConfig = true
			var cfg map[string]any
			if err := json.Unmarshal([]byte(args.Args[i+1]), &cfg); err != nil {
				t.Fatalf("mcp-config should be valid JSON: %v", err)
			}
			if _, ok := cfg["mcpServers"]; !ok {
				t.Error("mcp-config should have mcpServers key")
			}
		}
	}
	if !foundMCPConfig {
		t.Error("expected --mcp-config in prompt mode args")
	}
}

func TestClaudePromptModeArgs_PromptMode(t *testing.T) {
	agent := AgentTarget{Command: "claude", ApprovalMode: "prompt"}
	args := BuildClaudePromptModeArgsWithBridge(agent, "do something", "/path/to/bridge.mjs")
	for _, a := range args.Args {
		if a == "--dangerously-skip-permissions" {
			t.Error("should not have --dangerously-skip-permissions in prompt mode")
		}
	}
	assertHasPair(t, args.Args, "--permission-mode", "default")
	assertHasPair(t, args.Args, "--setting-sources", "project")
	assertHasPair(t, args.Args, "--permission-prompt-tool", ClaudeApprovalPromptTool)
}

func TestClaudeApprovalBridgeMCPConfig(t *testing.T) {
	cfg := ClaudeApprovalBridgeMCPConfig("/path/to/bridge.mjs")
	var parsed map[string]any
	if err := json.Unmarshal([]byte(cfg), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	servers, ok := parsed["mcpServers"].(map[string]any)
	if !ok {
		t.Fatal("expected mcpServers to be an object")
	}
	server, ok := servers[ClaudeApprovalMCPServer].(map[string]any)
	if !ok {
		t.Fatal("expected foolery_approval server config")
	}
	cmd, _ := server["command"].(string)
	if cmd != "/path/to/bridge.mjs" {
		t.Errorf("expected command=/path/to/bridge.mjs, got %s", cmd)
	}
}

func TestApprovalBridgeEnvVars(t *testing.T) {
	env := ApprovalBridgeEnvVars("session-123", "http://localhost:3000", "token-abc")
	if env[EnvTerminalSessionID] != "session-123" {
		t.Errorf("expected session-123, got %s", env[EnvTerminalSessionID])
	}
	if env[EnvApprovalBridgeBaseURL] != "http://localhost:3000" {
		t.Errorf("expected http://localhost:3000, got %s", env[EnvApprovalBridgeBaseURL])
	}
	if env[EnvApprovalBridgeToken] != "token-abc" {
		t.Errorf("expected token-abc, got %s", env[EnvApprovalBridgeToken])
	}
	envNoToken := ApprovalBridgeEnvVars("s1", "http://localhost:3000", "")
	if _, ok := envNoToken[EnvApprovalBridgeToken]; ok {
		t.Error("token should not be set when empty")
	}
}

func TestBuildAgentArgs_Interactive_Success(t *testing.T) {
	agent := AgentTarget{Command: "claude"}
	cmd, args, err := BuildAgentArgs(agent, DialectClaude, DispatchKindTake, true, false, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd != "claude" {
		t.Errorf("expected cmd=claude, got %s", cmd)
	}
	_ = args
}

func TestBuildAgentArgs_JSONRPC_Codex(t *testing.T) {
	agent := AgentTarget{Command: "codex", Model: "gpt-5"}
	cmd, args, err := BuildAgentArgs(agent, DialectCodex, DispatchKindTake, true, true, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd != "codex" {
		t.Errorf("expected cmd=codex, got %s", cmd)
	}
	foundModel := false
	for i, a := range args {
		if a == "-c" && i+1 < len(args) && strings.Contains(args[i+1], "gpt-5") {
			foundModel = true
		}
	}
	if !foundModel {
		t.Error("expected model config in codex args")
	}
}

func TestBuildAgentArgs_HTTPServer_OpenCode(t *testing.T) {
	agent := AgentTarget{Command: "opencode"}
	cmd, args, err := BuildAgentArgs(agent, DialectOpenCode, DispatchKindTake, true, false, true, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd != "opencode" {
		t.Errorf("expected cmd=opencode, got %s", cmd)
	}
	hasServe := false
	for _, a := range args {
		if a == "serve" {
			hasServe = true
		}
	}
	if !hasServe {
		t.Error("expected 'serve' in opencode args")
	}
}

func TestBuildAgentArgs_ACP_Gemini(t *testing.T) {
	agent := AgentTarget{Command: "gemini"}
	cmd, _, err := BuildAgentArgs(agent, DialectGemini, DispatchKindTake, true, false, false, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd != "gemini" {
		t.Errorf("expected cmd=gemini, got %s", cmd)
	}
}

func TestBuildAgentArgs_OneShotForbidden(t *testing.T) {
	agent := AgentTarget{Command: "opencode"}
	_, _, err := BuildAgentArgs(agent, DialectOpenCode, DispatchKindTake, false, false, false, false)
	if err == nil {
		t.Fatal("expected error for one-shot cli-arg dispatch")
	}
	if !strings.Contains(err.Error(), TerminalDispatchFailureMarker) {
		t.Errorf("error should contain TERMINAL_DISPATCH_FAILURE_MARKER, got: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "cli-arg") {
		t.Errorf("error should mention cli-arg transport, got: %s", err.Error())
	}
}

func TestBuildSpawnArgs_OneShotForbidden(t *testing.T) {
	agent := AgentTarget{Command: "codex"}
	_, _, err := BuildSpawnArgs(agent, DialectCodex, DispatchKindScene, false, false, false, false)
	if err == nil {
		t.Fatal("expected error for one-shot cli-arg dispatch")
	}
	if !strings.Contains(err.Error(), TerminalDispatchFailureMarker) {
		t.Errorf("error should contain TERMINAL_DISPATCH_FAILURE_MARKER, got: %s", err.Error())
	}
}

func TestBuildSpawnArgs_Interactive_Success(t *testing.T) {
	agent := AgentTarget{Command: "claude"}
	cmd, _, err := BuildSpawnArgs(agent, DialectClaude, DispatchKindTake, true, false, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd != "claude" {
		t.Errorf("expected cmd=claude, got %s", cmd)
	}
}

func TestTerminalDispatchKind(t *testing.T) {
	if DispatchKindFromParent(true) != DispatchKindScene {
		t.Error("expected scene for effectiveParent=true")
	}
	if DispatchKindFromParent(false) != DispatchKindTake {
		t.Error("expected take for effectiveParent=false")
	}
}

func TestFormatTakeSceneOneShotFailure(t *testing.T) {
	msg := FormatTakeSceneOneShotFailure(DialectOpenCode, DispatchKindTake, "cli-arg")
	if !strings.Contains(msg, TerminalDispatchFailureMarker) {
		t.Error("should contain TERMINAL_DISPATCH_FAILURE_MARKER")
	}
	if !strings.Contains(msg, "opencode") {
		t.Error("should mention dialect")
	}
	if !strings.Contains(msg, "take") {
		t.Error("should mention dispatch kind")
	}
	if !strings.Contains(msg, "cli-arg") {
		t.Error("should mention transport")
	}
}

func TestAssertTakeSceneInteractiveCapabilities_Success(t *testing.T) {
	err := AssertTakeSceneInteractiveCapabilities(DialectClaude, DispatchKindTake, true, "stdin-stream-json")
	if err != nil {
		t.Errorf("interactive with non-cli-arg transport should succeed, got: %v", err)
	}
}

func TestAssertTakeSceneInteractiveCapabilities_Fail(t *testing.T) {
	err := AssertTakeSceneInteractiveCapabilities(DialectOpenCode, DispatchKindTake, false, "cli-arg")
	if err == nil {
		t.Error("expected error for non-interactive cli-arg transport")
	}
}

func TestResolveInteractiveTransport(t *testing.T) {
	tests := []struct {
		dialect      AgentDialect
		interactive  bool
		wantTransport string
	}{
		{DialectClaude, true, "stdin-stream-json"},
		{DialectClaude, false, "cli-arg"},
		{DialectCodex, true, "jsonrpc-stdio"},
		{DialectCopilot, true, "stdin-stream-json"},
		{DialectOpenCode, true, "http-server"},
		{DialectGemini, true, "acp-stdio"},
	}
	for _, tt := range tests {
		got := ResolveInteractiveTransport(tt.dialect, tt.interactive)
		if got != tt.wantTransport {
			t.Errorf("ResolveInteractiveTransport(%s, %v) = %q, want %q", tt.dialect, tt.interactive, got, tt.wantTransport)
		}
	}
}

func TestSupportsInteractive(t *testing.T) {
	for _, d := range []AgentDialect{DialectClaude, DialectCodex, DialectCopilot, DialectOpenCode, DialectGemini} {
		if !SupportsInteractive(d) {
			t.Errorf("expected %s to support interactive", d)
		}
	}
}

func assertHasPair(t *testing.T, args []string, flag, value string) {
	t.Helper()
	for i, a := range args {
		if a == flag {
			if value == "" {
				return
			}
			if i+1 < len(args) && args[i+1] == value {
				return
			}
		}
	}
	if value == "" {
		t.Errorf("expected flag %s in args", flag)
	} else {
		t.Errorf("expected %s %s in args", flag, value)
	}
}

func TestCodexNormalizer_PlanFinalPreserved(t *testing.T) {
	n := CreateLineNormalizer(DialectCodex)
	planLine := `{"event":"plan_final","plan":{"summary":"Test","waves":[],"assumptions":[]}}`
	input := map[string]any{
		"type": "item.completed",
		"item": map[string]any{
			"id":   "msg_1",
			"type": "agent_message",
			"text": planLine,
		},
	}
	raw, _ := json.Marshal(input)
	result, ok := n(json.RawMessage(raw))
	if !ok {
		t.Fatal("expected ok")
	}
	if result["type"] != "assistant" {
		t.Errorf("expected type=assistant, got %v", result["type"])
	}
	msg := result["message"].(map[string]any)
	content := msg["content"].([]any)
	text := content[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "plan_final") {
		t.Errorf("expected plan_final in text, got %s", text)
	}
}

func TestCodexNormalizer_MultipleMessagesAccumulate(t *testing.T) {
	n := CreateLineNormalizer(DialectCodex)
	msg1 := map[string]any{
		"type": "item.completed",
		"item": map[string]any{"id": "msg_1", "type": "agent_message", "text": "First message"},
	}
	msg2 := map[string]any{
		"type": "item.completed",
		"item": map[string]any{"id": "msg_2", "type": "agent_message", "text": "Second message"},
	}
	msg3 := map[string]any{
		"type": "item.completed",
		"item": map[string]any{"id": "msg_3", "type": "agent_message", "text": "Third message"},
	}
	raw1, _ := json.Marshal(msg1)
	raw2, _ := json.Marshal(msg2)
	raw3, _ := json.Marshal(msg3)
	n(json.RawMessage(raw1))
	n(json.RawMessage(raw2))
	n(json.RawMessage(raw3))

	result, ok := n(json.RawMessage(`{"type":"turn.completed","usage":{}}`))
	if !ok {
		t.Fatal("expected ok")
	}
	if result["type"] != "result" {
		t.Errorf("expected type=result, got %v", result["type"])
	}
	resultText := result["result"].(string)
	if !strings.Contains(resultText, "First message") {
		t.Error("expected accumulated text to contain 'First message'")
	}
	if !strings.Contains(resultText, "Second message") {
		t.Error("expected accumulated text to contain 'Second message'")
	}
	if !strings.Contains(resultText, "Third message") {
		t.Error("expected accumulated text to contain 'Third message'")
	}
}

func TestCodexNormalizer_TaggedPlanJsonPreserved(t *testing.T) {
	n := CreateLineNormalizer(DialectCodex)
	taggedPlan := "<plan_json>\n{\"summary\":\"Test\"}\n</plan_json>"
	input := map[string]any{
		"type": "item.completed",
		"item": map[string]any{
			"id":   "msg_1",
			"type": "agent_message",
			"text": taggedPlan,
		},
	}
	raw, _ := json.Marshal(input)
	n(json.RawMessage(raw))

	result, ok := n(json.RawMessage(`{"type":"turn.completed","usage":{}}`))
	if !ok {
		t.Fatal("expected ok")
	}
	resultText := result["result"].(string)
	if !strings.Contains(resultText, "<plan_json>") {
		t.Error("expected <plan_json> in accumulated text")
	}
	if !strings.Contains(resultText, "</plan_json>") {
		t.Error("expected </plan_json> in accumulated text")
	}
}

func TestCodexNormalizer_WaveDraftPreserved(t *testing.T) {
	n := CreateLineNormalizer(DialectCodex)
	waveDraft := `{"event":"wave_draft","wave":{"wave_index":1,"name":"Foundation"}}`
	input := map[string]any{
		"type": "item.completed",
		"item": map[string]any{
			"id":   "msg_1",
			"type": "agent_message",
			"text": waveDraft,
		},
	}
	raw, _ := json.Marshal(input)
	result, ok := n(json.RawMessage(raw))
	if !ok {
		t.Fatal("expected ok")
	}
	msg := result["message"].(map[string]any)
	content := msg["content"].([]any)
	text := content[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "wave_draft") {
		t.Errorf("expected wave_draft in text, got %s", text)
	}
}

func TestClaudeNormalizer_IdempotentPassThrough(t *testing.T) {
	n := CreateLineNormalizer(DialectClaude)
	event := map[string]any{
		"type":    "assistant",
		"message": map[string]any{"content": []any{map[string]any{"type": "text", "text": "some plan content"}}},
	}
	raw, _ := json.Marshal(event)
	result1, ok1 := n(json.RawMessage(raw))
	if !ok1 {
		t.Fatal("expected ok")
	}
	raw2, _ := json.Marshal(event)
	result2, ok2 := n(json.RawMessage(raw2))
	if !ok2 {
		t.Fatal("expected ok on second call")
	}
	if result1["type"] != result2["type"] {
		t.Error("Claude normalizer should be idempotent")
	}
}