package inbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/chat"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/ingest"
	"github.com/gabrielassisxyz/kernl/internal/planning"
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

type classificationSuggestion struct {
	Target                      string
	ProjectID                   string
	SuggestedProjectTitle       string
	SuggestedProjectDescription string
	SuggestedInitialTasks       []string
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

	for _, group := range groupPendingCaptures(pending) {
		if err := c.classifyGroup(ctx, group, projects); err != nil {
			slog.Error("failed to classify capture group", "err", err)
		}
	}
	return nil
}

func groupPendingCaptures(pending []*nodes.Capture) [][]*nodes.Capture {
	byBatch := map[string][]*nodes.Capture{}
	var singles [][]*nodes.Capture
	for _, cap := range pending {
		if cap.BatchID == "" {
			singles = append(singles, []*nodes.Capture{cap})
			continue
		}
		byBatch[cap.BatchID] = append(byBatch[cap.BatchID], cap)
	}
	groups := make([][]*nodes.Capture, 0, len(singles)+len(byBatch))
	groups = append(groups, singles...)
	for _, group := range byBatch {
		sort.Slice(group, func(i, j int) bool {
			return group[i].BatchSequence < group[j].BatchSequence
		})
		groups = append(groups, group)
	}
	return groups
}

func (c *Classifier) classifyGroup(ctx context.Context, group []*nodes.Capture, projects []*nodes.Project) error {
	if len(group) == 0 {
		return nil
	}
	suggestions := map[string]classificationSuggestion{}
	if len(group) > 1 {
		batchSuggestions, err := c.classifyBatch(ctx, group, projects)
		if err == nil {
			suggestions = batchSuggestions
		} else {
			slog.Warn("batch classification failed; falling back to per-capture classification", "batch", group[0].BatchID, "err", err)
		}
	}
	for _, cap := range group {
		suggestion, ok := suggestions[cap.ID]
		if !ok {
			var err error
			suggestion, err = c.classify(ctx, cap.Body, projects)
			if err != nil {
				slog.Error("failed to classify capture", "id", cap.ID, "err", err)
				continue
			}
		}
		if err := c.saveSuggestion(ctx, cap, suggestion); err != nil {
			slog.Error("failed to save classification", "id", cap.ID, "err", err)
			continue
		}
		if c.opts.AutoPrep && looksLikeQuestion(cap.Body) {
			go func(captureID string) {
				if _, err := Prep(ctx, c.graph, c.llm, c.opts.VaultRoot, c.opts.DASubdir, captureID); err != nil {
					slog.Error("auto-prep failed", "id", captureID, "err", err)
				}
			}(cap.ID)
		}
	}
	return nil
}

func (c *Classifier) saveSuggestion(ctx context.Context, cap *nodes.Capture, suggestion classificationSuggestion) error {
	if suggestion.Target == "update" {
		if id, _ := ingest.ResolveMergeTargetFor(ctx, c.graph, cap.Body, cap.ID); id == "" {
			suggestion.Target = "note"
		}
	}
	return c.graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		if cap.SuggestedAction != "" {
			return nil
		}
		cap.SuggestedAction = suggestion.Target
		cap.SuggestedProjectID = suggestion.ProjectID
		cap.SuggestedProjectTitle = suggestion.SuggestedProjectTitle
		cap.SuggestedProjectDescription = suggestion.SuggestedProjectDescription
		cap.SuggestedInitialTasks = suggestion.SuggestedInitialTasks
		return nodes.UpdateCapture(ctx, tx, *cap, nodes.Author{Name: "classifier"})
	})
}

// classify asks the LLM which node kind a capture should become and preserves
// enough structured detail for the inbox to create a project when the capture
// is a multi-step effort.
func (c *Classifier) classify(ctx context.Context, text string, projects []*nodes.Project) (classificationSuggestion, error) {
	var projectList strings.Builder
	for _, p := range projects {
		fmt.Fprintf(&projectList, "- %s: %s\n", p.ID, p.Title)
	}
	if projectList.Len() == 0 {
		projectList.WriteString("(no projects exist yet)\n")
	}

	// Read the graph, not just the project titles: a capture often ties to a
	// project only through a term buried in that project's notes (e.g. a
	// sentinel word). Surface the notes matching the capture and, when a note
	// links to a project, name that project so the model can attach the task.
	relevant := c.relatedContext(ctx, text, projects)

	prompt := fmt.Sprintf(`You triage an Inbox capture into the user's Kernl graph.

Pick exactly one target:
- "project": a multi-step outcome, build idea, research/design effort, feature, tool, investigation, or initiative that should own follow-up tasks.
- "task": one concrete action that can be done in one sitting, or a next step inside an existing project.
- "update": the capture extends, revises, or adds a detail to a topic that almost certainly already has its own note in the knowledge base.
- "note": durable knowledge, a question, reflection, or idea to preserve, with no clear execution outcome yet.
- "bookmark": a URL or external reference to save.
- "discard": noise with no value.

Decision rules:
- If the capture implies more than one step, prefer "project" over "task" or "note".
- If it is an actionable build/research idea and no matching project exists, choose "project".
- If it clearly belongs to an existing project, choose "task" and set project_id.
- Use "note" only when the value is remembering/thinking, not doing.
- Do not classify an actionable project idea as "note" just because it is phrased informally.

Projects:
%s
Related notes already in the knowledge base (match the capture against these — if one names a project, prefer that project_id):
%s
Respond with ONLY a JSON object, no prose:
{
  "target": "project|task|update|note|bookmark|discard",
  "project_id": "",
  "project_title": "",
  "project_description": "",
  "initial_tasks": []
}

Field rules:
- project_id: existing project id only, for task when relevant.
- project_title/project_description: only when target is "project".
- initial_tasks: 3-6 short tasks only when target is "project".

Capture:
%s`, projectList.String(), relevant, text)

	resp, err := c.llm.Chat(ctx, []chat.Message{{Role: "user", Content: prompt}}, nil)
	if err != nil {
		return classificationSuggestion{}, err
	}
	return parseClassification(resp.Content, projects), nil
}

func (c *Classifier) classifyBatch(ctx context.Context, group []*nodes.Capture, projects []*nodes.Project) (map[string]classificationSuggestion, error) {
	var projectList strings.Builder
	for _, p := range projects {
		fmt.Fprintf(&projectList, "- %s: %s\n", p.ID, p.Title)
	}
	if projectList.Len() == 0 {
		projectList.WriteString("(no projects exist yet)\n")
	}
	contextTitle := ""
	var batch strings.Builder
	for _, cap := range group {
		if cap.BatchContextTitle != "" {
			contextTitle = cap.BatchContextTitle
		}
		fmt.Fprintf(&batch, "[%d]", cap.BatchSequence)
		if cap.BatchSender != "" {
			fmt.Fprintf(&batch, " %s:", cap.BatchSender)
		}
		fmt.Fprintf(&batch, " %s\n", cap.Body)
	}
	relevant := c.relatedContext(ctx, batch.String(), projects)
	var contextBlock strings.Builder
	if contextTitle != "" {
		fmt.Fprintf(&contextBlock, "Batch context title: %s\n", contextTitle)
	}
	fmt.Fprintf(&contextBlock, "Captures:\n%s", batch.String())
	prompt := fmt.Sprintf(`You triage a batch of related Inbox captures into the user's Kernl graph.

The captures came from one paste/import. They may be fragments of a single project idea. Do not treat each line in isolation.

Pick one target per sequence:
- "project": the sequence is the main multi-step outcome or initiative.
- "task": the sequence is a concrete next step, especially if it belongs to an existing project.
- "update": the sequence extends an existing note.
- "note": durable knowledge with no clear execution outcome.
- "bookmark": a URL or external reference.
- "discard": support text already captured by a project/task suggestion or noise.

When one sequence describes a project and later sequences list tasks for it, put those later task titles in the project's initial_tasks and mark those support sequences as "discard" unless they should also remain standalone.

Projects:
%s
Related notes:
%s
Respond with ONLY a JSON object:
{"items":[{"sequence":0,"target":"project|task|update|note|bookmark|discard","project_id":"","project_title":"","project_description":"","initial_tasks":[]}]}

%s`, projectList.String(), relevant, contextBlock.String())
	resp, err := c.llm.Chat(ctx, []chat.Message{{Role: "user", Content: prompt}}, nil)
	if err != nil {
		return nil, err
	}
	return parseBatchClassification(resp.Content, group, projects), nil
}

// relatedContext searches the graph for notes matching the capture and, when a
// matching note links to (or describes) a known project, records that project
// alongside the note. This is the seam that lets a capture inherit a project
// association from a term that lives only in the project's notes.
func (c *Classifier) relatedContext(ctx context.Context, text string, projects []*nodes.Project) string {
	notesFound, err := planning.BuildContext(ctx, c.graph, text, 6)
	if err != nil || len(notesFound) == 0 {
		return "(no related notes found)\n"
	}

	projectByID := make(map[string]string, len(projects))
	for _, p := range projects {
		projectByID[p.ID] = p.Title
	}

	var b strings.Builder
	_ = c.graph.DoRead(ctx, func(tx *graph.ReadTx) error {
		for _, n := range notesFound {
			// Any outgoing edge to a known project (describes for companion
			// notes, links_to for wikilinks) ties the note to that project.
			var projectHint string
			if outs, oerr := edges.Outgoing(ctx, tx, n.ID); oerr == nil {
				for _, e := range outs {
					if title, ok := projectByID[e.Dst]; ok {
						projectHint = fmt.Sprintf(" [project: %s (%s)]", title, e.Dst)
						break
					}
				}
			}
			fmt.Fprintf(&b, "- %s: %s%s\n", n.Title, n.Snippet, projectHint)
		}
		return nil
	})
	if b.Len() == 0 {
		return "(no related notes found)\n"
	}
	return b.String()
}

// parseClassification extracts the target and project id from the model output,
// tolerating prose around the JSON. The target falls back to "note" when
// unrecognised; project_id is kept only for a task target and only when it
// matches a real project (a hallucinated or stale id is dropped to unfiled).
func parseClassification(raw string, projects []*nodes.Project) classificationSuggestion {
	out := classificationSuggestion{Target: "note"}
	if obj := extractJSONObject(raw); obj != "" {
		var parsed struct {
			Target             string   `json:"target"`
			ProjectID          string   `json:"project_id"`
			ProjectTitle       string   `json:"project_title"`
			ProjectDescription string   `json:"project_description"`
			InitialTasks       []string `json:"initial_tasks"`
		}
		if json.Unmarshal([]byte(obj), &parsed) == nil {
			if t := normalizeTarget(parsed.Target); t != "" {
				out.Target = t
			}
			out.ProjectID = strings.TrimSpace(parsed.ProjectID)
			out.SuggestedProjectTitle = strings.TrimSpace(parsed.ProjectTitle)
			out.SuggestedProjectDescription = strings.TrimSpace(parsed.ProjectDescription)
			out.SuggestedInitialTasks = cleanInitialTasks(parsed.InitialTasks, 6)
		}
	} else {
		// No JSON — fall back to keyword sniffing on the raw text.
		if t := normalizeTarget(raw); t != "" {
			out.Target = t
		}
	}

	if out.Target != "task" {
		out.ProjectID = ""
	}
	if out.Target != "project" {
		out.SuggestedProjectTitle = ""
		out.SuggestedProjectDescription = ""
		out.SuggestedInitialTasks = nil
	}
	if out.Target != "task" {
		return out
	}
	// Validate the project id against the real list; drop anything unknown.
	for _, p := range projects {
		if p.ID == out.ProjectID {
			return out
		}
	}
	out.ProjectID = ""
	return out
}

func parseBatchClassification(raw string, group []*nodes.Capture, projects []*nodes.Project) map[string]classificationSuggestion {
	out := map[string]classificationSuggestion{}
	obj := extractJSONObject(raw)
	if obj == "" {
		return out
	}
	var parsed struct {
		Items []struct {
			Sequence           int      `json:"sequence"`
			Target             string   `json:"target"`
			ProjectID          string   `json:"project_id"`
			ProjectTitle       string   `json:"project_title"`
			ProjectDescription string   `json:"project_description"`
			InitialTasks       []string `json:"initial_tasks"`
		} `json:"items"`
	}
	if json.Unmarshal([]byte(obj), &parsed) != nil {
		return out
	}
	captureBySequence := map[int]*nodes.Capture{}
	for _, cap := range group {
		captureBySequence[cap.BatchSequence] = cap
	}
	for _, item := range parsed.Items {
		cap, ok := captureBySequence[item.Sequence]
		if !ok {
			continue
		}
		suggestion := parseClassification(mustJSON(map[string]any{
			"target":              item.Target,
			"project_id":          item.ProjectID,
			"project_title":       item.ProjectTitle,
			"project_description": item.ProjectDescription,
			"initial_tasks":       item.InitialTasks,
		}), projects)
		out[cap.ID] = suggestion
	}
	return out
}

// normalizeTarget maps free text onto a known target, or "" if none is present.
func normalizeTarget(s string) string {
	s = strings.ToLower(s)
	switch {
	case strings.Contains(s, "project"):
		return "project"
	case strings.Contains(s, "bookmark"):
		return "bookmark"
	case strings.Contains(s, "discard"):
		return "discard"
	case strings.Contains(s, "task"):
		return "task"
	case strings.Contains(s, "update"):
		return "update"
	case strings.Contains(s, "note"):
		return "note"
	}
	return ""
}

func cleanInitialTasks(tasks []string, max int) []string {
	out := make([]string, 0, len(tasks))
	for _, task := range tasks {
		task = strings.TrimSpace(task)
		if task == "" {
			continue
		}
		out = append(out, task)
		if len(out) >= max {
			break
		}
	}
	return out
}

func mustJSON(v any) string {
	data, _ := json.Marshal(v)
	return string(data)
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
