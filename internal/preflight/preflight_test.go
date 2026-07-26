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

func TestRunCollectsAllChecks(t *testing.T) {
	fakeLook := func(bin string) (string, error) {
		if bin == "bd" {
			return "/usr/bin/bd", nil
		}
		return "", errNotFound()
	}

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "kernl.yaml")
	content := []byte(`settings:
  agents:
    stub:
      command: stub
  pools: {}
`)
	if err := os.WriteFile(cfgPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	rep := Run(Deps{
		LookPath:   fakeLook,
		ConfigPath: cfgPath,
		GoVersion:  "go1.26",
	})

	if rep.Check("bd").OK != true {
		t.Error("bd check should pass when LookPath finds it")
	}
	if rep.Check("opencode").OK != false {
		t.Error("opencode check should fail when LookPath misses it")
	}
	if rep.Check("opencode").Fix == "" {
		t.Error("a failing check must carry an actionable Fix string")
	}
}

func writeValidConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "kernl.yaml")
	content := []byte("settings:\n  agents:\n    stub:\n      command: stub\n  pools: {}\n")
	if err := os.WriteFile(cfgPath, content, 0644); err != nil {
		t.Fatal(err)
	}
	return cfgPath
}

func TestOpencodeIsAlwaysAdvisory(t *testing.T) {
	// Only opencode missing; bd present, valid config, orchestrator on.
	look := func(bin string) (string, error) {
		if bin == "bd" {
			return "/usr/bin/bd", nil
		}
		return "", errNotFound()
	}
	rep := Run(Deps{LookPath: look, ConfigPath: writeValidConfig(t), GoVersion: "go1.26", Orchestrator: true})

	if !rep.Check("opencode").Advisory {
		t.Error("opencode must be advisory regardless of orchestrator mode")
	}
	if rep.RequiredFailed() {
		t.Error("a missing opencode alone must not fail required checks")
	}
}

func TestBdIsRequiredOnlyWhenOrchestrating(t *testing.T) {
	// Nothing on PATH; valid config. bd is the only thing that should gate.
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
