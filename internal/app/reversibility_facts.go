package app

import (
	"fmt"
	"os/exec"
	"path"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/config"
)

// ReversibilityFacts are what an integration rejection is judged against
// before anyone is asked anything. Everything here is measured, not inferred:
// that is the point of computing it up front, since the layers that use these
// are exactly the ones that must decide without a model.
type ReversibilityFacts struct {
	// EpicID is the epic these facts were measured for, carried so the
	// question put to the oracle names it.
	EpicID string
	// Published is true once this branch exists somewhere outside this
	// machine. Undoing published work is not a branch operation any more.
	Published bool
	// PublishedDetail names what was found, so an escalation says "the branch
	// is on refs/remotes/origin/feat/x" rather than "it is published".
	PublishedDetail string
	// IrreversibleSurfacesTouched are the repository-declared paths this
	// branch changed (registry.repos[].irreversibleSurfaces). Empty means the
	// branch touched none, which for a repository that declares none is always
	// true - and reads as "no mechanical reason to escalate", never as
	// "everything is irreversible".
	IrreversibleSurfacesTouched []string
	// ChangeSummary is the diffstat handed to the oracle.
	ChangeSummary string
	// FixupsSpent counts the fix-up beads this epic already drove through
	// their own cycle.
	FixupsSpent int
	// Budget is how many fix-up rounds this epic may spend, carried here so
	// the decision and its reported reason read the same number.
	Budget int
}

// BranchInspector answers the three questions the mechanical layer asks git.
// It is an interface for the reason every other git seam in this package is
// (see DiffStatter): a unit test may not shell out, and this code decides
// whether a human gets woken up.
type BranchInspector interface {
	// PublishedRefs lists refs outside this working tree that already carry
	// the branch.
	PublishedRefs(repoPath, branch string) ([]string, error)
	// ChangedPaths lists the repository-relative paths the branch changed
	// against base.
	ChangedPaths(repoPath, baseBranch, branch string) ([]string, error)
	// ChangeSummary describes the same range in a few lines of text.
	ChangeSummary(repoPath, baseBranch, branch string) (string, error)
}

// GitBranchInspector is the production BranchInspector.
type GitBranchInspector struct{}

// PublishedRefs reads the remote-tracking refs this repository already has for
// the branch.
//
// It is deliberately a local question. kernl's own shipment stage is what
// pushes, and a push updates the remote-tracking ref in the same repository,
// so the fact is here to be read without a network round trip in the middle of
// a decision. The blind spot is a push made from another clone entirely, which
// this cannot see - and before shipment, which is where this gate runs, the
// honest answer there is that nothing was published by this run.
func (GitBranchInspector) PublishedRefs(repoPath, branch string) ([]string, error) {
	out, err := exec.Command("git", "-C", repoPath, "for-each-ref", "--format=%(refname)", "refs/remotes/*/"+branch).Output()
	if err != nil {
		return nil, fmt.Errorf("git for-each-ref for %s in %s: %w", branch, repoPath, err)
	}
	return nonEmptyLines(string(out)), nil
}

// ChangedPaths lists what the branch changed since it diverged from base (the
// three-dot range), not what changed on base meanwhile.
func (GitBranchInspector) ChangedPaths(repoPath, baseBranch, branch string) ([]string, error) {
	out, err := exec.Command("git", "-C", repoPath, "diff", "--name-only", baseBranch+"..."+branch).Output()
	if err != nil {
		return nil, fmt.Errorf("git diff --name-only %s...%s in %s: %w", baseBranch, branch, repoPath, err)
	}
	return nonEmptyLines(string(out)), nil
}

// ChangeSummary is a diffstat, not a diff: the question it feeds is about the
// cost of reversing a change, which its shape answers and its contents do not.
func (GitBranchInspector) ChangeSummary(repoPath, baseBranch, branch string) (string, error) {
	out, err := exec.Command("git", "-C", repoPath, "diff", "--stat", baseBranch+"..."+branch).Output()
	if err != nil {
		return "", fmt.Errorf("git diff --stat %s...%s in %s: %w", baseBranch, branch, repoPath, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func nonEmptyLines(out string) []string {
	lines := make([]string, 0, 8)
	for _, line := range strings.Split(out, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return lines
}

// GatherReversibilityFactsInput is what the caller already knows before git is
// asked anything.
type GatherReversibilityFactsInput struct {
	EpicID     string
	RepoPath   string
	BaseBranch string
	EpicBranch string
	// IrreversibleSurfaces is registry.repos[].irreversibleSurfaces for this
	// repository.
	IrreversibleSurfaces []string
	FixupsSpent          int
	// Budget is orchestrator.fixupBudget. Zero or less falls back to
	// config.DefaultFixupBudget rather than meaning "no budget": a budget of
	// none is an unbounded loop, and this struct is built by hand in enough
	// places (tests, and any caller that did not load a config through
	// config.Load) that an unset field must not be the one that removes the
	// hard stop.
	Budget int
	// Inspector defaults to GitBranchInspector{} when nil, the same way every
	// other git seam in this package defaults to its real implementation.
	Inspector BranchInspector
}

// GatherReversibilityFacts measures the branch.
//
// Every git failure is returned, never swallowed into a zero value: a facts
// struct that says "not published, nothing irreversible touched" because git
// could not be reached is the one lie this gate cannot afford, since both
// fields read as permission to continue without a human.
func GatherReversibilityFacts(in GatherReversibilityFactsInput) (ReversibilityFacts, error) {
	inspector := in.Inspector
	if inspector == nil {
		inspector = GitBranchInspector{}
	}

	budget := in.Budget
	if budget <= 0 {
		budget = config.DefaultFixupBudget
	}
	facts := ReversibilityFacts{EpicID: in.EpicID, FixupsSpent: in.FixupsSpent, Budget: budget}

	refs, err := inspector.PublishedRefs(in.RepoPath, in.EpicBranch)
	if err != nil {
		return ReversibilityFacts{}, fmt.Errorf("cannot tell whether branch %s has been published: %w", in.EpicBranch, err)
	}
	if len(refs) > 0 {
		facts.Published = true
		facts.PublishedDetail = strings.Join(refs, ", ")
	}

	changed, err := inspector.ChangedPaths(in.RepoPath, in.BaseBranch, in.EpicBranch)
	if err != nil {
		return ReversibilityFacts{}, fmt.Errorf("cannot tell what branch %s changed: %w", in.EpicBranch, err)
	}
	facts.IrreversibleSurfacesTouched = matchIrreversibleSurfaces(changed, in.IrreversibleSurfaces)

	summary, err := inspector.ChangeSummary(in.RepoPath, in.BaseBranch, in.EpicBranch)
	if err != nil {
		return ReversibilityFacts{}, fmt.Errorf("cannot summarize what branch %s changed: %w", in.EpicBranch, err)
	}
	facts.ChangeSummary = summary

	return facts, nil
}

// matchIrreversibleSurfaces returns the changed paths a declared surface
// claims, in the order they were changed.
//
// Two spellings, both because a repository declares these by hand: a pattern
// ending in "/" claims that whole subtree, and anything else is matched with
// path.Match against the repository-relative path. A malformed pattern matches
// nothing rather than failing the run - it is configuration, and the cost of
// refusing a whole run over a stray bracket is higher than the cost of a
// surface that quietly does not match, which the escalation reason makes
// visible the first time it should have fired.
func matchIrreversibleSurfaces(changed, patterns []string) []string {
	if len(patterns) == 0 || len(changed) == 0 {
		return nil
	}
	touched := make([]string, 0, len(changed))
	for _, p := range changed {
		for _, pattern := range patterns {
			if matchesSurface(p, strings.TrimSpace(pattern)) {
				touched = append(touched, p)
				break
			}
		}
	}
	if len(touched) == 0 {
		return nil
	}
	return touched
}

func matchesSurface(changedPath, pattern string) bool {
	if pattern == "" {
		return false
	}
	if strings.HasSuffix(pattern, "/") {
		return strings.HasPrefix(changedPath, pattern)
	}
	ok, err := path.Match(pattern, changedPath)
	return err == nil && ok
}
