package api

import (
	"time"

	"github.com/gabrielassisxyz/kernl/internal/chat"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

// chatSessionDTO is the wire form of a chat session.
//
// nodes.ChatSession cannot be encoded straight to the response: it carries no
// json tags at all (so it emits Go field names), and the nested structs it
// holds carry snake_case tags that are the storage format, not the API's. The
// message and permission shapes come from internal/chat, which owns the same
// contract for its SSE events — see the note in internal/chat/wire.go.
type chatSessionDTO struct {
	ID                  string                      `json:"id"`
	CreatedAt           time.Time                   `json:"createdAt"`
	UpdatedAt           time.Time                   `json:"updatedAt"`
	Messages            []chat.WireMessage          `json:"messages"`
	PendingPermission   *chat.WirePendingPermission `json:"pendingPermission,omitempty"`
	DerivedScopeNodeID  string                      `json:"derivedScopeNodeId"`
	DiscardedCandidates []string                    `json:"discardedCandidates,omitempty"`
	DraftRouting        string                      `json:"draftRouting,omitempty"`
}

// newChatSessionDTO converts a stored session to its wire form.
func newChatSessionDTO(cs *nodes.ChatSession) chatSessionDTO {
	return chatSessionDTO{
		ID:                  cs.ID,
		CreatedAt:           cs.CreatedAt,
		UpdatedAt:           cs.UpdatedAt,
		Messages:            chat.NewWireMessages(cs.Messages),
		PendingPermission:   chat.NewWirePendingPermission(cs.PendingPermission),
		DerivedScopeNodeID:  cs.DerivedScopeNodeID,
		DiscardedCandidates: cs.DiscardedCandidates,
		DraftRouting:        cs.DraftRouting,
	}
}

// chatSessionCreatedDTO is the wire form of a freshly created session.
type chatSessionCreatedDTO struct {
	ID        string `json:"id"`
	CreatedAt string `json:"createdAt"`
}
