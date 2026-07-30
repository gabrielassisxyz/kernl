package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/epic"
	"github.com/gabrielassisxyz/kernl/internal/session"
	"github.com/gabrielassisxyz/kernl/internal/workflow"
)

// runEpicMerge now (re-)drives the epic integration stages; the drive logic is
// covered hermetically by internal/app TestDriveEpic_*. Here we only assert the
// cheap up-front argument/config validation.

func TestEpicMergeRequiresEpicID(t *testing.T) {
	a := &app.App{Config: &config.Config{Registry: config.RegistryConfig{Repos: []config.RepoEntry{{Path: "/test"}}}}}
	err := runEpicMerge(a, nil, func(string) {})
	if err == nil || !strings.Contains(err.Error(), "requires an epic ID") {
		t.Fatalf("want requires-an-epic-id error, got %v", err)
	}
}

func TestEpicMergeRequiresRepo(t *testing.T) {
	a := &app.App{Config: &config.Config{}}
	err := runEpicMerge(a, []string{"e1"}, func(string) {})
	if err == nil || !strings.Contains(err.Error(), "no repos registered") {
		t.Fatalf("want no-repos error, got %v", err)
	}
}

// runEpicMerge and runEpicAbort each build their own AgentStateStore from
// App.StateDir. Before the injection fix, both instead derived the directory
// from os.Getenv("HOME"), so the two happened to agree only because nothing
// ever set HOME and StateDir to different values. This test sets them apart
// on purpose: a run that resolves either path from HOME lands in homeDir's
// .kernl and fails one of the assertions below.
func TestEpicMergeAndEpicAbortAgreeOnStateDir(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git required")
	}

	repoPath := t.TempDir()
	for _, args := range [][]string{
		{"init"}, {"config", "user.email", "t@t"}, {"config", "user.name", "t"},
		{"commit", "--allow-empty", "-m", "base"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", repoPath}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	stateDir := t.TempDir()

	be := &epicRunTestBackend{beads: []backend.Bead{
		{ID: "e1", Type: "epic", Title: "merge/abort state-dir agreement probe"},
	}}
	scm := session.NewSessionConnectionManager(&epicRunProvider{}, nil)
	refuseSpawn := func(ctx context.Context, cmd string, args []string, cwd string, env []string) (app.Process, io.Reader, io.Reader, error) {
		return nil, nil, nil, fmt.Errorf("test: dispatch must not go further than the agent state store")
	}
	driver := app.NewSessionDriver(app.DriverDeps{Backend: be, Spawn: refuseSpawn, SCM: scm, LogDir: t.TempDir()})

	a := &app.App{
		Backend:  be,
		Driver:   driver,
		StateDir: stateDir,
		Config: &config.Config{
			Settings: config.Settings{
				Agents: map[string]config.AgentConfig{
					"opencode": {Command: "opencode", Args: []string{"run", "--format", "json"}, Label: "opencode"},
				},
				Pools: map[string]config.PoolConfig{
					"integration": {Agents: []config.WeightedAgent{{AgentID: "opencode", Weight: 1}}},
				},
			},
			Registry:     config.RegistryConfig{Repos: []config.RepoEntry{{Path: repoPath, VerifyCommand: "true", MemoryManager: "beads"}}},
			Orchestrator: config.OrchestratorConfig{WorktreeRoot: t.TempDir()},
		},
		EpicEvents: epic.NewEpicEventHub(),
	}

	// The spawn refusal stops the run right after dispatch reaches the
	// integration stage - by construction, after the AgentStateStore for
	// this run has already been built. Whether the run itself succeeds is
	// not the point; where it wrote is.
	_ = runEpicMerge(a, []string{"--dry-run", "e1"}, func(string) {})

	wantDir := filepath.Join(stateDir, "agentstate")
	if _, err := os.Stat(wantDir); err != nil {
		t.Fatalf("epic merge must create its agent state store under App.StateDir (%s), got: %v", wantDir, err)
	}
	if _, err := os.Stat(filepath.Join(homeDir, ".kernl")); !os.IsNotExist(err) {
		t.Fatalf("epic merge must not derive a directory from HOME, but %s/.kernl exists", homeDir)
	}

	// Seed a state file through the exact store epic merge just built, then
	// ask epic abort to purge it. If abort resolved a different directory
	// (e.g. one still derived from HOME) this file would survive.
	store, err := workflow.NewAgentStateStore(wantDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save("e1", workflow.AgentRuntime{}); err != nil {
		t.Fatal(err)
	}

	if err := runEpicAbort(a, []string{"e1", "--yes"}, func(string) {}); err != nil {
		t.Fatalf("runEpicAbort: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wantDir, "e1.json")); !os.IsNotExist(err) {
		t.Fatalf("epic abort must purge agent state from the same directory epic merge wrote to (%s), got: %v", wantDir, err)
	}
	if _, err := os.Stat(filepath.Join(homeDir, ".kernl")); !os.IsNotExist(err) {
		t.Fatalf("epic abort must not derive a directory from HOME, but %s/.kernl exists", homeDir)
	}
}
