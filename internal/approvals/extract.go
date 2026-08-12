package approvals

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ApprovalRequest is one permission prompt raised by a dispatched agent and
// parked until a human answers it. It is both the on-disk record and the wire
// shape the REST API serves, so a field renamed here changes what `kernl
// approval list` and the GUI read.
type ApprovalRequest struct {
	ID               string   `json:"id"`
	NotificationKey  string   `json:"notificationKey"`
	Status           string   `json:"status"`
	CreatedAt        string   `json:"createdAt"`
	UpdatedAt        string   `json:"updatedAt"`
	RepoPath         string   `json:"repoPath"`
	BeadID           string   `json:"beadId"`
	SessionID        string   `json:"sessionId"`
	Adapter          string   `json:"adapter"`
	Source           string   `json:"source"`
	ToolName         string   `json:"toolName"`
	SupportedActions []string `json:"supportedActions"`
	Actionable       bool     `json:"actionable"`

	// AgentName is the settings.agents key that was dispatched, which is what
	// an operator recognises - the adapter alone only says which dialect spoke.
	AgentName string `json:"agentName,omitempty"`

	// ToolCallID is the agent's own identifier for the call being gated
	// (claude's tool_use_id, pi's toolCallId). Carried so a decision can be
	// correlated back to the agent's transcript.
	ToolCallID string `json:"toolCallId,omitempty"`

	// ToolInput is the tool's arguments verbatim. claude's permission protocol
	// wants them echoed back on an allow, and a human cannot judge "may this
	// run" without seeing what "this" is.
	ToolInput map[string]any `json:"toolInput,omitempty"`

	// ParameterSummary is the one line a listing prints: the command for a
	// shell tool, the path for a file tool.
	ParameterSummary string `json:"parameterSummary,omitempty"`

	// ExpiresAt is when an unanswered request stops being answerable. It is
	// stamped at creation rather than enforced by a sweeper so that a record
	// left behind by a killed bridge still reads as expired instead of
	// pending forever.
	ExpiresAt string `json:"expiresAt,omitempty"`

	// DecidedAt and Reason are filled in from the decision file once one
	// exists, so a single object answers both "what was asked" and "what was
	// answered".
	DecidedAt string `json:"decidedAt,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

type ApprovalFilter struct {
	RepoPath     string
	ActiveOnly   bool
	Status       string
	UpdatedSince string
}

// SessionActions is what a pending request advertises it can be answered with.
// Remembering a decision is the bridge's job rather than the agent's, so the
// list does not vary by adapter - every gate kernl raises can be allowed,
// refused, or allowed-and-remembered.
var SessionActions = []string{"accept", "always_approve", "decline"}

// ExtractApprovalRequest turns one adapter's permission payload into a request.
//
// The two supported adapters name the same three things differently: claude's
// MCP permission tool sends tool_name/input/tool_use_id, and the pi extension
// shim sends toolName/input/toolCallId. Anything else is refused by name -
// guessing a field layout produces a request whose tool nobody can identify,
// which is worse than not gating at all.
func ExtractApprovalRequest(adapter string, raw map[string]any) (*ApprovalRequest, error) {
	var toolKey, callKey string
	switch adapter {
	case "claude":
		toolKey, callKey = "tool_name", "tool_use_id"
	case "pi":
		toolKey, callKey = "toolName", "toolCallId"
	default:
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: no approval payload layout for adapter %q - supported: claude, pi - Fix: dispatch this agent with approvalMode: auto, or add the adapter's layout to approvals.ExtractApprovalRequest", adapter)
	}

	toolName, _ := raw[toolKey].(string)
	if toolName == "" {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: approval payload from adapter %s carries no %s, so the gate cannot say which tool is asking - Fix: check the bridge is sending the adapter's own permission payload unmodified", adapter, toolKey)
	}
	input, _ := raw["input"].(map[string]any)
	callID, _ := raw[callKey].(string)

	return &ApprovalRequest{
		Adapter:          adapter,
		Source:           "tool_permission",
		ToolName:         toolName,
		ToolCallID:       callID,
		ToolInput:        input,
		ParameterSummary: SummarizeToolInput(input),
		SupportedActions: append([]string(nil), SessionActions...),
	}, nil
}

// SummarizeToolInput picks the one value that identifies what a tool call will
// do. The keys are checked most-specific first because a tool carrying both a
// command and a path (a shell tool given a working directory) is identified by
// the command, never by the directory.
func SummarizeToolInput(input map[string]any) string {
	if len(input) == 0 {
		return ""
	}
	for _, key := range []string{"command", "file_path", "path", "url", "pattern", "query"} {
		if v, ok := input[key].(string); ok && v != "" {
			return truncateSummary(v)
		}
	}
	// No known key: show the payload itself rather than nothing, with the
	// keys sorted so the same call always prints the same line.
	keys := make([]string, 0, len(input))
	for k := range input {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		v, err := json.Marshal(input[k])
		if err != nil {
			continue
		}
		parts = append(parts, k+"="+string(v))
	}
	return truncateSummary(strings.Join(parts, " "))
}

func truncateSummary(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	const max = 120
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

func BuildApprovalLogicalKey(approval *ApprovalRequest) string {
	return approval.SessionID + ":" + approval.BeadID + ":" + approval.ToolName
}

func FormatApprovalRequestBanner(approval *ApprovalRequest) string {
	return "KERNL APPROVAL REQUIRED tool=" + approval.ToolName + " bead=" + approval.BeadID
}
