//go:build integration

package integration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/epic"
	"golang.org/x/mod/semver"
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

const minBdVersion = "1.0.4"

func parseBdVersion(raw string) string {
	f := strings.Fields(raw)
	for _, tok := range f {
		tok = strings.TrimSpace(tok)
		if len(tok) > 0 && tok[0] >= '0' && tok[0] <= '9' {
			return tok
		}
	}
	return ""
}

func ensureBdVersion(t *testing.T) {
	t.Helper()

	out, err := exec.Command("bd", "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("bd CLI required (>= %s): %v\n%s", minBdVersion, err, out)
	}
	v := parseBdVersion(string(out))
	if v == "" {
		t.Fatalf("bd --version returned unrecognized output: %s", string(out))
	}
	if !semver.IsValid("v" + v) {
		t.Fatalf("bd version %q is not valid semver — run: go install github.com/gastownhall/beads@v%s", v, minBdVersion)
	}
	if semver.Compare("v"+v, "v"+minBdVersion) < 0 {
		t.Fatalf("bd version %s < required %s — run: go install github.com/gastownhall/beads@v%s", v, minBdVersion, minBdVersion)
	}
}

func newHarnessWithFixture(t *testing.T, fixtureName string) *Harness {
	t.Helper()

	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("integration test requires bd CLI in PATH")
	}
	ensureBdVersion(t)
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

	// bd 1.0+ uses Dolt as source of truth; the JSONL is only an export format.
	// Run `bd init` to initialize empty Dolt database, set custom statuses,
	// and import issues.jsonl to populate the Dolt database,
	// suppressing agent file / hook scaffolding so the tmpdir stays clean.
	bdInit := exec.Command("bd", "init",
		"--skip-agents",
		"--skip-hooks",
		"--non-interactive",
		"--role", "maintainer",
	)
	bdInit.Dir = repoDir
	if out, err := bdInit.CombinedOutput(); err != nil {
		t.Fatalf("bd init failed: %v\n%s", err, out)
	}

	bdConfig := exec.Command("bd", "config", "set", "status.custom", "awaiting_integration,awaiting_pr_review")
	bdConfig.Dir = repoDir
	if out, err := bdConfig.CombinedOutput(); err != nil {
		t.Fatalf("bd config set status.custom failed: %v\n%s", err, out)
	}

	// Write issues.jsonl after bd init and bd config set, so auto-export on empty database
	// init does not delete or overwrite the fixture JSONL.
	if err := os.WriteFile(issuesJSONL, data, 0o644); err != nil {
		t.Fatalf("write issues.jsonl: %v", err)
	}

	bdImport := exec.Command("bd", "import")
	bdImport.Dir = repoDir
	if out, err := bdImport.CombinedOutput(); err != nil {
		t.Fatalf("bd import failed: %v\n%s", err, out)
	}

	gitAdd := exec.Command("git", "-C", repoDir, "add", "-A")
	if out, err := gitAdd.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v\n%s", err, out)
	}
	gitCommit := exec.Command("git", "-C", repoDir, "commit", "--allow-empty", "-m", "fixture")
	if out, err := gitCommit.CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v\n%s", err, out)
	}

	return &Harness{
		Config:   cfg,
		RepoPath: repoDir,
		t:        t,
	}
}

func RequireOpencode(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("opencode"); err != nil {
		t.Skip("integration test requires opencode CLI in PATH")
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

	wm := epic.NewWorktreeManager(a.Config.Orchestrator.WorktreeRoot, h.RepoPath, nil, nil)

	ex := epic.NewExecutor(epic.ExecutorDeps{
		Epic: ep,
		RunBead: func(ctx context.Context, in epic.RunInput) (epic.RunResult, error) {
			input, err := app.ResolveAgentForBead(a.Config, a.Backend, in.BeadID, h.RepoPath)
			if err != nil {
				return epic.RunResult{}, err
			}
			input.BeadID = in.BeadID
			input.RepoPath = h.RepoPath

			res, err := a.Driver.RunBead(ctx, input)
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
