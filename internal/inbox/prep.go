package inbox

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/chat"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/search"
	"github.com/gabrielassisxyz/kernl/internal/planning"
)

// prepEdgeLabel ties a DA primer note to the capture it was prepared for.
const prepEdgeLabel = "prepared_for"

// briefingEdgeLabel ties a processed node (task/note/bookmark) to the DA primer
// prepared for its originating capture, so the briefing surfaces 1-hop when the
// user opens the node to act on it.
const briefingEdgeLabel = "briefing"

// Prep generates a DA "briefing" for a capture: a sectioned work brief (read,
// viability, approach, draft plan, open questions) drawn from the model's
// knowledge and grounded in the user's related notes, bookmarks, and (when the
// classifier suggested one) project. The brief is persisted as a DA-owned note,
// markdown in the vault's DA subfolder tagged `da`/`prep` and never mixed with
// the user's own notes, linked to the capture with a prepared_for edge. It is
// idempotent: a capture already prepped returns its existing note. Returns the
// prep note id.
func Prep(ctx context.Context, g *graph.Graph, llm chat.LLMClient, vaultRoot, daSubdir, captureID string) (string, error) {
	if llm == nil {
		return "", fmt.Errorf("prep: no llm configured")
	}

	var capture *nodes.Capture
	var existingPrep string
	var bookmarkTitles []string
	var project *nodes.Project
	var projectTasks []*nodes.Task

	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		capture, err = nodes.GetCapture(ctx, tx, captureID)
		if err != nil {
			return fmt.Errorf("get capture: %w", err)
		}

		// Idempotency: a capture already has a primer?
		in, err := edges.Incoming(ctx, tx, captureID)
		if err != nil {
			return err
		}
		for _, e := range in {
			if e.Label == prepEdgeLabel {
				existingPrep = e.Src
				return nil
			}
		}

		// Related bookmarks (FTS over salient terms).
		seen := map[string]bool{}
		for _, term := range salientTerms(capture.Body, 6) {
			hits, err := search.Search(ctx, tx, term, search.WithTypes("bookmark"))
			if err != nil {
				continue
			}
			for _, h := range hits {
				if seen[h.NodeID] {
					continue
				}
				seen[h.NodeID] = true
				bookmarkTitles = append(bookmarkTitles, h.Title)
				if len(bookmarkTitles) >= 5 {
					break
				}
			}
		}

		// Related project (only when the classifier already tied one in). A
		// capture can fan out into several actions; the first one naming an
		// existing project is the one worth briefing against.
		if projectID := suggestedProjectID(capture); projectID != "" {
			project, _ = nodes.GetProject(ctx, tx, projectID)
			projectTasks, _ = nodes.ListTasks(ctx, tx, projectID)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if existingPrep != "" {
		return existingPrep, nil
	}

	// Related notes (own read transaction inside BuildContext).
	notesCtx, _ := planning.BuildContext(ctx, g, capture.Body, 5)

	resp, err := llm.Chat(ctx, []chat.Message{{Role: "user", Content: buildPrepPrompt(capture, notesCtx, bookmarkTitles, project, projectTasks)}}, nil)
	if err != nil {
		return "", fmt.Errorf("prep llm: %w", err)
	}
	primer := strings.TrimSpace(resp.Content)
	if primer == "" {
		return "", fmt.Errorf("prep: empty primer")
	}

	title := "Briefing: " + captureDisplayTitle(capture)
	author := nodes.Author{Name: "da"}
	daDir := filepath.Join(vaultRoot, daSubdir)
	if err := os.MkdirAll(daDir, 0755); err != nil {
		return "", fmt.Errorf("prep: mkdir da dir: %w", err)
	}

	var noteID string
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		noteID, err = nodes.CreateNote(ctx, tx, nodes.Note{
			Title:  title,
			Body:   primer,
			Origin: "da",
			Tags:   []string{"da", "prep"},
		}, author)
		if err != nil {
			return fmt.Errorf("create da note: %w", err)
		}
		if _, err := edges.Create(ctx, tx, edges.Edge{
			Src:   noteID,
			Dst:   captureID,
			Label: prepEdgeLabel,
		}, author); err != nil {
			return fmt.Errorf("create prepared_for edge: %w", err)
		}

		slug := "prep-" + time.Now().Format("20060102150405")
		md := fmt.Sprintf("---\nid: %s\ntitle: %q\ntags: [da, prep]\norigin: da\n---\n\n%s\n", noteID, title, primer)
		if err := os.WriteFile(filepath.Join(daDir, slug+".md"), []byte(md), 0644); err != nil {
			return fmt.Errorf("write da note md: %w", err)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return noteID, nil
}

// buildPrepPrompt assembles the single-shot primer prompt from the capture and
// whatever related graph material was found.
func buildPrepPrompt(c *nodes.Capture, notes []planning.ContextNote, bookmarks []string, project *nodes.Project, tasks []*nodes.Task) string {
	var b strings.Builder
	b.WriteString("You are the user's DA preparing an Inbox item for future action.\n\n")
	b.WriteString("Goal: create a compact work brief that reduces the user's future planning work and reduces the future DA work needed to act on this in a new session. ")
	b.WriteString("Do not summarize the item from a distance. Write as if the user or a future DA session is about to start working on it now. ")
	b.WriteString("Write in the same language as the capture.\n\n")
	b.WriteString("Use the capture as the concrete request, idea, or problem; use related notes, bookmarks, and projects as grounding; and use your own technical/domain knowledge when helpful.\n\n")
	b.WriteString("Output markdown only with these sections:\n")
	b.WriteString("## Read\nWhat this is really asking for, in 1-2 direct sentences.\n\n")
	b.WriteString("## Viability\nA short assessment of feasibility, likely scope, and important unknowns.\n\n")
	b.WriteString("## Suggested Approach\n3-6 concrete bullets. Prefer implementation hints, data sources, commands, APIs, files, or search terms when relevant.\n\n")
	b.WriteString("## Draft Plan\nIf this should become a project, propose a project title and 3-6 initial tasks. If it is a small task, propose the one best task. If it is a note, state the note angle and the durable idea to preserve.\n\n")
	b.WriteString("## Questions / Checks\n0-3 questions or checks only if they materially change execution.\n\n")
	b.WriteString("Rules:\n")
	b.WriteString("- Never write phrases like \"this capture proposes\", \"the user captured\", or \"this note is about\".\n")
	b.WriteString("- Do not merely restate the capture.\n")
	b.WriteString("- Be specific about what you infer, and label uncertainty.\n")
	b.WriteString("- Reference related graph material only when it is actually useful.\n")
	b.WriteString("- Do not invent facts about the user's graph.\n")
	b.WriteString("- No preamble, no sign-off.\n\n")
	fmt.Fprintf(&b, "Capture:\n%s\n\n", strings.TrimSpace(c.Body))

	if len(notes) > 0 {
		b.WriteString("Related notes:\n")
		for _, n := range notes {
			fmt.Fprintf(&b, "- %s: %s\n", n.Title, n.Snippet)
		}
		b.WriteString("\n")
	}
	if len(bookmarks) > 0 {
		b.WriteString("Related bookmarks:\n")
		for _, t := range bookmarks {
			fmt.Fprintf(&b, "- %s\n", t)
		}
		b.WriteString("\n")
	}
	if project != nil {
		fmt.Fprintf(&b, "Related project: %s", project.Title)
		if project.Description != "" {
			fmt.Fprintf(&b, " — %s", project.Description)
		}
		b.WriteString("\n")
		if len(tasks) > 0 {
			b.WriteString("Project tasks:\n")
			for _, t := range tasks {
				fmt.Fprintf(&b, "- %s\n", t.Title)
			}
		}
		b.WriteString("\n")
	}
	b.WriteString("Work brief:\n")
	return b.String()
}

// suggestedProjectID returns the first existing project the classifier tied any
// of the capture's suggested actions to, or "" when none does.
func suggestedProjectID(c *nodes.Capture) string {
	for _, action := range c.SuggestedActions {
		if action.ProjectID != "" {
			return action.ProjectID
		}
	}
	return ""
}

// salientTerms pulls up to max distinct content words (len >= 4) from s for
// retrieval. Deliberately simple; the primer prompt does the heavy lifting.
func salientTerms(s string, max int) []string {
	seen := map[string]bool{}
	var out []string
	for _, raw := range strings.Fields(strings.ToLower(s)) {
		t := strings.Trim(raw, ".,;:!?\"'()[]{}")
		if len(t) < 4 || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
		if len(out) >= max {
			break
		}
	}
	return out
}

// looksLikeQuestion is the heuristic the classifier uses to decide whether to
// auto-generate a primer: the capture reads as a question.
func looksLikeQuestion(body string) bool {
	b := strings.TrimSpace(body)
	if strings.HasSuffix(b, "?") {
		return true
	}
	lower := strings.ToLower(b)
	for _, w := range []string{"how ", "what ", "why ", "when ", "como ", "qual ", "quais ", "por que ", "porque ", "o que "} {
		if strings.HasPrefix(lower, w) {
			return true
		}
	}
	return false
}

// PrepFor returns the id of the prep note attached to a capture, or "" if none.
func PrepFor(ctx context.Context, tx *graph.ReadTx, captureID string) (string, error) {
	in, err := edges.Incoming(ctx, tx, captureID)
	if err != nil {
		return "", err
	}
	for _, e := range in {
		if e.Label == prepEdgeLabel {
			return e.Src, nil
		}
	}
	return "", nil
}

// BriefingFor returns the id of the prep note surfaced for a processed node
// (via its briefing edge), or "" if none.
func BriefingFor(ctx context.Context, tx *graph.ReadTx, nodeID string) (string, error) {
	out, err := edges.Outgoing(ctx, tx, nodeID)
	if err != nil {
		return "", err
	}
	for _, e := range out {
		if e.Label == briefingEdgeLabel {
			return e.Dst, nil
		}
	}
	return "", nil
}
