//go:build integration

package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

func seedCapture(t *testing.T, a *app.App, body string) string {
	t.Helper()
	ctx := context.Background()
	var id string
	err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = nodes.CreateCapture(ctx, tx, nodes.Capture{Body: body}, nodes.Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("create capture: %v", err)
	}
	return id
}

// routingCall builds a suggest_routing tool call with the given actions, as the
// model would emit it (snake_case, the classifier's JSON contract).
func routingCall(actions ...map[string]any) ChatResponse {
	args, _ := json.Marshal(map[string]any{"actions": actions, "rationale": "two separate items"})
	return ChatResponse{
		ToolCalls: []ToolCall{{
			ID:       "call-1",
			Function: ToolFunction{Name: "suggest_routing", Arguments: string(args)},
		}},
	}
}

func systemBlob(mock *mockLLMClient) string {
	var b strings.Builder
	for _, m := range mock.GetMessages() {
		if m.Role == "system" {
			b.WriteString(m.Content)
			b.WriteString("\n")
		}
	}
	return b.String()
}

func routingEvent(t *testing.T, rec *eventRecorder) map[string]any {
	t.Helper()
	for _, evt := range rec.events() {
		if evt["event"] == "routing" {
			return evt
		}
	}
	t.Fatal("expected a routing event")
	return nil
}

func runRouting(t *testing.T, a *app.App, sessionID string, responses ...ChatResponse) (*mockLLMClient, *eventRecorder) {
	t.Helper()
	mock := newMockLLMClient(responses...)
	rec := &eventRecorder{}
	engine, err := NewChatEngine(a, sessionID, rec, mock, alwaysAllow{})
	if err != nil {
		t.Fatalf("NewChatEngine: %v", err)
	}
	if err := engine.RunSession(context.Background()); err != nil {
		t.Fatalf("RunSession: %v", err)
	}
	return mock, rec
}

// A session scoped to a capture is a triage conversation: the DA gets the
// capture verbatim, the target vocabulary, and the tool to propose a routing.
func TestRoutingModeArmsTheToolAndThePrompt(t *testing.T) {
	a := newTestApp(t)
	seedDAIdentity(t, a)
	captureID := seedCapture(t, a, "ligar pro dentista amanhã")
	sessionID := createSession(t, a)
	appendUserMessage(t, a, sessionID, "why a task?", captureID)

	mock, _ := runRouting(t, a, sessionID, ChatResponse{Content: "Because it is one concrete action."})

	system := systemBlob(mock)
	if !strings.Contains(system, "ligar pro dentista amanhã") {
		t.Errorf("the capture did not reach the model:\n%s", system)
	}
	if !strings.Contains(system, "One capture is routinely SEVERAL nodes") {
		t.Error("the target vocabulary did not reach the model")
	}
}

// The prompt goes out as ONE system message. Sending identity, telos and the
// triage vocabulary as three separate system turns is legal OpenAI, but several
// providers behind an openai-compatible proxy honour only the first and drop the
// rest — which shipped as a DA that had its identity, had never seen the capture,
// and asked "which capture do you mean?" with nothing in the logs.
func TestThePromptIsOneSystemMessage(t *testing.T) {
	a := newTestApp(t)
	seedDAIdentity(t, a)
	seedNote(t, a, "My Telos", "I value shipping the loop.", []string{"telos"})
	captureID := seedCapture(t, a, "ligar pro dentista")
	sessionID := createSession(t, a)
	appendUserMessage(t, a, sessionID, "why a task?", captureID)

	mock, _ := runRouting(t, a, sessionID, ChatResponse{Content: "Because it is one action."})

	var system []Message
	for _, m := range mock.GetMessages() {
		if m.Role == "system" {
			system = append(system, m)
		}
	}
	if len(system) != 1 {
		t.Fatalf("expected exactly 1 system message, got %d — the tail of them is what providers drop", len(system))
	}
	// And all three layers survived the merge.
	for _, want := range []string{"I value shipping the loop.", "ligar pro dentista", "One capture is routinely SEVERAL nodes"} {
		if !strings.Contains(system[0].Content, want) {
			t.Errorf("the system message lost %q:\n%s", want, system[0].Content)
		}
	}
}

// The routing tool is offered only when a capture is in scope: with nothing to
// route, the tool would only invite the model to hallucinate a capture.
func TestRoutingToolOnlyOfferedInScope(t *testing.T) {
	a := newTestApp(t)
	seedDAIdentity(t, a)

	toolNames := func(scopeID string) []string {
		sessionID := createSession(t, a)
		appendUserMessage(t, a, sessionID, "hello", scopeID)
		mock := newMockLLMClient(ChatResponse{Content: "hi"})
		rec := &eventRecorder{}
		engine, _ := NewChatEngine(a, sessionID, rec, mock, alwaysAllow{})
		if err := engine.RunSession(context.Background()); err != nil {
			t.Fatalf("RunSession: %v", err)
		}
		var names []string
		for _, tool := range engine.tools() {
			names = append(names, tool.Name)
		}
		return names
	}

	if got := toolNames(""); contains(got, "suggest_routing") {
		t.Errorf("suggest_routing offered with no capture in scope: %v", got)
	}
	captureID := seedCapture(t, a, "some capture")
	if got := toolNames(captureID); !contains(got, "suggest_routing") {
		t.Errorf("suggest_routing missing in routing mode: %v", got)
	}
}

// The whole point of the tool: it PROPOSES. The routing reaches the user as an
// accept/reject event, and the capture's stored suggestion is left untouched —
// writing stays the user's act.
func TestSuggestRoutingProposesAndNeverWrites(t *testing.T) {
	a := newTestApp(t)
	seedDAIdentity(t, a)
	captureID := seedCapture(t, a, "ligar pro dentista\ncomprar café")
	sessionID := createSession(t, a)
	appendUserMessage(t, a, sessionID, "split it", captureID)

	_, rec := runRouting(t, a, sessionID,
		routingCall(
			map[string]any{"target": "task", "title": "Ligar pro dentista", "due_date": "2026-04-02"},
			map[string]any{"target": "note", "title": "Café"},
		),
		ChatResponse{Content: "Proposed two nodes."},
	)

	evt := routingEvent(t, rec)
	if evt["captureId"] != captureID {
		t.Errorf("routing event carries the wrong capture: %v", evt["captureId"])
	}
	actions, _ := evt["actions"].([]any)
	if len(actions) != 2 {
		t.Fatalf("expected 2 proposed actions, got %d", len(actions))
	}

	// The event is camelCase on the wire — the browser reads dueDate, not
	// due_date. This is the whole reason the wire shape is shared.
	first, _ := actions[0].(map[string]any)
	if first["dueDate"] != "2026-04-02" {
		t.Errorf("expected camelCase dueDate on the wire, got: %v", first)
	}

	// Nothing was written: the capture still carries no suggestion of its own.
	ctx := context.Background()
	var capture *nodes.Capture
	if err := a.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		capture, err = nodes.GetCapture(ctx, tx, captureID)
		return err
	}); err != nil {
		t.Fatalf("get capture: %v", err)
	}
	if len(capture.SuggestedActions) != 0 {
		t.Errorf("the DA wrote a routing to the graph; it must only propose: %v", capture.SuggestedActions)
	}
}

// A model that keeps calling a tool must be stopped. Every tool result feeds the
// next completion, so an unbounded loop spins forever burning tokens and never
// answers — which is exactly what shipped: the same note edit proposed five times
// over, with no text ever reaching the user.
func TestTheAgentLoopIsBounded(t *testing.T) {
	a := newTestApp(t)
	seedDAIdentity(t, a)
	captureID := seedCapture(t, a, "the selfish gene")
	sessionID := createSession(t, a)
	appendUserMessage(t, a, sessionID, "route it", captureID)

	// A model that never stops calling the tool: the mock repeats its last
	// response once its list runs out.
	forever := routingCall(map[string]any{"target": "note", "title": "Again"})
	mock, rec := runRouting(t, a, sessionID, forever, forever, forever, forever, forever, forever, forever, forever, forever, forever, forever, forever)

	if calls := mock.GetCallIndex(); calls > maxToolTurns {
		t.Errorf("the loop ran %d completions, past the %d-turn bound", calls, maxToolTurns)
	}
	if !rec.hasEventType("done") {
		t.Error("a loop that hits the bound must still close the stream")
	}
}

// The tag vocabulary is closed. A tag the model coined — however well it seems
// to fit — is dropped, and so is one that merely restates the node's own type.
func TestSuggestRoutingDropsCoinedTags(t *testing.T) {
	a := newTestApp(t)
	seedDAIdentity(t, a)
	captureID := seedCapture(t, a, "usar o gravador de voz com fone")
	sessionID := createSession(t, a)
	appendUserMessage(t, a, sessionID, "route it", captureID)

	_, rec := runRouting(t, a, sessionID,
		routingCall(map[string]any{
			"target": "note",
			"title":  "Gravador de voz",
			// "behavior" is on the list; "capture" restates the node's origin and
			// "language-learning" was invented on the spot.
			"tags": []any{"behavior", "capture", "language-learning"},
		}),
		ChatResponse{Content: "Proposed."},
	)

	evt := routingEvent(t, rec)
	actions, _ := evt["actions"].([]any)
	first, _ := actions[0].(map[string]any)
	tags, _ := first["tags"].([]any)

	if len(tags) != 1 || tags[0] != "behavior" {
		t.Errorf("expected only the vocabulary tag to survive, got %v", tags)
	}
}

// A target the model invented is rejected as a tool result, not written and not
// crashed on: the model gets told and can correct itself in the same turn.
func TestSuggestRoutingRejectsAnUnknownTarget(t *testing.T) {
	a := newTestApp(t)
	seedDAIdentity(t, a)
	captureID := seedCapture(t, a, "something")
	sessionID := createSession(t, a)
	appendUserMessage(t, a, sessionID, "route it", captureID)

	mock, rec := runRouting(t, a, sessionID,
		routingCall(map[string]any{"target": "reminder", "title": "Nope"}),
		ChatResponse{Content: "Sorry, let me retry."},
	)

	for _, evt := range rec.events() {
		if evt["event"] == "routing" {
			t.Fatal("an unknown target was presented to the user")
		}
	}
	var toolBlob string
	for _, m := range mock.GetMessages() {
		if m.Role == "tool" {
			toolBlob += m.Content
		}
	}
	if !strings.Contains(toolBlob, "is not a target") {
		t.Errorf("the model was not told why the routing was rejected: %q", toolBlob)
	}
}

// An update merges into one note and is reviewed on its own, so it cannot be one
// leg of a fan-out. The inbox rejects that on process; catching it here means the
// DA tells the user rather than a write failing later.
func TestSuggestRoutingRejectsUpdateInAFanOut(t *testing.T) {
	a := newTestApp(t)
	seedDAIdentity(t, a)
	captureID := seedCapture(t, a, "something")
	sessionID := createSession(t, a)
	appendUserMessage(t, a, sessionID, "route it", captureID)

	mock, rec := runRouting(t, a, sessionID,
		routingCall(
			map[string]any{"target": "update", "title": "Merge"},
			map[string]any{"target": "task", "title": "Do the thing"},
		),
		ChatResponse{Content: "Right, an update stands alone."},
	)

	for _, evt := range rec.events() {
		if evt["event"] == "routing" {
			t.Fatal("an update was presented alongside another node")
		}
	}
	var toolBlob string
	for _, m := range mock.GetMessages() {
		if m.Role == "tool" {
			toolBlob += m.Content
		}
	}
	if !strings.Contains(toolBlob, "cannot be combined") {
		t.Errorf("the model was not told why the routing was rejected: %q", toolBlob)
	}
}

// The trap this feature exists to avoid: the chat is write-then-stream, so a
// draft that lives only in the browser never reaches the DA. If the user retypes
// a node and then asks about it, the DA must discuss what is on screen — not the
// routing it proposed before the edit.
func TestTheOnScreenDraftReachesTheModel(t *testing.T) {
	a := newTestApp(t)
	seedDAIdentity(t, a)
	captureID := seedCapture(t, a, "ligar pro dentista")
	sessionID := createSession(t, a)

	// The user retyped the DA's task into a note, then asks about it.
	appendUserMessage(t, a, sessionID, "should this really be a note?", captureID)
	cs := getSession(t, a, sessionID)
	cs.DraftRouting = "- note: Dentista (user retyped this from task)\n"
	if err := a.Graph.DoWrite(context.Background(), func(tx *graph.WriteTx) error {
		return nodes.SaveChatSession(context.Background(), tx, cs, nodes.Author{Name: "kernl"})
	}); err != nil {
		t.Fatalf("save draft: %v", err)
	}

	mock, _ := runRouting(t, a, sessionID, ChatResponse{Content: "No — it is an action."})

	system := systemBlob(mock)
	if !strings.Contains(system, "user retyped this from task") {
		t.Errorf("the on-screen draft never reached the model:\n%s", system)
	}
}

// One request, two tools. "Add this book to my Anti-library note and make a
// note for the book itself" is one thought, and a model answers it with two
// tool calls in a single assistant turn. The engine used to return inside the
// dispatch loop, so it ran the first call and dropped the second on the floor —
// and since the assistant turn it fed back still ADVERTISED both calls, the
// model read its own transcript, saw the routing it had asked for, and told the
// user it had proposed one. The user got a diff, no routing card, and a claim
// that both were done.
func TestEveryToolCallInATurnRuns(t *testing.T) {
	a := newTestApp(t)
	seedDAIdentity(t, a)
	ctx := context.Background()

	noteID := seedNote(t, a, "Anti-library", "old body\n", nil)
	rel := "anti-library.md"
	file := "---\nid: " + noteID + "\ntitle: Anti-library\n---\n\nold body\n"
	if err := os.WriteFile(filepath.Join(a.Config.Vault.Root, rel), []byte(file), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := tx.Exec(`INSERT INTO note_paths (uuid, path, content_hash, updated_at)
			VALUES (?, ?, '', strftime('%Y-%m-%dT%H:%M:%SZ','now'))`, noteID, rel)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	captureID := seedCapture(t, a, "the selfish gene")
	sessionID := createSession(t, a)
	appendUserMessage(t, a, sessionID, "add it to Anti-library and make a note for the book", captureID)

	routingArgs, _ := json.Marshal(map[string]any{
		"actions": []map[string]any{{"target": "note", "title": "The Selfish Gene"}},
	})
	mock, rec := runRouting(t, a, sessionID,
		ChatResponse{ToolCalls: []ToolCall{
			{ID: "c1", Type: "function", Function: ToolFunction{
				Name:      "suggest_note_edit",
				Arguments: fmt.Sprintf(`{"node_id":%q,"new_body":"old body\n\n- The Selfish Gene — Richard Dawkins\n"}`, noteID),
			}},
			{ID: "c2", Type: "function", Function: ToolFunction{
				Name:      "suggest_routing",
				Arguments: string(routingArgs),
			}},
		}},
		ChatResponse{Content: "Proposed the bullet and the note."},
	)

	if !rec.hasEventType("diff") {
		t.Error("the note edit was never presented")
	}
	if !rec.hasEventType("routing") {
		t.Error("the second tool call was dropped: no routing was presented")
	}

	// Every call must be answered by id. A tool_call left without its result is a
	// malformed transcript, and it is what let the model believe it had routed.
	answered := map[string]bool{}
	for _, m := range mock.GetMessages() {
		if m.Role == "tool" {
			answered[m.ToolCallID] = true
		}
	}
	for _, id := range []string{"c1", "c2"} {
		if !answered[id] {
			t.Errorf("tool call %s got no result back", id)
		}
	}
}

// The model does not always answer with the keys the schema asked for: it
// reaches for "type" instead of "target", and for a bare "tag": "to-read"
// instead of "tags": ["to-read"]. encoding/json drops an unknown key without a
// word, so the action arrived with no target at all, the tool rejected it, and
// the model retried — supplying the target and quietly losing the tag. What the
// user saw was a routing with no tags and a DA that talked about one, and every
// rejected call cost a turn. A synonym is not worth a round trip.
func TestSuggestRoutingAcceptsTheKeysModelsActuallySend(t *testing.T) {
	a := newTestApp(t)
	seedDAIdentity(t, a)
	captureID := seedCapture(t, a, "the selfish gene")
	sessionID := createSession(t, a)
	appendUserMessage(t, a, sessionID, "make a note for the book", captureID)

	// Exactly what kimi-k2.7 emitted: "type" for the target, singular "tag" as a
	// string, and the tag itself miscased.
	args := `{"actions":[{"type":"note","title":"The Selfish Gene","tag":"To-Read "}]}`
	_, rec := runRouting(t, a, sessionID,
		ChatResponse{ToolCalls: []ToolCall{{
			ID:       "c1",
			Function: ToolFunction{Name: "suggest_routing", Arguments: args},
		}}},
		ChatResponse{Content: "Proposed."},
	)

	evt := routingEvent(t, rec)
	actions, _ := evt["actions"].([]any)
	first, _ := actions[0].(map[string]any)
	if first["target"] != "note" {
		t.Errorf(`"type" was not read as the target: %v`, first)
	}
	tags, _ := first["tags"].([]any)
	if len(tags) != 1 || tags[0] != "to-read" {
		t.Errorf(`the tag was lost between "tag" and "tags": %v`, tags)
	}
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
