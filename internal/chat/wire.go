package chat

import (
	"time"

	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

// The wire shapes below are kernl's own camelCase contract for a chat message
// and a pending permission. They exist as separate types because the node
// structs they mirror are the STORAGE format: nodes.ChatMessage and
// nodes.PendingPermissionState are marshalled into a node's `attrs` column
// under their snake_case tags and read straight back, so retagging them to suit
// the API would make every stored session load with zeroed fields.
//
// They live in this package rather than in internal/api because both surfaces
// need them and only one direction of import exists: internal/api imports
// internal/chat for the engine, so a type declared in internal/api is
// unreachable from the SSE events the engine emits. One definition, shared,
// beats two copies drifting apart.

// WireLearnedCandidate is the wire form of a DA-proposed durable memory.
type WireLearnedCandidate struct {
	Subject   string `json:"subject"`
	Statement string `json:"statement"`
}

// WireMessage is the wire form of a single chat message.
type WireMessage struct {
	Role             string                `json:"role"`
	Content          string                `json:"content"`
	Timestamp        time.Time             `json:"timestamp"`
	LearnedCandidate *WireLearnedCandidate `json:"learnedCandidate,omitempty"`
}

// WirePendingPermission is the wire form of a pending tool-call permission.
type WirePendingPermission struct {
	ToolCallID        string    `json:"toolCallId"`
	RequestedNodeID   string    `json:"requestedNodeId"`
	RequestedNodePath string    `json:"requestedNodePath"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"createdAt"`
}

// NewWireMessages converts stored messages to their wire form. An empty slice
// stays an empty slice so the JSON is [] rather than null.
func NewWireMessages(messages []nodes.ChatMessage) []WireMessage {
	out := make([]WireMessage, 0, len(messages))
	for _, m := range messages {
		out = append(out, newWireMessage(m))
	}
	return out
}

func newWireMessage(m nodes.ChatMessage) WireMessage {
	wm := WireMessage{Role: m.Role, Content: m.Content, Timestamp: m.Timestamp}
	if m.LearnedCandidate != nil {
		wm.LearnedCandidate = &WireLearnedCandidate{
			Subject:   m.LearnedCandidate.Subject,
			Statement: m.LearnedCandidate.Statement,
		}
	}
	return wm
}

// NewWirePendingPermission converts a stored pending permission to its wire
// form, preserving nil so the key can be omitted entirely.
func NewWirePendingPermission(pp *nodes.PendingPermissionState) *WirePendingPermission {
	if pp == nil {
		return nil
	}
	return &WirePendingPermission{
		ToolCallID:        pp.ToolCallID,
		RequestedNodeID:   pp.RequestedNodeID,
		RequestedNodePath: pp.RequestedNodePath,
		Status:            pp.Status,
		CreatedAt:         pp.CreatedAt,
	}
}
