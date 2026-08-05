package app

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
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
	gate, ok := backend.FindExitGateByType(wf, ctx.FromState, "artifact_verdict")
	if !ok {
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
	// Model is the concrete identifier that ran, or nil when kernl has no
	// idea what that was - neither the CLI reported one nor the operator
	// configured an alias. ModelResolved says where a non-nil value came
	// from: true when the CLI itself reported it (claude's "result" event),
	// false when it is only the operator's configured alias
	// (settings.agents.<id>.model) because the dialect reports none (codex,
	// always). A row that cannot tell "resolved" apart from "fallback" apart
	// from "unknown" cannot be compared against a different row six months
	// from now, after the alias has been repointed at a different model.
	Model         *string `json:"model"`
	ModelResolved bool    `json:"modelResolved"`
	Pool          string  `json:"pool"`

	// what
	EpicID        string `json:"epicId"`
	BeadID        string `json:"beadId"`
	Stage         string `json:"stage"`
	AttemptNumber int    `json:"attemptNumber"`
	SessionID     string `json:"sessionId"`

	// how it went
	StartedAt time.Time `json:"startedAt"`
	// DurationMs is always a real elapsed measurement, even for a dispatch
	// that never produced a process (BuildStageAttemptRecord is always
	// handed time.Since(startTime) by its caller).
	DurationMs int64 `json:"durationMs"`
	// ExitCode is nil when no process ever exited to report one - a spawn
	// failure, or any other error surfaced before proc.Wait() returned an
	// exit status. -1 means the process was terminated by a signal rather
	// than exiting normally (see exec.ExitError.ExitCode()). A present value
	// is always the process's own real exit status, never a fabricated
	// stand-in for "something went wrong."
	ExitCode          *int    `json:"exitCode"`
	CommitSHA         string  `json:"commitSHA,omitempty"`
	DiffLinesAdded    *int    `json:"diffLinesAdded"`
	DiffLinesRemoved  *int    `json:"diffLinesRemoved"`
	GatePassed        bool    `json:"gatePassed"`
	GateFailureReason *string `json:"gateFailureReason"`
	ReviewVerdict     *string `json:"reviewVerdict"`
	FirstPassApproved *bool   `json:"firstPassApproved"`
	// CausedBy points at the review artifact whose deliberate rejection sent
	// this bead back, when this attempt is the retry that rejection caused.
	// Non-nil is the ledger's definition of rework: this attempt is redoing
	// work a reviewer rejected.
	//
	// It has two sources, and the order between them is the point. The
	// driver DECLARES it (StageAttemptInput.CausedBy) when this dispatch is
	// the retake of a rewind that same call performed: it does not need to
	// work out whether this is rework, it is the code that decided to rewind.
	// Only when nothing was declared does the writer fall back to inferring
	// it from the bead's own prior rows (findCausedBy).
	//
	// The fallback is not redundancy. A rewind and its retake do not have to
	// happen in one process - an epic's integration is rewound by
	// DriveEpicIntegrationTail and re-driven by a later call, and any
	// resumed run crosses the same boundary - and an in-memory declaration
	// does not survive that. Inference is what still marks those; it is also
	// the only thing that could mark the 116 attempts recorded before the
	// declaration existed.
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
	// CostFloorUSD and CostSource are populated at read time by DeriveAttemptCost
	// for stats display and reporting. They are transient and must never be written to disk / persisted.
	CostFloorUSD *float64 `json:"costFloorUSD,omitempty"`
	CostSource   string   `json:"costSource,omitempty"`
	Turns        *int64   `json:"turns"`
}

// DiffStatter reports how many lines a stage's own commits added and
// removed. It exists as a seam - GitDiffStatter is the real implementation
// (git diff --numstat), and tests inject a fake instead, so exercising the
// ledger never requires a real git binary on the host (AGENTS.md §4: unit
// tests must not shell out).
type DiffStatter interface {
	DiffStat(worktree, baseSHA, commitSHA string) (added, removed *int)
}

// GitDiffStatter is the production DiffStatter: git diff --numstat across
// exactly baseSHA..commitSHA, the same range commit_marker gates scan, so
// the count reflects only what this stage's own commits introduced, not the
// branch's prior history.
type GitDiffStatter struct{}

// DiffStat returns nil (not zero) whenever the range is not meaningful: no
// worktree, no base/commit capture, or a stage that produced no new commit.
func (GitDiffStatter) DiffStat(worktree, baseSHA, commitSHA string) (added, removed *int) {
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

// StageAttemptInput bundles the facts a stage dispatch already has in hand
// once its exit gate has been evaluated - the same values buildStageComment
// formats into free text - so DriveBeadToTerminal hands them to the ledger
// without recomputing anything.
type StageAttemptInput struct {
	AgentID string
	Dialect string
	// ConfiguredModel is settings.agents.<id>.model - the alias used when
	// Usage.Model is nil (the dialect reported no resolved identifier). Can
	// itself be empty (the config field is optional), in which case the
	// row's Model stays nil rather than recording an empty string that
	// would look like a real, if blank, value.
	ConfiguredModel string
	Pool            string
	BeadID          string
	Stage           string
	SessionID       string
	StartedAt       time.Time
	Duration        time.Duration
	// ExitCode is nil when no process ever exited - see the identically
	// named field on StageAttemptRecord.
	ExitCode *int
	// BaseSHA/CommitSHA/Worktree scope the diff-line count to exactly the
	// commits this stage produced, the same range commit_marker gates use.
	BaseSHA           string
	CommitSHA         string
	Worktree          string
	GatePassed        bool
	GateFailureReason string
	ReviewVerdict     *string
	// CausedBy is the review artifact whose rejection this dispatch is
	// answering, set by the driver when this dispatch is the retake of a
	// rewind that same call performed. Empty means "not declared", which is
	// the normal case for every stage that is not a retake - and leaves the
	// writer to fall back to inference. See StageAttemptRecord.CausedBy.
	CausedBy      string
	FollowUpCount int
	Nudged        bool
	Usage         *session.TokenUsageCounts
	// DiffStats is the DiffStatter to use. Nil defaults to GitDiffStatter{}
	// (the real git-shelling implementation) - production call sites never
	// need to set this; only tests inject a fake.
	DiffStats DiffStatter
}

// BuildStageAttemptRecord turns one dispatch's facts into the ledger row
// shape. AttemptNumber and FirstPassApproved are left unset here -
// AppendStageAttempt derives them from the ledger's own prior rows, because
// they depend on history this function does not have. CausedBy is carried
// through when the caller declared one, and left unset otherwise for the
// same writer to infer.
func BuildStageAttemptRecord(in StageAttemptInput) StageAttemptRecord {
	var model *string
	modelResolved := false
	if in.Usage != nil && in.Usage.Model != nil && *in.Usage.Model != "" {
		model = in.Usage.Model
		modelResolved = true
	} else if in.ConfiguredModel != "" {
		configured := in.ConfiguredModel
		model = &configured
	}

	diffStats := in.DiffStats
	if diffStats == nil {
		diffStats = GitDiffStatter{}
	}
	added, removed := diffStats.DiffStat(in.Worktree, in.BaseSHA, in.CommitSHA)

	var gateFailureReason *string
	if !in.GatePassed && in.GateFailureReason != "" {
		reason := in.GateFailureReason
		gateFailureReason = &reason
	}

	var causedBy *string
	if in.CausedBy != "" {
		declared := in.CausedBy
		causedBy = &declared
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
		CausedBy:          causedBy,
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

// AppendStageAttempt writes one line to <StateDir>/run/<epicID>/attempts.jsonl,
// deriving AttemptNumber, CausedBy and FirstPassApproved from the file's
// existing rows before writing - the ledger's own history is the only input
// those three facts need, so callers never pass them in and never disagree
// with each other about how they were computed.
//
// The whole read-derive-write sequence runs under an exclusive advisory
// flock on the ledger file itself, not an in-process mutex: a mutex keyed on
// a Go string only ever serializes goroutines inside one kernl process, and
// two kernl processes dispatching the same epic - the orchestrator and a
// manual `kernl bead run`, or two orchestrator instances - both read the
// file, both derive the same AttemptNumber, and both append it. flock (see
// flock(2)) contends correctly even between file descriptors opened by the
// same process, so it is also what replaced the old sync.Map mutex for
// goroutines inside one process. A numbering scheme that avoids reading the
// file back was the other option; it was rejected because CausedBy
// inherently needs to scan this bead's prior rows regardless, so avoiding
// the read here would not avoid it, only relocate it.
//
// The write itself leaves the file in one of exactly two states: the new
// line fully appended, or the file unchanged. A short write (a real
// possibility - see the loop inside Go's writeFile) or any write error is
// undone by truncating back to the length observed before the write, so a
// transient failure (a full disk) can never leave a partial trailing line
// for the next call to trip over. If an *earlier* call still managed to
// leave one anyway (the process itself was killed between the syscall and
// the truncate-back), this call detects and repairs it before doing
// anything else: a dangling row is never counted, and the physical garbage
// is trimmed away so the file stays valid JSONL. That combination means one
// interrupted write costs exactly the row it happened during, never every
// row after it.
func AppendStageAttempt(stateDir, epicID string, rec StageAttemptRecord) error {
	return appendStageAttempt(stateDir, epicID, rec, openLedgerFileForAppend)
}

// ledgerFile is the subset of *os.File AppendStageAttempt needs. It exists
// as a seam: real ENOSPC and Close() failures cannot be triggered
// hermetically from a unit test, but a fake implementing this interface can
// inject a short write or a Close error on top of a real underlying file
// (so flock, which needs a genuine fd, still behaves correctly), which is
// how the truncate-back-on-failure and close-error-propagation logic below
// gets exercised without touching the host's actual disk limits.
type ledgerFile interface {
	io.Reader
	io.Writer
	io.Seeker
	Truncate(size int64) error
	Close() error
	Fd() uintptr
}

func openLedgerFileForAppend(path string) (ledgerFile, error) {
	return os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
}

func appendStageAttempt(stateDir, epicID string, rec StageAttemptRecord, open func(path string) (ledgerFile, error)) (err error) {
	path, err := resolveAttemptLedgerPath(stateDir, epicID)
	if err != nil {
		return err
	}

	f, err := open(path)
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: opening attempt ledger %s for bead %s: %w", path, rec.BeadID, err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("KERNL DISPATCH FAILURE: closing attempt ledger %s for bead %s after write: %w", path, rec.BeadID, cerr)
		}
	}()

	if lockErr := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); lockErr != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: locking attempt ledger %s for bead %s: %w", path, rec.BeadID, lockErr)
	}
	defer func() {
		if uerr := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); uerr != nil && err == nil {
			err = fmt.Errorf("KERNL DISPATCH FAILURE: unlocking attempt ledger %s for bead %s: %w", path, rec.BeadID, uerr)
		}
	}()

	data, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: reading attempt ledger %s for bead %s: %w", path, rec.BeadID, err)
	}

	existing, validSize, err := parseLedgerBytes(path, data)
	if err != nil {
		return err
	}
	if validSize != int64(len(data)) {
		slog.Error("KERNL DISPATCH FAILURE: attempt ledger had a dangling incomplete row - repairing before append (an earlier write was likely interrupted, e.g. by disk exhaustion or a killed process)",
			"path", path, "danglingBytes", int64(len(data))-validSize)
	}

	rec.EpicID = epicID
	rec.AttemptNumber = countPriorAttempts(existing, rec.BeadID, rec.Stage) + 1
	// A declaration from the driver wins: it comes from the code that
	// performed the rewind, which knows what this dispatch is answering
	// rather than working it out from the rows around it.
	if rec.CausedBy == nil {
		rec.CausedBy = findCausedBy(existing, rec.BeadID, rec.Stage)
	}
	if rec.ReviewVerdict != nil {
		firstPassApproved := rec.AttemptNumber == 1 && *rec.ReviewVerdict == "PASS"
		rec.FirstPassApproved = &firstPassApproved
	}

	line, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: encoding stage attempt record for bead %s: %w", rec.BeadID, err)
	}
	line = append(line, '\n')

	if _, err := f.Seek(validSize, io.SeekStart); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: seeking attempt ledger %s for bead %s: %w", path, rec.BeadID, err)
	}
	if err := f.Truncate(validSize); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: truncating attempt ledger %s for bead %s to drop a dangling row before appending: %w", path, rec.BeadID, err)
	}

	n, writeErr := f.Write(line)
	if writeErr == nil && n != len(line) {
		writeErr = fmt.Errorf("short write: wrote %d of %d bytes", n, len(line))
	}
	if writeErr != nil {
		// Undo whatever landed on disk so this call's own failure cannot
		// become the dangling row the NEXT call has to repair - a complete
		// line or nothing is the only state this function ever leaves the
		// file in.
		if terr := f.Truncate(validSize); terr != nil {
			return fmt.Errorf("KERNL DISPATCH FAILURE: writing attempt ledger %s for bead %s failed (%v) AND truncating the partial write back to %d bytes also failed (%v) - the ledger may now contain a corrupt trailing row and needs manual inspection", path, rec.BeadID, writeErr, validSize, terr)
		}
		return fmt.Errorf("KERNL DISPATCH FAILURE: writing attempt ledger %s for bead %s: %w", path, rec.BeadID, writeErr)
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

// findCausedBy answers whether this attempt is redoing work a reviewer
// rejected, and names the review artifact that rejected it.
//
// It walks backward to this bead's most recent recorded attempt (any stage)
// and requires three things of what it finds, each ruling out a shape that
// really occurs in a recorded ledger and is not rework:
//
// The prior attempt must have failed with "verdict_reject: <artifact>" (see
// backend.evaluateSingleExitGate). The prefix is not verdict_not_pass, and
// the difference is the whole meaning of the field: only two states can send
// work back (backend.IsRejectableVerdictState), and only for them does a
// verdict of REJECT produce verdict_reject. verdict_not_pass is every OTHER
// way a verdict gate fails - a missing artifact, a truncated document, a
// reviewer that ran out of budget - which blocks the bead for a human
// rather than handing it to an implementer to redo.
//
// This attempt must be at a DIFFERENT stage than the one that rejected.
// A review stage re-running straight after its own rejection is a second
// opinion on unchanged work, not a redo of it - and marking it charged the
// rework to the reviewer, which is precisely the attribution this field
// exists to avoid. It happens: one such re-run rejected and then passed the
// same code with no implementation between the two.
//
// And this attempt's stage must already have run for this bead. The stage
// that follows a rejection is not always a retake: an epic whose
// integration_review rejected has been seen recording a shipment attempt
// next, and a stage running for the first time cannot be redoing anything.
//
// What survives all three is an attempt at a stage that had already run and
// was sent back to run again, which is what "rework" names.
func findCausedBy(existing []StageAttemptRecord, beadID, stage string) *string {
	const verdictRejectionPrefix = "verdict_reject: "
	if countPriorAttempts(existing, beadID, stage) == 0 {
		return nil
	}
	for i := len(existing) - 1; i >= 0; i-- {
		rec := existing[i]
		if rec.BeadID != beadID {
			continue
		}
		if rec.Stage == stage {
			return nil
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

// parseLedgerBytes parses every complete, newline-terminated line in data as
// a StageAttemptRecord, and returns validSize: the byte offset up to which
// the file is fully valid JSONL. Only the trailing segment - whatever
// follows the last '\n', including the entire file when it contains no '\n'
// at all - is ever treated as "possibly incomplete": every successful
// AppendStageAttempt call writes "<json>\n" as one line, so any content that
// is not itself newline-terminated cannot be something a prior call finished
// writing, whether or not it happens to parse as valid JSON on its own. That
// trailing segment is dropped without error; validSize stops right before
// it, so AppendStageAttempt truncates it away as part of its own append.
//
// A malformed line that DOES have a following '\n' (i.e. it is not the
// trailing segment) is a harder failure: something wrote a broken row in
// the middle of the file, which the same repair logic must not paper over,
// so parsing stops and reports it.
func parseLedgerBytes(path string, data []byte) ([]StageAttemptRecord, int64, error) {
	var records []StageAttemptRecord
	var offset int64

	segments := strings.Split(string(data), "\n")
	for i, seg := range segments {
		isTrailingSegment := i == len(segments)-1
		if isTrailingSegment {
			if strings.TrimSpace(seg) != "" {
				// Whatever follows the last newline (or the whole file, if
				// there was never one) - never something a completed
				// append could have produced.
				slog.Warn("KERNL DISPATCH FAILURE: dropping a dangling trailing row in attempt ledger - it has no line terminator, so a prior write to it was never confirmed complete", "path", path)
			}
			break
		}
		segBytes := int64(len(seg) + 1) // +1 for the newline that terminated this segment
		if strings.TrimSpace(seg) == "" {
			offset += segBytes
			continue
		}
		var rec StageAttemptRecord
		if err := json.Unmarshal([]byte(seg), &rec); err != nil {
			return nil, 0, fmt.Errorf("KERNL DISPATCH FAILURE: parsing attempt ledger %s: corrupt row before the end of file (not the trailing row, so not treated as an interrupted write and not auto-repaired): %w", path, err)
		}
		records = append(records, rec)
		offset += segBytes
	}
	return records, offset, nil
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

// LastGateFailure returns the reason the most recent recorded attempt at
// beadID's given stage failed its exit gate, or "" when the last attempt
// passed, when there is no attempt yet, or when the ledger cannot be read.
//
// It exists because a rejected REVIEW was the only gate outcome that ever
// reached the next implementer: the rejection text lives in an artifact the
// prompt reads back (see StagePromptInput.RejectedReview). Every other gate
// failure - a missing commit marker, a decision record that is not valid
// JSON - was recorded in the ledger, commented on the bead, and then never
// shown to the agent that had to fix it. Observed for real: an implementer
// wrote an invalid escape into decision-record.json, the bead blocked, and
// the next attempt was dispatched with no way to know, because that file
// lives in the artifact directory rather than in the worktree the agent can
// see. It could only rewrite it and hope.
//
// Scoped to one stage on purpose. A review rejection is recorded against the
// REVIEW stage, so filtering by the stage about to run cannot pick one up
// and repeat, in this section, what renderRejectedReview already says in
// its own.
//
// A read failure is silence rather than an error: this feeds one advisory
// prompt section, and a run must not be stopped because an optional hint
// could not be assembled.
func LastGateFailure(stateDir, epicID, beadID, stage string) string {
	path, err := resolveAttemptLedgerPath(stateDir, epicID)
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	records, _, err := parseLedgerBytes(path, data)
	if err != nil {
		return ""
	}
	for i := len(records) - 1; i >= 0; i-- {
		rec := records[i]
		if rec.BeadID != beadID || rec.Stage != stage {
			continue
		}
		if rec.GatePassed || rec.GateFailureReason == nil {
			return ""
		}
		return *rec.GateFailureReason
	}
	return ""
}
