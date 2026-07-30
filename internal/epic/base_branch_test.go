package epic

import (
	"fmt"
	"strings"
	"testing"
)

// scriptedGit answers a fixed set of git invocations and fails every other
// one, so a test states exactly which repository it is describing.
type scriptedGit struct {
	replies map[string]string
	calls   []string
}

func (s *scriptedGit) run(dir string, args ...string) (string, error) {
	key := strings.Join(args, " ")
	s.calls = append(s.calls, key)
	if out, ok := s.replies[key]; ok {
		return out, nil
	}
	return "", fmt.Errorf("exit status 1")
}

const (
	remoteHeadCall = "symbolic-ref --short refs/remotes/origin/HEAD"
	currentCall    = "rev-parse --abbrev-ref HEAD"
)

func existsCall(branch string) string {
	return "rev-parse --verify --quiet refs/heads/" + branch
}

func TestResolveBaseBranchUsesRemoteHead(t *testing.T) {
	git := &scriptedGit{replies: map[string]string{
		remoteHeadCall:       "origin/main\n",
		existsCall("main"):   "abc123\n",
		currentCall:          "some-feature\n",
		existsCall("master"): "def456\n",
	}}

	got, err := ResolveBaseBranch("/repo", "", git.run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "main" {
		t.Fatalf("base branch = %q, want main", got)
	}
}

func TestResolveBaseBranchPrefersOverride(t *testing.T) {
	git := &scriptedGit{replies: map[string]string{
		remoteHeadCall:      "origin/main\n",
		existsCall("trunk"): "abc123\n",
	}}

	got, err := ResolveBaseBranch("/repo", "trunk", git.run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "trunk" {
		t.Fatalf("base branch = %q, want trunk", got)
	}
	for _, call := range git.calls {
		if call == remoteHeadCall {
			t.Fatal("an explicit defaultBranch must not consult origin/HEAD")
		}
	}
}

func TestResolveBaseBranchRejectsMissingOverride(t *testing.T) {
	git := &scriptedGit{replies: map[string]string{remoteHeadCall: "origin/main\n"}}

	_, err := ResolveBaseBranch("/repo", "nope", git.run)
	if err == nil {
		t.Fatal("expected a loud failure for a defaultBranch that does not exist")
	}
	if !strings.Contains(err.Error(), "defaultBranch") {
		t.Fatalf("error must name the config key that fixes it, got: %v", err)
	}
}

// A repository created with `git init` + `remote add` has no origin/HEAD. That
// is normal, so resolution falls through to the checked-out branch instead of
// guessing a name.
func TestResolveBaseBranchFallsBackToCheckedOutBranch(t *testing.T) {
	git := &scriptedGit{replies: map[string]string{currentCall: "develop\n"}}

	got, err := ResolveBaseBranch("/repo", "", git.run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "develop" {
		t.Fatalf("base branch = %q, want develop", got)
	}
}

func TestResolveBaseBranchRejectsDetachedHead(t *testing.T) {
	git := &scriptedGit{replies: map[string]string{currentCall: "HEAD\n"}}

	_, err := ResolveBaseBranch("/repo", "", git.run)
	if err == nil {
		t.Fatal("expected a loud failure on a detached HEAD")
	}
	if !strings.Contains(err.Error(), "detached") {
		t.Fatalf("error must say what is wrong with the repository, got: %v", err)
	}
}

// The whole point of the finding: nothing anywhere may fall back to "master".
func TestResolveBaseBranchNeverGuessesMaster(t *testing.T) {
	git := &scriptedGit{replies: map[string]string{}}

	got, err := ResolveBaseBranch("/repo", "", git.run)
	if err == nil {
		t.Fatalf("expected a loud failure, got base branch %q", got)
	}
}
