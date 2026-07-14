package nodes

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/graph"
)

// TypeChatSession is the node type constant for chat sessions.
const TypeChatSession = "chat_session"

// ChatSession represents a chat conversation node.
type ChatSession struct {
	ID                 string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	Messages           []ChatMessage
	PendingPermission  *PendingPermissionState
	DerivedScopeNodeID string
	// DiscardedCandidates records learned-memory candidate statements the user
	// rejected, so the same candidate is not re-proposed later in the session
	// (the discard "negative signal" in the DA-learned Keep/Edit/Discard flow).
	DiscardedCandidates []string
	// DraftRouting is the routing the user currently has on screen while triaging
	// a capture, rendered for the prompt. The chat is write-then-stream — the LLM
	// runs from the persisted session, not from the request — so a draft that
	// lives only in the browser never reaches the DA, and it would argue about a
	// routing the user has already changed.
	DraftRouting string
}

// LearnedCandidateState represents a durable memory extracted from the chat.
type LearnedCandidateState struct {
	Subject   string `json:"subject"`
	Statement string `json:"statement"`
}

// ChatMessage is a single message in a chat session.
type ChatMessage struct {
	Role             string                 `json:"role"`
	Content          string                 `json:"content"`
	Timestamp        time.Time              `json:"timestamp"`
	LearnedCandidate *LearnedCandidateState `json:"learned_candidate,omitempty"`
}

// PendingPermissionState represents a pending tool-call permission request.
type PendingPermissionState struct {
	ToolCallID        string    `json:"tool_call_id"`
	RequestedNodeID   string    `json:"requested_node_id"`
	RequestedNodePath string    `json:"requested_node_path"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
}

// Meta implements NodeSpec.
func (cs ChatSession) Meta() *Meta {
	return &Meta{ID: cs.ID, CreatedAt: cs.CreatedAt, UpdatedAt: cs.UpdatedAt}
}

// NodeAttrs implements NodeSpec. Empty Messages serializes as [] (not null),
// nil PendingPermission omits the key entirely.
func (cs ChatSession) NodeAttrs() []byte {
	messages := cs.Messages
	if messages == nil {
		messages = []ChatMessage{}
	}
	attrs := map[string]any{
		"messages":              messages,
		"derived_scope_node_id": cs.DerivedScopeNodeID,
	}
	if len(cs.DiscardedCandidates) > 0 {
		attrs["discarded_candidates"] = cs.DiscardedCandidates
	}
	if cs.DraftRouting != "" {
		attrs["draft_routing"] = cs.DraftRouting
	}
	if cs.PendingPermission != nil {
		attrs["pending_permission"] = cs.PendingPermission
	}
	data, _ := json.Marshal(attrs)
	return data
}

// NodeTags implements NodeSpec.
func (cs ChatSession) NodeTags() []string { return nil }

// FTSFields implements NodeSpec. ChatSession is not searchable.
func (cs ChatSession) FTSFields() FTSFields { return FTSFields{} }

// CreateChatSession inserts a new chat_session node and returns its ID.
func CreateChatSession(ctx context.Context, tx *graph.WriteTx, cs *ChatSession, author Author) (string, error) {
	return createNode(ctx, tx, TypeChatSession, cs, author)
}

// chatSessionAttrs is the deserialized attrs payload for a chat_session node.
type chatSessionAttrs struct {
	Messages            []ChatMessage           `json:"messages"`
	PendingPermission   *PendingPermissionState `json:"pending_permission,omitempty"`
	DerivedScopeNodeID  string                  `json:"derived_scope_node_id"`
	DiscardedCandidates []string                `json:"discarded_candidates,omitempty"`
	DraftRouting        string                  `json:"draft_routing,omitempty"`
}

// GetChatSession fetches a single chat session by ID.
func GetChatSession(ctx context.Context, tx *graph.ReadTx, nodeID string) (*ChatSession, error) {
	var title, attrsRaw sql.NullString
	var createdAt, updatedAt sql.NullString

	err := tx.QueryRow(
		`SELECT title, attrs, created_at, updated_at FROM nodes WHERE id = ? AND type = ?`,
		nodeID, TypeChatSession,
	).Scan(&title, &attrsRaw, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, graph.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("GetChatSession: %w", err)
	}

	var attrs chatSessionAttrs
	if attrsRaw.Valid && attrsRaw.String != "" {
		if err := json.Unmarshal([]byte(attrsRaw.String), &attrs); err != nil {
			return nil, fmt.Errorf("GetChatSession: unmarshal attrs: %w", err)
		}
	}

	// Ensure nil Messages becomes [] (not null) on readback.
	if attrs.Messages == nil {
		attrs.Messages = []ChatMessage{}
	}

	return &ChatSession{
		ID:                  nodeID,
		CreatedAt:           tryParseTime(createdAt.String),
		UpdatedAt:           tryParseTime(updatedAt.String),
		Messages:            attrs.Messages,
		PendingPermission:   attrs.PendingPermission,
		DerivedScopeNodeID:  attrs.DerivedScopeNodeID,
		DiscardedCandidates: attrs.DiscardedCandidates,
		DraftRouting:        attrs.DraftRouting,
	}, nil
}

// SaveChatSession updates an existing chat_session node.
func SaveChatSession(ctx context.Context, tx *graph.WriteTx, cs *ChatSession, author Author) error {
	if cs.ID == "" {
		return fmt.Errorf("SaveChatSession: ID is required")
	}
	return updateNode(ctx, tx, cs, author)
}
