//go:build integration

package integration

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/session"
)

type fakeSessionProvider struct{}

func (fakeSessionProvider) GetSessionEntry(id string) (session.SessionInfo, bool) { return session.SessionInfo{}, true }
func (fakeSessionProvider) ListSessionIDs() []session.SessionInfo                 { return nil }
func (fakeSessionProvider) PushEvent(id string, evt session.TerminalEvent)     {}

type procWrap struct{ cmd *exec.Cmd }

func (p *procWrap) Wait() error { return p.cmd.Wait() }
func (p *procWrap) Kill() error { return p.cmd.Process.Kill() }

// TestWorkflowStaleLabelLoop proves that a bead with a stale wf:state:* label
// still advances end-to-end because defaultState() now prefers the real bd
// status over labels.
//
// This is the regression test for the infinite-loop bug where agents update
// bd status but do not touch labels, leaving NormalizeBead stuck on the old
// label value and re-spawning the same agent in the same state forever.
func TestWorkflowStaleLabelLoop(t *testing.T) {
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("integration test requires bd CLI in PATH")
	}

	repoPath := t.TempDir()

	// ---- 1. Init git + bd repo ----
	gitInit := exec.Command("git", "init", repoPath)
	if out, err := gitInit.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	beadsDir := filepath.Join(repoPath, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}
	if err := os.WriteFile(filepath.Join(beadsDir, "issues.jsonl"), []byte(`{"id":"stub-1","type":"task","title":"stub","status":"open","priority":1}`+"\n"), 0o644); err != nil {
		t.Fatalf("write issues.jsonl: %v", err)
	}
	bdInit := exec.Command("bd", "init",
		"--from-jsonl", "--skip-agents", "--skip-hooks",
		"--non-interactive", "--role", "maintainer",
	)
	bdInit.Dir = repoPath
	if out, err := bdInit.CombinedOutput(); err != nil {
		t.Fatalf("bd init: %v\n%s", err, out)
	}
	bdConfig := exec.Command("bd", "config", "set", "status.custom",
		"ready_for_implementation,implementation,ready_for_implementation_review,implementation_review,ready_for_shipment,shipment,ready_for_shipment_review,shipment_review,shipped",
	)
	bdConfig.Dir = repoPath
	if out, err := bdConfig.CombinedOutput(); err != nil {
		t.Fatalf("bd config set status.custom: %v\n%s", err, out)
	}
	gitAdd := exec.Command("git", "-C", repoPath, "add", "-A")
	if out, err := gitAdd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	gitCommit := exec.Command("git", "-C", repoPath, "commit", "--allow-empty", "-m", "fixture")
	if out, err := gitCommit.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}

	// ---- 2. Create bead with stale label ----
	be := backend.NewBdCliBackend(repoPath)
	createOut, err := be.Create(backend.CreateBeadInput{
		Title:    "stale-label regression bead",
		Type:     "task",
		Priority: 1,
	}, repoPath)
	if err != nil {
		t.Fatalf("create bead: %v", err)
	}
	beadID := createOut.ID

	// Advance to ready_for_implementation via bd, then add a STALE label that
	// claims the bead is still at "implementation".
	if err := be.Update(beadID, backend.UpdateBeadInput{
		State:     "ready_for_implementation",
		SetLabels: []string{"wf:state:implementation", "profile:autopilot_no_planning"},
	}, repoPath); err != nil {
		t.Fatalf("setup bead state: %v", err)
	}

	// ---- 3. Write fake-agent script ----
	fakeAgentPath := filepath.Join(repoPath, "fake-agent.sh")
	fakeAgentSrc := `#!/bin/bash
set -eu
bid="${BEAD_ID:?}"
repo="${REPO_PATH:?}"
current=$(bd -C "$repo" show "$bid" --json | python3 -c "import json,sys; d=json.load(sys.stdin); d=d[0] if isinstance(d,list) else d; print(d.get('status',''))")
next=""
case "$current" in
  implementation)          next="ready_for_implementation_review" ;;
  implementation_review)   next="ready_for_shipment"               ;;
  shipment)                next="ready_for_shipment_review"       ;;
  shipment_review)         next="shipped"                          ;;
  *) exit 0 ;;
esac
bd -C "$repo" update "$bid" --status "$next" >/dev/null
`
	if err := os.WriteFile(fakeAgentPath, []byte(fakeAgentSrc), 0o755); err != nil {
		t.Fatalf("write fake-agent: %v", err)
	}

	// ---- 4. Assemble minimal config ----
	cfg := &config.Config{
		Settings: config.Settings{
			Agents: map[string]config.AgentConfig{
				"fake-agent": {Command: fakeAgentPath, Args: []string{}},
			},
			Pools: map[string]config.PoolConfig{
				"implementation":        {Agents: []config.WeightedAgent{{AgentID: "fake-agent", Weight: 1}}},
				"implementation_review": {Agents: []config.WeightedAgent{{AgentID: "fake-agent", Weight: 1}}},
				"shipment":              {Agents: []config.WeightedAgent{{AgentID: "fake-agent", Weight: 1}}},
				"shipment_review":       {Agents: []config.WeightedAgent{{AgentID: "fake-agent", Weight: 1}}},
			},
		},
		Registry: config.RegistryConfig{
			Repos: []config.RepoEntry{{Path: repoPath}},
		},
	}
	cfg.Orchestrator.StageRetryAttempts = 1

	// ---- 5. Build driver with real spawn + fake session provider ----
	driver := app.NewSessionDriver(app.DriverDeps{
		Backend: be,
		Spawn: func(ctx context.Context, cmd string, args []string, cwd string, env []string) (app.Process, io.Reader, io.Reader, error) {
			c := exec.CommandContext(ctx, cmd, args...)
			if cwd != "" {
				c.Dir = cwd
			}
			if len(env) > 0 {
				c.Env = append(os.Environ(), env...)
			}
			stdout, err := c.StdoutPipe()
			if err != nil {
				return nil, nil, nil, fmt.Errorf("stdout pipe: %w", err)
			}
			stderr, err := c.StderrPipe()
			if err != nil {
				return nil, nil, nil, fmt.Errorf("stderr pipe: %w", err)
			}
			if err := c.Start(); err != nil {
				return nil, nil, nil, fmt.Errorf("start: %w", err)
			}
			return &procWrap{cmd: c}, stdout, stderr, nil
		},
		SCM:           session.NewSessionConnectionManager(fakeSessionProvider{}, nil),
		NudgeRegistry: session.NewNudgeRegistry(),
	})

	// ---- 6. Run ----
	res, err := app.DriveBeadToTerminal(context.Background(), app.DriveBeadDeps{
		Backend:  be,
		Driver:   driver,
		Config:   cfg,
		BeadID:   beadID,
		RepoPath: repoPath,
		Worktree: repoPath,
	})
	if err != nil {
		t.Fatalf("DriveBeadToTerminal: %v", err)
	}

	finalBead, _ := be.Get(beadID, repoPath)
	finalState := ""
	if finalBead != nil {
		finalState = finalBead.State
	}

	if !res.Success || finalState != "shipped" {
		t.Fatalf("expected Success=true + state=shipped, got Success=%v state=%q err=%v",
			res.Success, finalState, err)
	}
	if res.FinalState != "shipped" {
		t.Fatalf("expected FinalState=shipped, got %q", res.FinalState)
	}
}
