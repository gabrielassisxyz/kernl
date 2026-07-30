package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// defaultBrBin is the PATH-resolved br binary used when BR_BIN is unset.
const defaultBrBin = "br"

// BrCliBackend speaks to br (beads_rust), the tracker the repositories this
// orchestrator serves actually use.
//
// It implements only what driving an epic exercises. Every other method of
// BackendPort fails loud rather than half-working: a tracker adapter that
// silently does nothing is indistinguishable from a tracker with nothing to do,
// which is the failure mode this whole adapter exists to remove.
type BrCliBackend struct {
	repoPath string
	brBin    string
}

var _ BackendPort = (*BrCliBackend)(nil)

func NewBrCliBackend(repoPath string) *BrCliBackend {
	brBin := os.Getenv("BR_BIN")
	if brBin == "" {
		brBin = defaultBrBin
	}
	return &BrCliBackend{repoPath: repoPath, brBin: brBin}
}

// BrDatabasePath finds the SQLite store br keeps under a repository's .beads/.
//
// br has no `-C`: it discovers the database by walking up from the working
// directory, which fails for every caller that is not standing in the
// repository - the orchestrator, and every agent it dispatches, work inside a
// worktree under ~/.kernl/worktrees. So the path is resolved here and passed
// explicitly with --db, by kernl and in the prompt text alike.
//
// Exported because the stage prompts need the same path: an agent told to run
// `br update` from its worktree gets "Beads not initialized" without it.
func BrDatabasePath(repoPath string) (string, error) {
	pattern := filepath.Join(repoPath, ".beads", "*.db")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: looking for br's database under %s: %w", pattern, err)
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: no br database under %s - Fix: run `br init` in %s, or correct registry.repos[].path in kernl.yaml", pattern, repoPath)
	default:
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: %d databases under %s (%s), so which one br would open is not decidable - Fix: leave exactly one .db file in that directory", len(matches), pattern, strings.Join(matches, ", "))
	}
}

// brError is br's failure envelope. br prints it as JSON on stdout and exits
// non-zero, so a caller that only checks the exit code loses the reason.
type brError struct {
	Error *brErrorBody `json:"error"`
}

type brErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint"`
}

// brValue renders a flag and its value as one argument.
//
// br's parser reads a value beginning with "-" as the next flag and exits 2 -
// so a bead whose description or comment body starts with a dash aborts the
// run. The `--flag=value` form has no such ambiguity, and neither does a
// positional after `--`, which is why the positional call sites use that.
func brValue(flag, value string) string {
	return flag + "=" + value
}

// run executes one br command against a repository and returns its stdout.
func (b *BrCliBackend) run(ctx context.Context, repoPath string, args ...string) ([]byte, error) {
	if repoPath == "" {
		repoPath = b.repoPath
	}
	dbPath, err := BrDatabasePath(repoPath)
	if err != nil {
		return nil, err
	}

	full := append([]string{"--db", dbPath, "--json"}, args...)
	cmd := exec.CommandContext(ctx, b.brBin, full...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	stdout, runErr := cmd.Output()

	payload, envelope := splitBrOutput(stdout)
	// br's own reason is more precise than the exit status, so it wins whenever
	// there is one.
	if envelope != nil {
		hint := ""
		if envelope.Hint != "" {
			hint = " - Hint: " + envelope.Hint
		}
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: br %s: %s: %s%s", strings.Join(args, " "), envelope.Code, envelope.Message, hint)
	}
	if runErr != nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: br %s: %w: %s", strings.Join(args, " "), runErr, strings.TrimSpace(stderr.String()))
	}
	return payload, nil
}

// splitBrOutput separates the command's result from br's error envelope.
//
// stdout can carry more than one JSON document: `br close` on an already-closed
// issue prints its per-issue result AND then an error envelope, back to back.
// Decoding the whole buffer at once fails on the trailing document, so the
// envelope went undetected and the caller got the exit status with an empty
// stderr instead of "NOTHING_TO_DO: all 1 issue(s) skipped".
//
// Only a document whose top level is {"error": {...}} counts, so a result that
// merely contains the word error somewhere is not mistaken for a failure.
func splitBrOutput(stdout []byte) ([]byte, *brErrorBody) {
	decoder := json.NewDecoder(bytes.NewReader(stdout))
	var payload []byte
	for {
		var doc json.RawMessage
		if err := decoder.Decode(&doc); err != nil {
			break
		}
		var envelope brError
		if json.Unmarshal(doc, &envelope) == nil && envelope.Error != nil {
			return payload, envelope.Error
		}
		if payload == nil {
			payload = doc
		}
	}
	if payload == nil {
		return stdout, nil
	}
	return payload, nil
}

// brIssue is br's issue object. Snake_case throughout, and absent keys rather
// than empty ones when a field is unset.
type brIssue struct {
	ID                 string         `json:"id"`
	Title              string         `json:"title"`
	Description        string         `json:"description"`
	Notes              string         `json:"notes"`
	AcceptanceCriteria string         `json:"acceptance_criteria"`
	Status             string         `json:"status"`
	Priority           int            `json:"priority"`
	IssueType          string         `json:"issue_type"`
	Assignee           string         `json:"assignee"`
	Owner              string         `json:"owner"`
	Parent             string         `json:"parent"`
	Labels             []string       `json:"labels"`
	Dependencies       []brIssueDep   `json:"dependencies"`
	CreatedAt          string         `json:"created_at"`
	UpdatedAt          string         `json:"updated_at"`
	ClosedAt           string         `json:"closed_at"`
	CloseReason        string         `json:"close_reason"`
	Metadata           map[string]any `json:"metadata"`
	EstimatedMinutes   int            `json:"estimated_minutes"`
	Due                string         `json:"due"`
}

// brIssueDep is a dependency as `br show` embeds it: the id of the *other*
// end, with no field naming this one. bd instead emits
// {issue_id, depends_on_id}, so the two shapes cannot share a decoder - and
// getting it wrong builds an epic DAG with the arrows reversed.
type brIssueDep struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Status         string `json:"status"`
	Priority       int    `json:"priority"`
	DependencyType string `json:"dependency_type"`
}

// brDepRow is a row of `br dep list`, which does name both ends.
type brDepRow struct {
	IssueID     string `json:"issue_id"`
	DependsOnID string `json:"depends_on_id"`
	Type        string `json:"type"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	Priority    int    `json:"priority"`
}

// brListEnvelope is the shape of `br list`, which - unlike show, ready and
// dep list - wraps its results in an object.
type brListEnvelope struct {
	Issues []brIssue `json:"issues"`
	Total  int       `json:"total"`
}

// toRawBead converts br's issue into the shape NormalizeBead already knows.
//
// The dependency mapping is the part worth reading: the issue being shown is
// the dependent, and each entry names what it depends on. That is the
// source-is-dependent convention LoadEpic accepts, and it is what makes the
// epic DAG come out with its edges pointing the right way.
func (i brIssue) toRawBead() RawBead {
	deps := make([]RawDependency, 0, len(i.Dependencies))
	for _, d := range i.Dependencies {
		deps = append(deps, RawDependency{SourceID: i.ID, TargetID: d.ID, DepType: d.DependencyType})
	}
	return RawBead{
		ID:                 i.ID,
		Title:              i.Title,
		Description:        i.Description,
		Notes:              i.Notes,
		AcceptanceCriteria: i.AcceptanceCriteria,
		IssueType:          i.IssueType,
		Status:             i.Status,
		Priority:           i.Priority,
		Labels:             i.Labels,
		Assignee:           i.Assignee,
		Owner:              i.Owner,
		Parent:             i.Parent,
		Due:                i.Due,
		EstimatedMinutes:   i.EstimatedMinutes,
		CreatedAt:          i.CreatedAt,
		UpdatedAt:          i.UpdatedAt,
		ClosedAt:           i.ClosedAt,
		CloseReason:        i.CloseReason,
		Metadata:           i.Metadata,
		Dependencies:       deps,
	}
}

func (b *BrCliBackend) Capabilities() BackendCapabilities {
	return FullCapabilities
}

func (b *BrCliBackend) Get(id string, repoPath string) (*Bead, error) {
	out, err := b.run(context.Background(), repoPath, "show", id)
	if err != nil {
		return nil, err
	}
	var issues []brIssue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: parsing `br show %s`: %w", id, err)
	}
	if len(issues) == 0 {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: br show %s returned no issue - Fix: verify the id exists in %s", id, repoPath)
	}
	bead := NormalizeBead(issues[0].toRawBead())
	return &bead, nil
}

// List answers a filtered query, and answers the parent filter the long way
// round because br has no --parent.
//
// The children of an epic come from the reverse dependency lookup, and then
// have to be fetched: `br list` reports dependency_count but never the
// dependencies themselves, and LoadEpic builds the epic's DAG out of exactly
// those. Serving this from the dep rows alone yields an epic whose children
// have no edges - a run that succeeds having executed everything in the wrong
// order, or nothing at all.
func (b *BrCliBackend) List(filters *BeadListFilters, repoPath string) ([]Bead, error) {
	if filters != nil && filters.Parent != "" {
		return b.listChildren(filters, repoPath)
	}

	// --limit defaults to 50, which would silently truncate any epic with more
	// children than that. 0 is unlimited.
	args := []string{"list", "--limit=0"}
	if filters != nil {
		if filters.State != "" {
			args = append(args, brValue("--status", filters.State))
		} else {
			args = append(args, "--all")
		}
		if filters.Type != "" {
			args = append(args, brValue("--type", filters.Type))
		}
		if filters.Label != "" {
			args = append(args, brValue("--label", filters.Label))
		}
		if filters.Assignee != "" {
			args = append(args, brValue("--assignee", filters.Assignee))
		}
		if filters.Priority != 0 {
			args = append(args, brValue("--priority", strconv.Itoa(filters.Priority)))
		}
	} else {
		args = append(args, "--all")
	}

	out, err := b.run(context.Background(), repoPath, args...)
	if err != nil {
		return nil, err
	}
	var envelope brListEnvelope
	if err := json.Unmarshal(out, &envelope); err != nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: parsing `br list`: %w", err)
	}
	beads := make([]Bead, 0, len(envelope.Issues))
	for _, issue := range envelope.Issues {
		beads = append(beads, NormalizeBead(issue.toRawBead()))
	}
	return beads, nil
}

func (b *BrCliBackend) listChildren(filters *BeadListFilters, repoPath string) ([]Bead, error) {
	// --direction up is "what depends on this issue". Without it br answers
	// the opposite question and returns the epic's own parents, which for an
	// epic is nothing at all: zero children, and a run that reports success
	// having done no work.
	out, err := b.run(context.Background(), repoPath, "dep", "list", filters.Parent, "--direction=up", "--type=parent-child")
	if err != nil {
		return nil, err
	}
	var rows []brDepRow
	if err := json.Unmarshal(out, &rows); err != nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: parsing `br dep list %s`: %w", filters.Parent, err)
	}
	if len(rows) == 0 {
		return []Bead{}, nil
	}

	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.IssueID)
	}
	hydrated, err := b.run(context.Background(), repoPath, append([]string{"show"}, ids...)...)
	if err != nil {
		return nil, err
	}
	var issues []brIssue
	if err := json.Unmarshal(hydrated, &issues); err != nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: parsing `br show` for the children of %s: %w", filters.Parent, err)
	}

	beads := make([]Bead, 0, len(issues))
	for _, issue := range issues {
		bead := NormalizeBead(issue.toRawBead())
		if !matchesFilters(bead, filters) {
			continue
		}
		beads = append(beads, bead)
	}
	return beads, nil
}

// matchesFilters applies the non-parent filters client-side. br cannot combine
// them with the reverse dependency lookup the parent filter is served by, and
// an epic's children are few enough that filtering them here costs nothing.
func matchesFilters(bead Bead, filters *BeadListFilters) bool {
	if filters.State != "" && bead.State != filters.State {
		return false
	}
	if filters.Type != "" && bead.Type != filters.Type {
		return false
	}
	if filters.Assignee != "" && bead.Assignee != filters.Assignee {
		return false
	}
	if filters.Priority != 0 && bead.Priority != filters.Priority {
		return false
	}
	if filters.Label != "" {
		found := false
		for _, l := range bead.Labels {
			if l == filters.Label {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func (b *BrCliBackend) ListReady(filters *BeadListFilters, repoPath string) ([]Bead, error) {
	ready := &BeadListFilters{}
	if filters != nil {
		*ready = *filters
	}
	ready.State = "ready_for_implementation"
	return b.List(ready, repoPath)
}

func (b *BrCliBackend) Update(id string, input UpdateBeadInput, repoPath string) error {
	args := []string{"update", id}
	if input.Title != "" {
		args = append(args, brValue("--title", input.Title))
	}
	if input.Description != "" {
		args = append(args, brValue("--description", input.Description))
	}
	if input.Type != "" {
		args = append(args, brValue("--type", input.Type))
	}
	if input.State != "" {
		args = append(args, brValue("--status", input.State))
	}
	if input.Priority != nil {
		args = append(args, brValue("--priority", strconv.Itoa(*input.Priority)))
	}
	if input.Assignee != "" {
		args = append(args, brValue("--assignee", input.Assignee))
	}
	if input.Acceptance != "" {
		args = append(args, brValue("--acceptance", input.Acceptance))
	}
	if input.Notes != "" {
		args = append(args, brValue("--notes", input.Notes))
	}
	for _, l := range input.Labels {
		args = append(args, brValue("--add-label", l))
	}
	if len(args) == 2 {
		return fmt.Errorf("KERNL DISPATCH FAILURE: update of %s asks for no change - Fix: populate at least one field of UpdateBeadInput", id)
	}
	if _, err := b.run(context.Background(), repoPath, args...); err != nil {
		return err
	}
	// br has no set-labels: the whole wf:state:* set has to be replaced by
	// removing what is there and adding what was asked for. Done after the
	// update so a failed status change never leaves the labels claiming a
	// state the status does not have.
	if len(input.SetLabels) > 0 {
		return b.setLabels(id, input.SetLabels, repoPath)
	}
	return nil
}

func (b *BrCliBackend) setLabels(id string, labels []string, repoPath string) error {
	current, err := b.Get(id, repoPath)
	if err != nil {
		return err
	}
	wanted := make(map[string]bool, len(labels))
	for _, l := range labels {
		wanted[l] = true
	}
	for _, existing := range current.Labels {
		if !wanted[existing] {
			if _, err := b.run(context.Background(), repoPath, "label", "remove", id, "--", existing); err != nil {
				return err
			}
		}
	}
	for _, l := range labels {
		if _, err := b.run(context.Background(), repoPath, "label", "add", id, "--", l); err != nil {
			return err
		}
	}
	return nil
}

func (b *BrCliBackend) Close(id string, reason string, repoPath string) (*TerminalState, error) {
	args := []string{"close", id}
	if reason != "" {
		args = append(args, brValue("--reason", reason))
	}
	out, err := b.run(context.Background(), repoPath, args...)
	if err != nil {
		return nil, err
	}
	var closed []brIssue
	if err := json.Unmarshal(out, &closed); err != nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: parsing `br close %s`: %w", id, err)
	}
	if len(closed) == 0 {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: br close %s reported no issue closed", id)
	}
	return &TerminalState{State: closed[0].Status, Reason: closed[0].CloseReason}, nil
}

func (b *BrCliBackend) MarkTerminal(id string, targetState string, reason string, repoPath string) error {
	args := []string{"update", id, brValue("--status", targetState)}
	if reason != "" {
		args = append(args, brValue("--notes", reason))
	}
	if _, err := b.run(context.Background(), repoPath, args...); err != nil {
		return fmt.Errorf("KERNL WORKFLOW CORRECTION FAILURE: mark terminal %s -> %s: %w", id, targetState, err)
	}
	return nil
}

// AddDependency records that blockedID waits on blockerID.
//
// br spells this `br dep add <issue> <depends-on>` - the first argument is the
// one that will depend on something - so the parameters cross over. The port's
// names are the authority: the blocked issue is the one that depends.
func (b *BrCliBackend) AddDependency(blockerID string, blockedID string, repoPath string) error {
	_, err := b.run(context.Background(), repoPath, "dep", "add", blockedID, blockerID)
	return err
}

func (b *BrCliBackend) ListDependencies(id string, repoPath string, options *DependencyListOptions) ([]BeadDependency, error) {
	args := []string{"dep", "list", id}
	if options != nil && options.Type != "" {
		args = append(args, brValue("--type", options.Type))
	}
	out, err := b.run(context.Background(), repoPath, args...)
	if err != nil {
		return nil, err
	}
	var rows []brDepRow
	if err := json.Unmarshal(out, &rows); err != nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: parsing `br dep list %s`: %w", id, err)
	}
	deps := make([]BeadDependency, 0, len(rows))
	for _, row := range rows {
		deps = append(deps, BeadDependency{
			SourceID:       row.IssueID,
			TargetID:       row.DependsOnID,
			DependencyType: row.Type,
			Type:           row.Type,
			Title:          row.Title,
			State:          row.Status,
			Priority:       row.Priority,
		})
	}
	return deps, nil
}

func (b *BrCliBackend) Comment(id string, body string, repoPath string) error {
	_, err := b.run(context.Background(), repoPath, "comments", "add", id, "--", body)
	return err
}

// Everything below is BackendPort surface that driving an epic never touches.
// It fails loud rather than being written blind: an adapter method that has
// never been run against the real CLI is a guess, and a guess that returns an
// empty result reads as "there is nothing to do".

func (b *BrCliBackend) ListWorkflows(repoPath string) ([]WorkflowDescriptor, error) {
	return nil, brUnimplemented("listWorkflows")
}

func (b *BrCliBackend) Create(input CreateBeadInput, repoPath string) (*Bead, error) {
	return nil, brUnimplemented("create")
}

func (b *BrCliBackend) Delete(id string, repoPath string) error {
	return brUnimplemented("delete")
}

func (b *BrCliBackend) Reopen(id string, reason string, repoPath string) error {
	return brUnimplemented("reopen")
}

func (b *BrCliBackend) Rewind(id string, targetState string, reason string, repoPath string) error {
	return brUnimplemented("rewind")
}

func (b *BrCliBackend) Search(query string, filters *BeadListFilters, repoPath string) ([]Bead, error) {
	return nil, brUnimplemented("search")
}

func (b *BrCliBackend) Query(expression string, options *BeadQueryOptions, repoPath string) ([]Bead, error) {
	return nil, brUnimplemented("query")
}

func (b *BrCliBackend) RemoveDependency(blockerID string, blockedID string, repoPath string) error {
	return brUnimplemented("removeDependency")
}

func (b *BrCliBackend) BuildTakePrompt(beadID string, options *TakePromptOptions, repoPath string) (*TakePromptResult, error) {
	return nil, brUnimplemented("buildTakePrompt")
}

func (b *BrCliBackend) BuildPollPrompt(options *PollPromptOptions, repoPath string) (*PollPromptResult, error) {
	return nil, brUnimplemented("buildPollPrompt")
}

func brUnimplemented(method string) error {
	return fmt.Errorf("KERNL DISPATCH FAILURE: the br backend does not implement %s - it covers what driving an epic exercises and nothing else - Fix: implement it in internal/backend/brcli.go against the real br contract, or use a repository whose memoryManager is one that supports it", method)
}
