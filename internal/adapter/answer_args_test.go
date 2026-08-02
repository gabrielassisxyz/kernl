package adapter

import (
	"strings"
	"testing"
)

// This asserts the whole argv against a literal, which is exactly why it did
// NOT catch the ordering defect it now pins: a literal-equality test passes
// whichever order is written into its own want, so it agreed with the broken
// argv for as long as that argv shipped. TestBuildAnswerModeArgs_ClaudePromptPrecedesTools
// below is what actually guards the property.
func TestBuildAnswerModeArgs_Claude(t *testing.T) {
	got, err := BuildAnswerModeArgs(AgentTarget{Command: "claude", Model: "opus"}, "why does this matter")
	if err != nil {
		t.Fatalf("BuildAnswerModeArgs: %v", err)
	}
	want := []string{"-p", "why does this matter", "--tools", "", "--model", "opus"}
	if strings.Join(got.Args, "\x00") != strings.Join(want, "\x00") {
		t.Errorf("Args = %q, want %q", got.Args, want)
	}
}

// claude's --tools is variadic, so a prompt trailing it is swallowed as one
// more tool name and the CLI answers nothing at all. The property that keeps
// the answer reaching the model is positional, so it is the position this
// checks, not the exact slice.
func TestBuildAnswerModeArgs_ClaudePromptPrecedesTools(t *testing.T) {
	got, err := BuildAnswerModeArgs(AgentTarget{Command: "claude", Model: "opus"}, "a distinctive prompt marker")
	if err != nil {
		t.Fatalf("BuildAnswerModeArgs: %v", err)
	}
	promptIdx := indexOf(got.Args, "a distinctive prompt marker")
	toolsIdx := indexOf(got.Args, "--tools")
	if promptIdx == -1 || toolsIdx == -1 {
		t.Fatalf("Args = %q, want both the prompt and --tools present", got.Args)
	}
	if promptIdx >= toolsIdx {
		t.Errorf("Args = %q, want the prompt (index %d) before --tools (index %d)", got.Args, promptIdx, toolsIdx)
	}
}

func indexOf(args []string, want string) int {
	for i, a := range args {
		if a == want {
			return i
		}
	}
	return -1
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
