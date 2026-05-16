package epic

import (
	"fmt"
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
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: worktree path %s already exists — a previous run left it. Fix: remove the directory and re-run the epic", path)
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
