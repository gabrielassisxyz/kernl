package epic

import (
	"fmt"
	"strings"
)

// GitRunner runs `git -C <dir> <args...>` and returns its stdout. Named here
// because branch resolution is a question about the repository that the
// worktree manager only happens to be the first to ask.
type GitRunner func(dir string, args ...string) (string, error)

// ResolveBaseBranch answers what work in a repository branches off from.
//
// It never falls back to a branch name. "master" used to be the literal in
// every call site that needed this, so a repository whose default is "main"
// died at `git branch feat/<epic> master` before an agent was ever spawned,
// with an error that reads like a broken repository rather than a wrong guess.
// The order is: the operator's per-repo override, then the remote's published
// HEAD, then the branch the repository itself has checked out.
func ResolveBaseBranch(repoPath, override string, gitRun GitRunner) (string, error) {
	if gitRun == nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: cannot resolve the base branch of %s - no git executor is wired - Fix: pass a git runner to ResolveBaseBranch", repoPath)
	}

	if override != "" {
		if !branchExists(repoPath, override, gitRun) {
			return "", fmt.Errorf("KERNL DISPATCH FAILURE: base branch %q does not exist in %s - Fix: correct registry.repos[].defaultBranch in kernl.yaml, or drop it to resolve the branch from the repository", override, repoPath)
		}
		return override, nil
	}

	if branch := remoteHeadBranch(repoPath, gitRun); branch != "" {
		if !branchExists(repoPath, branch, gitRun) {
			return "", fmt.Errorf("KERNL DISPATCH FAILURE: origin publishes %q as the default branch of %s but no such branch exists locally - Fix: `git -C %s fetch origin %s`, or set registry.repos[].defaultBranch in kernl.yaml", branch, repoPath, repoPath, branch)
		}
		return branch, nil
	}

	branch, err := checkedOutBranch(repoPath, gitRun)
	if err != nil {
		return "", err
	}
	return branch, nil
}

// remoteHeadBranch reads the default branch origin advertises. A repository
// built with `git init` + `remote add` never has this ref, which is a normal
// state and not an error - hence the empty string rather than a failure.
func remoteHeadBranch(repoPath string, gitRun GitRunner) string {
	out, err := gitRun(repoPath, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.TrimSpace(out), "origin/")
}

// checkedOutBranch is the last resort: whatever the repository is sitting on.
// A detached HEAD names no branch, so it fails loud instead of handing back
// the literal "HEAD" for someone to branch off.
func checkedOutBranch(repoPath string, gitRun GitRunner) (string, error) {
	out, err := gitRun(repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: cannot read the checked-out branch of %s - %w - Fix: verify %s is a git repository with at least one commit, or set registry.repos[].defaultBranch in kernl.yaml", repoPath, err, repoPath)
	}
	branch := strings.TrimSpace(out)
	if branch == "" || branch == "HEAD" {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: %s has a detached HEAD, so it names no base branch - Fix: check out a branch, or set registry.repos[].defaultBranch in kernl.yaml", repoPath)
	}
	return branch, nil
}

func branchExists(repoPath, branch string, gitRun GitRunner) bool {
	_, err := gitRun(repoPath, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}
