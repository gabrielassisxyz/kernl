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
