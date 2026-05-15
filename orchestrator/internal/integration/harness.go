//go:build integration

package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
)

type Harness struct {
	Config   *config.Config
	RepoPath string
	t        *testing.T
}

// packageDir returns the directory of the current source file at test time.
func packageDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Dir(file)
}

func NewHarness(t *testing.T) *Harness {
	t.Helper()

	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("integration test requires bd CLI in PATH")
	}
	if _, err := exec.LookPath("opencode"); err != nil {
		t.Skip("integration test requires opencode CLI in PATH")
	}

	cfgPath := filepath.Join(packageDir(), "testdata", "kernl.yaml")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("failed to load integration config fixture: %v", err)
	}

	repoDir := t.TempDir()
	gitInit := exec.Command("git", "init", repoDir)
	if out, err := gitInit.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}

	beadsDir := filepath.Join(repoDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}
	issuesJSONL := filepath.Join(beadsDir, "issues.jsonl")
	fixturePath := filepath.Join(packageDir(), "testdata", "beads-single", ".beads", "issues.jsonl")
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read bead fixture: %v", err)
	}
	if err := os.WriteFile(issuesJSONL, data, 0o644); err != nil {
		t.Fatalf("write issues.jsonl: %v", err)
	}

	gitAdd := exec.Command("git", "-C", repoDir, "add", "-A")
	if out, err := gitAdd.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v\n%s", err, out)
	}
	gitCommit := exec.Command("git", "-C", repoDir, "commit", "-m", "fixture")
	if out, err := gitCommit.CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v\n%s", err, out)
	}

	return &Harness{
		Config:   cfg,
		RepoPath: repoDir,
		t:        t,
	}
}

func (h *Harness) Cleanup() {
	// t.TempDir() handles cleanup automatically
}

func (h *Harness) App() *app.App {
	h.t.Helper()

	cfg := *h.Config
	cfg.Registry.Repos = []config.RepoEntry{{Path: h.RepoPath}}

	a, err := app.NewApp(&cfg)
	if err != nil {
		h.t.Fatalf("App(): %v", err)
	}
	return a
}

func (h *Harness) SeedBead(t *testing.T, state string) string {
	t.Helper()
	return "task-1"
}

func (h *Harness) BeadState(t *testing.T, sessionID string) string {
	t.Helper()

	be := backend.NewBdCliBackend(h.RepoPath)
	bead, err := be.Get("task-1", h.RepoPath)
	if err != nil {
		t.Fatalf("BeadState: %v", err)
	}
	if bead == nil {
		t.Fatal("BeadState: bead not found")
	}
	return bead.State
}

func (h *Harness) IsAdvanced(state string) bool {
	return state != "ready_for_implementation"
}
