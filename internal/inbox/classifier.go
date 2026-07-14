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
	"github.com/gabrielassisxyz/kernl/internal/inbox/wire"
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
			actions, err = c.classify(ctx, cap, projects)
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
func (c *Classifier) classify(ctx context.Context, capture *nodes.Capture, projects []*nodes.Project) ([]nodes.CaptureAction, error) {
	text := capture.Body
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
%s
Respond with ONLY a JSON object, no prose:
{"actions":[{"target":"project|task|update|note|bookmark|discard","title":"","body":"","project_id":"","project_title":"","project_description":"","initial_tasks":[],"tags":[],"due_date":null}]}

%s

Capture:
%s`, targetVocabulary, projectList.String(), relevant, dateAnchorBlock([]time.Time{captureReferenceTime(capture)}), actionFieldRules, text)

	resp, err := c.llm.Chat(ctx, []chat.Message{{Role: "user", Content: prompt}}, nil)
	if err != nil {
		return nil, err
	}
	return parseActions(resp.Content, projects), nil
}

// captureReferenceTime is the moment a capture was WRITTEN — the only sane
// origin for a relative deadline. The WhatsApp header timestamp wins when there
// is one, because a paste of a months-old export is created in the graph today:
// "amanhã" in a message from April 1st is April 2nd, not tomorrow.
func captureReferenceTime(capture *nodes.Capture) time.Time {
	if ref, ok := parseBatchTimestamp(capture.BatchTimestamp); ok {
		return ref
	}
	if !capture.CreatedAt.IsZero() {
		return capture.CreatedAt
	}
	return time.Now()
}

// parseBatchTimestamp reads the "date time" string parseWhatsAppHeader produced.
// WhatsApp writes the date in the exporting phone's locale; the exports we
// handle are month-first (4/1/26 is April 1st), so that is tried first and
// day-first only as the fallback that rescues a day past the 12th.
func parseBatchTimestamp(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	layouts := []string{
		"1/2/06 15:04", "1/2/06 15:04:05", "1/2/2006 15:04", "1/2/2006 15:04:05",
		"2/1/06 15:04", "2/1/06 15:04:05", "2/1/2006 15:04", "2/1/2006 15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// dateAnchorBlock hands the model a lookup table instead of a date to compute:
// every relative word a capture might use, already resolved against the day that
// capture was written. The model's job is to COPY one of these, never to do
// arithmetic and never to reach for the real today.
func dateAnchorBlock(refs []time.Time) string {
	var b strings.Builder
	b.WriteString("Date anchors. A relative deadline is resolved against the day the CAPTURE was written — NEVER against the real current date, which is irrelevant here (this inbox is months behind). Copy the date from the line for that capture:\n")
	seen := map[string]bool{}
	for _, ref := range refs {
		line := dateAnchorLine(ref)
		if seen[line] {
			continue
		}
		seen[line] = true
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// dateAnchorLine resolves the relative words that actually turn up in a WhatsApp
// inbox. A weekday means its NEXT occurrence after the capture ("by friday" said
// on a Wednesday is that same week's Friday); the capture's own weekday means a
// week later, which is what "next wednesday" means when said on a Wednesday.
func dateAnchorLine(ref time.Time) string {
	day := ref.Format(nodes.DueDateLayout)
	var b strings.Builder
	fmt.Fprintf(&b, "- written %s (%s): today=%s tomorrow=%s day-after-tomorrow=%s",
		day, ref.Weekday(), day,
		ref.AddDate(0, 0, 1).Format(nodes.DueDateLayout),
		ref.AddDate(0, 0, 2).Format(nodes.DueDateLayout),
	)
	for i := 1; i <= 7; i++ {
		next := ref.AddDate(0, 0, i)
		fmt.Fprintf(&b, " %s=%s", strings.ToLower(next.Weekday().String()), next.Format(nodes.DueDateLayout))
	}
	fmt.Fprintf(&b, " next-week=%s", ref.AddDate(0, 0, 7).Format(nodes.DueDateLayout))
	return b.String()
}

// The shared definition of what each node kind means and when a capture must be
// split into several. Both prompts here embed it verbatim, so the single-capture
// and batch paths cannot drift apart.
//
// It lives in inbox/wire because the chat engine's routing mode embeds the same
// text and cannot import this package without a cycle. Aliased here so the
// prompt sites below read as they always did.
const (
	targetVocabulary = wire.TargetVocabulary
	actionFieldRules = wire.ActionFieldRules
)

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
	refs := make([]time.Time, 0, len(group))
	for _, cap := range group {
		if cap.BatchContextTitle != "" {
			contextTitle = cap.BatchContextTitle
		}
		ref := captureReferenceTime(cap)
		refs = append(refs, ref)
		// Each capture carries the day it was written: a paste spans days, so a
		// deadline is relative to ITS line, not to the batch or to today.
		fmt.Fprintf(&batch, "[%d] (written %s)", cap.BatchSequence, ref.Format(nodes.DueDateLayout))
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

When one sequence describes a project and LATER SEQUENCES list tasks for it, put those later task titles in the project's initial_tasks and mark those support sequences as "discard" unless they should also remain standalone. This is about grouping ACROSS sequences — a list of items inside a SINGLE sequence still splits into one action per item, by the rules above.

Projects:
%s
Related notes:
%s
%s
Respond with ONLY a JSON object. Each sequence gets its OWN LIST of actions:
{"items":[{"sequence":0,"actions":[{"target":"project|task|update|note|bookmark|discard","title":"","body":"","project_id":"","project_title":"","project_description":"","initial_tasks":[],"tags":[],"due_date":null}]}]}

%s

%s`, targetVocabulary, projectList.String(), relevant, dateAnchorBlock(refs), actionFieldRules, contextBlock.String())
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
	DueDate            string   `json:"due_date"`
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
// real project (a hallucinated or stale id is dropped to unfiled), keeps the
// project fields only on a project, and keeps a due date only on a task and only
// when it is a date we can read.
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
			// A date we cannot read is dropped, not guessed at: this is a
			// proposal the human reviews, and a wrong deadline is worse than none.
			action.DueDate, _ = nodes.ParseDueDate(item.DueDate)
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

// cleanTags normalizes the model's tags and drops anything off the closed
// vocabulary. The prompt says "never coin a tag"; an LLM still occasionally does,
// and a coined tag is worse than no tag — it fragments the vocabulary silently
// and filters nothing ("capture" on a capture-derived note).
func cleanTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag != "" {
			out = append(out, tag)
		}
	}
	out = wire.FilterTags(out)
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
