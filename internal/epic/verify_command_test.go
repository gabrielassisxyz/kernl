package epic

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func repoWithCI(t *testing.T, mode os.FileMode) string {
	t.Helper()
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "bin"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "bin", "ci"), []byte("#!/bin/sh\n"), mode); err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestResolveVerifyCommandFindsBinCI(t *testing.T) {
	got, err := ResolveVerifyCommand(repoWithCI(t, 0755), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != DefaultVerifyCommand {
		t.Fatalf("verify command = %q, want %q", got, DefaultVerifyCommand)
	}
}

// A configured command can be anything the repository runs, so it is taken as
// given rather than checked against a filesystem that cannot describe it.
func TestResolveVerifyCommandTakesAConfiguredCommandAsGiven(t *testing.T) {
	got, err := ResolveVerifyCommand(t.TempDir(), "cargo test --all")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "cargo test --all" {
		t.Fatalf("verify command = %q", got)
	}
}

func TestResolveVerifyCommandRefusesARepoThatCannotSayHowToVerify(t *testing.T) {
	t.Run("no bin/ci", func(t *testing.T) {
		_, err := ResolveVerifyCommand(t.TempDir(), "")
		if err == nil {
			t.Fatal("expected a loud failure rather than dispatching work nothing can check")
		}
		if !strings.Contains(err.Error(), "verifyCommand") {
			t.Fatalf("error must name the config key that fixes it, got: %v", err)
		}
	})

	t.Run("bin/ci not executable", func(t *testing.T) {
		_, err := ResolveVerifyCommand(repoWithCI(t, 0644), "")
		if err == nil {
			t.Fatal("expected a loud failure for a bin/ci nobody can run")
		}
		if !strings.Contains(err.Error(), "chmod") {
			t.Fatalf("error must say how to fix it, got: %v", err)
		}
	})
}
