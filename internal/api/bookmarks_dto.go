package api

import (
	"time"

	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

// The bookmark wire format, kept separate from the storage format.
//
// nodes.Highlight's json tags are the *persistence* shape: Bookmark.NodeAttrs
// puts []Highlight straight into the attrs column, and GetBookmark/ListBookmarks
// read it back through the same struct. Renaming created_at there to satisfy the
// REST camelCase contract would make every already-stored highlight read back
// with a zero timestamp. So the wire format lives here instead, and the storage
// struct is never touched.
//
// nodes.Bookmark itself carries camelCase tags for the same contract, which
// makes them redundant on this path now. They are left in place: other code may
// serialize a Bookmark directly, and removing them is churn with no upside.

// highlightResponse is the wire shape of a saved passage.
type highlightResponse struct {
	Text      string    `json:"text"`
	Note      string    `json:"note,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// bookmarkResponse is the wire shape of a bookmark, highlights included.
type bookmarkResponse struct {
	ID          string              `json:"id"`
	CreatedAt   time.Time           `json:"createdAt"`
	UpdatedAt   time.Time           `json:"updatedAt"`
	Title       string              `json:"title"`
	URL         string              `json:"url"`
	Description string              `json:"description"`
	ArchivedAt  *time.Time          `json:"archivedAt"`
	Excerpt     string              `json:"excerpt"`
	Tags        []string            `json:"tags"`
	Highlights  []highlightResponse `json:"highlights"`
}

func newHighlightResponse(h nodes.Highlight) highlightResponse {
	return highlightResponse{Text: h.Text, Note: h.Note, CreatedAt: h.CreatedAt}
}

func newHighlightResponses(highlights []nodes.Highlight) []highlightResponse {
	if len(highlights) == 0 {
		return nil
	}
	out := make([]highlightResponse, 0, len(highlights))
	for _, h := range highlights {
		out = append(out, newHighlightResponse(h))
	}
	return out
}

func newBookmarkResponse(b *nodes.Bookmark) bookmarkResponse {
	return bookmarkResponse{
		ID:          b.ID,
		CreatedAt:   b.CreatedAt,
		UpdatedAt:   b.UpdatedAt,
		Title:       b.Title,
		URL:         b.URL,
		Description: b.Description,
		ArchivedAt:  b.ArchivedAt,
		Excerpt:     b.Excerpt,
		Tags:        b.Tags,
		Highlights:  newHighlightResponses(b.Highlights),
	}
}

func newBookmarkResponses(list []*nodes.Bookmark) []bookmarkResponse {
	// nil in, nil out: an empty list already encoded as JSON null on this route,
	// and switching it to [] is a wire change this conversion does not need.
	if len(list) == 0 {
		return nil
	}
	out := make([]bookmarkResponse, 0, len(list))
	for _, b := range list {
		out = append(out, newBookmarkResponse(b))
	}
	return out
}
