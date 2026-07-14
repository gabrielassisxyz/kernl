package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/inbox/wire"
)

// Routing mode: the DA discussing where a capture should go.
//
// It is the same agent, the same session and the same tools — it only gains one
// tool and one system message when the conversation is scoped to a capture. And
// like suggest_note_edit, the tool PROPOSES: it emits an event the user accepts
// or rejects, and the accepted routing still has to be processed by hand. The
// capture body is never rewritten, and nothing reaches the graph from here.

// routingSystemPrompt is prepended when the session is scoped to a capture. It
// embeds the classifier's own vocabulary verbatim (via inbox/wire), so the DA
// and the classifier cannot quietly disagree about what a "project" is.
func routingSystemPrompt(captureBody, draft, projects string) string {
	// The instruction that matters goes FIRST and stays short. This prompt used to
	// open with 3.4k characters of classifier vocabulary, and the models answered
	// in prose instead of calling the tool about two times in three — the same
	// models called it 3/3 with a short prompt. Everything below earns its length.
	var b strings.Builder
	b.WriteString(`You are helping the user triage ONE capture from their inbox: what it should become in their graph.

TO CHANGE ANYTHING, CALL A TOOL. Talking about a change does not make it.

- suggest_routing — the nodes this capture becomes. Pass the COMPLETE set every time (the unchanged ones too): it replaces the whole proposal. A title, body, tag, project or due date the user asks for exists only once it is in this call.
- suggest_note_edit — a note that ALREADY EXISTS ("add this book to my Anti-library note"). Find it with search_notes, pass its full revised body. This is not a node the capture becomes.

A request that needs both gets both calls, in the same reply. Never say you added, created or proposed anything unless the tool call is in that same reply.

The capture, verbatim:
---
`)
	b.WriteString(captureBody)
	b.WriteString("\n---\n")

	if strings.TrimSpace(draft) != "" {
		// The user has been editing the proposal on screen. Without this the DA
		// argues with a routing that no longer exists.
		b.WriteString("\nThe routing currently on the user's screen (yours, as they have since edited it):\n")
		b.WriteString(draft)
		b.WriteString("\n")
	}
	if strings.TrimSpace(projects) != "" {
		b.WriteString("\nThe user's existing projects (id — title):\n")
		b.WriteString(projects)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(wire.TargetGlossary)
	b.WriteString(`

Tags come from a closed list — ` + wire.TagVocabulary + ` — and nothing else; none is a fine answer. A due date only when the capture itself states one.

When the user is DECIDED (an instruction, a correction stated as fact: "make it a note too", "split this in two"), call the tool right away, then say in one line what you proposed. When they are UNSURE (hedged, asking: "I think this might also be a note", "why a task?"), answer them first and call the tool once you agree.

Never rewrite the capture body — it is the user's own words. Nothing you propose is saved: the user accepts it and processes the capture themselves.`)
	return b.String()
}

func suggestRoutingTool() Tool {
	return Tool{
		Name:        "suggest_routing",
		Description: "Propose what the capture under discussion should become: the complete list of nodes, replacing the current proposal. The routing is NOT applied — it is shown to the user as an accept/reject card, and they still process the capture themselves. Call this instead of describing a routing in prose when you and the user have agreed on one.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"actions": {
					"type": "array",
					"description": "The complete set of nodes the capture becomes. One capture is routinely several nodes.",
					"items": {
						"type": "object",
						"properties": {
							"target": {"type": "string", "enum": ["note", "update", "bookmark", "task", "project", "discard"]},
							"title": {"type": "string", "description": "Short, imperative, human. Always write one."},
							"body": {"type": "string", "description": "The fragment of the capture this node owns. Omit when it owns the whole capture."},
							"project_id": {"type": "string", "description": "An existing project id, for a task that belongs to it."},
							"project_title": {"type": "string"},
							"project_description": {"type": "string"},
							"initial_tasks": {"type": "array", "items": {"type": "string"}},
							"tags": {"type": "array", "items": {"type": "string"}},
							"due_date": {"type": "string", "description": "YYYY-MM-DD, on a task only, and only when the capture itself states a deadline."}
						},
						"required": ["target", "title"]
					}
				},
				"rationale": {"type": "string", "description": "One line: why this routing. Shown to the user above the proposal."}
			},
			"required": ["actions"]
		}`),
	}
}

// routingArgs is the tool's argument shape. The action fields are snake_case
// here because that is what the model is told to emit (it mirrors the
// classifier's JSON contract); wire.CaptureAction is camelCase because that is
// what the browser reads. The two are bridged in presentRouting, deliberately in
// one place.
type routingArgs struct {
	Actions   []routingAction `json:"actions"`
	Rationale string          `json:"rationale"`
}

// routingAction accepts the names models actually reach for, not only the ones
// the schema asks for. Models routinely answered with "type" instead of
// "target", and with a bare "tag": "to-read" instead of "tags": ["to-read"].
// encoding/json drops an unknown key without a word, so the action arrived with
// no target, the tool rejected it, and the model retried — and its correction
// supplied the target while quietly dropping the tag. The user saw a routing
// with no tags and a reply that talked about one, and every rejected call cost a
// turn. Be liberal in what you accept from a model: a synonym is not an error
// worth a round trip.
type routingAction struct {
	Target             string          `json:"target"`
	Type               string          `json:"type"`
	Title              string          `json:"title"`
	Body               string          `json:"body"`
	ProjectID          string          `json:"project_id"`
	ProjectTitle       string          `json:"project_title"`
	ProjectDescription string          `json:"project_description"`
	InitialTasks       []string        `json:"initial_tasks"`
	Tags               []string        `json:"tags"`
	Tag                json.RawMessage `json:"tag"`
	DueDate            string          `json:"due_date"`
}

// target is the action's target under either name.
func (a routingAction) target() string {
	if a.Target != "" {
		return a.Target
	}
	return a.Type
}

// tags gathers the tags under either name. "tag" arrives as a string or, less
// often, as an array; both are read, and wire.FilterTags has the last word on
// what survives.
func (a routingAction) tags() []string {
	out := append([]string(nil), a.Tags...)
	if len(a.Tag) > 0 {
		var one string
		var many []string
		if err := json.Unmarshal(a.Tag, &one); err == nil {
			out = append(out, one)
		} else if err := json.Unmarshal(a.Tag, &many); err == nil {
			out = append(out, many...)
		}
	}
	return wire.FilterTags(out)
}

// presentRouting validates the proposed routing and emits it as an accept/reject
// card. It never writes: accepting only replaces the draft in the user's editor,
// and processing the capture stays a separate, user-initiated act (POST
// /api/inbox/{id}/process). Returns a short status for the LLM's next turn.
func (e *ChatEngine) presentRouting(ctx context.Context, captureID string, args routingArgs) string {
	if len(args.Actions) == 0 {
		return "a routing needs at least one node; nothing was proposed"
	}

	actions := make([]wire.CaptureAction, 0, len(args.Actions))
	for _, a := range args.Actions {
		if !wire.ValidTarget(a.target()) {
			return fmt.Sprintf("%q is not a target; use one of: %s", a.target(), strings.Join(wire.Targets, ", "))
		}
		actions = append(actions, wire.CaptureAction{
			Target:             a.target(),
			Title:              a.Title,
			Body:               a.Body,
			ProjectID:          a.ProjectID,
			ProjectTitle:       a.ProjectTitle,
			ProjectDescription: a.ProjectDescription,
			InitialTasks:       a.InitialTasks,
			// The vocabulary is closed. A tag the model coined anyway is dropped
			// rather than proposed — the same backstop the classifier applies.
			Tags:    a.tags(),
			DueDate: a.DueDate,
		})
	}

	// An update is reviewed hunk by hunk against one note, so it cannot be one
	// leg of a fan-out. The inbox rejects that on process; catching it here means
	// the user is told by the DA rather than by a failed write.
	if len(actions) > 1 {
		for _, a := range actions {
			if a.Target == "update" {
				return "an update merges into an existing note and is reviewed on its own, so it cannot be combined with other nodes; propose it alone, or route this one differently"
			}
		}
	}

	if err := e.emitRoutingEvent(captureID, args.Rationale, actions); err != nil {
		slog.Warn("emit routing event", "error", err)
		return "failed to present the routing"
	}
	return fmt.Sprintf("routing presented to the user for accept/reject (%d node(s)); it is NOT applied — they still process the capture themselves. This routing is done — do not propose it again.", len(actions))
}

func (e *ChatEngine) emitRoutingEvent(captureID, rationale string, actions []wire.CaptureAction) error {
	return e.writeEvent(map[string]any{
		"event":     "routing",
		"captureId": captureID,
		"rationale": rationale,
		"actions":   actions,
	})
}

// projectList renders the user's projects as "id — title" lines. The DA needs
// the ids to file a task under an existing project; without them it can only
// ever propose new ones.
func (e *ChatEngine) projectList(ctx context.Context) string {
	var projects []*nodes.Project
	err := e.app.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
		p, err := nodes.ListProjects(ctx, tx)
		if err != nil {
			return err
		}
		projects = p
		return nil
	})
	if err != nil {
		slog.Warn("list projects for routing prompt", "error", err)
		return ""
	}

	var b strings.Builder
	for _, p := range projects {
		fmt.Fprintf(&b, "- %s — %s\n", p.ID, p.Title)
	}
	return b.String()
}

// capturedScope reports the capture the session is scoped to, if any. Routing
// mode is entirely derived from this: no scope, no capture, no routing tool.
func (e *ChatEngine) capturedScope(ctx context.Context, cs *nodes.ChatSession) *nodes.Capture {
	if cs.DerivedScopeNodeID == "" {
		return nil
	}
	var capture *nodes.Capture
	err := e.app.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
		c, err := nodes.GetCapture(ctx, tx, cs.DerivedScopeNodeID)
		if err != nil {
			return err
		}
		capture = c
		return nil
	})
	if errors.Is(err, graph.ErrNotFound) {
		// The scope is some other node kind (a note, a project) — the normal case
		// for a general chat, not an error.
		return nil
	}
	if err != nil {
		// Anything else means routing mode failed to arm for a capture that DOES
		// exist. Silently returning nil here just makes the DA ask "which capture
		// are you talking about?" with no trace of why.
		slog.Warn("routing scope: could not load the capture", "capture", cs.DerivedScopeNodeID, "error", err)
		return nil
	}
	return capture
}
