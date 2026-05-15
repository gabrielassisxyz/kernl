package epic

import (
	"fmt"
	"os"
	"path/filepath"
)

type WorktreeManager struct {
	root string
}

func NewWorktreeManager(root string) *WorktreeManager {
	return &WorktreeManager{root: root}
}

func (m *WorktreeManager) Add(epicID, beadID string) (string, error) {
	if err := os.MkdirAll(m.root, 0755); err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: cannot create worktree root %s: %w", m.root, err)
	}
	path := filepath.Join(m.root, epicID, beadID)
	if err := os.MkdirAll(path, 0755); err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: cannot create worktree for bead %s in epic %s: %w", beadID, epicID, err)
	}
	return path, nil
}
