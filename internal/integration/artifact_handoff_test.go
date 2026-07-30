//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
)

type artifactHandoffBackend struct {
	mu       sync.Mutex
	beads    map[string]*backend.Bead
	comments []struct {
		ID   string
		Body string
	}
	updateErr error
}

func newArtifactHandoffBackend() *artifactHandoffBackend {
	return &artifactHandoffBackend{beads: make(map[string]*backend.Bead)}
}

func (b *artifactHandoffBackend) Get(id string, _ string) (*backend.Bead, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if bd, ok := b.beads[id]; ok {
		cp := *bd
		cp.Labels = append([]string(nil), bd.Labels...)
		return &cp, nil
	}
	return nil, nil
}

func (b *artifactHandoffBackend) Update(id string, in backend.UpdateBeadInput, _ string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.updateErr != nil {
		return b.updateErr
	}
	bd, ok := b.beads[id]
	if !ok {
		return nil
	}
	if in.State != "" {
		bd.State = in.State
	}
	if in.SetLabels != nil {
		bd.Labels = append([]string(nil), in.SetLabels...)
	}
	return nil
}

func (b *artifactHandoffBackend) Comment(id string, body string, _ string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.comments = append(b.comments, struct {
		ID   string
		Body string
	}{ID: id, Body: body})
	return nil
}

func (b *artifactHandoffBackend) List(_ *backend.BeadListFilters, _ string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *artifactHandoffBackend) ListReady(_ *backend.BeadListFilters, _ string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *artifactHandoffBackend) Create(_ backend.CreateBeadInput, _ string) (*backend.Bead, error) {
	return nil, nil
}
func (b *artifactHandoffBackend) Delete(_ string, _ string) error { return nil }
func (b *artifactHandoffBackend) Close(_ string, _ string, _ string) (*backend.TerminalState, error) {
	return nil, nil
}
func (b *artifactHandoffBackend) MarkTerminal(_, _, _, _ string) error { return nil }
func (b *artifactHandoffBackend) Reopen(_, _, _ string) error          { return nil }
func (b *artifactHandoffBackend) Rewind(_, _, _, _ string) error       { return nil }
func (b *artifactHandoffBackend) Search(_ string, _ *backend.BeadListFilters, _ string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *artifactHandoffBackend) Query(_ string, _ *backend.BeadQueryOptions, _ string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *artifactHandoffBackend) AddDependency(_, _, _ string) error    { return nil }
func (b *artifactHandoffBackend) RemoveDependency(_, _, _ string) error { return nil }
func (b *artifactHandoffBackend) ListDependencies(_, _ string, _ *backend.DependencyListOptions) ([]backend.BeadDependency, error) {
	return nil, nil
}
func (b *artifactHandoffBackend) BuildTakePrompt(_ string, _ *backend.TakePromptOptions, _ string) (*backend.TakePromptResult, error) {
	return nil, nil
}
func (b *artifactHandoffBackend) BuildPollPrompt(_ *backend.PollPromptOptions, _ string) (*backend.PollPromptResult, error) {
	return nil, nil
}
func (b *artifactHandoffBackend) ListWorkflows(_ string) ([]backend.WorkflowDescriptor, error) {
	return nil, nil
}
func (b *artifactHandoffBackend) Capabilities() backend.BackendCapabilities {
	return backend.BackendCapabilities{}
}

type artifactWritingDriver struct {
	worktree   string
	beadID     string
	stateDir   string
	calls      int
	writePlans bool
}

// artifactDir mirrors resolveArtifactDir in internal/app/drive_bead.go: this
// bead has no parent, so it is its own epic scope, and plan.md now lives
// under StateDir/run/<bead>/<bead>/, outside the worktree entirely.
func (d *artifactWritingDriver) artifactDir() string {
	return filepath.Join(d.stateDir, "run", d.beadID, d.beadID)
}

func (d *artifactWritingDriver) RunBead(ctx context.Context, in app.RunBeadInput) (app.RunBeadResult, error) {
	d.calls++
	if d.writePlans && d.calls == 1 {
		dir := d.artifactDir()
		if err := os.MkdirAll(dir, 0755); err != nil {
			return app.RunBeadResult{}, fmt.Errorf("mkdir artifact dir: %w", err)
		}
		planPath := filepath.Join(dir, "plan.md")
		if err := os.WriteFile(planPath, []byte("## Bead Plan\n\n- Task 1\n- Task 2\n"), 0644); err != nil {
			return app.RunBeadResult{}, fmt.Errorf("write plan.md: %w", err)
		}
		gitCommit := exec.Command("git", "-C", d.worktree, "commit", "-m", "planning stage artifact", "--allow-empty")
		if out, err := gitCommit.CombinedOutput(); err != nil {
			return app.RunBeadResult{}, fmt.Errorf("git commit: %w\n%s", err, string(out))
		}
	}
	return app.RunBeadResult{FinalState: "ok", Success: true, SessionID: "ses_artifact_test"}, nil
}

func artifactHandoffTestConfig() *config.Config {
	return &config.Config{
		Settings: config.Settings{
			Agents: map[string]config.AgentConfig{
				"opencode": {Command: "opencode", Args: []string{"run"}},
			},
			Pools: map[string]config.PoolConfig{
				"planning":              {Agents: []config.WeightedAgent{{AgentID: "opencode", Weight: 1}}},
				"plan_review":           {Agents: []config.WeightedAgent{{AgentID: "opencode", Weight: 1}}},
				"implementation":        {Agents: []config.WeightedAgent{{AgentID: "opencode", Weight: 1}}},
				"implementation_review": {Agents: []config.WeightedAgent{{AgentID: "opencode", Weight: 1}}},
				"integration":           {Agents: []config.WeightedAgent{{AgentID: "opencode", Weight: 1}}},
				"integration_review":    {Agents: []config.WeightedAgent{{AgentID: "opencode", Weight: 1}}},
				"shipment":              {Agents: []config.WeightedAgent{{AgentID: "opencode", Weight: 1}}},
				"shipment_review":       {Agents: []config.WeightedAgent{{AgentID: "opencode", Weight: 1}}},
			},
		},
	}
}

func TestArtifactHandoff_PlanningArtifactWrittenAndCommented(t *testing.T) {
	repoDir := t.TempDir()
	worktreeDir := filepath.Join(repoDir, "worktrees", "epic-1", "kb-1")

	initArgs := []string{"init", "--initial-branch", "main", repoDir}
	cmd := exec.Command("git", initArgs...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}

	gitignorePath := filepath.Join(repoDir, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte(".opencode/\n.claude/\n\n!.kernl/\n"), 0644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	if err := os.MkdirAll(worktreeDir, 0755); err != nil {
		t.Fatalf("mkdir worktree: %v", err)
	}

	wtAdd := exec.Command("git", "-C", repoDir, "worktree", "add", worktreeDir, "-b", "kernl/kb-1")
	if out, err := wtAdd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v\n%s", err, out)
	}

	be := newArtifactHandoffBackend()
	be.beads["kb-1"] = &backend.Bead{
		ID:        "kb-1",
		State:     "planning",
		ProfileID: "autopilot",
	}

	stateDir := t.TempDir()
	driver := &artifactWritingDriver{
		worktree:   worktreeDir,
		beadID:     "kb-1",
		stateDir:   stateDir,
		writePlans: true,
	}

	_, err := app.DriveBeadToTerminal(context.Background(), app.DriveBeadDeps{
		StateDir:       stateDir,
		TrackerCommand: "bd",
		VerifyCommand:  "bin/ci",
		Backend:        be,
		Driver:         driver,
		Config:         artifactHandoffTestConfig(),
		BeadID:         "kb-1",
		RepoPath:       repoDir,
		Worktree:       worktreeDir,
		MaxStages:      16,
	})
	if err != nil {
		t.Fatalf("DriveBeadToTerminal: %v", err)
	}

	// The planning artifact lives outside the worktree entirely - inside it
	// is exactly the location a stage's own `git add <files>` can sweep into
	// the target repository's commits, which is the defect this directory
	// move exists to close.
	planPath := filepath.Join(driver.artifactDir(), "plan.md")
	if _, err := os.Stat(planPath); os.IsNotExist(err) {
		t.Fatalf("planning artifact not found at %q after DriveBeadToTerminal", planPath)
	}
	if strings.HasPrefix(planPath, worktreeDir) {
		t.Fatalf("planning artifact %q must not be inside the worktree %q", planPath, worktreeDir)
	}

	be.mu.Lock()
	comments := be.comments
	be.mu.Unlock()

	if len(comments) == 0 {
		t.Fatal("expected at least one stage comment")
	}

	foundPlanningComment := false
	for _, c := range comments {
		if strings.Contains(c.Body, "stage: planning") {
			foundPlanningComment = true
			if !strings.Contains(c.Body, "artifact: "+planPath) {
				t.Errorf("planning comment missing artifact path %q:\n%s", planPath, c.Body)
			}
			if !strings.Contains(c.Body, "agent: opencode") {
				t.Errorf("planning comment missing agent:\n%s", c.Body)
			}
		}
	}
	if !foundPlanningComment {
		t.Fatal("no comment found for planning stage")
	}
}
