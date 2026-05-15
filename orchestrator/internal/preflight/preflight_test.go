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
