package adapter

import (
	"strings"
	"testing"
)

func TestBuildAnswerModeArgs_Claude(t *testing.T) {
	got, err := BuildAnswerModeArgs(AgentTarget{Command: "claude", Model: "opus"}, "why does this matter")
	if err != nil {
		t.Fatalf("BuildAnswerModeArgs: %v", err)
	}
	want := []string{"-p", "why does this matter", "--model", "opus"}
	if strings.Join(got.Args, "\x00") != strings.Join(want, "\x00") {
		t.Errorf("Args = %q, want %q", got.Args, want)
	}
}

func TestBuildAnswerModeArgs_Pi(t *testing.T) {
	got, err := BuildAnswerModeArgs(AgentTarget{Command: "pi", Model: "litellm/kimi-k2.7"}, "why does this matter")
	if err != nil {
		t.Fatalf("BuildAnswerModeArgs: %v", err)
	}
	want := []string{"-p", "--no-tools", "--model", "litellm/kimi-k2.7", "why does this matter"}
	if strings.Join(got.Args, "\x00") != strings.Join(want, "\x00") {
		t.Errorf("Args = %q, want %q", got.Args, want)
	}
}

// Asking for a paragraph is not work that needs write access. A stage
// dispatch grants it; this must not, in any dialect.
func TestBuildAnswerModeArgs_NeverBypassesPermissions(t *testing.T) {
	for _, command := range []string{"claude", "pi"} {
		got, err := BuildAnswerModeArgs(AgentTarget{Command: command, Model: "m"}, "q")
		if err != nil {
			t.Fatalf("%s: %v", command, err)
		}
		joined := strings.Join(got.Args, " ")
		for _, forbidden := range []string{"--dangerously-skip-permissions", "--dangerously-bypass-approvals-and-sandbox", "--allow-all"} {
			if strings.Contains(joined, forbidden) {
				t.Errorf("%s args %q contain %s", command, got.Args, forbidden)
			}
		}
	}
}

// codex frames its answer and refuses to run outside a trusted git directory;
// agy offers no way to run without tools. Both are refusals with a reason,
// not silent fallbacks to claude's argv.
func TestBuildAnswerModeArgs_RefusesDialectsThatCannotAnswerPlainly(t *testing.T) {
	for _, command := range []string{"codex", "agy", "gemini", "copilot", "opencode", "some-other-cli"} {
		if _, err := BuildAnswerModeArgs(AgentTarget{Command: command}, "q"); err == nil {
			t.Errorf("BuildAnswerModeArgs(%q) returned no error, want a refusal", command)
		} else if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
			t.Errorf("%s: error %q lacks the dispatch-failure marker", command, err)
		}
	}
}

func TestBuildAnswerModeArgs_PromptSurvivesWithoutAModel(t *testing.T) {
	got, err := BuildAnswerModeArgs(AgentTarget{Command: "claude"}, "why does this matter")
	if err != nil {
		t.Fatalf("BuildAnswerModeArgs: %v", err)
	}
	if !strings.Contains(strings.Join(got.Args, " "), "why does this matter") {
		t.Errorf("Args = %q, want the prompt present", got.Args)
	}
}
