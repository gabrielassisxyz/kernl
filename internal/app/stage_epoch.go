package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// stageEpochFile holds one bead's current stage epoch. It lives in the
// artifact dir rather than in the worktree for two reasons: the dispatches an
// epoch spans can belong to different `epic run` processes, and anything kept
// inside the worktree is one `git add` away from being committed into the
// target repository (see resolveArtifactDir).
const stageEpochFile = "stage-epoch.json"

// stageEpochRecord is the base commit every dispatch of ONE uninterrupted
// stage entry measures its commit_marker gate against.
//
// Worktree is part of the identity rather than decoration: a bead whose
// worktree was recreated under it carries a base SHA that need not exist in
// the new tree at all, and reusing it would ask the gate about history that
// is not there.
type stageEpochRecord struct {
	State    string `json:"state"`
	BaseSHA  string `json:"baseSHA"`
	Worktree string `json:"worktree"`
}

// resolveStageEpochBase answers the base SHA this dispatch's exit gate scopes
// its scan to, capturing a new one only when the stage is genuinely being
// entered.
//
// It exists because capturing HEAD once per dispatch cannot survive a stage
// being re-entered: the second dispatch of a stage that already committed
// measures against the stage's OWN commit, so `base..HEAD` is empty and
// commit_marker reproves work that is visibly in the worktree - three rows
// in the ledger whose agent closed its session naming the very commit the
// gate then refused. The base belongs to a stage ENTRY, not to a dispatch.
//
// freshEntry is the caller's answer to "did this iteration claim the bead
// into a genuinely new stage": the same boundary that already clears
// wf:blocked-retries:* in DriveBeadToTerminal, because the mechanical-block
// retry budget and the gate's base have exactly one lifecycle between them.
func resolveStageEpochBase(artifactDir, state, worktree string, freshEntry bool, headSHA HeadSHAResolver) (string, error) {
	if !freshEntry {
		rec, found, err := readStageEpoch(artifactDir)
		if err != nil {
			return "", err
		}
		if found && rec.State == state && rec.Worktree == worktree && rec.BaseSHA != "" {
			return rec.BaseSHA, nil
		}
	}
	base := headSHA.HeadSHA(worktree)
	if base == "" {
		// An unreadable HEAD is not an epoch. Persisting "" would pin every
		// later dispatch of this stage to a base the gate rejects outright
		// (commit_marker_unscoped), turning one transient git failure into a
		// permanent one; leaving it unwritten keeps this dispatch behaving
		// exactly as it did before epochs existed.
		return "", nil
	}
	if err := writeStageEpoch(artifactDir, stageEpochRecord{State: state, BaseSHA: base, Worktree: worktree}); err != nil {
		return "", err
	}
	return base, nil
}

// readStageEpoch loads the record, reporting whether one was there at all. An
// unreadable or malformed file is an error rather than a silent recapture:
// recapturing is precisely the behaviour the epoch exists to remove, and it
// would fail as an unexplained commit_marker_missing three dispatches later
// instead of here.
func readStageEpoch(artifactDir string) (stageEpochRecord, bool, error) {
	path := filepath.Join(artifactDir, stageEpochFile)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return stageEpochRecord{}, false, nil
	}
	if err != nil {
		return stageEpochRecord{}, false, fmt.Errorf("KERNL DISPATCH FAILURE: reading the stage epoch at %s: %w - Fix: make the file readable, or delete it to re-scope this stage's exit gate to the worktree's current HEAD", path, err)
	}
	var rec stageEpochRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return stageEpochRecord{}, false, fmt.Errorf("KERNL DISPATCH FAILURE: the stage epoch at %s is not readable JSON (%w) - Fix: delete the file to re-scope this stage's exit gate to the worktree's current HEAD", path, err)
	}
	return rec, true, nil
}

func writeStageEpoch(artifactDir string, rec stageEpochRecord) error {
	path := filepath.Join(artifactDir, stageEpochFile)
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: encoding the stage epoch for state %s: %w", rec.State, err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: writing the stage epoch to %s: %w - Fix: check that the artifact directory is writable", path, err)
	}
	return nil
}
