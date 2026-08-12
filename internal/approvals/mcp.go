package approvals

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
)

// The MCP server kernl exposes to an agent that speaks the permission-prompt
// protocol (claude). The agent spawns this as a child process and calls the
// single tool below before every tool use it is not already allowed to make;
// the call blocks until a human answers, which is what turns a permission
// prompt into a judgment gate.
//
// Only the four messages an agent actually sends are implemented: initialize,
// notifications/initialized, tools/list and tools/call. Anything else gets a
// method-not-found error rather than an empty success, so a protocol change
// shows up as a refusal instead of a gate that silently stops gating.
const (
	mcpToolName       = "ask"
	mcpServerName     = "kernl_approval"
	mcpFallbackProto  = "2025-11-25"
	mcpMethodNotFound = -32601
)

// maxMCPLine bounds one JSON-RPC message. A permission payload carries the
// tool's full input, and a file write can be megabytes - bufio's 64KB default
// would truncate exactly the calls most worth gating.
const maxMCPLine = 16 << 20

type mcpMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpToolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// ServeMCP runs the bridge until stdin closes.
//
// Each tools/call is answered on its own goroutine: an agent making two tool
// calls in one turn would otherwise have the second gate queued behind the
// first human answer, which looks exactly like the bridge having hung.
func ServeMCP(ctx context.Context, s *Store, adapter string, rc RequestContext, in io.Reader, out io.Writer) error {
	w := &mcpWriter{out: out}
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), maxMCPLine)

	var wg sync.WaitGroup
	defer wg.Wait()

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var msg mcpMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			slog.Warn("[approval-bridge] mcp.unparseable_message", "error", err)
			continue
		}

		if msg.Method == "tools/call" {
			wg.Add(1)
			go func(msg mcpMessage) {
				defer wg.Done()
				w.reply(msg.ID, handleToolCall(ctx, s, adapter, rc, msg))
			}(msg)
			continue
		}
		if result, respond := handleControlMessage(msg); respond {
			w.reply(msg.ID, result)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: reading MCP input on the approval bridge: %w", err)
	}
	return nil
}

// handleControlMessage answers everything that is not a gate. The bool reports
// whether a reply is owed at all: a notification carries no id and must not be
// answered.
func handleControlMessage(msg mcpMessage) (any, bool) {
	if len(msg.ID) == 0 {
		return nil, false
	}
	switch msg.Method {
	case "initialize":
		return map[string]any{
			"protocolVersion": negotiatedProtocol(msg.Params),
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": mcpServerName, "version": "1"},
		}, true
	case "tools/list":
		return map[string]any{"tools": []any{toolDescriptor()}}, true
	case "ping":
		return map[string]any{}, true
	default:
		return mcpError{Code: mcpMethodNotFound, Message: "the kernl approval bridge implements initialize, tools/list and tools/call only, and was asked for " + msg.Method}, true
	}
}

// handleToolCall raises the gate and shapes the answer the way the permission
// protocol expects: a text content block holding a JSON verdict.
func handleToolCall(ctx context.Context, s *Store, adapter string, rc RequestContext, msg mcpMessage) any {
	var params mcpToolCallParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return mcpError{Code: mcpMethodNotFound, Message: "unreadable tools/call params: " + err.Error()}
	}
	if params.Name != mcpToolName {
		return mcpError{Code: mcpMethodNotFound, Message: fmt.Sprintf("the kernl approval bridge exposes only %q, and was asked for %q", mcpToolName, params.Name)}
	}

	outcome, err := Gate(ctx, s, adapter, params.Arguments, rc)
	if err != nil {
		// A gate that cannot be raised must not read as permission granted.
		slog.Error("[approval-bridge] gate.failed", "error", err)
		return verdictContent(map[string]any{
			"behavior": "deny",
			"message":  "kernl could not raise an approval gate for this call: " + err.Error(),
		})
	}
	if outcome.Allowed {
		input, _ := params.Arguments["input"].(map[string]any)
		if input == nil {
			input = map[string]any{}
		}
		return verdictContent(map[string]any{"behavior": "allow", "updatedInput": input})
	}
	return verdictContent(map[string]any{"behavior": "deny", "message": outcome.Reason})
}

func verdictContent(verdict map[string]any) any {
	// Marshal cannot fail for this shape, and an error here would have to be
	// reported as something - reporting it as a denial is the safe direction.
	data, err := json.Marshal(verdict)
	if err != nil {
		data = []byte(`{"behavior":"deny","message":"kernl could not encode the approval verdict"}`)
	}
	return map[string]any{
		"content": []any{map[string]any{"type": "text", "text": string(data)}},
	}
}

func toolDescriptor() map[string]any {
	return map[string]any{
		"name":        mcpToolName,
		"description": "Ask the operator for permission to run a tool. Blocks until answered.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"tool_name":   map[string]any{"type": "string"},
				"input":       map[string]any{"type": "object"},
				"tool_use_id": map[string]any{"type": "string"},
			},
			"required": []any{"tool_name", "input"},
		},
	}
}

// negotiatedProtocol echoes the client's protocol version. MCP expects the
// server to agree with a version it supports; echoing is what the measured
// clients accept, and inventing a version the client does not know ends the
// session before the first gate.
func negotiatedProtocol(params json.RawMessage) string {
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := json.Unmarshal(params, &p); err == nil && p.ProtocolVersion != "" {
		return p.ProtocolVersion
	}
	return mcpFallbackProto
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// mcpWriter serializes replies. Gates are answered on their own goroutines, so
// two responses could otherwise interleave mid-line and corrupt both.
type mcpWriter struct {
	mu  sync.Mutex
	out io.Writer
}

func (w *mcpWriter) reply(id json.RawMessage, payload any) {
	envelope := map[string]any{"jsonrpc": "2.0"}
	if len(id) > 0 {
		envelope["id"] = id
	}
	if e, isErr := payload.(mcpError); isErr {
		envelope["error"] = e
	} else {
		envelope["result"] = payload
	}

	data, err := json.Marshal(envelope)
	if err != nil {
		slog.Error("[approval-bridge] mcp.unencodable_reply", "error", err)
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, err := w.out.Write(append(data, '\n')); err != nil {
		slog.Error("[approval-bridge] mcp.write_failed", "error", err)
	}
}
