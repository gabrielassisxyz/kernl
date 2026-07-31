package prompt

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/gabrielassisxyz/kernl/internal/merge"
)

// Child identifies a single bead branch that must be merged into its epic.
type Child struct {
	ID           string
	Branch       string
	WorktreePath string
	// ArtifactDir is where this child's own exit-gate artifacts (plan.md,
	// review verdicts) were written - outside its worktree, under the same
	// run root as the epic's own artifact directory. Without it here, the
	// integration agent has no path by which to read a child's artifacts:
	// before artifacts moved outside the worktree they arrived for free
	// when a child branch was merged; now they never travel in a commit,
	// so the reader needs the directory named explicitly.
	ArtifactDir string
}

// IntegrationInput feeds the integration prompt template.
type IntegrationInput struct {
	EpicID     string
	EpicTitle  string
	EpicBranch string
	BaseBranch string
	Children   []Child
	// VerifyCommand is how the target repository says "this works". The step
	// used to be `go build ./...`, which is a fact about kernl and means
	// nothing in a repository that is not written in Go.
	VerifyCommand string
	// TrackerCommand is how this repository's tracker is typed from inside a
	// worktree. The literal "bd" was a fact about kernl too.
	TrackerCommand string
}

const integrationTemplate = `You are the kernl integration agent for epic {{.EpicID}}: "{{.EpicTitle}}".

Your job: merge each child branch into the epic branch in topological order, resolve any conflicts, and verify the combined tree passes this repository's own check. Do NOT push and do NOT create a PR - that is the separate shipment stage.

Inputs:
- epic_branch: {{.EpicBranch}}
- base_branch: {{.BaseBranch}}
- children (ordered):{{range .Children}}
  - {{.ID}}: branch={{.Branch}}, worktree={{.WorktreePath}}, artifacts={{.ArtifactDir}}
{{end}}
Procedure:
1. Work in the epic worktree. The epic branch {{.EpicBranch}} is already checked out there - do not re-checkout or switch branches.
2. For each child in the listed order:
   a. git merge --no-ff {{"{{.Branch}}"}}
   b. On conflict: read the conflict markers; resolve safely using the full context of the epic and child descriptions, then git add the resolved files and git commit. If you cannot converge within your follow-up budget, write
      "merge_outcome: merge_conflict" and "merge_conflict_at: {{"{{.Branch}}"}}" to the epic bead description (via {{.TrackerCommand}} update) and STOP.
   Resolving a conflict means writing code, and this repository's conventions are the ones that apply to it: read AGENTS.md in your worktree (or CONTRIBUTING.md, or the README) before you write any. Nothing about how any other project works applies here. A child's own plan and review artifacts, if you need the context behind its changes, are at the artifacts path listed for it above - not inside its worktree or branch.
3. After all merges succeed, verify the combined tree with this repository's own check:
   {{.VerifyCommand}}
   If it fails, the merge is not done: fix the combined tree and re-run it. Whatever this command checks is what "done" means here - do not substitute a check of your own.
4. Do NOT push to origin and do NOT open a pull request - the separate shipment stage handles pushing and the PR.

The only merge_outcome the integration stage may write is "merge_conflict". For reference, the full merge_outcome enum is:{{range .Outcomes}}
  - {{.}}
{{end}}`

type integrationView struct {
	IntegrationInput
	Outcomes []merge.Outcome
}

var integrationTmpl = template.Must(template.New("integration").Parse(integrationTemplate))

// RenderIntegration renders the integration-stage prompt for a given epic and its children.
func RenderIntegration(in IntegrationInput) (string, error) {
	if in.EpicBranch == "" || in.BaseBranch == "" {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: missing branches in IntegrationInput - EpicBranch=%q BaseBranch=%q - Fix: populate both branches", in.EpicBranch, in.BaseBranch)
	}
	if in.VerifyCommand == "" {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: integration prompt has no verify command, so the merged tree would be handed on unchecked - Fix: resolve it with epic.ResolveVerifyCommand before dispatch")
	}
	if in.TrackerCommand == "" {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: integration prompt has no tracker command, so a conflict would be reported by running nothing - Fix: resolve it with backend.TrackerInvocation before dispatch")
	}
	var buf bytes.Buffer
	if err := integrationTmpl.Execute(&buf, integrationView{IntegrationInput: in, Outcomes: merge.All()}); err != nil {
		return "", err
	}
	return buf.String(), nil
}
