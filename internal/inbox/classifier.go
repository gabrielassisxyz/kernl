package inbox

import (
	"context"
	"encoding/json"
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
	opts  ClassifierOptions
}

// ClassifierOptions configures the optional proactive primer (Prep). When
// AutoPrep is true, captures the classifier reads as questions get a DA briefing
// generated in the background.
type ClassifierOptions struct {
	AutoPrep  bool
	VaultRoot string
	DASubdir  string
}

func NewClassifier(g *graph.Graph, llm chat.LLMClient, opts ClassifierOptions) *Classifier {
	return &Classifier{
		graph: g,
		llm:   llm,
		opts:  opts,
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

// processPending finds unclassified pending captures and assigns a suggestion.
func (c *Classifier) processPending(ctx context.Context) error {
	var pending []*nodes.Capture
	var projects []*nodes.Project

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
		projects, err = nodes.ListProjects(ctx, tx)
		return err
	})
	if err != nil {
		return err
	}

	for _, p := range pending {
		target, projectID, err := c.classify(ctx, p.Body, projects)
		if err != nil {
			slog.Error("failed to classify capture", "id", p.ID, "err", err)
			continue
		}

		err = c.graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
			if p.SuggestedAction != "" {
				return nil
			}
			p.SuggestedAction = target
			p.SuggestedProjectID = projectID
			return nodes.UpdateCapture(ctx, tx, *p, nodes.Author{Name: "classifier"})
		})
		if err != nil {
			slog.Error("failed to save classification", "id", p.ID, "err", err)
			continue
		}

		// Proactively brief the user on captures that read as questions.
		if c.opts.AutoPrep && looksLikeQuestion(p.Body) {
			go func(captureID, body string) {
				if _, err := Prep(ctx, c.graph, c.llm, c.opts.VaultRoot, c.opts.DASubdir, captureID); err != nil {
					slog.Error("auto-prep failed", "id", captureID, "err", err)
				}
			}(p.ID, p.Body)
		}
	}
	return nil
}

// classify asks the LLM which node kind a capture should become and, for a task,
// which existing project (if any) it belongs under. It returns a clean target
// ("note" | "bookmark" | "task" | "discard") that maps directly onto the
// inbox /process endpoint, plus a project id (empty when none fits).
func (c *Classifier) classify(ctx context.Context, text string, projects []*nodes.Project) (target, projectID string, err error) {
	var projectList strings.Builder
	for _, p := range projects {
		fmt.Fprintf(&projectList, "- %s: %s\n", p.ID, p.Title)
	}
	if projectList.Len() == 0 {
		projectList.WriteString("(no projects exist yet)\n")
	}

	prompt := fmt.Sprintf(`You triage a captured thought into the user's knowledge graph.

Pick exactly one target:
- "bookmark": the capture is a URL or a reference to save.
- "task": the capture is an actionable to-do. If it clearly belongs to one of the projects below, set project_id to that project's id; otherwise leave project_id empty.
- "note": an idea, question, or piece of knowledge to keep.
- "discard": noise with no value.

Projects:
%s
Respond with ONLY a JSON object, no prose: {"target": "...", "project_id": "..."}

Capture:
%s`, projectList.String(), text)

	resp, err := c.llm.Chat(ctx, []chat.Message{{Role: "user", Content: prompt}}, nil)
	if err != nil {
		return "", "", err
	}
	target, projectID = parseClassification(resp.Content, projects)
	return target, projectID, nil
}

// parseClassification extracts the target and project id from the model output,
// tolerating prose around the JSON. The target falls back to "note" when
// unrecognised; project_id is kept only for a task target and only when it
// matches a real project (a hallucinated or stale id is dropped to unfiled).
func parseClassification(raw string, projects []*nodes.Project) (target, projectID string) {
	target = "note"
	if obj := extractJSONObject(raw); obj != "" {
		var parsed struct {
			Target    string `json:"target"`
			ProjectID string `json:"project_id"`
		}
		if json.Unmarshal([]byte(obj), &parsed) == nil {
			if t := normalizeTarget(parsed.Target); t != "" {
				target = t
			}
			projectID = strings.TrimSpace(parsed.ProjectID)
		}
	} else {
		// No JSON — fall back to keyword sniffing on the raw text.
		if t := normalizeTarget(raw); t != "" {
			target = t
		}
	}

	if target != "task" {
		return target, ""
	}
	// Validate the project id against the real list; drop anything unknown.
	for _, p := range projects {
		if p.ID == projectID {
			return target, projectID
		}
	}
	return target, ""
}

// normalizeTarget maps free text onto a known target, or "" if none is present.
func normalizeTarget(s string) string {
	s = strings.ToLower(s)
	switch {
	case strings.Contains(s, "bookmark"):
		return "bookmark"
	case strings.Contains(s, "discard"):
		return "discard"
	case strings.Contains(s, "task"):
		return "task"
	case strings.Contains(s, "note"):
		return "note"
	}
	return ""
}

// extractJSONObject returns the first {...} span in s, or "" if there is none.
func extractJSONObject(s string) string {
	i := strings.IndexByte(s, '{')
	j := strings.LastIndexByte(s, '}')
	if i < 0 || j <= i {
		return ""
	}
	return s[i : j+1]
}
