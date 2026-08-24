package inbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/chat"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

func TestPrep(t *testing.T) {
	ctx := context.Background()
	vaultRoot := t.TempDir()
	g, err := graph.Open(ctx, graph.Config{Path: filepath.Join(t.TempDir(), "graph.db")})
	if err != nil {
		t.Fatalf("graph.Open: %v", err)
	}
	defer g.Close()

	var captureID string
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		id, err := nodes.CreateCapture(ctx, tx, nodes.Capture{Body: "How does a nuclear plant work?", Tags: []string{"pending"}}, nodes.Author{Name: "t"})
		captureID = id
		return err
	}); err != nil {
		t.Fatalf("CreateCapture: %v", err)
	}

	llm := &mockLLM{content: "A nuclear plant boils water with fission heat to spin turbines."}
	noteID, err := Prep(ctx, g, llm, vaultRoot, "DA", captureID, 3)
	if err != nil {
		t.Fatalf("Prep: %v", err)
	}
	if noteID == "" {
		t.Fatal("expected a prep note id")
	}

	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		note, err := nodes.GetNote(ctx, tx, noteID)
		if err != nil {
			return err
		}
		if note.Origin != nodes.OriginPrep {
			t.Errorf("origin = %q, want %q", note.Origin, nodes.OriginPrep)
		}
		if note.Body == "" {
			t.Errorf("expected primer body")
		}
		hasDA, hasPrep := false, false
		for _, tg := range note.Tags {
			if tg == "da" {
				hasDA = true
			}
			if tg == "prep" {
				hasPrep = true
			}
		}
		if !hasDA || !hasPrep {
			t.Errorf("tags = %v, want da+prep", note.Tags)
		}
		// prepared_for edge note -> capture
		out, err := edges.Outgoing(ctx, tx, noteID)
		if err != nil {
			return err
		}
		found := false
		for _, e := range out {
			if e.Label == prepEdgeLabel && e.Dst == captureID {
				found = true
			}
		}
		if !found {
			t.Errorf("expected prepared_for edge note->capture")
		}
		return nil
	}); err != nil {
		t.Fatalf("DoRead: %v", err)
	}

	// Markdown materialized in the DA folder.
	files, _ := os.ReadDir(filepath.Join(vaultRoot, "DA"))
	if len(files) != 1 {
		t.Errorf("expected 1 md file in DA folder, got %d", len(files))
	}

	// Idempotent: a second prep returns the same note, creates nothing new.
	again, err := Prep(ctx, g, llm, vaultRoot, "DA", captureID, 3)
	if err != nil {
		t.Fatalf("Prep (2nd): %v", err)
	}
	if again != noteID {
		t.Errorf("second prep id = %q, want same %q", again, noteID)
	}
}

func TestProcessLinksBriefing(t *testing.T) {
	ctx := context.Background()
	vaultRoot := t.TempDir()
	g, err := graph.Open(ctx, graph.Config{Path: filepath.Join(t.TempDir(), "graph.db")})
	if err != nil {
		t.Fatalf("graph.Open: %v", err)
	}
	defer g.Close()

	var captureID string
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		id, err := nodes.CreateCapture(ctx, tx, nodes.Capture{Body: "Build a skill?", Tags: []string{"pending"}}, nodes.Author{Name: "t"})
		captureID = id
		return err
	}); err != nil {
		t.Fatalf("CreateCapture: %v", err)
	}
	prepID, err := Prep(ctx, g, &mockLLM{content: "primer body"}, vaultRoot, "DA", captureID, 3)
	if err != nil {
		t.Fatalf("Prep: %v", err)
	}
	if err := ProcessCapture(ctx, g, vaultRoot, nil, captureID, ProcessRequest{Actions: []Action{{Target: "task"}}}); err != nil {
		t.Fatalf("ProcessCapture: %v", err)
	}

	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		tasks, err := nodes.ListTasks(ctx, tx, "")
		if err != nil {
			return err
		}
		if len(tasks) != 1 {
			t.Fatalf("expected 1 task, got %d", len(tasks))
		}
		got, err := BriefingFor(ctx, tx, tasks[0].ID)
		if err != nil {
			return err
		}
		if got != prepID {
			t.Errorf("BriefingFor(task) = %q, want prep %q", got, prepID)
		}
		return nil
	}); err != nil {
		t.Fatalf("DoRead: %v", err)
	}
}

func TestDiscardDeletesPrep(t *testing.T) {
	ctx := context.Background()
	vaultRoot := t.TempDir()
	g, err := graph.Open(ctx, graph.Config{Path: filepath.Join(t.TempDir(), "graph.db")})
	if err != nil {
		t.Fatalf("graph.Open: %v", err)
	}
	defer g.Close()

	var captureID string
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		id, err := nodes.CreateCapture(ctx, tx, nodes.Capture{Body: "noise?", Tags: []string{"pending"}}, nodes.Author{Name: "t"})
		captureID = id
		return err
	}); err != nil {
		t.Fatalf("CreateCapture: %v", err)
	}
	prepID, err := Prep(ctx, g, &mockLLM{content: "primer"}, vaultRoot, "DA", captureID, 3)
	if err != nil {
		t.Fatalf("Prep: %v", err)
	}
	if err := ProcessCapture(ctx, g, vaultRoot, nil, captureID, ProcessRequest{Actions: []Action{{Target: "discard"}}}); err != nil {
		t.Fatalf("ProcessCapture discard: %v", err)
	}

	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		if _, err := nodes.GetNote(ctx, tx, prepID); err == nil {
			t.Errorf("expected prep note deleted on discard")
		}
		return nil
	}); err != nil {
		t.Fatalf("DoRead: %v", err)
	}
	// Prep markdown removed from the DA folder.
	files, _ := os.ReadDir(filepath.Join(vaultRoot, "DA"))
	if len(files) != 0 {
		t.Errorf("expected prep md removed, got %d files", len(files))
	}
}

func TestLooksLikeQuestion(t *testing.T) {
	cases := map[string]bool{
		"How does X work?":            true,
		"Como funciona uma usina?":    true,
		"Qual a melhor abordagem":     true,
		"Buy milk":                    false,
		"Refactor the dispatch layer": false,
	}
	for body, want := range cases {
		if got := looksLikeQuestion(body); got != want {
			t.Errorf("looksLikeQuestion(%q) = %v, want %v", body, got, want)
		}
	}
}

// capturingLLM records the user prompt it was asked to complete, so a test can
// assert what context Prep gathered without depending on the model's answer.
type capturingLLM struct {
	prompt string
}

func (m *capturingLLM) Chat(ctx context.Context, messages []chat.Message, tools []chat.Tool) (*chat.ChatResponse, error) {
	for _, msg := range messages {
		if msg.Role == "user" {
			m.prompt = msg.Content
		}
	}
	return &chat.ChatResponse{Content: "primer"}, nil
}

// TestPrep_StillMatchesBodies guards the shared search.Search against a change
// that narrows all callers to title-only at once: a bookmark is found by what
// it says, not by its title, so a term in the description only must still reach
// the prep prompt.
func TestPrep_StillMatchesBodies(t *testing.T) {
	ctx := context.Background()
	vaultRoot := t.TempDir()
	g, err := graph.Open(ctx, graph.Config{Path: filepath.Join(t.TempDir(), "graph.db")})
	if err != nil {
		t.Fatalf("graph.Open: %v", err)
	}
	defer g.Close()

	var captureID string
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		id, err := nodes.CreateCapture(ctx, tx, nodes.Capture{Body: "resticprofile backup", Tags: []string{"pending"}}, nodes.Author{Name: "t"})
		if err != nil {
			return err
		}
		captureID = id
		_, err = nodes.CreateBookmark(ctx, tx, nodes.Bookmark{
			Title:       "Unrelated bookmark",
			URL:         "https://example.com",
			Description: "resticprofile backs up to a remote repository.",
		}, nodes.Author{Name: "t"})
		return err
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	llm := &capturingLLM{}
	if _, err := Prep(ctx, g, llm, vaultRoot, "DA", captureID, 3); err != nil {
		t.Fatalf("Prep: %v", err)
	}
	if !strings.Contains(llm.prompt, "Unrelated bookmark") {
		t.Errorf("expected the body-only bookmark match in the prep prompt, got:\n%s", llm.prompt)
	}
}
