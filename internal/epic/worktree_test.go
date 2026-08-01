package epic

import (
	"fmt"
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

// An unresolved base branch used to be papered over with the literal "master".
// With git wired, both branch-cutting paths must refuse rather than guess.
func TestBranchCuttingRefusesWithoutABaseBranch(t *testing.T) {
	root := t.TempDir()

	t.Run("epic branch", func(t *testing.T) {
		fr := newFakeGitRunner()
		wm := NewWorktreeManager(root, root, "", fr.run, nil)
		if _, err := wm.EnsureEpicBranch("e1"); err == nil {
			t.Fatal("expected a loud failure when no base branch was resolved")
		}
	})

	t.Run("bead branch", func(t *testing.T) {
		fr := newFakeGitRunner()
		wm := NewWorktreeManager(root, root, "", fr.run, nil)
		if _, err := wm.Add("e1", "kb-1", nil); err == nil {
			t.Fatal("expected a loud failure when no base branch was resolved")
		}
	})
}

func TestEnsureEpicBranchCreatesWhenAbsent(t *testing.T) {
	root := t.TempDir()
	fr := newFakeGitRunner()
	fd := &fakeDescUpdater{}
	wm := NewWorktreeManager(root, root, "main", fr.run, fd.update)

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
		if c[0] == "branch" && c[1] == "feat/e1" && c[2] == "main" {
			foundCreate = true
		}
	}
	if !foundList {
		t.Error("never listed feat/e1")
	}
	if !foundCreate {
		t.Error("never created feat/e1 from main - it should have been absent on first list call")
	}
}

func TestEnsureEpicBranchIsIdempotent(t *testing.T) {
	root := t.TempDir()
	fr := newFakeGitRunner()
	fd := &fakeDescUpdater{}
	wm := NewWorktreeManager(root, root, "main", fr.run, fd.update)

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
		if call[0] == "branch" && call[1] == "feat/e1" && call[2] == "main" {
			t.Error("second call should not recreate feat/e1 - branch already exists in fake")
		}
	}
}

func TestEnsureEpicBranchStoresInDescription(t *testing.T) {
	root := t.TempDir()
	fr := newFakeGitRunner()
	fd := &fakeDescUpdater{}
	wm := NewWorktreeManager(root, root, "main", fr.run, fd.update)

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
	wm := NewWorktreeManager(root, root, "main", fr.run, nil)

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
	wm := NewWorktreeManager(root, root, "main", fr.run, nil)

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
		if a == "main" {
			foundMaster = true
		}
	}
	if !foundMaster {
		t.Errorf("worktree add not based on the resolved base branch when epic branch absent: %v", addArgs)
	}
}

func TestAddMergesDependencyBranches(t *testing.T) {
	root := t.TempDir()
	fr := newFakeGitRunner()
	fr.branch["kernl/dep-1"] = true // the dependency already produced its branch
	wm := NewWorktreeManager(root, root, "main", fr.run, nil)

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
	// dep-2 never produced a branch (branch map empty) - nothing to merge.
	wm := NewWorktreeManager(root, root, "main", fr.run, nil)

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
	wm := NewWorktreeManager(t.TempDir(), "", "", nil, nil)
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
	wm := NewWorktreeManager(root, root, "main", fr.run, nil)

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
	wm := NewWorktreeManager(root, root, "main", fr.run, nil)

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

// TestAddReusesExistingBeadBranchWithoutDeletingIt guards against the data
// loss this package used to cause on every re-run: a bead that reached
// implementation and committed real work to kernl/<bead>, then blocked at a
// later gate, had its branch force-deleted and recreated empty the next time
// the epic ran - discarding the commits with no recovery path. The branch
// must be reused, and `branch -D` must never run against it.
func TestAddReusesExistingBeadBranchWithoutDeletingIt(t *testing.T) {
	root := t.TempDir()
	fr := newFakeGitRunner()
	fr.branch["kernl/child-a"] = true // simulates a prior run's committed work
	wm := NewWorktreeManager(root, root, "main", fr.run, nil)

	_, err := wm.Add("e1", "child-a", nil)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	for _, call := range fr.calls {
		if call[0] == "branch" && len(call) >= 2 && call[1] == "-D" {
			t.Errorf("branch -D must never run against an existing bead branch, got: %v", call)
		}
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
	for _, a := range addArgs {
		if a == "-b" {
			t.Errorf("an existing bead branch must be reused, not recreated with -b: %v", addArgs)
		}
	}
	found := false
	for _, a := range addArgs {
		if a == "kernl/child-a" {
			found = true
		}
	}
	if !found {
		t.Errorf("worktree add should check out the existing branch kernl/child-a: %v", addArgs)
	}
}

// TestAddRecoversFromExistingPathReusesItsBranch covers the combined case: a
// crashed run left both a stale worktree directory AND a bead branch with
// committed work behind. Cleaning the leftover directory must not take the
// branch down with it.
func TestAddRecoversFromExistingPathReusesItsBranch(t *testing.T) {
	root := t.TempDir()
	existing := filepath.Join(root, "e1", "dup")
	if err := os.MkdirAll(existing, 0755); err != nil {
		t.Fatal(err)
	}

	fr := newFakeGitRunner()
	fr.branch["kernl/dup"] = true
	wm := NewWorktreeManager(root, root, "main", fr.run, nil)

	if _, err := wm.Add("e1", "dup", nil); err != nil {
		t.Fatalf("Add: %v", err)
	}

	for _, call := range fr.calls {
		if call[0] == "branch" && len(call) >= 2 && call[1] == "-D" {
			t.Errorf("recovering a leftover worktree must not delete its branch, got: %v", call)
		}
	}
}

// TestAddFailsLoudlyWhenBeadBranchAlreadyCheckedOutElsewhere covers the case
// the reuse path introduces: `git worktree add <path> <branch>` refuses when
// that branch is already checked out in another worktree. That failure must
// reach the caller, not be swallowed the way the old unconditional
// `branch -D` swallowed everything.
func TestAddFailsLoudlyWhenBeadBranchAlreadyCheckedOutElsewhere(t *testing.T) {
	root := t.TempDir()
	fr := newFakeGitRunner()
	fr.branch["kernl/child-a"] = true

	wm := NewWorktreeManager(root, root, "main", func(dir string, args ...string) (string, error) {
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "add" && len(args) == 4 {
			// 4 args ("worktree" "add" path branch) is the reuse call, distinct
			// from the 6-arg fresh-branch call ("worktree" "add" path "-b" branch base).
			return "", fmt.Errorf("fatal: 'kernl/child-a' is already used by worktree at '/other/path'")
		}
		return fr.run(dir, args...)
	}, nil)

	_, err := wm.Add("e1", "child-a", nil)
	if err == nil {
		t.Fatal("expected an error when the bead branch is already checked out elsewhere")
	}
	if !strings.Contains(err.Error(), "kernl/child-a") {
		t.Errorf("error should name the conflicting branch, got: %v", err)
	}
}

// TestAddFailsLoudlyWhenCheckingBeadBranchFails covers the other half of "no
// discarded errors": if git itself cannot answer whether the branch exists,
// Add must not silently fall through to either the reuse or the create path.
func TestAddFailsLoudlyWhenCheckingBeadBranchFails(t *testing.T) {
	root := t.TempDir()
	fr := newFakeGitRunner()

	wm := NewWorktreeManager(root, root, "main", func(dir string, args ...string) (string, error) {
		if len(args) >= 3 && args[0] == "branch" && args[1] == "--list" && args[2] == "kernl/child-a" {
			return "", fmt.Errorf("simulated git failure")
		}
		return fr.run(dir, args...)
	}, nil)

	if _, err := wm.Add("e1", "child-a", nil); err == nil {
		t.Fatal("expected an error when checking the bead branch fails")
	}
}

func TestCleanupEpic_RemovesWorktreesAndBranches(t *testing.T) {
	root := t.TempDir()
	repoPath := t.TempDir()
	fr := newFakeGitRunner()
	fr.branch["feat/e1"] = true
	fr.branch["kernl/c1"] = true
	fr.branch["kernl/c2"] = true
	wm := NewWorktreeManager(root, repoPath, "main", fr.run, nil)

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
