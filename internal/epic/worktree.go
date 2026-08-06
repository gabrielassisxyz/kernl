package epic

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/workflow"
)

type WorktreeManager struct {
	root     string
	repoPath string
	// baseBranch is what epic and bead branches are cut from. It is resolved
	// per repository by ResolveBaseBranch and passed in rather than assumed,
	// because a wrong guess here fails at `git branch` with an error that
	// looks like a broken repository. Empty is only valid alongside a nil
	// gitRun, where no branch is ever created.
	baseBranch string
	gitRun     func(dir string, args ...string) (string, error)
	updateDesc func(beadID string, fn func(oldDesc string) string) error
}

func NewWorktreeManager(root string, repoPath string, baseBranch string, gitRun func(dir string, args ...string) (string, error), updateDesc func(beadID string, fn func(oldDesc string) string) error) *WorktreeManager {
	return &WorktreeManager{root: root, repoPath: repoPath, baseBranch: baseBranch, gitRun: gitRun, updateDesc: updateDesc}
}

func (m *WorktreeManager) EnsureEpicBranch(epicID string) (string, error) {
	branchName := "feat/" + epicID

	if m.gitRun == nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: gitRun not wired - EnsureEpicBranch cannot operate without a git executor - Fix: wire a git executor via NewWorktreeManager")
	}
	if m.baseBranch == "" {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: no base branch for %s - epic branch %s has nothing to be cut from - Fix: pass the branch resolved by ResolveBaseBranch to NewWorktreeManager", m.repoPath, branchName)
	}

	output, err := m.gitRun(m.repoPath, "branch", "--list", branchName)
	if err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: checking epic branch %s - %w - Fix: verify the repo exists at %s", branchName, err, m.repoPath)
	}
	branchExists := strings.TrimSpace(output) != ""

	if !branchExists {
		if _, err := m.gitRun(m.repoPath, "branch", branchName, m.baseBranch); err != nil {
			return "", fmt.Errorf("KERNL DISPATCH FAILURE: creating epic branch %s from %s - %w - Fix: verify %s exists in the repo at %s", branchName, m.baseBranch, err, m.baseBranch, m.repoPath)
		}
	}

	if m.updateDesc != nil {
		if err := m.updateDesc(epicID, func(oldDesc string) string {
			return workflow.SetEpicBranch(oldDesc, branchName)
		}); err != nil {
			return "", fmt.Errorf("KERNL DISPATCH FAILURE: storing epic branch %s in epic %s description - %w - Fix: verify the backend is reachable", branchName, epicID, err)
		}
	}

	return branchName, nil
}

// AddEpicWorktree creates (or recovers) a worktree checked out to the epic's
// own branch feat/<epicID>, where the integration and shipment agents run.
// Unlike Add, it does NOT create a new branch - it checks out the EXISTING
// epic branch so the child merges and the PR push land on feat/<epicID>.
// Call EnsureEpicBranch first so the branch exists.
func (m *WorktreeManager) AddEpicWorktree(epicID string) (string, error) {
	if err := os.MkdirAll(m.root, 0755); err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: cannot create worktree root %s: %w", m.root, err)
	}

	path := filepath.Join(m.root, epicID, "_epic")
	epicBranch := "feat/" + epicID

	if _, err := os.Stat(path); err == nil {
		slog.Warn("epic worktree leftover detected, auto-cleaning", "path", path, "epic", epicID)
		if m.gitRun != nil {
			_, _ = m.gitRun(m.repoPath, "worktree", "remove", "--force", path)
		}
		if err := os.RemoveAll(path); err != nil {
			return "", fmt.Errorf("KERNL DISPATCH FAILURE: epic worktree path %s exists and auto-clean failed - %w - Fix: remove the directory manually", path, err)
		}
	}

	if m.gitRun == nil {
		if err := os.MkdirAll(path, 0755); err != nil {
			return "", fmt.Errorf("KERNL DISPATCH FAILURE: cannot create epic worktree for %s: %w", epicID, err)
		}
		return path, nil
	}

	_, _ = m.gitRun(m.repoPath, "worktree", "prune")

	if _, err := m.gitRun(m.repoPath, "worktree", "add", path, epicBranch); err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: git worktree add failed for epic branch %s - %w - Fix: ensure %s exists (EnsureEpicBranch) and is not already checked out elsewhere", epicBranch, err, epicBranch)
	}

	return path, nil
}

// CleanupEpic removes all artifacts for an epic from the local filesystem.
// It deletes:
//   - the epic worktree directory root/<epicID> (including all child worktrees)
//   - the feat/<epicID> branch (if fully merged) or renames it to archive/<epicID>-<timestamp> (if unmerged)
//   - the kernl/<childID> branches for every child bead (or renames them to archive/<epicID>-<childID>-<timestamp> if unmerged).
//
// Branch non-existence is ignored because branches may never have been created
// (hermetic tests or a bead that never reached implementation).
func (m *WorktreeManager) CleanupEpic(epicID string, childIDs []string) error {
	epicDir := filepath.Join(m.root, epicID)
	if err := os.RemoveAll(epicDir); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: cannot remove worktree directory %s for epic %s: %w - Fix: verify permissions", epicDir, epicID, err)
	}

	if m.gitRun == nil {
		return nil
	}

	if m.baseBranch == "" {
		return fmt.Errorf("KERNL DISPATCH FAILURE: no base branch for %s - cannot safely check unmerged commits during CleanupEpic - Fix: pass baseBranch resolved by ResolveBaseBranch to NewWorktreeManager", m.repoPath)
	}

	if err := m.cleanupBranch(epicID, "", "feat/"+epicID); err != nil {
		return err
	}
	for _, childID := range childIDs {
		if err := m.cleanupBranch(epicID, childID, "kernl/"+childID); err != nil {
			return err
		}
	}
	return nil
}

func (m *WorktreeManager) cleanupBranch(epicID, beadID, branchName string) error {
	out, err := m.gitRun(m.repoPath, "branch", "--list", branchName)
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: checking branch %s - %w - Fix: verify repo at %s", branchName, err, m.repoPath)
	}
	if strings.TrimSpace(out) == "" {
		return nil
	}

	countOut, err := m.gitRun(m.repoPath, "rev-list", "--count", m.baseBranch+".."+branchName)
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: checking unmerged commits for branch %s against %s - %w - Fix: verify branch %s exists in %s", branchName, m.baseBranch, err, m.baseBranch, m.repoPath)
	}

	var count int
	if _, err := fmt.Sscanf(strings.TrimSpace(countOut), "%d", &count); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: parsing unmerged commit count %q for branch %s: %w", countOut, branchName, err)
	}

	if count == 0 {
		if _, err := m.gitRun(m.repoPath, "branch", "-D", branchName); err != nil {
			return fmt.Errorf("KERNL DISPATCH FAILURE: deleting branch %s - %w", branchName, err)
		}
		return nil
	}

	ts := time.Now().Format("20060102-150405")
	var archiveBranch string
	if beadID != "" {
		archiveBranch = fmt.Sprintf("archive/%s-%s-%s", epicID, beadID, ts)
	} else {
		archiveBranch = fmt.Sprintf("archive/%s-%s", epicID, ts)
	}

	if _, err := m.gitRun(m.repoPath, "branch", "-m", branchName, archiveBranch); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: renaming branch %s with unmerged commits to %s - %w", branchName, archiveBranch, err)
	}

	slog.Warn("preserving branch with unmerged commits",
		"old_branch", branchName,
		"new_branch", archiveBranch,
		"unmerged_commits", count,
	)
	return nil
}

// Path answers where Add would create a worktree for a given epicID/beadID
// pair, without creating, removing or otherwise touching anything. Callers
// that need to refuse a re-run rather than silently let Add force-recreate
// (and thereby auto-clean) an existing worktree check this first.
func (m *WorktreeManager) Path(epicID, beadID string) string {
	return filepath.Join(m.root, epicID, beadID)
}

// Add creates the isolated worktree for a child bead on its own branch
// kernl/<beadID>. depBeadIDs are the bead's direct dependencies; their branches
// (kernl/<dep>) are merged into the new worktree so a dependent child starts
// from a tree that already contains its dependencies' committed work rather
// than branching blind off the epic base. The executor only dispatches a bead
// after every dependency reached awaiting_integration, so each dep branch
// exists by the time we merge it. Transitive deps need no special handling:
// each dep branch already merged ITS deps when it was created.
func (m *WorktreeManager) Add(epicID, beadID string, depBeadIDs []string) (string, error) {
	if err := os.MkdirAll(m.root, 0755); err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: cannot create worktree root %s: %w", m.root, err)
	}

	path := filepath.Join(m.root, epicID, beadID)

	if _, err := os.Stat(path); err == nil {
		// Auto-recover from a leftover worktree from a previous failed run.
		// Loud warning, not loud error - the user previously had to manually
		// `rm -rf ~/.kernl/worktrees/<epic>` between every failed epic run.
		slog.Warn("worktree leftover detected, auto-cleaning",
			"path", path, "epic", epicID, "bead", beadID)
		if err := m.removeLeftover(path); err != nil {
			return "", fmt.Errorf("KERNL DISPATCH FAILURE: worktree path %s exists and auto-clean failed - %w - Fix: remove the directory manually", path, err)
		}
	}

	if m.gitRun == nil {
		if err := os.MkdirAll(path, 0755); err != nil {
			return "", fmt.Errorf("KERNL DISPATCH FAILURE: cannot create worktree for bead %s in epic %s: %w", beadID, epicID, err)
		}
		return path, nil
	}

	// Always prune stale worktree registrations before adding - covers the
	// case where the dir was rm -rf'd externally so git's bookkeeping in
	// .git/worktrees/<name>/ still claims the path is registered. Without
	// this, `git worktree add` fails with "missing but already registered".
	_, _ = m.gitRun(m.repoPath, "worktree", "prune")

	if m.baseBranch == "" {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: no base branch for %s - bead branch kernl/%s has nothing to be cut from - Fix: pass the branch resolved by ResolveBaseBranch to NewWorktreeManager", m.repoPath, beadID)
	}

	baseBranch := m.baseBranch
	epicBranch := "feat/" + epicID
	output, err := m.gitRun(m.repoPath, "branch", "--list", epicBranch)
	if err == nil && strings.TrimSpace(output) != "" {
		baseBranch = epicBranch
	}

	if err := m.addBeadWorktree(path, beadID, baseBranch); err != nil {
		return "", err
	}

	if err := m.mergeDependencyBranches(path, beadID, baseBranch, depBeadIDs); err != nil {
		return "", err
	}

	return path, nil
}

// addBeadWorktree checks out kernl/<beadID> at path. If the branch already
// exists it is reused rather than recreated: a bead that reached
// implementation and committed work before blocking at a later gate carries
// that work on this branch, and an epic re-run must resume it instead of
// force-deleting and recutting the branch (which is how that work used to be
// lost silently). Only when the branch is genuinely new is it cut fresh from
// baseBranch.
func (m *WorktreeManager) addBeadWorktree(path, beadID, baseBranch string) error {
	branchName := "kernl/" + beadID

	out, err := m.gitRun(m.repoPath, "branch", "--list", branchName)
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: checking bead branch %s - %w - Fix: verify the repo exists at %s", branchName, err, m.repoPath)
	}

	if strings.TrimSpace(out) != "" {
		if _, err := m.gitRun(m.repoPath, "worktree", "add", path, branchName); err != nil {
			return fmt.Errorf("KERNL DISPATCH FAILURE: git worktree add failed reusing bead branch %s - %w - Fix: %s is likely already checked out in another worktree; run `git worktree list` in %s to find it, remove that worktree (or its stale registration with `git worktree prune`), then retry", branchName, err, branchName, m.repoPath)
		}
		return nil
	}

	if _, err := m.gitRun(m.repoPath, "worktree", "add", path, "-b", branchName, baseBranch); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: git worktree add failed for bead %s based on %s - %w - Fix: verify the repo at %s is clean and the branch %s exists", beadID, baseBranch, err, m.repoPath, baseBranch)
	}
	return nil
}

// mergeDependencyBranches merges each dependency's branch (kernl/<dep>) into the
// freshly-created worktree at path so the dependent child sees its deps' work.
// A dep branch that does not exist (hermetic test, or a dep that produced no
// commits) is skipped. A merge conflict aborts the merge and fails loud - the
// dependent cannot start from an inconsistent tree.
func (m *WorktreeManager) mergeDependencyBranches(path, beadID, baseBranch string, depBeadIDs []string) error {
	for _, dep := range depBeadIDs {
		depBranch := "kernl/" + dep
		out, err := m.gitRun(m.repoPath, "branch", "--list", depBranch)
		if err != nil || strings.TrimSpace(out) == "" {
			continue
		}
		if _, err := m.gitRun(path, "merge", "--no-edit", depBranch); err != nil {
			_, _ = m.gitRun(path, "merge", "--abort")
			return fmt.Errorf("KERNL DISPATCH FAILURE: merging dependency branch %s into bead %s - %w - Fix: %s conflicts with %s; reconcile them or split the beads so they do not edit the same lines", depBranch, beadID, err, depBranch, baseBranch)
		}
	}
	return nil
}

// removeLeftover deletes a stranded worktree checkout from a previous failed
// run. Tries `git worktree remove --force` first so git's bookkeeping stays
// consistent; falls back to plain os.RemoveAll when gitRun is unwired
// (hermetic tests, or paths that were never registered with git).
//
// It deliberately does not touch the bead branch: the worktree directory can
// be left behind by a crash even though the branch already carries committed
// work, and Add reuses that branch right after this returns. Deleting it here
// would discard the work before the caller ever gets a chance to resume it.
func (m *WorktreeManager) removeLeftover(path string) error {
	if m.gitRun != nil {
		// Best effort - ignore exit codes since the path may have been removed
		// from git's index already.
		_, _ = m.gitRun(m.repoPath, "worktree", "remove", "--force", path)
	}
	return os.RemoveAll(path)
}
