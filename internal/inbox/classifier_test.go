package inbox

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/chat"
	"github.com/gabrielassisxyz/kernl/internal/graph"
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
	classifier := NewClassifier(g, llm)

	err = classifier.processPending(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	err = g.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		cap, err := nodes.GetCapture(context.Background(), tx, capID)
		if err != nil {
			return err
		}
		if cap.SuggestedAction != "convert_to_bookmark" {
			t.Errorf("expected convert_to_bookmark, got %q", cap.SuggestedAction)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
