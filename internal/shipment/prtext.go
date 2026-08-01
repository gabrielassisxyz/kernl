package shipment

import (
	"fmt"
	"os/exec"
	"strings"
)

// PRTextRunner runs a repository's own text gate with the pull request text on
// stdin and returns whatever it printed. It is a seam for the same reason
// every other command seam in this repository is one: a unit test may not
// spawn a process (AGENTS.md §4), and this decides whether a run publishes.
type PRTextRunner func(repoPath, command, stdin string) (output string, err error)

// ShellPRTextRunner is the production runner.
//
// The command goes through a shell because that is how it is written in
// kernl.yaml and how the operator types it, flags and all - the same posture
// verifyCommand already takes. Output is combined: a gate that names the
// offending line on stderr is the useful half of its answer, and reporting a
// refusal with no reason would leave the operator running it again by hand to
// find out what it said.
func ShellPRTextRunner(repoPath, command, stdin string) (string, error) {
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = repoPath
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// ComposePRText joins a pull request's title and body into the one document
// the gate reads. Both, not just the body: a title is prose that gets
// published too, and it is the line a reviewer sees first.
func ComposePRText(title, body string) string {
	return strings.TrimSpace(title) + "\n\n" + strings.TrimSpace(body)
}

// PRTextCheckInput is one check: whose gate, which command, and the text.
type PRTextCheckInput struct {
	RepoPath string
	// Command is registry.repos[].prTextCommand. Empty means this repository
	// declares no rule about pull request prose, which is a legitimate answer
	// and passes - it never means "refuse".
	Command string
	Title   string
	Body    string
	// Run defaults to ShellPRTextRunner when nil.
	Run PRTextRunner
}

// CheckPRText asks the repository whether it accepts this pull request's text.
//
// It exists because the text a run publishes was never checked against a rule
// the repository already owns and already runs. A pull request was opened with
// prose its own repository refuses, the check failed the moment it appeared,
// and the code in it was fine: nothing in the pipeline had looked at the one
// artifact the shipment stage writes entirely by itself.
func CheckPRText(in PRTextCheckInput) error {
	command := strings.TrimSpace(in.Command)
	if command == "" {
		return nil
	}
	run := in.Run
	if run == nil {
		run = ShellPRTextRunner
	}
	output, err := run(in.RepoPath, command, ComposePRText(in.Title, in.Body))
	if err == nil {
		return nil
	}
	detail := strings.TrimSpace(output)
	if detail == "" {
		// A gate that refuses without saying anything still refuses. Naming
		// the command is then the only thing the operator can act on.
		detail = err.Error()
	}
	return fmt.Errorf("KERNL DISPATCH FAILURE: this pull request's text was refused by %s's own gate (%s):\n%s\n - Fix: rewrite the title and body until that command passes, then update the pull request", in.RepoPath, command, detail)
}
