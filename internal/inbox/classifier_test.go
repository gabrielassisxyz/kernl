package inbox

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/chat"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

type mockLLM struct {
	content string
}

func (m *mockLLM) Chat(ctx context.Context, messages []chat.Message, tools []chat.Tool) (*chat.ChatResponse, error) {
	return &chat.ChatResponse{Content: m.content}, nil
}

func TestClassifier(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "graph.db")

	g, err := graph.Open(context.Background(), graph.Config{Path: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()

	// Seed capture
	var capID string
	err = g.DoWrite(context.Background(), func(tx *graph.WriteTx) error {
		c := nodes.Capture{
			Body:         "https://example.com",
			CapturedFrom: "cli",
			Tags:         []string{"pending"},
		}
		id, err := nodes.CreateCapture(context.Background(), tx, c, nodes.Author{Name: "test"})
		if err != nil {
			return err
		}
		capID = id
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	llm := &mockLLM{content: "bookmark"}
	classifier := NewClassifier(g, llm, ClassifierOptions{})

	err = classifier.processPending(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	err = g.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		cap, err := nodes.GetCapture(context.Background(), tx, capID)
		if err != nil {
			return err
		}
		if len(cap.SuggestedActions) != 1 || cap.SuggestedActions[0].Target != "bookmark" {
			t.Errorf("expected one bookmark action, got %#v", cap.SuggestedActions)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestParseActions(t *testing.T) {
	projects := []*nodes.Project{{ID: "p-web", Title: "Web UI"}, {ID: "p-core", Title: "Core"}}
	cases := []struct {
		name        string
		raw         string
		wantTarget  string
		wantProject string
	}{
		{"task with valid project", `{"actions":[{"target":"task","project_id":"p-web"}]}`, "task", "p-web"},
		{"task hallucinated project dropped", `{"actions":[{"target":"task","project_id":"p-ghost"}]}`, "task", ""},
		{"note ignores project", `{"actions":[{"target":"note","project_id":"p-web"}]}`, "note", ""},
		{"surrounding prose tolerated", "Sure!\n{\"actions\":[{\"target\":\"bookmark\"}]}\nDone.", "bookmark", ""},
		{"no json falls back to keyword", "this should be a discard", "discard", ""},
		{"garbage falls back to note", "??!!", "note", ""},
		{"empty action list falls back to note", `{"actions":[]}`, "note", ""},
		{"unknown target dropped, falls back to note", `{"actions":[{"target":"sandwich"}]}`, "note", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseActions(tc.raw, projects)
			if len(got) != 1 {
				t.Fatalf("parseActions(%q) = %#v, want exactly 1 action", tc.raw, got)
			}
			if got[0].Target != tc.wantTarget || got[0].ProjectID != tc.wantProject {
				t.Errorf("parseActions(%q) = (%q, %q), want (%q, %q)", tc.raw, got[0].Target, got[0].ProjectID, tc.wantTarget, tc.wantProject)
			}
		})
	}
}

// The whole point of the rework: one capture, several actions, each with its
// own title and body fragment.
func TestParseActionsSplitsCompositeCapture(t *testing.T) {
	raw := `{"actions":[
		{"target":"note","title":"Writing is prompting","body":"Writing well is the same skill as prompting well."},
		{"target":"task","title":"Adjust phone modes","body":"Set up focus modes on the phone."},
		{"target":"discard","title":"Filler"}
	]}`
	got := parseActions(raw, nil)
	if len(got) != 3 {
		t.Fatalf("parseActions returned %d actions, want 3", len(got))
	}
	want := []struct{ target, title string }{
		{"note", "Writing is prompting"},
		{"task", "Adjust phone modes"},
		{"discard", "Filler"},
	}
	for i, w := range want {
		if got[i].Target != w.target || got[i].Title != w.title {
			t.Errorf("action %d = (%q, %q), want (%q, %q)", i, got[i].Target, got[i].Title, w.target, w.title)
		}
	}
	if got[0].Body != "Writing well is the same skill as prompting well." {
		t.Errorf("per-action body not preserved: %q", got[0].Body)
	}
}

// An update cannot be combined with other actions (ProcessCapture rejects it),
// so a fan-out containing one keeps the capture by demoting it to a note.
func TestSaveSuggestionDemotesUpdateInsideFanOut(t *testing.T) {
	ctx := context.Background()
	g, err := graph.Open(ctx, graph.Config{Path: filepath.Join(t.TempDir(), "graph.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()

	var capID string
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		capID, err = nodes.CreateCapture(ctx, tx, nodes.Capture{Body: "two things", Tags: []string{"pending"}}, nodes.Author{Name: "t"})
		return err
	}); err != nil {
		t.Fatal(err)
	}

	c := NewClassifier(g, &mockLLM{}, ClassifierOptions{})
	var capture *nodes.Capture
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		capture, err = nodes.GetCapture(ctx, tx, capID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := c.saveSuggestion(ctx, capture, []nodes.CaptureAction{
		{Target: "update", Title: "Extend the note"},
		{Target: "task", Title: "Do the thing"},
	}); err != nil {
		t.Fatal(err)
	}

	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		saved, err := nodes.GetCapture(ctx, tx, capID)
		if err != nil {
			return err
		}
		if len(saved.SuggestedActions) != 2 {
			t.Fatalf("expected 2 actions, got %#v", saved.SuggestedActions)
		}
		if saved.SuggestedActions[0].Target != "note" {
			t.Errorf("update inside a fan-out should be demoted to note, got %q", saved.SuggestedActions[0].Target)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// A capture whose only tie to a project is a term buried in that project's note
// must inherit the project. This proves the classifier reads the graph, not
// just the project titles (UAT I2 regression).
func TestClassifyReadsGraphForProjectAssociation(t *testing.T) {
	ctx := context.Background()
	g, err := graph.Open(ctx, graph.Config{Path: filepath.Join(t.TempDir(), "graph.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()

	var projectID string
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		pid, err := nodes.CreateProject(ctx, tx, nodes.Project{Title: "Atlas Verde"}, nodes.Author{Name: "test"})
		if err != nil {
			return err
		}
		projectID = pid
		// A note carrying the sentinel term, linked to the project.
		noteID, err := nodes.CreateNote(ctx, tx, nodes.Note{
			Title: "Companion",
			Body:  "The sentinel term azimute-cobalto-17 lives only here.",
		}, nodes.Author{Name: "test"})
		if err != nil {
			return err
		}
		_, err = edges.Create(ctx, tx, edges.Edge{Src: noteID, Dst: pid, Label: "describes"}, nodes.Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	// The mock echoes the prompt back so we can assert the project hint reached it.
	var seenPrompt string
	capturing := &promptCapturingLLM{onPrompt: func(p string) { seenPrompt = p }, reply: `{"actions":[{"target":"task","project_id":"` + projectID + `"}]}`}
	c := NewClassifier(g, capturing, ClassifierOptions{})

	got, err := c.classify(ctx, "turn azimute-cobalto-17 into a task", []*nodes.Project{{ID: projectID, Title: "Atlas Verde"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(seenPrompt, "azimute-cobalto-17") {
		t.Errorf("prompt should include the matching note body, got:\n%s", seenPrompt)
	}
	if !strings.Contains(seenPrompt, projectID) {
		t.Error("prompt should surface the note's project association")
	}
	if len(got) != 1 || got[0].Target != "task" || got[0].ProjectID != projectID {
		t.Errorf("expected one task attached to %s, got %#v", projectID, got)
	}
}

func TestClassifyBatchKeepsRelatedMessagesTogether(t *testing.T) {
	ctx := context.Background()
	g, err := graph.Open(ctx, graph.Config{Path: filepath.Join(t.TempDir(), "graph.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()

	result, err := CreateBatch(ctx, g, BatchInput{
		RawText:      "[06/07/2026, 14:32] Me: Build an ai-memory explainer project\n[06/07/2026, 14:33] Me: Task: map the repo architecture\n[06/07/2026, 14:34] Me: Task: write usage examples",
		Source:       "whatsapp",
		SplitMode:    BatchSplitWhatsApp,
		ContextTitle: "ai-memory planning",
	})
	if err != nil {
		t.Fatal(err)
	}

	var seenPrompt string
	llm := &promptCapturingLLM{
		onPrompt: func(p string) { seenPrompt = p },
		reply: `{"items":[
			{"sequence":0,"actions":[{"target":"project","title":"ai-memory explainer","project_title":"ai-memory explainer","project_description":"Explain ai-memory from the repo context.","initial_tasks":["Map the repo architecture","Write usage examples"]}]},
			{"sequence":1,"actions":[{"target":"discard"}]},
			{"sequence":2,"actions":[{"target":"discard"}]}
		]}`,
	}
	classifier := NewClassifier(g, llm, ClassifierOptions{})
	if err := classifier.processPending(ctx); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(seenPrompt, "Task: map the repo architecture") || !strings.Contains(seenPrompt, "Do not treat each line in isolation") {
		t.Fatalf("batch prompt did not include related-message guidance:\n%s", seenPrompt)
	}

	var first *nodes.Capture
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		first, err = nodes.GetCapture(ctx, tx, result.IDs[0])
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(first.SuggestedActions) != 1 {
		t.Fatalf("SuggestedActions = %#v, want 1", first.SuggestedActions)
	}
	action := first.SuggestedActions[0]
	if action.Target != "project" {
		t.Fatalf("first action target = %q, want project", action.Target)
	}
	if action.ProjectTitle != "ai-memory explainer" {
		t.Fatalf("ProjectTitle = %q", action.ProjectTitle)
	}
	if len(action.InitialTasks) != 2 {
		t.Fatalf("InitialTasks = %#v, want 2 tasks", action.InitialTasks)
	}
}

type promptCapturingLLM struct {
	onPrompt func(string)
	reply    string
}

func (m *promptCapturingLLM) Chat(ctx context.Context, messages []chat.Message, tools []chat.Tool) (*chat.ChatResponse, error) {
	if m.onPrompt != nil && len(messages) > 0 {
		m.onPrompt(messages[len(messages)-1].Content)
	}
	return &chat.ChatResponse{Content: m.reply}, nil
}
