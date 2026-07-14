// Package wire owns the camelCase view of a capture action: the shape the web
// client reads and posts back.
//
// It exists because that shape has more than one producer. The REST layer
// (internal/api) serves the DA's suggestions and takes the user's edits, and the
// chat engine (internal/chat) streams a routing proposal over SSE — an event
// that never passes through internal/api and so cannot borrow a DTO defined
// there. Nor can it borrow one from internal/inbox: that package already imports
// internal/chat, and the reverse edge would be an import cycle.
//
// So the wire shape lives in a leaf package that depends on nothing but nodes,
// and both producers map through the same pair of functions. The alternative —
// a second hand-written struct in internal/chat — is how a field like dueDate
// ends up correct in one surface and silently empty in the other.
package wire

import "github.com/gabrielassisxyz/kernl/internal/graph/nodes"

// CaptureAction is one node a capture becomes, as the web client sees it.
// nodes.CaptureAction is the same data with snake_case tags (the persisted attrs
// shape); this is the API's camelCase contract, and the two are only ever
// bridged by FromNodes/ToNodes below.
type CaptureAction struct {
	Target             string   `json:"target"`
	Title              string   `json:"title"`
	Body               string   `json:"body"`
	ProjectID          string   `json:"projectId"`
	ProjectTitle       string   `json:"projectTitle"`
	ProjectDescription string   `json:"projectDescription"`
	InitialTasks       []string `json:"initialTasks"`
	Tags               []string `json:"tags"`
	// DueDate is a calendar day (YYYY-MM-DD), like taskDTO's — empty when the
	// action has none. On a task action only.
	DueDate string `json:"dueDate"`
	LinkTo  string `json:"linkTo"`
}

// FromNodes maps stored actions onto the wire.
func FromNodes(actions []nodes.CaptureAction) []CaptureAction {
	out := make([]CaptureAction, 0, len(actions))
	for _, a := range actions {
		out = append(out, CaptureAction{
			Target:             a.Target,
			Title:              a.Title,
			Body:               a.Body,
			ProjectID:          a.ProjectID,
			ProjectTitle:       a.ProjectTitle,
			ProjectDescription: a.ProjectDescription,
			InitialTasks:       a.InitialTasks,
			Tags:               a.Tags,
			DueDate:            nodes.FormatDueDate(a.DueDate),
			LinkTo:             a.LinkTo,
		})
	}
	return out
}

// ToNodes maps posted actions back. A due date that arrives malformed is
// reported rather than silently dropped: the capture is being triaged once, and
// a deadline that vanishes here vanishes for good.
func ToNodes(actions []CaptureAction) ([]nodes.CaptureAction, error) {
	out := make([]nodes.CaptureAction, 0, len(actions))
	for _, a := range actions {
		dueDate, err := nodes.ParseDueDate(a.DueDate)
		if err != nil {
			return nil, err
		}
		out = append(out, nodes.CaptureAction{
			Target:             a.Target,
			Title:              a.Title,
			Body:               a.Body,
			ProjectID:          a.ProjectID,
			ProjectTitle:       a.ProjectTitle,
			ProjectDescription: a.ProjectDescription,
			InitialTasks:       a.InitialTasks,
			Tags:               a.Tags,
			DueDate:            dueDate,
			LinkTo:             a.LinkTo,
		})
	}
	return out, nil
}
