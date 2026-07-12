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
	"github.com/gabrielassisxyz/kernl/internal/graph/tags"
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
			Tags:         []string{tags.Pending},
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
		if cap.SuggestedAction != "bookmark" {
			t.Errorf("expected bookmark, got %q", cap.SuggestedAction)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestParseClassification(t *testing.T) {
	projects := []*nodes.Project{{ID: "p-web", Title: "Web UI"}, {ID: "p-core", Title: "Core"}}
	cases := []struct {
		name        string
		raw         string
		wantTarget  string
		wantProject string
	}{
		{"json task with valid project", `{"target":"task","project_id":"p-web"}`, "task", "p-web"},
		{"json task hallucinated project dropped", `{"target":"task","project_id":"p-ghost"}`, "task", ""},
		{"json note ignores project", `{"target":"note","project_id":"p-web"}`, "note", ""},
		{"json with surrounding prose", "Sure!\n{\"target\": \"bookmark\"}\nDone.", "bookmark", ""},
		{"no json falls back to keyword", "this should be a discard", "discard", ""},
		{"garbage falls back to note", "??!!", "note", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseClassification(tc.raw, projects)
			if got.Target != tc.wantTarget || got.ProjectID != tc.wantProject {
				t.Errorf("parseClassification(%q) = (%q, %q), want (%q, %q)", tc.raw, got.Target, got.ProjectID, tc.wantTarget, tc.wantProject)
			}
		})
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
	capturing := &promptCapturingLLM{onPrompt: func(p string) { seenPrompt = p }, reply: `{"target":"task","project_id":"` + projectID + `"}`}
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
	if got.Target != "task" || got.ProjectID != projectID {
		t.Errorf("expected task attached to %s, got (%q, %q)", projectID, got.Target, got.ProjectID)
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
			{"sequence":0,"target":"project","project_title":"ai-memory explainer","project_description":"Explain ai-memory from the repo context.","initial_tasks":["Map the repo architecture","Write usage examples"]},
			{"sequence":1,"target":"discard"},
			{"sequence":2,"target":"discard"}
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
	if first.SuggestedAction != "project" {
		t.Fatalf("first SuggestedAction = %q, want project", first.SuggestedAction)
	}
	if first.SuggestedProjectTitle != "ai-memory explainer" {
		t.Fatalf("SuggestedProjectTitle = %q", first.SuggestedProjectTitle)
	}
	if len(first.SuggestedInitialTasks) != 2 {
		t.Fatalf("SuggestedInitialTasks = %#v, want 2 tasks", first.SuggestedInitialTasks)
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
