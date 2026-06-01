package inbox

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/chat"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

type Classifier struct {
	graph *graph.Graph
	llm   chat.LLMClient
}

func NewClassifier(g *graph.Graph, llm chat.LLMClient) *Classifier {
	return &Classifier{
		graph: g,
		llm:   llm,
	}
}

// Run listens for pending captures and classifies them in a background loop.
func (c *Classifier) Run(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.processPending(ctx); err != nil {
				slog.Error("classifier process error", "err", err)
			}
		}
	}
}

// processPending finds unclassified pending captures and assigns an action.
func (c *Classifier) processPending(ctx context.Context) error {
	var pending []*nodes.Capture

	err := c.graph.DoRead(ctx, func(tx *graph.ReadTx) error {
		caps, err := nodes.ListCaptures(ctx, tx, nodes.CaptureFilter{
			Tags: []string{"pending"},
		})
		if err != nil {
			return err
		}
		for _, cap := range caps {
			if cap.SuggestedAction == "" {
				pending = append(pending, cap)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	for _, p := range pending {
		action, err := c.classify(ctx, p.Body)
		if err != nil {
			slog.Error("failed to classify capture", "id", p.ID, "err", err)
			continue
		}

		err = c.graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
			if p.SuggestedAction != "" {
				return nil
			}
			p.SuggestedAction = action
			return nodes.UpdateCapture(ctx, tx, *p, nodes.Author{Name: "classifier"})
		})
		if err != nil {
			slog.Error("failed to save classification", "id", p.ID, "err", err)
		}
	}
	return nil
}

func (c *Classifier) classify(ctx context.Context, text string) (string, error) {
	prompt := fmt.Sprintf("Classify this capture: 'bookmark', 'note', or 'discard'. Output only the single word.\n\nCapture:\n%s", text)
	resp, err := c.llm.Chat(ctx, []chat.Message{{Role: "user", Content: prompt}}, nil)
	if err != nil {
		return "", err
	}
	
	result := strings.ToLower(strings.TrimSpace(resp.Content))
	if strings.Contains(result, "bookmark") {
		return "convert_to_bookmark", nil
	}
	if strings.Contains(result, "discard") {
		return "convert_to_discard", nil
	}
	return "convert_to_note", nil
}
