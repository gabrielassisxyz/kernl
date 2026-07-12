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
			if len(cap.SuggestedActions) == 0 {
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
	suggestions := map[string][]nodes.CaptureAction{}
	if len(group) > 1 {
		batchSuggestions, err := c.classifyBatch(ctx, group, projects)
		if err == nil {
			suggestions = batchSuggestions
		} else {
			slog.Warn("batch classification failed; falling back to per-capture classification", "batch", group[0].BatchID, "err", err)
		}
	}
	for _, cap := range group {
		actions, ok := suggestions[cap.ID]
		if !ok {
			var err error
			actions, err = c.classify(ctx, cap.Body, projects)
			if err != nil {
				slog.Error("failed to classify capture", "id", cap.ID, "err", err)
				continue
			}
		}
		if err := c.saveSuggestion(ctx, cap, actions); err != nil {
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

// saveSuggestion persists the proposed actions on the capture. An "update" that
// resolves to no existing note is demoted to a plain note, so a suggestion the
// user accepts can never lose the capture. An update is exclusive (ProcessCapture
// rejects it alongside other actions), so a fan-out that contains one drops it.
func (c *Classifier) saveSuggestion(ctx context.Context, cap *nodes.Capture, actions []nodes.CaptureAction) error {
	for i := range actions {
		if actions[i].Target != "update" {
			continue
		}
		if len(actions) > 1 {
			actions[i].Target = "note"
			continue
		}
		if id, _ := ingest.ResolveMergeTargetFor(ctx, c.graph, cap.Body, cap.ID); id == "" {
			actions[i].Target = "note"
		}
	}
	return c.graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		if len(cap.SuggestedActions) > 0 {
			return nil
		}
		cap.SuggestedActions = actions
		return nodes.UpdateCapture(ctx, tx, *cap, nodes.Author{Name: "classifier"})
	})
}

// classify asks the LLM what a capture should become. The answer is a LIST:
// a capture is routinely several things — a reflection that also implies a next
// step, a "tomorrow:" message that is four tasks — and collapsing that into one
// node is where information was being lost.
func (c *Classifier) classify(ctx context.Context, text string, projects []*nodes.Project) ([]nodes.CaptureAction, error) {
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

%s
Projects:
%s
Related notes already in the knowledge base (match the capture against these — if one names a project, prefer that project_id):
%s
Respond with ONLY a JSON object, no prose:
{"actions":[{"target":"project|task|update|note|bookmark|discard","title":"","body":"","project_id":"","project_title":"","project_description":"","initial_tasks":[]}]}

%s

Capture:
%s`, targetVocabulary, projectList.String(), relevant, actionFieldRules, text)

	resp, err := c.llm.Chat(ctx, []chat.Message{{Role: "user", Content: prompt}}, nil)
	if err != nil {
		return nil, err
	}
	return parseActions(resp.Content, projects), nil
}

// targetVocabulary is the shared definition of what each node kind means and
// when a capture must be split into several. Both prompts embed it verbatim so
// the single-capture and batch paths cannot drift apart.
const targetVocabulary = `A capture is often MORE THAN ONE THING. Return one action per distinct item — never fold two things into one.

Targets:
- "project": anything that can be broken into smaller actionable pieces. This is the rule: if it decomposes, it is a project — no matter how small it sounds.
- "task": one concrete action, done in one sitting, indivisible. A question is a task (answering it is the action; the note is what gets written once it is answered).
- "update": the capture extends or revises a topic that almost certainly already has its own note. Use it alone, never combined with other actions.
- "note": durable knowledge, a reflection, or an insight worth preserving.
- "bookmark": a URL or external reference to save.
- "discard": this fragment is noise. Discarding one action does not discard the capture.

Splitting rules:
- A message holding several items (a "tomorrow:" list, two unrelated ideas typed in one go) yields ONE ACTION PER ITEM.
- A reflection that also implies an action is a "note" AND a "task".
- A verb-initial bookmark ("Reread: <url>", "Watch: <url>") is a "bookmark" AND a "task".
- Do not shrink a project into a task because it sounds small; do not classify an actionable idea as a note because it is phrased informally.`

// actionFieldRules describes the per-action fields. The title rule is the one
// that makes a long paste reviewable: the user reads titles, not bodies.
const actionFieldRules = `Field rules:
- title: ALWAYS write one. Short, imperative, human. Never the truncated body.
- body: the fragment of the capture this action owns. Omit when the action owns the whole capture.
- project_id: an existing project id from the list above, for a task that belongs to it.
- project_title/project_description/initial_tasks: only for "project"; 3-6 short initial_tasks.`

func (c *Classifier) classifyBatch(ctx context.Context, group []*nodes.Capture, projects []*nodes.Project) (map[string][]nodes.CaptureAction, error) {
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

%s

When one sequence describes a project and later sequences list tasks for it, put those later task titles in the project's initial_tasks and mark those support sequences as "discard" unless they should also remain standalone.

Projects:
%s
Related notes:
%s
Respond with ONLY a JSON object. Each sequence gets its OWN LIST of actions:
{"items":[{"sequence":0,"actions":[{"target":"project|task|update|note|bookmark|discard","title":"","body":"","project_id":"","project_title":"","project_description":"","initial_tasks":[]}]}]}

%s

%s`, targetVocabulary, projectList.String(), relevant, actionFieldRules, contextBlock.String())
	resp, err := c.llm.Chat(ctx, []chat.Message{{Role: "user", Content: prompt}}, nil)
	if err != nil {
		return nil, err
	}
	return parseBatchActions(resp.Content, group, projects), nil
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

// rawAction is the model's proposal for one node, before normalization.
type rawAction struct {
	Target             string   `json:"target"`
	Title              string   `json:"title"`
	Body               string   `json:"body"`
	ProjectID          string   `json:"project_id"`
	ProjectTitle       string   `json:"project_title"`
	ProjectDescription string   `json:"project_description"`
	InitialTasks       []string `json:"initial_tasks"`
	Tags               []string `json:"tags"`
}

// parseActions extracts the proposed action list from the model output,
// tolerating prose around the JSON. Output that carries no usable action at all
// falls back to a single note, so a capture is never dropped on a bad response.
func parseActions(raw string, projects []*nodes.Project) []nodes.CaptureAction {
	obj := extractJSONObject(raw)
	if obj == "" {
		// No JSON — fall back to keyword sniffing on the raw text.
		return []nodes.CaptureAction{{Target: fallbackTarget(raw)}}
	}
	var parsed struct {
		Actions []rawAction `json:"actions"`
	}
	if json.Unmarshal([]byte(obj), &parsed) != nil {
		return []nodes.CaptureAction{{Target: fallbackTarget(raw)}}
	}
	out := normalizeActions(parsed.Actions, projects)
	if len(out) == 0 {
		return []nodes.CaptureAction{{Target: fallbackTarget(raw)}}
	}
	return out
}

func parseBatchActions(raw string, group []*nodes.Capture, projects []*nodes.Project) map[string][]nodes.CaptureAction {
	out := map[string][]nodes.CaptureAction{}
	obj := extractJSONObject(raw)
	if obj == "" {
		return out
	}
	var parsed struct {
		Items []struct {
			Sequence int         `json:"sequence"`
			Actions  []rawAction `json:"actions"`
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
		actions := normalizeActions(item.Actions, projects)
		if len(actions) == 0 {
			continue // leave it to the per-capture fallback in classifyGroup
		}
		out[cap.ID] = actions
	}
	return out
}

// normalizeActions cleans the model's proposals: it drops actions with no
// recognisable target, keeps project_id only on a task and only when it names a
// real project (a hallucinated or stale id is dropped to unfiled), and keeps the
// project fields only on a project.
func normalizeActions(raw []rawAction, projects []*nodes.Project) []nodes.CaptureAction {
	out := make([]nodes.CaptureAction, 0, len(raw))
	for _, item := range raw {
		target := normalizeTarget(item.Target)
		if target == "" {
			continue
		}
		action := nodes.CaptureAction{
			Target: target,
			Title:  strings.TrimSpace(item.Title),
			Body:   strings.TrimSpace(item.Body),
			Tags:   cleanTags(item.Tags),
		}
		if target == "task" {
			action.ProjectID = knownProjectID(strings.TrimSpace(item.ProjectID), projects)
		}
		if target == "project" {
			action.ProjectTitle = strings.TrimSpace(item.ProjectTitle)
			action.ProjectDescription = strings.TrimSpace(item.ProjectDescription)
			action.InitialTasks = cleanInitialTasks(item.InitialTasks, 6)
		}
		out = append(out, action)
	}
	return out
}

// fallbackTarget sniffs a target out of unstructured model output; anything
// unrecognisable becomes a note, which preserves the capture.
func fallbackTarget(raw string) string {
	if t := normalizeTarget(raw); t != "" {
		return t
	}
	return "note"
}

func knownProjectID(id string, projects []*nodes.Project) string {
	for _, p := range projects {
		if p.ID == id {
			return id
		}
	}
	return ""
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

func cleanTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag != "" {
			out = append(out, tag)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
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
