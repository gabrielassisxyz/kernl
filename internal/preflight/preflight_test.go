package preflight

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func errNotFound() error {
	return errors.New("not found")
}

func TestRunChecksTheBinariesTheConfigNames(t *testing.T) {
	// claude and codex configured against a knots repo: not one of the three
	// binaries the old fixed list looked at.
	looked := map[string]bool{}
	fakeLook := func(bin string) (string, error) {
		looked[bin] = true
		if bin == "claude" {
			return "/usr/bin/claude", nil
		}
		return "", errNotFound()
	}

	cfgPath := filepath.Join(t.TempDir(), "kernl.yaml")
	content := []byte(`settings:
  agents:
    claude:
      command: claude
    codex:
      command: codex
  pools: {}
registry:
  repos:
    - path: /repo/one
      memoryManager: knots
`)
	if err := os.WriteFile(cfgPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	rep := Run(Deps{LookPath: fakeLook, ConfigPath: cfgPath, GoVersion: "go1.26"})

	if rep.Check("claude") == nil || !rep.Check("claude").OK {
		t.Error("a configured agent that is installed must be reported as present")
	}
	codex := rep.Check("codex")
	if codex == nil || codex.OK {
		t.Error("a configured agent that is missing must be reported")
	}
	if codex != nil && codex.Fix == "" {
		t.Error("a failing check must carry an actionable Fix string")
	}
	if rep.Check("kno") == nil {
		t.Error("the tracker behind a knots repo is kno, and it must be checked")
	}
	if rep.Check("opencode") != nil {
		t.Error("opencode is not configured here and must not be reported on")
	}
	if looked["bd"] {
		t.Error("bd is not the tracker of any registered repo and must not be looked up")
	}
}

// A tracker doctor cannot name is reported as unresolved. It used to fall back
// to bd, so an operator who mistyped the key got a green `bd` and a run that
// executed something else.
func TestDoctorReportsATrackerItCannotName(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "kernl.yaml")
	content := []byte("settings:\n  agents:\n    stub:\n      command: stub\n  pools: {}\nregistry:\n  repos:\n    - path: /repo/one\n      memoryManager: beadz\n")
	if err := os.WriteFile(cfgPath, content, 0644); err != nil {
		t.Fatal(err)
	}
	looked := map[string]bool{}
	look := func(name string) (string, error) {
		looked[name] = true
		return "/usr/bin/" + name, nil
	}

	rep := Run(Deps{LookPath: look, ConfigPath: cfgPath, GoVersion: "go1.26"})

	check := rep.Check("tracker")
	if check == nil {
		t.Fatal("a repo whose tracker cannot be resolved must produce a check")
	}
	if check.OK {
		t.Error("an unresolvable tracker must not be reported as healthy")
	}
	if !strings.Contains(check.Detail, "beadz") {
		t.Errorf("the detail must name the value that could not be resolved, got %q", check.Detail)
	}
	if looked["bd"] {
		t.Error("a mistyped memoryManager must not silently become bd")
	}
}

func writeValidConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "kernl.yaml")
	content := []byte("settings:\n  agents:\n    stub:\n      command: stub\n  pools: {}\nregistry:\n  repos:\n    - path: /repo/one\n      memoryManager: beads\n")
	if err := os.WriteFile(cfgPath, content, 0644); err != nil {
		t.Fatal(err)
	}
	return cfgPath
}

func TestAgentBinariesAreAlwaysAdvisory(t *testing.T) {
	// Only the agent missing; the tracker is present, config valid,
	// orchestrator on.
	look := func(bin string) (string, error) {
		if bin == "bd" {
			return "/usr/bin/bd", nil
		}
		return "", errNotFound()
	}
	rep := Run(Deps{LookPath: look, ConfigPath: writeValidConfig(t), GoVersion: "go1.26", Orchestrator: true})

	if !rep.Check("stub").Advisory {
		t.Error("a configured agent must be advisory regardless of orchestrator mode")
	}
	if rep.RequiredFailed() {
		t.Error("a missing agent alone must not fail required checks")
	}
}

func TestTrackerIsRequiredOnlyWhenOrchestrating(t *testing.T) {
	// Nothing on PATH; valid config. The tracker is the only thing that gates.
	missing := func(string) (string, error) { return "", errNotFound() }
	cfg := writeValidConfig(t)

	withOrch := Run(Deps{LookPath: missing, ConfigPath: cfg, GoVersion: "go1.26", Orchestrator: true})
	if withOrch.Check("bd").Advisory {
		t.Error("bd must be required (non-advisory) when orchestrating")
	}
	if !withOrch.RequiredFailed() {
		t.Error("missing bd must fail required checks when orchestrating")
	}

	noOrch := Run(Deps{LookPath: missing, ConfigPath: cfg, GoVersion: "go1.26", Orchestrator: false})
	if !noOrch.Check("bd").Advisory {
		t.Error("bd must be advisory when orchestration is disabled")
	}
	if noOrch.RequiredFailed() {
		t.Error("missing bd must NOT fail required checks when orchestration is disabled")
	}
}

// writeMinimalConfig writes a config that passes the config check, so a test can
// assert on the verdict of another check without the config one dragging it down.
func writeMinimalConfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kernl.yaml")
	content := []byte("settings:\n  agents:\n    stub:\n      command: stub\n  pools: {}\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestVaultLayoutCheckAbsentWithoutScanner(t *testing.T) {
	rep := Run(Deps{LookPath: func(string) (string, error) { return "", errNotFound() }})
	if rep.Check("vault-layout") != nil {
		t.Error("vault-layout must be skipped when no scanner is injected (serve has no graph yet)")
	}
}

func TestVaultLayoutCheckPassesOnCleanVault(t *testing.T) {
	rep := Run(Deps{
		LookPath:     func(string) (string, error) { return "", errNotFound() },
		VaultOrphans: func() ([]string, error) { return nil, nil },
	})
	c := rep.Check("vault-layout")
	if c == nil || !c.OK {
		t.Fatalf("expected a passing vault-layout check, got %+v", c)
	}
}

func TestVaultLayoutCheckNamesTheOrphansAndStaysAdvisory(t *testing.T) {
	rep := Run(Deps{
		LookPath:     func(string) (string, error) { return "", errNotFound() },
		ConfigPath:   writeMinimalConfig(t),
		GoVersion:    "go1.26",
		VaultOrphans: func() ([]string, error) { return []string{"tasks/mine.md (no describes edge)"}, nil },
	})
	c := rep.Check("vault-layout")
	if c == nil || c.OK {
		t.Fatalf("expected a failing vault-layout check, got %+v", c)
	}
	if !c.Advisory {
		t.Error("an unclaimed note is drift to report, not a reason to block")
	}
	if !strings.Contains(c.Detail, "tasks/mine.md") {
		t.Errorf("detail must name the file, got %q", c.Detail)
	}
	if rep.RequiredFailed() {
		t.Error("an advisory failure must not fail doctor")
	}
}

func TestVaultLayoutCheckSurvivesAScanError(t *testing.T) {
	rep := Run(Deps{
		LookPath:     func(string) (string, error) { return "", errNotFound() },
		VaultOrphans: func() ([]string, error) { return nil, errors.New("db locked") },
	})
	c := rep.Check("vault-layout")
	if c == nil || c.OK || !c.Advisory {
		t.Fatalf("a scan error must surface as an advisory failure, got %+v", c)
	}
}
