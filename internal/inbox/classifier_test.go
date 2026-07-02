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
			gotTarget, gotProject := parseClassification(tc.raw, projects)
			if gotTarget != tc.wantTarget || gotProject != tc.wantProject {
				t.Errorf("parseClassification(%q) = (%q, %q), want (%q, %q)", tc.raw, gotTarget, gotProject, tc.wantTarget, tc.wantProject)
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

	target, gotProject, err := c.classify(ctx, "turn azimute-cobalto-17 into a task", []*nodes.Project{{ID: projectID, Title: "Atlas Verde"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(seenPrompt, "azimute-cobalto-17") {
		t.Errorf("prompt should include the matching note body, got:\n%s", seenPrompt)
	}
	if !strings.Contains(seenPrompt, projectID) {
		t.Error("prompt should surface the note's project association")
	}
	if target != "task" || gotProject != projectID {
		t.Errorf("expected task attached to %s, got (%q, %q)", projectID, target, gotProject)
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
