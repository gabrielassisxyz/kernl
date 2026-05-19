package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type fakeRunner struct {
	args   [][]string
	output string
	err    error
	wd     string
}

func (f *fakeRunner) run(dir string, args ...string) (string, error) {
	f.wd = dir
	f.args = append(f.args, args)
	return f.output, f.err
}

func TestAddCreatesWorktreeAtEpicBeadPath(t *testing.T) {
	fr := &fakeRunner{}
	m := New(Deps{Root: "/tmp/kr", Run: fr.run})
	path, err := m.Add("epic-1", "kb-3")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	want := filepath.Join("/tmp/kr", "epic-1", "kb-3")
	if path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
	if len(fr.args) != 1 {
		t.Fatalf("expected 1 call to Run, got %d", len(fr.args))
	}
	args := fr.args[0]
	if args[0] != "worktree" || args[1] != "add" {
		t.Errorf("expected `git worktree add`, got %v", args)
	}
	if !contains(args, "kernl/kb-3") {
		t.Errorf("expected branch kernl/kb-3 in %v", args)
	}
}

func TestAddWhenPathExistsReturnsError(t *testing.T) {
	tmp := t.TempDir()
	existing := filepath.Join(tmp, "epic-1", "kb-3")
	if err := os.MkdirAll(existing, 0755); err != nil {
		t.Fatal(err)
	}

	fr := &fakeRunner{}
	m := New(Deps{Root: tmp, Run: fr.run})
	_, err := m.Add("epic-1", "kb-3")
	if err == nil {
		t.Fatal("expected error when path already exists")
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("error should contain KERNL DISPATCH FAILURE marker, got: %v", err)
	}
	if !strings.Contains(err.Error(), existing) {
		t.Errorf("error should mention the path, got: %v", err)
	}
}

func TestRemoveRemovesWorktree(t *testing.T) {
	fr := &fakeRunner{}
	m := New(Deps{Root: "/tmp/kr", Run: fr.run})
	err := m.Remove("epic-1", "kb-3")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if len(fr.args) != 1 {
		t.Fatalf("expected 1 call to Run, got %d", len(fr.args))
	}
	args := fr.args[0]
	if args[0] != "worktree" || args[1] != "remove" {
		t.Errorf("expected `git worktree remove`, got %v", args)
	}
	if !contains(args, "/tmp/kr/epic-1/kb-3") {
		t.Errorf("expected path in remove args, got %v", args)
	}
}

func TestPrunePrunes(t *testing.T) {
	fr := &fakeRunner{}
	m := New(Deps{Root: "/tmp/kr", Run: fr.run})
	err := m.Prune()
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if len(fr.args) != 1 {
		t.Fatalf("expected 1 call to Run, got %d", len(fr.args))
	}
	args := fr.args[0]
	if args[0] != "worktree" || args[1] != "prune" {
		t.Errorf("expected `git worktree prune`, got %v", args)
	}
}

func TestPathReturnsJoinedPath(t *testing.T) {
	m := New(Deps{Root: "/tmp/kr"})
	path := m.Path("epic-1", "kb-3")
	want := filepath.Join("/tmp/kr", "epic-1", "kb-3")
	if path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
}

func TestWorktreeGitignoreAllowsKernlDir(t *testing.T) {
	dir := t.TempDir()

	initArgs := []string{"init", "--initial-branch", "main", dir}
	cmd := exec.Command("git", initArgs...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}

	gitignorePath := filepath.Join(dir, ".gitignore")
	content := ".opencode/\n.claude/\n*.jsonl\n.dolt/\n\n!.kernl/\n"
	if err := os.WriteFile(gitignorePath, []byte(content), 0644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	kernlDir := filepath.Join(dir, ".kernl", "test-id")
	if err := os.MkdirAll(kernlDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	artifactPath := filepath.Join(kernlDir, "plan.md")
	if err := os.WriteFile(artifactPath, []byte("## Plan\n"), 0644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	addCmd := exec.Command("git", "-C", dir, "add", "-A")
	if out, err := addCmd.CombinedOutput(); err != nil {
		t.Fatalf("git add -A: %v\n%s", err, out)
	}

	checkCmd := exec.Command("git", "-C", dir, "check-ignore", ".kernl/")
	out, _ := checkCmd.CombinedOutput()
	if len(strings.TrimSpace(string(out))) > 0 {
		t.Errorf("git reported .kernl/ as ignored (it should not be):\n%s", string(out))
	}
}

func contains(s []string, sub string) bool {
	for _, v := range s {
		if strings.Contains(v, sub) {
			return true
		}
	}
	return false
}
