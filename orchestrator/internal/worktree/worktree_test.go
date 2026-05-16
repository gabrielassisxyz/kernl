package worktree

import (
	"errors"
	"os"
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
	if !contains(args, "feat/epic-1") {
		t.Errorf("expected base branch feat/epic-1 in %v", args)
	}
}

func TestEnsureEpicBranchCreatesWhenAbsent(t *testing.T) {
	var calls [][]string
	run := func(dir string, args ...string) (string, error) {
		calls = append(calls, args)
		if args[0] == "branch" && args[1] == "--list" {
			return "", nil
		}
		return "", nil
	}
	m := New(Deps{Root: "/repo", Run: run})
	name, err := m.EnsureEpicBranch("epic-1")
	if err != nil {
		t.Fatalf("EnsureEpicBranch: %v", err)
	}
	if name != "feat/epic-1" {
		t.Errorf("branch name = %q, want feat/epic-1", name)
	}
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(calls))
	}
	if calls[0][0] != "branch" || calls[0][1] != "--list" || calls[0][2] != "feat/epic-1" {
		t.Errorf("first call should be branch --list feat/epic-1, got %v", calls[0])
	}
	if calls[1][0] != "branch" || calls[1][1] != "feat/epic-1" || calls[1][2] != "master" {
		t.Errorf("second call should be branch feat/epic-1 master, got %v", calls[1])
	}
}

func TestEnsureEpicBranchIdempotent(t *testing.T) {
	var calls [][]string
	run := func(dir string, args ...string) (string, error) {
		calls = append(calls, args)
		if args[0] == "branch" && args[1] == "--list" {
			return "  feat/epic-1\n", nil
		}
		return "", nil
	}
	m := New(Deps{Root: "/repo", Run: run})
	name, err := m.EnsureEpicBranch("epic-1")
	if err != nil {
		t.Fatalf("EnsureEpicBranch: %v", err)
	}
	if name != "feat/epic-1" {
		t.Errorf("branch name = %q, want feat/epic-1", name)
	}
	if len(calls) != 1 {
		t.Fatalf("expected 1 call (list only), got %d", len(calls))
	}
}

func TestEnsureEpicBranchListFails(t *testing.T) {
	run := func(dir string, args ...string) (string, error) {
		return "", errors.New("git error")
	}
	m := New(Deps{Root: "/repo", Run: run})
	_, err := m.EnsureEpicBranch("epic-1")
	if err == nil {
		t.Fatal("expected error when branch --list fails")
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("error should contain KERNL DISPATCH FAILURE marker, got: %v", err)
	}
}

func TestEnsureEpicBranchCreateFails(t *testing.T) {
	run := func(dir string, args ...string) (string, error) {
		if args[0] == "branch" && args[1] == "--list" {
			return "", nil
		}
		return "", errors.New("cannot create branch")
	}
	m := New(Deps{Root: "/repo", Run: run})
	_, err := m.EnsureEpicBranch("epic-1")
	if err == nil {
		t.Fatal("expected error when branch creation fails")
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("error should contain KERNL DISPATCH FAILURE marker, got: %v", err)
	}
}

func TestEpicWithTwoChildrenBasedOnEpicBranch(t *testing.T) {
	var calls [][]string
	run := func(dir string, args ...string) (string, error) {
		calls = append(calls, args)
		if args[0] == "branch" && args[1] == "--list" {
			return "", nil
		}
		return "", nil
	}
	m := New(Deps{Root: "/repo", Run: run})

	name, err := m.EnsureEpicBranch("epic-1")
	if err != nil {
		t.Fatalf("EnsureEpicBranch: %v", err)
	}
	if name != "feat/epic-1" {
		t.Errorf("branch name = %q, want feat/epic-1", name)
	}

	_, err = m.Add("epic-1", "child-1")
	if err != nil {
		t.Fatalf("Add child-1: %v", err)
	}

	_, err = m.Add("epic-1", "child-2")
	if err != nil {
		t.Fatalf("Add child-2: %v", err)
	}

	foundList := false
	for _, call := range calls {
		if call[0] == "branch" && call[1] == "--list" && call[2] == "feat/epic-1" {
			foundList = true
			break
		}
	}
	if !foundList {
		t.Error("expected git branch --list feat/epic-1 to be called")
	}

	childCount := 0
	for _, call := range calls {
		if call[0] == "worktree" && call[1] == "add" {
			childCount++
			baseBranch := call[len(call)-1]
			if baseBranch != "feat/epic-1" {
				t.Errorf("child worktree based on %q, want feat/epic-1", baseBranch)
			}
		}
	}
	if childCount != 2 {
		t.Errorf("expected 2 child worktree adds, got %d", childCount)
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

func contains(s []string, sub string) bool {
	for _, v := range s {
		if strings.Contains(v, sub) {
			return true
		}
	}
	return false
}
