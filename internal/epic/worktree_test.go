package epic

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeGitRunner struct {
	calls  [][]string
	branch map[string]bool
}

func newFakeGitRunner() *fakeGitRunner {
	return &fakeGitRunner{branch: make(map[string]bool)}
}

func (f *fakeGitRunner) run(dir string, args ...string) (string, error) {
	f.calls = append(f.calls, args)
	switch args[0] {
	case "branch":
		if args[1] == "--list" {
			if f.branch[args[2]] {
				return args[2] + "\n", nil
			}
			return "", nil
		}
		if len(args) >= 3 && args[0] == "branch" {
			f.branch[args[1]] = true
		}
		return "", nil
	case "worktree":
		return "", nil
	}
	return "", nil
}

type fakeDescUpdater struct {
	updates map[string]string
}

func (f *fakeDescUpdater) update(beadID string, fn func(oldDesc string) string) error {
	if f.updates == nil {
		f.updates = make(map[string]string)
	}
	f.updates[beadID] = fn(f.updates[beadID])
	return nil
}

func (f *fakeDescUpdater) lastDesc(beadID string) string {
	return f.updates[beadID]
}

func TestEnsureEpicBranchCreatesWhenAbsent(t *testing.T) {
	root := t.TempDir()
	fr := newFakeGitRunner()
	fd := &fakeDescUpdater{}
	wm := NewWorktreeManager(root, root, fr.run, fd.update)

	branch, err := wm.EnsureEpicBranch("e1")
	if err != nil {
		t.Fatalf("EnsureEpicBranch: %v", err)
	}
	if branch != "feat/e1" {
		t.Errorf("branch = %q, want feat/e1", branch)
	}
	foundList := false
	foundCreate := false
	for _, call := range fr.calls {
		c := call
		if c[0] == "branch" && c[1] == "--list" && c[2] == "feat/e1" {
			foundList = true
		}
		if c[0] == "branch" && c[1] == "feat/e1" && c[2] == "master" {
			foundCreate = true
		}
	}
	if !foundList {
		t.Error("never listed feat/e1")
	}
	if !foundCreate {
		t.Error("never created feat/e1 from master — it should have been absent on first list call")
	}
}

func TestEnsureEpicBranchIsIdempotent(t *testing.T) {
	root := t.TempDir()
	fr := newFakeGitRunner()
	fd := &fakeDescUpdater{}
	wm := NewWorktreeManager(root, root, fr.run, fd.update)

	_, err := wm.EnsureEpicBranch("e1")
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	fr.calls = nil
	_, err = wm.EnsureEpicBranch("e1")
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	for _, call := range fr.calls {
		if call[0] == "branch" && call[1] == "feat/e1" && call[2] == "master" {
			t.Error("second call should not recreate feat/e1 — branch already exists in fake")
		}
	}
}

func TestEnsureEpicBranchStoresInDescription(t *testing.T) {
	root := t.TempDir()
	fr := newFakeGitRunner()
	fd := &fakeDescUpdater{}
	wm := NewWorktreeManager(root, root, fr.run, fd.update)

	_, err := wm.EnsureEpicBranch("e1")
	if err != nil {
		t.Fatalf("EnsureEpicBranch: %v", err)
	}
	desc := fd.lastDesc("e1")
	if !strings.Contains(desc, "epic_branch: feat/e1") {
		t.Errorf("description missing epic_branch, got: %q", desc)
	}
}

func TestAddBasesWorktreeOnEpicBranchWhenPresent(t *testing.T) {
	root := t.TempDir()
	fr := newFakeGitRunner()
	fr.branch["feat/e1"] = true
	wm := NewWorktreeManager(root, root, fr.run, nil)

	_, err := wm.Add("e1", "child-a", nil)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	var addArgs []string
	for _, call := range fr.calls {
		if call[0] == "worktree" && call[1] == "add" {
			addArgs = call
			break
		}
	}
	if addArgs == nil {
		t.Fatal("git worktree add was never called")
	}
	foundBase := false
	for _, a := range addArgs {
		if a == "feat/e1" {
			foundBase = true
		}
	}
	if !foundBase {
		t.Errorf("worktree add not based on feat/e1: %v", addArgs)
	}
}

func TestAddBasesWorktreeOnMasterWhenEpicBranchAbsent(t *testing.T) {
	root := t.TempDir()
	fr := newFakeGitRunner()
	wm := NewWorktreeManager(root, root, fr.run, nil)

	_, err := wm.Add("e1", "child-b", nil)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	var addArgs []string
	for _, call := range fr.calls {
		if call[0] == "worktree" && call[1] == "add" {
			addArgs = call
			break
		}
	}
	if addArgs == nil {
		t.Fatal("git worktree add was never called")
	}
	foundMaster := false
	for _, a := range addArgs {
		if a == "master" {
			foundMaster = true
		}
	}
	if !foundMaster {
		t.Errorf("worktree add not based on master when epic branch absent: %v", addArgs)
	}
}

func TestAddMergesDependencyBranches(t *testing.T) {
	root := t.TempDir()
	fr := newFakeGitRunner()
	fr.branch["kernl/dep-1"] = true // the dependency already produced its branch
	wm := NewWorktreeManager(root, root, fr.run, nil)

	if _, err := wm.Add("e1", "child-d", []string{"dep-1"}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	merged := false
	for _, call := range fr.calls {
		if call[0] == "merge" && call[len(call)-1] == "kernl/dep-1" {
			merged = true
		}
	}
	if !merged {
		t.Errorf("expected merge of kernl/dep-1 into the dependent worktree; calls: %v", fr.calls)
	}
}

func TestAddSkipsMissingDependencyBranch(t *testing.T) {
	root := t.TempDir()
	fr := newFakeGitRunner()
	// dep-2 never produced a branch (branch map empty) — nothing to merge.
	wm := NewWorktreeManager(root, root, fr.run, nil)

	if _, err := wm.Add("e1", "child-e", []string{"dep-2"}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	for _, call := range fr.calls {
		if call[0] == "merge" {
			t.Errorf("merge must be skipped when dependency branch is absent; calls: %v", fr.calls)
		}
	}
}

func TestAddFallsBackToMkdirWhenNoGitRun(t *testing.T) {
	wm := NewWorktreeManager(t.TempDir(), "", nil, nil)
	path, err := wm.Add("epic-1", "kb-3", nil)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if path == "" {
		t.Error("path should not be empty")
	}
}

func TestAddRecoversFromExistingPath(t *testing.T) {
	// A leftover worktree from a previous failed run should be auto-cleaned
	// so the user does not have to manually `rm -rf` between every retry.
	// The previous behavior was a loud error; we now warn and recover.
	root := t.TempDir()
	existing := filepath.Join(root, "e1", "dup")
	if err := os.MkdirAll(existing, 0755); err != nil {
		t.Fatal(err)
	}
	// Drop a sentinel file so we can confirm the dir was actually replaced.
	if err := os.WriteFile(filepath.Join(existing, "leftover.txt"), []byte("stale"), 0644); err != nil {
		t.Fatal(err)
	}

	fr := newFakeGitRunner()
	wm := NewWorktreeManager(root, root, fr.run, nil)

	path, err := wm.Add("e1", "dup", nil)
	if err != nil {
		t.Fatalf("expected auto-recover, got error: %v", err)
	}
	if path == "" {
		t.Fatal("expected non-empty path after recovery")
	}
	if _, err := os.Stat(filepath.Join(existing, "leftover.txt")); !os.IsNotExist(err) {
		t.Errorf("leftover file should have been removed during recovery, got: %v", err)
	}
}

func TestAddCreatesBranchWithKernlPrefix(t *testing.T) {
	root := t.TempDir()
	fr := newFakeGitRunner()
	wm := NewWorktreeManager(root, root, fr.run, nil)

	_, err := wm.Add("e1", "child-c", nil)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	var addArgs []string
	for _, call := range fr.calls {
		if call[0] == "worktree" && call[1] == "add" {
			addArgs = call
			break
		}
	}
	if addArgs == nil {
		t.Fatal("git worktree add was never called")
	}
	foundKernlPrefix := false
	for _, a := range addArgs {
		if a == "kernl/child-c" {
			foundKernlPrefix = true
		}
	}
	if !foundKernlPrefix {
		t.Errorf("branch name should be kernl/child-c, got: %v", addArgs)
	}
}

func TestCleanupEpic_RemovesWorktreesAndBranches(t *testing.T) {
	root := t.TempDir()
	repoPath := t.TempDir()
	fr := newFakeGitRunner()
	fr.branch["feat/e1"] = true
	fr.branch["kernl/c1"] = true
	fr.branch["kernl/c2"] = true
	wm := NewWorktreeManager(root, repoPath, fr.run, nil)

	_ = os.MkdirAll(filepath.Join(root, "e1", "c1"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "e1", "c1", "file.txt"), []byte("stale"), 0o644)

	err := wm.CleanupEpic("e1", []string{"c1", "c2"})
	if err != nil {
		t.Fatalf("CleanupEpic: %v", err)
	}

	if _, serr := os.Stat(filepath.Join(root, "e1")); !os.IsNotExist(serr) {
		t.Fatalf("expected epic dir to be removed, still exists")
	}

	foundEpicBranch := false
	foundC1Branch := false
	foundC2Branch := false
	for _, call := range fr.calls {
		if call[0] == "branch" && len(call) >= 3 {
			if call[2] == "feat/e1" {
				foundEpicBranch = true
			}
			if call[2] == "kernl/c1" {
				foundC1Branch = true
			}
			if call[2] == "kernl/c2" {
				foundC2Branch = true
			}
		}
	}
	if !foundEpicBranch {
		t.Error("expected branch -D feat/e1")
	}
	if !foundC1Branch {
		t.Error("expected branch -D kernl/c1")
	}
	if !foundC2Branch {
		t.Error("expected branch -D kernl/c2")
	}
}
