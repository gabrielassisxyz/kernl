package shipment

import (
	"errors"
	"strings"
	"testing"
)

// recordingPRTextRunner is a named fake standing in for the repository's own
// gate: it records what it was handed and answers with a canned verdict, so
// these tests spawn no process.
type recordingPRTextRunner struct {
	calls    int
	repoPath string
	command  string
	stdin    string
	output   string
	err      error
}

func (r *recordingPRTextRunner) run(repoPath, command, stdin string) (string, error) {
	r.calls++
	r.repoPath, r.command, r.stdin = repoPath, command, stdin
	return r.output, r.err
}

func TestCheckPRText_PassesTheTitleAndBodyToTheRepositorysGate(t *testing.T) {
	runner := &recordingPRTextRunner{}

	err := CheckPRText(PRTextCheckInput{
		RepoPath: "/repos/example",
		Command:  "bin/slop-guard --stdin 'PR text'",
		Title:    "feat(report): cap the report's length",
		Body:     "## Changes\n\n- the anomaly gate now counts sessions",
		Run:      runner.run,
	})
	if err != nil {
		t.Fatalf("a passing gate must publish: %v", err)
	}
	if runner.calls != 1 {
		t.Fatalf("the gate ran %d times, want exactly once", runner.calls)
	}
	if runner.command != "bin/slop-guard --stdin 'PR text'" || runner.repoPath != "/repos/example" {
		t.Errorf("gate invoked as %q in %q, want the declared command in the repository", runner.command, runner.repoPath)
	}
	// The title is prose that gets published too, and it is the first line a
	// reviewer reads - checking only the body would leave it ungated.
	if !strings.Contains(runner.stdin, "feat(report): cap the report's length") {
		t.Errorf("stdin = %q, want the title in it", runner.stdin)
	}
	if !strings.Contains(runner.stdin, "the anomaly gate now counts sessions") {
		t.Errorf("stdin = %q, want the body in it", runner.stdin)
	}
}

func TestCheckPRText_RefusalNamesTheGateAndItsOwnOutput(t *testing.T) {
	runner := &recordingPRTextRunner{
		output: "PR text:16: em-dash in prose\n==> slop-guard FAILED (PR text)",
		err:    errors.New("exit status 1"),
	}

	err := CheckPRText(PRTextCheckInput{
		RepoPath: "/repos/example",
		Command:  "bin/slop-guard --stdin 'PR text'",
		Title:    "a title",
		Body:     "a body",
		Run:      runner.run,
	})
	if err == nil {
		t.Fatal("a refused text must fail loud")
	}
	for _, want := range []string{
		"KERNL DISPATCH FAILURE",
		"bin/slop-guard --stdin 'PR text'",
		"em-dash in prose",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to contain %q - an operator who is not told what the gate said runs it again by hand to find out", err, want)
		}
	}
}

// A gate that refuses and prints nothing still refuses. The command's name is
// then the only thing left to act on, so it must not be swallowed.
func TestCheckPRText_SilentRefusalStillReports(t *testing.T) {
	runner := &recordingPRTextRunner{err: errors.New("exit status 2")}

	err := CheckPRText(PRTextCheckInput{RepoPath: "/repos/example", Command: "bin/check", Run: runner.run})
	if err == nil {
		t.Fatal("a refused text must fail loud even when the gate said nothing")
	}
	if !strings.Contains(err.Error(), "exit status 2") || !strings.Contains(err.Error(), "bin/check") {
		t.Errorf("error = %q, want the command and the failure it produced", err)
	}
}

// A repository that declares no rule about prose declares NOTHING. Reading the
// empty value as a refusal would stop every run that publishes, from a
// repository that never asked for a check at all.
func TestCheckPRText_UnsetCommandPublishesAndRunsNothing(t *testing.T) {
	runner := &recordingPRTextRunner{err: errors.New("must never be called")}

	for _, command := range []string{"", "   "} {
		if err := CheckPRText(PRTextCheckInput{RepoPath: "/repos/example", Command: command, Run: runner.run}); err != nil {
			t.Errorf("command %q: %v", command, err)
		}
	}
	if runner.calls != 0 {
		t.Errorf("the gate ran %d times for a repository that declared none", runner.calls)
	}
}

func TestComposePRText(t *testing.T) {
	got := ComposePRText("  a title  ", "\na body\n")
	if got != "a title\n\na body" {
		t.Errorf("ComposePRText = %q, want the title and body separated by one blank line", got)
	}
}
