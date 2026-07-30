package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/session"
)

// reviewVerdictForGate reads the actual verdict text of an artifact_verdict
// exit gate (the review stages' contract), independent of gatePassed - the
// ledger records what the review concluded, not merely whether the run
// advanced. Returns nil for any other gate type (there is no verdict to
// report) or when the artifact cannot be read.
//
// This duplicates the small file-read backend.EvaluateExitGate already does
// internally rather than changing that function's signature: EvaluateExitGate
// is called from every exit-gate check in the codebase, and its pass/reason
// contract is not ledger-specific - only this writer needs the verdict text
// itself.
func reviewVerdictForGate(wf backend.WorkflowDescriptor, ctx backend.ExitGateContext) *string {
	gate, ok := wf.ExitGates[ctx.FromState]
	if !ok || gate.Type != "artifact_verdict" {
		return nil
	}
	if strings.Contains(gate.Path, "<artifact_dir>") && ctx.ArtifactDir == "" {
		return nil
	}
	abs := backend.ResolveArtifactFSPath(gate.Path, ctx.BeadID, ctx.WorktreePath, ctx.ArtifactDir)
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil
	}
	verdict := "FAIL"
	if strings.HasSuffix(strings.TrimSpace(string(data)), "VERDICT: PASS") {
		verdict = "PASS"
	}
	return &verdict
}

// StageAttemptRecord is one line of the per-epic stage-attempt ledger: every
// fact a dispatch already had in hand once its exit gate was evaluated,
// recorded at the finest grain available. It intentionally carries no
// aggregate or rollup - "which agent should implement" is a query over many
// rows, not a value this writer decides.
type StageAttemptRecord struct {
	// who
	AgentID string `json:"agentId"`
	Dialect string `json:"dialect"`
	// Model is the concrete identifier that ran. ModelResolved says where it
	// came from: true when the CLI itself reported it (claude's "result"
	// event), false when it is only the operator's configured alias
	// (settings.agents.<id>.model) because the dialect reports none (codex,
	// always). A row that cannot tell those two apart cannot be compared
	// against a different row six months from now, after the alias has been
	// repointed at a different model.
	Model         string `json:"model"`
	ModelResolved bool   `json:"modelResolved"`
	Pool          string `json:"pool"`

	// what
	EpicID        string `json:"epicId"`
	BeadID        string `json:"beadId"`
	Stage         string `json:"stage"`
	AttemptNumber int    `json:"attemptNumber"`
	SessionID     string `json:"sessionId"`

	// how it went
	StartedAt         time.Time `json:"startedAt"`
	DurationMs        int64     `json:"durationMs"`
	ExitCode          int       `json:"exitCode"`
	CommitSHA         string    `json:"commitSHA,omitempty"`
	DiffLinesAdded    *int      `json:"diffLinesAdded"`
	DiffLinesRemoved  *int      `json:"diffLinesRemoved"`
	GatePassed        bool      `json:"gatePassed"`
	GateFailureReason *string   `json:"gateFailureReason"`
	ReviewVerdict     *string   `json:"reviewVerdict"`
	FirstPassApproved *bool     `json:"firstPassApproved"`
	// CausedBy points at the review artifact that most recently rejected
	// this bead, when this attempt is a retry - a fact derived once, here,
	// from the ledger's own history, so every later reader (a "charge to
	// the implementer" rollup, a "charge to whoever fixed it" rollup) starts
	// from the same rows instead of re-deriving it differently.
	CausedBy      *string `json:"causedBy"`
	FollowUpCount int     `json:"followUpCount"`
	Nudged        bool    `json:"nudged"`

	// what it cost - nil, never estimated, whenever the dialect did not
	// report the field.
	InputTokens      *int64   `json:"inputTokens"`
	OutputTokens     *int64   `json:"outputTokens"`
	CacheReadTokens  *int64   `json:"cacheReadTokens"`
	CacheWriteTokens *int64   `json:"cacheWriteTokens"`
	ReasoningTokens  *int64   `json:"reasoningTokens"`
	CostUSD          *float64 `json:"costUSD"`
	Turns            *int64   `json:"turns"`
}

// StageAttemptInput bundles the facts a stage dispatch already has in hand
// once its exit gate has been evaluated - the same values buildStageComment
// formats into free text - so DriveBeadToTerminal hands them to the ledger
// without recomputing anything.
type StageAttemptInput struct {
	AgentID string
	Dialect string
	// ConfiguredModel is settings.agents.<id>.model - the alias used when
	// Usage.Model is nil (the dialect reported no resolved identifier).
	ConfiguredModel string
	Pool            string
	BeadID          string
	Stage           string
	SessionID       string
	StartedAt       time.Time
	Duration        time.Duration
	ExitCode        int
	// BaseSHA/CommitSHA/Worktree scope the diff-line count to exactly the
	// commits this stage produced, the same range commit_marker gates use.
	BaseSHA           string
	CommitSHA         string
	Worktree          string
	GatePassed        bool
	GateFailureReason string
	ReviewVerdict     *string
	FollowUpCount     int
	Nudged            bool
	Usage             *session.TokenUsageCounts
}

// BuildStageAttemptRecord turns one dispatch's facts into the ledger row
// shape. AttemptNumber, CausedBy and FirstPassApproved are left unset here -
// AppendStageAttempt derives them from the ledger's own prior rows, because
// they depend on history this function does not have.
func BuildStageAttemptRecord(in StageAttemptInput) StageAttemptRecord {
	model := in.ConfiguredModel
	modelResolved := false
	if in.Usage != nil && in.Usage.Model != nil && *in.Usage.Model != "" {
		model = *in.Usage.Model
		modelResolved = true
	}

	added, removed := diffLineStats(in.Worktree, in.BaseSHA, in.CommitSHA)

	var gateFailureReason *string
	if !in.GatePassed && in.GateFailureReason != "" {
		reason := in.GateFailureReason
		gateFailureReason = &reason
	}

	rec := StageAttemptRecord{
		AgentID:           in.AgentID,
		Dialect:           in.Dialect,
		Model:             model,
		ModelResolved:     modelResolved,
		Pool:              in.Pool,
		BeadID:            in.BeadID,
		Stage:             in.Stage,
		SessionID:         in.SessionID,
		StartedAt:         in.StartedAt,
		DurationMs:        in.Duration.Milliseconds(),
		ExitCode:          in.ExitCode,
		CommitSHA:         in.CommitSHA,
		DiffLinesAdded:    added,
		DiffLinesRemoved:  removed,
		GatePassed:        in.GatePassed,
		GateFailureReason: gateFailureReason,
		ReviewVerdict:     in.ReviewVerdict,
		FollowUpCount:     in.FollowUpCount,
		Nudged:            in.Nudged,
	}
	if in.Usage != nil {
		inputTokens := in.Usage.InputTokens
		outputTokens := in.Usage.OutputTokens
		rec.InputTokens = &inputTokens
		rec.OutputTokens = &outputTokens
		rec.CacheReadTokens = in.Usage.CacheReadTokens
		rec.CacheWriteTokens = in.Usage.CacheWriteTokens
		rec.ReasoningTokens = in.Usage.ReasoningTokens
		rec.CostUSD = in.Usage.CostUSD
		rec.Turns = in.Usage.Turns
	}
	return rec
}

// attemptLedgerLocks serializes appends per ledger file within this
// process - the read-existing/compute-derived-fields/append sequence in
// AppendStageAttempt is not otherwise atomic, and two beads of the same
// epic can be dispatched concurrently.
var attemptLedgerLocks sync.Map // map[string]*sync.Mutex, keyed by ledger path

func lockForLedger(path string) *sync.Mutex {
	v, _ := attemptLedgerLocks.LoadOrStore(path, &sync.Mutex{})
	return v.(*sync.Mutex)
}

// AppendStageAttempt writes one line to <StateDir>/run/<epicID>/attempts.jsonl,
// deriving AttemptNumber, CausedBy and FirstPassApproved from the file's
// existing rows before writing - the ledger's own history is the only input
// those three facts need, so callers never pass them in and never disagree
// with each other about how they were computed.
func AppendStageAttempt(stateDir, epicID string, rec StageAttemptRecord) error {
	path, err := resolveAttemptLedgerPath(stateDir, epicID)
	if err != nil {
		return err
	}

	mu := lockForLedger(path)
	mu.Lock()
	defer mu.Unlock()

	existing, err := readAttemptLedger(path)
	if err != nil {
		return err
	}

	rec.EpicID = epicID
	rec.AttemptNumber = countPriorAttempts(existing, rec.BeadID, rec.Stage) + 1
	rec.CausedBy = findCausedBy(existing, rec.BeadID)
	if rec.ReviewVerdict != nil {
		firstPassApproved := rec.AttemptNumber == 1 && *rec.ReviewVerdict == "PASS"
		rec.FirstPassApproved = &firstPassApproved
	}

	line, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: encoding stage attempt record for bead %s: %w", rec.BeadID, err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: opening attempt ledger %s for bead %s: %w", path, rec.BeadID, err)
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: writing attempt ledger %s for bead %s: %w", path, rec.BeadID, err)
	}
	return nil
}

// countPriorAttempts counts rows already recorded for the same bead+stage,
// so a fresh AppendStageAttempt call becomes attempt N+1.
func countPriorAttempts(existing []StageAttemptRecord, beadID, stage string) int {
	n := 0
	for _, rec := range existing {
		if rec.BeadID == beadID && rec.Stage == stage {
			n++
		}
	}
	return n
}

// findCausedBy walks backward to this bead's most recent recorded attempt
// (any stage). If it failed its gate because a review verdict rejected it
// ("verdict_not_pass: <artifact>" - see backend.EvaluateExitGate), the new
// attempt is presumed caused by that rejection and CausedBy names the
// artifact. Any other outcome (the prior attempt passed, or failed for a
// reason that is not a review rejection) yields nil: this function only
// ever states what the immediately preceding row recorded, never a
// judgment about whether today's attempt is "really" a retry.
func findCausedBy(existing []StageAttemptRecord, beadID string) *string {
	const verdictRejectionPrefix = "verdict_not_pass: "
	for i := len(existing) - 1; i >= 0; i-- {
		rec := existing[i]
		if rec.BeadID != beadID {
			continue
		}
		if rec.GatePassed || rec.GateFailureReason == nil {
			return nil
		}
		if !strings.HasPrefix(*rec.GateFailureReason, verdictRejectionPrefix) {
			return nil
		}
		artifact := strings.TrimPrefix(*rec.GateFailureReason, verdictRejectionPrefix)
		return &artifact
	}
	return nil
}

func readAttemptLedger(path string) ([]StageAttemptRecord, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: reading attempt ledger %s: %w", path, err)
	}
	var records []StageAttemptRecord
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var rec StageAttemptRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return nil, fmt.Errorf("KERNL DISPATCH FAILURE: parsing attempt ledger %s: %w", path, err)
		}
		records = append(records, rec)
	}
	return records, nil
}

// resolveAttemptLedgerPath is resolveArtifactDir's sibling: same StateDir,
// same "beneath <StateDir>/run" invariant, same reason (epicID is tracker
// data kernl does not own) - but scoped to the epic rather than one bead,
// because the ledger is one file per epic, not one per bead. It reuses
// isSafePathComponent and escapesRoot rather than writing a second check.
func resolveAttemptLedgerPath(stateDir, epicID string) (string, error) {
	if stateDir == "" {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: no state directory for the epic %q attempt ledger, so kernl has nowhere of its own to record stage attempts outside the worktree - Fix: set DriveBeadDeps.StateDir (app.DefaultStateDir() outside tests)", epicID)
	}
	if !isSafePathComponent(epicID) {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: unsafe epic id %q for the attempt ledger path - Fix: the id must be a single path segment with no '/', '\\', '.', or '..'", epicID)
	}

	runRoot := filepath.Join(stateDir, "run")
	epicDir := filepath.Join(runRoot, epicID)
	if escapesRoot(runRoot, epicDir) {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: attempt ledger dir %s for epic %q escapes %s - Fix: epic id %q must resolve to a path beneath it", epicDir, epicID, runRoot, epicID)
	}

	if err := os.MkdirAll(epicDir, 0o755); err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: creating attempt ledger dir %s for epic %q: %w", epicDir, epicID, err)
	}
	return filepath.Join(epicDir, "attempts.jsonl"), nil
}

// diffLineStats sums added/removed lines across baseSHA..commitSHA, the
// exact range commit_marker gates scan - so the count reflects only what
// this stage's own commits introduced, not the branch's prior history. Nil
// (not zero) whenever the range is not meaningful: no worktree, no
// base/commit capture, or a stage that produced no new commit.
func diffLineStats(worktree, baseSHA, commitSHA string) (added, removed *int) {
	if worktree == "" || baseSHA == "" || commitSHA == "" || baseSHA == commitSHA {
		return nil, nil
	}
	out, err := exec.Command("git", "-C", worktree, "diff", "--numstat", baseSHA+".."+commitSHA).Output()
	if err != nil {
		return nil, nil
	}
	var a, r int
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) < 2 {
			continue
		}
		// Binary files report "-" for both counts - Atoi fails and the
		// field is skipped, which is correct: a binary diff has no line
		// count to add.
		if n, err := strconv.Atoi(fields[0]); err == nil {
			a += n
		}
		if n, err := strconv.Atoi(fields[1]); err == nil {
			r += n
		}
	}
	return &a, &r
}
