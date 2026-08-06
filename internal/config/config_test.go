package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadValidConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "kernl.yaml")
	content := []byte(`
settings:
  agents:
    claude:
      command: claude
      type: claude
  pools:
    implementation:
      agents:
        - agentId: claude
          weight: 1
  defaults:
    interactiveSessionTimeoutMinutes: 30
registry:
  repos:
    - path: /tmp/test-repo
      memoryManager: beads
server:
  port: 9090
`)
	if err := os.WriteFile(cfgPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Server.Port)
	}
	if cfg.Settings.Defaults.InteractiveSessionTimeoutMinutes != 30 {
		t.Errorf("expected timeout 30, got %d", cfg.Settings.Defaults.InteractiveSessionTimeoutMinutes)
	}
	if len(cfg.Settings.Agents) != 1 {
		t.Errorf("expected 1 agent, got %d", len(cfg.Settings.Agents))
	}
	if cfg.Settings.Agents["claude"].Command != "claude" {
		t.Errorf("expected command 'claude', got %s", cfg.Settings.Agents["claude"].Command)
	}
}

func TestLoadDefaultsPort(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "kernl.yaml")
	content := []byte(`
settings:
  agents:
    stub:
      command: stub
  pools: {}
registry:
  repos: []
`)
	if err := os.WriteFile(cfgPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Settings.Defaults.InteractiveSessionTimeoutMinutes != 10 {
		t.Errorf("expected default timeout 10, got %d", cfg.Settings.Defaults.InteractiveSessionTimeoutMinutes)
	}
}

func TestLoadAppliesOrchestratorDefaults(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "kernl.yaml")
	content := []byte(`
settings:
  agents:
    stub:
      command: stub
  pools: {}
`)
	if err := os.WriteFile(cfgPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Orchestrator.MaxConcurrentBeads != 5 {
		t.Errorf("MaxConcurrentBeads default = %d, want 5", cfg.Orchestrator.MaxConcurrentBeads)
	}
	if cfg.Orchestrator.WorktreeRoot == "" {
		t.Error("WorktreeRoot default must be set (~/.kernl/worktrees)")
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load("/nonexistent/path/kernl.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "kernl.yaml")
	content := []byte(`{invalid yaml [[[[`)
	if err := os.WriteFile(cfgPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

// TestLoadDABlock covers all four combinations of da.agent/da.workDir: both
// unset is a normal, supported state (the fork gate is off), both set is
// accepted and carried through, and exactly one set is an operator mistake
// that must fail loud - Load never checks whether da.agent names a real
// settings.agents entry or whether da.workDir exists on disk (app.NewDA
// does, at resolution time), only that the pairing itself is not half-done.
func TestLoadDABlock(t *testing.T) {
	base := `settings:
  agents:
    stub:
      command: stub
  pools: {}
`
	cases := []struct {
		name      string
		daBlock   string
		wantErr   bool
		wantAgent string
		wantDir   string
	}{
		{name: "neither set is not an error", daBlock: "", wantErr: false},
		{
			name: "both set is accepted",
			daBlock: `da:
  agent: stub
  workDir: /home/operator/system-repo
`,
			wantErr:   false,
			wantAgent: "stub",
			wantDir:   "/home/operator/system-repo",
		},
		{
			name: "agent only is an operator mistake",
			daBlock: `da:
  agent: stub
`,
			wantErr: true,
		},
		{
			name: "workDir only is an operator mistake",
			daBlock: `da:
  workDir: /home/operator/system-repo
`,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			cfgPath := filepath.Join(dir, "kernl.yaml")
			if err := os.WriteFile(cfgPath, []byte(base+tc.daBlock), 0644); err != nil {
				t.Fatal(err)
			}

			cfg, err := Load(cfgPath)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error for a half-set da: block")
				}
				if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
					t.Errorf("error missing KERNL DISPATCH FAILURE marker: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error: %v", err)
			}
			if cfg.DA.Agent != tc.wantAgent {
				t.Errorf("DA.Agent = %q, want %q", cfg.DA.Agent, tc.wantAgent)
			}
			if cfg.DA.WorkDir != tc.wantDir {
				t.Errorf("DA.WorkDir = %q, want %q", cfg.DA.WorkDir, tc.wantDir)
			}
		})
	}
}

func TestLoadRejectsEmptyAgentsWithActionableError(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "kernl.yaml")
	content := []byte(`settings:
  agents: {}
  pools: {}
`)
	if err := os.WriteFile(cfgPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for config with zero agents")
	}
	for _, want := range []string{"KERNL DISPATCH FAILURE", "settings.agents", "kernl.yaml.example"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q: %v", want, err)
		}
	}
}

func TestLoadRegistryRepoPathExpansion(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir error: %v", err)
	}

	base := `settings:
  agents:
    stub:
      command: stub
  pools: {}
`

	cases := []struct {
		name     string
		repoPath string
		wantPath string
		wantErr  bool
	}{
		{
			name:     "expand tilde with subpath",
			repoPath: "~/repositories/foo",
			wantPath: filepath.Join(home, "repositories/foo"),
		},
		{
			name:     "expand tilde alone",
			repoPath: "~",
			wantPath: home,
		},
		{
			name:     "absolute path unaltered",
			repoPath: "/var/repos/bar",
			wantPath: "/var/repos/bar",
		},
		{
			name:     "relative path unaltered",
			repoPath: "relative/path/baz",
			wantPath: "relative/path/baz",
		},
		{
			name:     "unsupported username tilde expansion fails loud",
			repoPath: "~otheruser/repo",
			wantErr:  true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			cfgPath := filepath.Join(dir, "kernl.yaml")
			content := base + "registry:\n  repos:\n    - path: " + fmt.Sprintf("%q", tc.repoPath) + "\n      memoryManager: br\n"
			if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
				t.Fatal(err)
			}

			cfg, err := Load(cfgPath)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for path %q, got nil", tc.repoPath)
				}
				if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
					t.Errorf("error missing KERNL DISPATCH FAILURE marker: %v", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("Load() error: %v", err)
			}
			if len(cfg.Registry.Repos) != 1 {
				t.Fatalf("expected 1 repo entry, got %d", len(cfg.Registry.Repos))
			}
			if cfg.Registry.Repos[0].Path != tc.wantPath {
				t.Errorf("repo path = %q, want %q", cfg.Registry.Repos[0].Path, tc.wantPath)
			}
		})
	}
}

func TestLoadOrchestratorPathExpansion(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir error: %v", err)
	}

	base := `settings:
  agents:
    stub:
      command: stub
  pools: {}
`

	t.Run("defaults when left empty", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "kernl.yaml")
		content := base + `orchestrator: {}
`
		if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(cfgPath)
		if err != nil {
			t.Fatalf("Load() error: %v", err)
		}
		wantWorktree := filepath.Join(home, ".kernl", "worktrees")
		wantRunState := filepath.Join(home, ".kernl", "runstate.db")
		if cfg.Orchestrator.WorktreeRoot != wantWorktree {
			t.Errorf("WorktreeRoot = %q, want default %q", cfg.Orchestrator.WorktreeRoot, wantWorktree)
		}
		if cfg.Orchestrator.RunStatePath != wantRunState {
			t.Errorf("RunStatePath = %q, want default %q", cfg.Orchestrator.RunStatePath, wantRunState)
		}
	})

	t.Run("tilde expansion", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "kernl.yaml")
		content := base + `orchestrator:
  worktreeRoot: "~/custom/worktrees"
  runStatePath: "~/custom/runstate.db"
`
		if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(cfgPath)
		if err != nil {
			t.Fatalf("Load() error: %v", err)
		}
		wantWorktree := filepath.Join(home, "custom/worktrees")
		wantRunState := filepath.Join(home, "custom/runstate.db")
		if cfg.Orchestrator.WorktreeRoot != wantWorktree {
			t.Errorf("WorktreeRoot = %q, want %q", cfg.Orchestrator.WorktreeRoot, wantWorktree)
		}
		if cfg.Orchestrator.RunStatePath != wantRunState {
			t.Errorf("RunStatePath = %q, want %q", cfg.Orchestrator.RunStatePath, wantRunState)
		}
	})

	t.Run("absolute path unaltered", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "kernl.yaml")
		content := base + `orchestrator:
  worktreeRoot: "/var/kernl/worktrees"
  runStatePath: "/var/kernl/runstate.db"
`
		if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(cfgPath)
		if err != nil {
			t.Fatalf("Load() error: %v", err)
		}
		wantWorktree := "/var/kernl/worktrees"
		wantRunState := "/var/kernl/runstate.db"
		if cfg.Orchestrator.WorktreeRoot != wantWorktree {
			t.Errorf("WorktreeRoot = %q, want %q", cfg.Orchestrator.WorktreeRoot, wantWorktree)
		}
		if cfg.Orchestrator.RunStatePath != wantRunState {
			t.Errorf("RunStatePath = %q, want %q", cfg.Orchestrator.RunStatePath, wantRunState)
		}
	})

	t.Run("unsupported username tilde expansion fails loud", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "kernl.yaml")
		content := base + `orchestrator:
  worktreeRoot: "~otheruser/worktrees"
`
		if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		_, err := Load(cfgPath)
		if err == nil {
			t.Fatal("expected error for ~otheruser path, got nil")
		}
		if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
			t.Errorf("error missing KERNL DISPATCH FAILURE marker: %v", err)
		}
	})
}
