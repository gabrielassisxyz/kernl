//go:build integration

package integration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/epic"
)

type Harness struct {
	Config   *config.Config
	RepoPath string
	t        *testing.T
}

func packageDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Dir(file)
}

func newHarnessWithFixture(t *testing.T, fixtureName string) *Harness {
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
	fixturePath := filepath.Join(packageDir(), "testdata", fixtureName, ".beads", "issues.jsonl")
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read bead fixture %s: %v", fixtureName, err)
	}
	if err := os.WriteFile(issuesJSONL, data, 0o644); err != nil {
		t.Fatalf("write issues.jsonl: %v", err)
	}

	// bd 1.0+ uses Dolt as source of truth; the JSONL is only an export format.
	// Run `bd init --from-jsonl` to populate the Dolt database from the fixture,
	// suppressing agent file / hook scaffolding so the tmpdir stays clean.
	bdInit := exec.Command("bd", "init",
		"--from-jsonl",
		"--skip-agents",
		"--skip-hooks",
		"--non-interactive",
		"--role", "maintainer",
	)
	bdInit.Dir = repoDir
	if out, err := bdInit.CombinedOutput(); err != nil {
		t.Fatalf("bd init --from-jsonl failed: %v\n%s", err, out)
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

func NewHarness(t *testing.T) *Harness {
	t.Helper()
	return newHarnessWithFixture(t, "beads-single")
}

func NewEpicHarness(t *testing.T) *Harness {
	t.Helper()
	return newHarnessWithFixture(t, "beads-epic-diamond")
}

func (h *Harness) Cleanup() {
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

func (h *Harness) BeadState(t *testing.T, beadID string) string {
	t.Helper()

	be := backend.NewBdCliBackend(h.RepoPath)
	bead, err := be.Get(beadID, h.RepoPath)
	if err != nil {
		t.Fatalf("BeadState(%s): %v", beadID, err)
	}
	if bead == nil {
		t.Fatalf("BeadState(%s): bead not found", beadID)
	}
	return bead.State
}

func (h *Harness) IsAdvanced(state string) bool {
	return state != "open"
}

func (h *Harness) IsTerminal(state string) bool {
	switch state {
	case "done", "closed", "blocked", "skipped":
		return true
	}
	return false
}

func (h *Harness) SeedEpic(t *testing.T, fixtureName string) string {
	t.Helper()
	return "epic-1"
}

func (h *Harness) ChildIDs(epicID string) []string {
	h.t.Helper()
	be := backend.NewBdCliBackend(h.RepoPath)
	children, err := be.List(&backend.BeadListFilters{Parent: epicID}, h.RepoPath)
	if err != nil {
		h.t.Fatalf("ChildIDs(%s): %v", epicID, err)
	}
	ids := make([]string, 0, len(children))
	for _, c := range children {
		ids = append(ids, c.ID)
	}
	return ids
}

func (h *Harness) RunEpic(t *testing.T, epicID string) *epic.Executor {
	t.Helper()

	a := h.App()
	ep, err := epic.LoadEpic(a.Backend, epicID, h.RepoPath)
	if err != nil {
		t.Fatalf("RunEpic: load epic %s: %v", epicID, err)
	}

	wm := epic.NewWorktreeManager(a.Config.Orchestrator.WorktreeRoot)

	ex := epic.NewExecutor(epic.ExecutorDeps{
		Epic: ep,
		RunBead: func(ctx context.Context, in epic.RunInput) (epic.RunResult, error) {
			res, err := a.Driver.RunBead(ctx, app.RunBeadInput{
				BeadID:   in.BeadID,
				RepoPath: h.RepoPath,
				AgentID:  "opencode",
			})
			if err != nil {
				return epic.RunResult{}, err
			}
			return epic.RunResult{FinalState: res.FinalState, Success: res.Success}, nil
		},
		Worktree:      wm,
		MaxConcurrent: a.Config.Orchestrator.MaxConcurrentBeads,
		Emit: func(ev epic.EpicEvent) {
			a.EpicEvents.Publish(ev)
		},
	})

	if err := ex.Run(context.Background()); err != nil {
		t.Fatalf("RunEpic: %v", err)
	}
	return ex
}
