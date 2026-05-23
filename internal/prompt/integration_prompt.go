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
}

// IntegrationInput feeds the integration prompt template.
type IntegrationInput struct {
	EpicID     string
	EpicTitle  string
	EpicBranch string
	BaseBranch string
	Children   []Child
}

const integrationTemplate = `You are the kernl integration agent for epic {{.EpicID}}: "{{.EpicTitle}}".

Your job: merge each child branch into the epic branch in topological order, resolve any conflicts, verify the combined tree builds, and finish with a marker commit. Do NOT push and do NOT create a PR — that is the separate shipment stage.

Inputs:
- epic_branch: {{.EpicBranch}}
- base_branch: {{.BaseBranch}}
- children (ordered):{{range .Children}}
  - {{.ID}}: branch={{.Branch}}, worktree={{.WorktreePath}}
{{end}}
Procedure:
1. Work in the epic worktree. The epic branch {{.EpicBranch}} is already checked out there — do not re-checkout or switch branches.
2. For each child in the listed order:
   a. git merge --no-ff {{"{{.Branch}}"}}
   b. On conflict: read the conflict markers; resolve safely using the full context of the epic and child descriptions, then git add the resolved files and git commit. If you cannot converge within your follow-up budget, write
      "merge_outcome: merge_conflict" and "merge_conflict_at: {{"{{.Branch}}"}}" to the epic bead description (via bd update) and STOP.
3. After all merges succeed: verify the combined tree builds with "go build ./...". Run quick tests with "go test ./..." if applicable.
4. Finish with a marker commit whose message contains EXACTLY the literal "stage: integration", for example:
   git commit --allow-empty -m "stage: integration: merged N child branches into {{.EpicBranch}}"
   This marker commit is REQUIRED — an exit gate checks for it.
5. Do NOT push to origin and do NOT open a pull request — the separate shipment stage handles pushing and the PR.

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
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: missing branches in IntegrationInput — EpicBranch=%q BaseBranch=%q — Fix: populate both branches", in.EpicBranch, in.BaseBranch)
	}
	var buf bytes.Buffer
	if err := integrationTmpl.Execute(&buf, integrationView{IntegrationInput: in, Outcomes: merge.All()}); err != nil {
		return "", err
	}
	return buf.String(), nil
}
