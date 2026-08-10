package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	// Read the directory rather than glob it: a repository path is data, and
	// filepath.Glob would read a `[` in it as pattern syntax - failing outright
	// on some paths and matching a sibling directory's database on others.
	beadsDir := filepath.Join(repoPath, ".beads")
	entries, err := os.ReadDir(beadsDir)
	if err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: cannot read %s, so br's database cannot be found - %w - Fix: run `br init` in %s, or correct registry.repos[].path in kernl.yaml", beadsDir, err, repoPath)
	}

	var found []string
	for _, entry := range entries {
		// Directories named *.db are not databases, and the -wal/-shm
		// companions do not end in .db so they exclude themselves.
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".db") {
			continue
		}
		found = append(found, filepath.Join(beadsDir, entry.Name()))
	}

	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: no br database in %s - Fix: run `br init` in %s, or correct registry.repos[].path in kernl.yaml", beadsDir, repoPath)
	default:
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: %d databases in %s (%s), so which one br would open is not decidable - Fix: leave exactly one .db file in that directory", len(found), beadsDir, strings.Join(found, ", "))
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

// brCommandError is the structured form of run()'s envelope-error return.
// run() still formats the same "KERNL DISPATCH FAILURE: ..." text every
// existing caller already matches against (see Error() below), but a caller
// that needs more than that text - Close, telling a bead that is already
// closed apart from one br refuses for a real reason - recovers the
// envelope's code and any payload that arrived on stdout ahead of it with
// errors.As, instead of re-parsing the formatted string.
type brCommandError struct {
	code    string
	message string
	// hint is the envelope's own advisory field - a fixed, two-sentence
	// shape (see classifyCloseRefusal) - kept apart from message and
	// payload because it is what recoverNoOpClose reads to classify a close
	// refusal without pattern-matching the free-form per-issue reason.
	hint    string
	payload []byte
	text    string
}

func (e *brCommandError) Error() string { return e.text }

// ErrAlreadyClosed reports that Close's target was already in its terminal
// state before this call ran - the call itself did nothing. Close returns
// it as a non-nil error rather than masking the no-op as an ordinary
// success, because "there was nothing to do" is information some callers
// care about: a sweep re-run wants to say a bead was already closed rather
// than claim this run closed it. A caller that only wants to know whether
// the bead ended up closed can still treat this as success by checking
// errors.Is(err, backend.ErrAlreadyClosed).
var ErrAlreadyClosed = errors.New("bead already closed")

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
		hintSuffix := ""
		if envelope.Hint != "" {
			hintSuffix = " - Hint: " + envelope.Hint
		}
		return nil, &brCommandError{
			code:    envelope.Code,
			message: envelope.Message,
			hint:    envelope.Hint,
			payload: payload,
			text:    fmt.Sprintf("KERNL DISPATCH FAILURE: br %s: %s: %s%s", strings.Join(args, " "), envelope.Code, envelope.Message, hintSuffix),
		}
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
	Priority           *int           `json:"priority"`
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
	// Due is tagged due_at because br (beads_rust >=0.2.10) emits the field as
	// due_at across br show, br create, br list, and br search.
	Due string `json:"due_at"`
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

// priorityOrDefault answers what RawBead's int field cannot: whether br said
// nothing about priority or said zero.
//
// NormalizeBead maps 0 to 2, which is right for bd - an absent key decodes as
// 0 there and means "unset". br emits priority on every issue and 0 is a real
// P0, so passing it through unchanged would quietly demote every P0 to P2. The
// pointer keeps the distinction only as far as this decoder; nothing outside it
// has to care.
func (i brIssue) priorityOrDefault() int {
	if i.Priority == nil {
		return 0
	}
	return *i.Priority
}

// isExplicitPriority reports whether br stated a priority this decoder must
// preserve past NormalizeBead's unset-means-P2 rule.
func (i brIssue) isExplicitPriority() bool {
	return i.Priority != nil && *i.Priority >= 0 && *i.Priority <= 4
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
		Priority:           i.priorityOrDefault(),
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

// toBead is the only conversion this adapter uses, so the priority rule below
// cannot be forgotten at one of the three call sites.
func (i brIssue) toBead() Bead {
	bead := NormalizeBead(i.toRawBead())
	if i.isExplicitPriority() {
		bead.Priority = *i.Priority
	}
	return bead
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
	bead := issues[0].toBead()
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
		beads = append(beads, issue.toBead())
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
		bead := issue.toBead()
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
	// --set-labels is repeatable and replaces the whole set, so the state
	// change and the labels that mirror it go in one command. Doing it as
	// remove-then-add afterwards was both unnecessary and not atomic: a
	// failure partway left the bead with a new status and a half-replaced
	// label set, which is the stale-label state the workflow reads as truth.
	for _, l := range input.SetLabels {
		args = append(args, brValue("--set-labels", l))
	}
	for _, l := range input.RemoveLabels {
		args = append(args, brValue("--remove-label", l))
	}
	if len(args) == 2 {
		return fmt.Errorf("KERNL DISPATCH FAILURE: update of %s asks for no change - Fix: populate at least one field of UpdateBeadInput", id)
	}
	_, err := b.run(context.Background(), repoPath, args...)
	return err
}

func (b *BrCliBackend) Close(id string, reason string, repoPath string) (*TerminalState, error) {
	args := []string{"close", id}
	if reason != "" {
		args = append(args, brValue("--reason", reason))
	}
	out, err := b.run(context.Background(), repoPath, args...)
	if err != nil {
		return b.recoverNoOpClose(id, repoPath, err)
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

// brCloseResult is `br close --json`'s per-issue result when at least one
// target was skipped - printed on stdout ahead of the error envelope (see
// splitBrOutput). brSkippedRow is the same {id, reason} shape `br reopen`
// already returns for its own no-op case, reused here rather than inventing
// a second vocabulary for the same {id, reason} pair.
type brCloseResult struct {
	Closed  []brIssue      `json:"closed"`
	Skipped []brSkippedRow `json:"skipped"`
}

// recoverNoOpClose re-examines a close br refused with NOTHING_TO_DO.
//
// Measured live against br 0.2.19: that one code and exit status cover two
// different situations - the target is already closed (this call was
// redundant, not a failure), or a dependency refused it, e.g. an epic with
// an open child (a real failure). This br version does name which one, in
// the "skipped" document ahead of the envelope and in the envelope's own
// hint - but that is prose from a CLI that has already changed its wording
// once across a patch release (0.2.10 gave the identical hint for both
// situations, measured against a live 0.2.10 install). Pattern-matching that
// text again would only reproduce this exact bug the next time br's wording
// moves. What the two situations cannot disagree about is the bead's own
// recorded closed_at, so a re-read of the bead - not the error code, not the
// message text - is what decides the outcome. The skip reason is still
// surfaced, but only to word a genuine failure, never to decide one.
func (b *BrCliBackend) recoverNoOpClose(id, repoPath string, closeErr error) (*TerminalState, error) {
	var brErr *brCommandError
	if !errors.As(closeErr, &brErr) || brErr.code != "NOTHING_TO_DO" {
		return nil, closeErr
	}

	skipReason := brErr.message
	var result brCloseResult
	if json.Unmarshal(brErr.payload, &result) == nil {
		for _, s := range result.Skipped {
			if s.ID == id && s.Reason != "" {
				skipReason = s.Reason
				break
			}
		}
	}

	bead, getErr := b.Get(id, repoPath)
	if getErr != nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: br close %s refused (%s), and re-reading its state to confirm also failed: %w", id, skipReason, getErr)
	}
	if bead.ClosedAt != "" {
		return nil, fmt.Errorf("%s: %w", id, ErrAlreadyClosed)
	}
	// Telling an ordering refusal apart from a genuine tracker failure is a
	// lower-stakes read of br's wording than the closed-vs-failure decision
	// above: a caller (sweep) that recovers this with errors.As and finds
	// none still has the same generic failure text it always had, so a
	// wording change degrades to "unrecognised", never to a wrong verdict.
	if kind, ok := classifyCloseRefusal(brErr.hint); ok {
		return nil, &CloseRefusedError{ID: id, Kind: kind, Reason: redactForceHint(kind, skipReason)}
	}
	return nil, fmt.Errorf("KERNL DISPATCH FAILURE: br close %s refused: %s", id, skipReason)
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

// Rewind moves id back to an earlier queue state so it is picked up again -
// the mechanism a revert-and-reopen composite verb needs and that used to be
// brUnimplemented here, which meant the composite verb's own half that talks
// to the real tracker failed loud against every repository this orchestrator
// actually drives (br, not knots). It runs the same `br update --status`
// invocation MarkTerminal already uses against production - the same
// mechanism an operator already runs by hand (see the plan's manual
// recovery recipe) - rather than inventing a second one for the status
// change. The notes half is not identical to MarkTerminal's: see
// rewindNotes for why a bare `--notes <reason>` is not safe here.
//
// targetState must be a queue state (ready_for_*): rewinding into anything
// else leaves the bead sitting in an active, non-queue state that nothing
// ever dispatches from, which is a stuck bead wearing the appearance of a
// successful rewind. KnotsBackend.Rewind already enforces exactly this for
// the one backend that has a real Rewind implementation; this mirrors it
// rather than leaving br as the one path with no such check. This is a
// necessary check but not a sufficient one - it only rejects targets with
// the wrong shape, not a well-formed but nonexistent state (a typo inside
// the ready_for_* family); the caller composing a revert is expected to
// validate targetState against the bead's own resolved workflow states
// before calling this, since br accepts any status string it is given.
//
// defaultState (dto.go) prefers a known workflow status over a stale
// wf:state:* label, and targetState is always one of those known states
// here, so status alone is sufficient - the label is not additionally
// reset. A caller inspecting `br show` by eye can still see a stale label,
// which is a pre-existing, already-tolerated gap (MarkTerminal has the same
// one) and not something this change introduces.
func (b *BrCliBackend) Rewind(id string, targetState string, reason string, repoPath string) error {
	if !strings.HasPrefix(targetState, "ready_for_") {
		return fmt.Errorf("KERNL WORKFLOW CORRECTION FAILURE: rewind target %q must be a queue state (ready_for_*) - Fix: rewind %s to the ready_for_<stage> state that should re-run it, not the active stage itself", targetState, id)
	}
	args := []string{"update", id, brValue("--status", targetState)}
	if reason != "" {
		notes, err := b.rewindNotes(id, repoPath, reason)
		if err != nil {
			return fmt.Errorf("KERNL WORKFLOW CORRECTION FAILURE: rewind %s -> %s: reading current notes to preserve invariants: %w", id, targetState, err)
		}
		args = append(args, brValue("--notes", notes))
	}
	if _, err := b.run(context.Background(), repoPath, args...); err != nil {
		return fmt.Errorf("KERNL WORKFLOW CORRECTION FAILURE: rewind %s -> %s: %w", id, targetState, err)
	}
	return nil
}

// rewindNotes composes the notes value Rewind writes, so a rewind reason
// never destroys invariants already embedded in the bead's notes field.
//
// br has no separate storage for invariants: parseInvariantsFromNotes /
// embedInvariantsInNotes (dto.go) exist because they live entirely inside
// this one free-text field, extracted out into Bead.Invariants on read. A
// bare `--notes <reason>` write - what MarkTerminal already does, and what
// this function used to do - replaces the field wholesale (same as
// --description and --acceptance) and silently drops whatever [Invariants]
// block was there. This function is new in this change, unlike
// MarkTerminal's pre-existing use of the same bare write, so it does not
// inherit that gap: it reads the bead's current (already-split) notes and
// invariants and re-embeds both before writing.
func (b *BrCliBackend) rewindNotes(id, repoPath, reason string) (string, error) {
	current, err := b.Get(id, repoPath)
	if err != nil {
		return "", err
	}
	if current == nil {
		return reason, nil
	}
	prose := strings.TrimSpace(current.Notes)
	if prose != "" {
		prose = prose + "\n\n" + reason
	} else {
		prose = reason
	}
	return embedInvariantsInNotes(prose, current.Invariants), nil
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

// ListWorkflows has no br counterpart at all, and this is not a gap left by
// partial coverage: a workflow (states, exit gates, stage contracts) is
// kernl's own concept for driving an epic, and br has no notion of one - it
// just tracks issues and their status strings. There is nothing to translate,
// so this stays a stub permanently rather than becoming a place where the
// adapter would have to invent workflow semantics br was never asked to have.
func (b *BrCliBackend) ListWorkflows(repoPath string) ([]WorkflowDescriptor, error) {
	return nil, brUnimplemented("listWorkflows")
}

// Create is what Phase 6's fix-up beads need: a fix-up bead is a new bead in
// the target repository's own tracker, created mid-run with no operator
// involvement, so it has to exist here rather than fail loud like the rest
// of this file.
//
// Two things measured against a live throwaway `br` workspace turned out
// different from what had been assumed going in, and both are corrections
// worth recording rather than re-deriving:
//
//  1. `br create --parent <id>` exists and creates a real parent-child
//     dependency (`br dep list <parent> --direction up --type=parent-child`
//     finds it) - unlike the assumption that parent-child could only be
//     added afterward via a second `dep add` call. So ParentID maps directly
//     onto `--parent`, and BrCliBackend.AddDependency is not used here: it
//     has no --type flag at all, so it always creates br's default "blocks"
//     edge, never "parent-child" - it would silently create the wrong kind
//     of dependency for this purpose.
//  2. `br create` has NO `--acceptance` and NO `--notes` flag (confirmed via
//     `br create --help` and a live call, which br rejects at argument
//     parsing with exit 2) - unlike `br update`, which supports both. So
//     Acceptance/Notes/Invariants cannot be set in the same call that
//     creates the bead; they are a second, immediate Update, not a silently
//     dropped field.
//
// ProfileID and WorkflowID have no br create equivalent at all - not even a
// two-step one - so a caller that sets either gets a loud refusal rather
// than a bead that silently lacks it: this project already has a convention
// for conveying a profile (the wf:profile: label, set via Update once the
// bead's caller decides which profile it runs), and reinventing a second one
// here would only create two ways to do the same thing.
func (b *BrCliBackend) Create(input CreateBeadInput, repoPath string) (*Bead, error) {
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: br create requires a title - Fix: populate CreateBeadInput.Title")
	}
	if input.ProfileID != "" || input.WorkflowID != "" {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: br create has no profile/workflow concept - ProfileID=%q WorkflowID=%q would be silently dropped - Fix: create the bead without them, then set the wf:profile:/wf:state: labels with Update, the same way epic children already get theirs", input.ProfileID, input.WorkflowID)
	}

	// Options are collected before the title, not after: "create" followed
	// immediately by a bare title (the pre-existing shape) hands br a title
	// beginning with "-" as the next flag instead of a positional argument -
	// unlike every other value in this file, brValue cannot help here
	// because the title is positional, not a flag's own value. Every field
	// this adapter maps to a create-time flag is a value that already goes
	// through brValue for the exact same reason; the title needs the other
	// half of br's own escape mechanism instead: "--" ends flag parsing, so
	// whatever follows - including one starting with "-" - is read as a
	// positional argument, never a flag. This surface was previously
	// unreachable (br create was unimplemented), so nothing exercised a
	// hostile title before Phase 6 made Create callable at all - including
	// from POST /api/beads, which accepts any string a caller sends.
	args := []string{"create"}
	if input.Type != "" {
		args = append(args, brValue("--type", input.Type))
	}
	// Always sent: CreateBeadInput.Priority is a plain int, not a pointer
	// like UpdateBeadInput.Priority, so this adapter has no way to tell "the
	// caller wants br's own default" apart from "the caller wants P0" - Go's
	// zero value already reads as the latter, and that is what gets sent.
	args = append(args, brValue("--priority", strconv.Itoa(input.Priority)))
	if input.Description != "" {
		args = append(args, brValue("--description", input.Description))
	}
	if input.Assignee != "" {
		args = append(args, brValue("--assignee", input.Assignee))
	}
	if input.Due != "" {
		args = append(args, brValue("--due", input.Due))
	}
	if len(input.Labels) > 0 {
		args = append(args, brValue("--labels", strings.Join(input.Labels, ",")))
	}
	if input.ParentID != "" {
		args = append(args, brValue("--parent", input.ParentID))
	}
	if input.Estimate != 0 {
		args = append(args, brValue("--estimate", strconv.Itoa(input.Estimate)))
	}
	args = append(args, "--", title)

	out, err := b.run(context.Background(), repoPath, args...)
	if err != nil {
		return nil, err
	}
	// Unlike show/list/dep-list, `br create --json` prints one issue object,
	// not an array - measured against a live create, not assumed from the
	// other commands' envelope shapes.
	var issue brIssue
	if err := json.Unmarshal(out, &issue); err != nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: parsing `br create %q`: %w", title, err)
	}
	bead := issue.toBead()

	notes := input.Notes
	if len(input.Invariants) > 0 {
		notes = embedInvariantsInNotes(notes, input.Invariants)
	}
	if input.Acceptance != "" || notes != "" {
		if err := b.Update(bead.ID, UpdateBeadInput{Acceptance: input.Acceptance, Notes: notes}, repoPath); err != nil {
			// The issue itself was already created (and linked, and
			// labeled - everything create's own first call sets). Returning
			// a plain error here would discard bead.ID entirely, and a
			// caller told "nothing was created, retry" would call Create
			// again for the same title and end up with two issues for one
			// request. CreatePartialError carries the bead that DOES exist,
			// so a caller can finish the one remaining step (write
			// acceptance/notes directly) instead of re-creating it.
			return nil, &CreatePartialError{Bead: &bead, error: fmt.Errorf("KERNL DISPATCH FAILURE: bead %s was created (and linked, if a parent was given) but writing its acceptance/notes failed: %w - Fix: run `br update %s --acceptance ... --notes ...` against the bead that already exists; do not call Create again for the same title, it would create a duplicate", bead.ID, err, bead.ID)}
		}
		bead.Acceptance = input.Acceptance
		bead.Notes = notes
	}
	return &bead, nil
}

// CreatePartialError marks that Create's underlying issue creation
// succeeded but a required follow-up write failed - today, only the
// acceptance/notes update `br create` cannot do in one call (see Create's
// own doc comment). Bead is never nil when this error is returned: a caller
// using errors.As can recover the created bead's id and resume the one
// remaining step, instead of treating "Create returned an error" as "Create
// created nothing" and retrying the whole operation into a duplicate.
type CreatePartialError struct {
	Bead *Bead
	error
}

// NewCreatePartialError builds a CreatePartialError from another package -
// the embedded error field's name is unexported (it is literally "error"),
// so a composite literal naming it can only be written inside this package;
// this is the constructor any other caller that must fabricate one for a
// test uses instead.
func NewCreatePartialError(bead *Bead, err error) *CreatePartialError {
	return &CreatePartialError{Bead: bead, error: err}
}

// brDeleteResult is `br delete`'s response.
//
// A dependent issue blocks the delete outright, but not by way of br's error
// envelope: br exits 0 and reports "preview": true, describing what it WOULD
// delete rather than what it did - confirmed against a live `br delete` on an
// issue with a dependent, run with neither --cascade nor --force. Reading
// only the exit code (or only for `error`) makes this look like a delete that
// succeeded; the issue is untouched. This adapter never passes
// --cascade/--force on a caller's behalf - that is a real destructive choice,
// not a default this port method gets to make silently - so a preview result
// is surfaced as a failure instead.
type brDeleteResult struct {
	Preview           bool     `json:"preview,omitempty"`
	Deleted           []string `json:"deleted,omitempty"`
	BlockedDependents []string `json:"blocked_dependents,omitempty"`
}

func (b *BrCliBackend) Delete(id string, repoPath string) error {
	out, err := b.run(context.Background(), repoPath, "delete", id)
	if err != nil {
		return err
	}
	var result brDeleteResult
	if err := json.Unmarshal(out, &result); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: parsing `br delete %s`: %w", id, err)
	}
	if result.Preview {
		return fmt.Errorf("KERNL DISPATCH FAILURE: br delete %s: blocked by dependent issue(s) %v - Fix: remove the dependency first with RemoveDependency, or delete the dependents themselves; this adapter does not pass --cascade/--force on a caller's behalf", id, result.BlockedDependents)
	}
	if len(result.Deleted) == 0 {
		return fmt.Errorf("KERNL DISPATCH FAILURE: br delete %s reported no issue deleted", id)
	}
	return nil
}

// brReopenResult is `br reopen`'s response.
//
// Like delete, a no-op reopen is not reported through br's error envelope: an
// issue that is already open, or a tombstoned one that cannot be reopened at
// all, comes back as exit 0 with an empty "reopened" array and the real
// reason tucked into "skipped" - confirmed against a live `br reopen` of both
// an already-open issue and a deleted (tombstoned) one.
type brReopenResult struct {
	Reopened []brIssue      `json:"reopened"`
	Skipped  []brSkippedRow `json:"skipped"`
}

type brSkippedRow struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

func (b *BrCliBackend) Reopen(id string, reason string, repoPath string) error {
	args := []string{"reopen", id}
	if reason != "" {
		args = append(args, brValue("--reason", reason))
	}
	out, err := b.run(context.Background(), repoPath, args...)
	if err != nil {
		return err
	}
	var result brReopenResult
	if err := json.Unmarshal(out, &result); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: parsing `br reopen %s`: %w", id, err)
	}
	if len(result.Reopened) == 0 {
		skipReason := "no reason reported"
		if len(result.Skipped) > 0 {
			skipReason = result.Skipped[0].Reason
		}
		return fmt.Errorf("KERNL DISPATCH FAILURE: br reopen %s did nothing: %s", id, skipReason)
	}
	return nil
}

// Search maps onto `br search`, which - measured live - returns a bare array,
// the same shape as show and dep list, not list's {"issues": [...]} envelope.
// It shares List's filter flags (--status, --type, --label, --assignee,
// --priority) and List's two traps: --limit defaults to 50 (0 is unlimited),
// and omitting --status also excludes closed issues by default, so a filter
// asking for a specific state passes --status and an unfiltered search passes
// --all instead, exactly as List does.
func (b *BrCliBackend) Search(query string, filters *BeadListFilters, repoPath string) ([]Bead, error) {
	args := []string{"search", "--limit=0"}
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
	// The query is positional, so it needs the same "--" boundary Create uses
	// for its title: without it, a query beginning with "-" is read as the
	// next flag instead of search text.
	args = append(args, "--", query)

	out, err := b.run(context.Background(), repoPath, args...)
	if err != nil {
		return nil, err
	}
	var issues []brIssue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: parsing `br search`: %w", err)
	}
	beads := make([]Bead, 0, len(issues))
	for _, issue := range issues {
		beads = append(beads, issue.toBead())
	}
	return beads, nil
}

// Query stays a stub: `br query` manages SAVED queries (save/run/list/delete
// a named filter set - confirmed against `br query --help`, which lists
// exactly those four subcommands and nothing else). There is no `br query
// <expression>` that evaluates a free-form expression and returns matching
// issues, which is what this port method's signature promises. The two are
// different features with the same English name, not one feature under two
// spellings, so this cannot be wired up as a thin translation the way Search
// or RemoveDependency are - it would need this adapter to invent expression
// parsing that br itself does not do.
func (b *BrCliBackend) Query(expression string, options *BeadQueryOptions, repoPath string) ([]Bead, error) {
	return nil, brUnimplemented("query")
}

// RemoveDependency undoes AddDependency. Same crossed parameter mapping as
// AddDependency: br spells this `br dep remove <issue> <depends-on>`, and the
// first argument is the one that depends on the second - so blockedID (the
// port's name for the dependent) goes first, matching AddDependency's own
// blockedID/blockerID swap.
func (b *BrCliBackend) RemoveDependency(blockerID string, blockedID string, repoPath string) error {
	_, err := b.run(context.Background(), repoPath, "dep", "remove", blockedID, blockerID)
	return err
}

// BuildTakePrompt and BuildPollPrompt have no br counterpart either, for the
// same reason as ListWorkflows: br has no command that assembles a
// take/poll prompt, and driving an epic against br never calls through this
// pair anyway - internal/app/drive_bead.go builds the stage prompt itself
// (BuildBeadStagePrompt), using backend.TrackerInvocation for the tracker
// command line rather than asking the backend for a finished prompt. Both
// methods are inherited from the bd/knots era, where a prompt builder lived
// behind the tracker port because bd's own CLI conventions shaped the
// resulting text; there is no equivalent br behavior to translate here.
func (b *BrCliBackend) BuildTakePrompt(beadID string, options *TakePromptOptions, repoPath string) (*TakePromptResult, error) {
	return nil, brUnimplemented("buildTakePrompt")
}

func (b *BrCliBackend) BuildPollPrompt(options *PollPromptOptions, repoPath string) (*PollPromptResult, error) {
	return nil, brUnimplemented("buildPollPrompt")
}

func brUnimplemented(method string) error {
	return fmt.Errorf("KERNL DISPATCH FAILURE: the br backend does not implement %s - it covers what driving an epic exercises and nothing else - Fix: implement it in internal/backend/brcli.go against the real br contract, or use a repository whose memoryManager is one that supports it", method)
}
