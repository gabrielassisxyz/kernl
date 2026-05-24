package epic

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/workflow"
)

type WorktreeManager struct {
	root        string
	repoPath    string
	gitRun      func(dir string, args ...string) (string, error)
	updateDesc  func(beadID string, fn func(oldDesc string) string) error
}

func NewWorktreeManager(root string, repoPath string, gitRun func(dir string, args ...string) (string, error), updateDesc func(beadID string, fn func(oldDesc string) string) error) *WorktreeManager {
	return &WorktreeManager{root: root, repoPath: repoPath, gitRun: gitRun, updateDesc: updateDesc}
}

func (m *WorktreeManager) EnsureEpicBranch(epicID string) (string, error) {
	branchName := "feat/" + epicID

	if m.gitRun == nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: gitRun not wired — EnsureEpicBranch cannot operate without a git executor — Fix: wire a git executor via NewWorktreeManager")
	}

	output, err := m.gitRun(m.repoPath, "branch", "--list", branchName)
	if err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: checking epic branch %s — %w — Fix: verify the repo exists at %s", branchName, err, m.repoPath)
	}
	branchExists := strings.TrimSpace(output) != ""

	if !branchExists {
		if _, err := m.gitRun(m.repoPath, "branch", branchName, "master"); err != nil {
			return "", fmt.Errorf("KERNL DISPATCH FAILURE: creating epic branch %s from master — %w — Fix: verify master exists in the repo at %s", branchName, err, m.repoPath)
		}
	}

	if m.updateDesc != nil {
		if err := m.updateDesc(epicID, func(oldDesc string) string {
			return workflow.SetEpicBranch(oldDesc, branchName)
		}); err != nil {
			return "", fmt.Errorf("KERNL DISPATCH FAILURE: storing epic branch %s in epic %s description — %w — Fix: verify the backend is reachable", branchName, epicID, err)
		}
	}

	return branchName, nil
}

// AddEpicWorktree creates (or recovers) a worktree checked out to the epic's
// own branch feat/<epicID>, where the integration and shipment agents run.
// Unlike Add, it does NOT create a new branch — it checks out the EXISTING
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
			return "", fmt.Errorf("KERNL DISPATCH FAILURE: epic worktree path %s exists and auto-clean failed — %w — Fix: remove the directory manually", path, err)
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
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: git worktree add failed for epic branch %s — %w — Fix: ensure %s exists (EnsureEpicBranch) and is not already checked out elsewhere", epicBranch, err, epicBranch)
	}

	return path, nil
}

// CleanupEpic removes all artifacts for an epic from the local filesystem.
// It deletes:
//   - the epic worktree directory root/<epicID> (including all child worktrees)
//   - the feat/<epicID> branch
//   - the kernl/<childID> branches for every child bead.
//
// Branch deletion errors are silently ignored because the branches may never
// have been created (hermetic tests or a bead that never reached implementation).
func (m *WorktreeManager) CleanupEpic(epicID string, childIDs []string) error {
	epicDir := filepath.Join(m.root, epicID)
	if err := os.RemoveAll(epicDir); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: cannot remove worktree directory %s for epic %s: %w — Fix: verify permissions", epicDir, epicID, err)
	}

	if m.gitRun == nil {
		return nil
	}

	_, _ = m.gitRun(m.repoPath, "branch", "-D", "feat/"+epicID)
	for _, childID := range childIDs {
		_, _ = m.gitRun(m.repoPath, "branch", "-D", "kernl/"+childID)
	}
	return nil
}

func (m *WorktreeManager) Add(epicID, beadID string) (string, error) {
	if err := os.MkdirAll(m.root, 0755); err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: cannot create worktree root %s: %w", m.root, err)
	}

	path := filepath.Join(m.root, epicID, beadID)

	if _, err := os.Stat(path); err == nil {
		// Auto-recover from a leftover worktree from a previous failed run.
		// Loud warning, not loud error — the user previously had to manually
		// `rm -rf ~/.kernl/worktrees/<epic>` between every failed epic run.
		slog.Warn("worktree leftover detected, auto-cleaning",
			"path", path, "epic", epicID, "bead", beadID)
		if err := m.removeLeftover(path, beadID); err != nil {
			return "", fmt.Errorf("KERNL DISPATCH FAILURE: worktree path %s exists and auto-clean failed — %w — Fix: remove the directory manually", path, err)
		}
	}

	if m.gitRun == nil {
		if err := os.MkdirAll(path, 0755); err != nil {
			return "", fmt.Errorf("KERNL DISPATCH FAILURE: cannot create worktree for bead %s in epic %s: %w", beadID, epicID, err)
		}
		return path, nil
	}

	// Always prune stale worktree registrations before adding — covers the
	// case where the dir was rm -rf'd externally so git's bookkeeping in
	// .git/worktrees/<name>/ still claims the path is registered. Without
	// this, `git worktree add` fails with "missing but already registered".
	_, _ = m.gitRun(m.repoPath, "worktree", "prune")
	// Also force-delete any stale bead branch from a prior run so we don't
	// collide with `add -b kernl/<bead>`.
	_, _ = m.gitRun(m.repoPath, "branch", "-D", "kernl/"+beadID)

	baseBranch := "master"
	epicBranch := "feat/" + epicID
	output, err := m.gitRun(m.repoPath, "branch", "--list", epicBranch)
	if err == nil && strings.TrimSpace(output) != "" {
		baseBranch = epicBranch
	}

	if _, err := m.gitRun(m.repoPath, "worktree", "add", path, "-b", "kernl/"+beadID, baseBranch); err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: git worktree add failed for bead %s based on %s — %w — Fix: verify the repo at %s is clean and the branch %s exists", beadID, baseBranch, err, m.repoPath, baseBranch)
	}

	return path, nil
}

// removeLeftover deletes a stranded worktree from a previous failed run.
// Tries `git worktree remove --force` first so git's bookkeeping stays
// consistent; falls back to plain os.RemoveAll when gitRun is unwired
// (hermetic tests, or paths that were never registered with git).
func (m *WorktreeManager) removeLeftover(path, beadID string) error {
	if m.gitRun != nil {
		// Best effort — ignore exit codes since the path may have been removed
		// from git's index already.
		_, _ = m.gitRun(m.repoPath, "worktree", "remove", "--force", path)
		_, _ = m.gitRun(m.repoPath, "branch", "-D", "kernl/"+beadID)
	}
	return os.RemoveAll(path)
}
