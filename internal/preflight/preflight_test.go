package preflight

import (
	"errors"
	"os"
	"path/filepath"
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
