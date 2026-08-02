package adapter

import (
	"strings"
	"testing"
)

// The prompt sits right after -p, before --tools/--model. claude's --tools
// is variadic (`claude --help`: `--tools <tools...>`), so this order is not
// cosmetic: with the prompt trailing --tools, claude reads it as one more
// tool name and refuses to run at all - measured, see BuildAnswerModeArgs's
// own doc comment. An earlier version of this test asserted the OLD,
// BROKEN order as the expected literal and would have passed the shipped
// defect; TestBuildAnswerModeArgs_ClaudePromptPrecedesTools below is what
// actually guards the ordering property now.
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

// This is the regression test for the shipped defect: a literal
// slice-equality assertion (TestBuildAnswerModeArgs_Claude above) also
// passes if someone reorders the args back to prompt-trailing AND updates
// the literal to match - a test that only pins one exact slice cannot tell
// "reordered on purpose" from "reordered back into the trap and the
// expectation was updated right along with it". This test instead checks
// the ORDERING PROPERTY directly: whatever else changes, the prompt's index
// must stay lower than --tools' index for claude, because --tools is
// variadic and a prompt at or after it is consumed as another tool name
// (see this package's own answer_args.go doc comment, measured against
// claude 2.1.220).
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
		t.Errorf("Args = %q, want the prompt (index %d) before --tools (index %d) - claude's --tools is variadic and swallows a trailing prompt as another tool name", got.Args, promptIdx, toolsIdx)
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

// --- BuildConsultModeArgs: the mirror image - tools stay ON, restricted to
// read-only ones, because the DA is worthless unless it can reach what the
// operator wrote down. ---

func TestBuildConsultModeArgs_Claude(t *testing.T) {
	got, err := BuildConsultModeArgs(AgentTarget{Command: "claude", Model: "opus"}, "which option should win")
	if err != nil {
		t.Fatalf("BuildConsultModeArgs: %v", err)
	}
	want := []string{"-p", "which option should win", "--tools", "Read,Grep,Glob", "--model", "opus"}
	if strings.Join(got.Args, "\x00") != strings.Join(want, "\x00") {
		t.Errorf("Args = %q, want %q", got.Args, want)
	}
}

// The same ordering property as TestBuildAnswerModeArgs_ClaudePromptPrecedesTools,
// checked here too rather than assumed from the sibling builder: claude's
// --tools is variadic regardless of which function built the argv, so both
// builders have to keep the prompt ahead of it.
func TestBuildConsultModeArgs_ClaudePromptPrecedesTools(t *testing.T) {
	got, err := BuildConsultModeArgs(AgentTarget{Command: "claude", Model: "opus"}, "a distinctive prompt marker")
	if err != nil {
		t.Fatalf("BuildConsultModeArgs: %v", err)
	}
	promptIdx := indexOf(got.Args, "a distinctive prompt marker")
	toolsIdx := indexOf(got.Args, "--tools")
	if promptIdx == -1 || toolsIdx == -1 {
		t.Fatalf("Args = %q, want both the prompt and --tools present", got.Args)
	}
	if promptIdx >= toolsIdx {
		t.Errorf("Args = %q, want the prompt (index %d) before --tools (index %d) - claude's --tools is variadic and swallows a trailing prompt as another tool name", got.Args, promptIdx, toolsIdx)
	}
}

func TestBuildConsultModeArgs_Pi(t *testing.T) {
	got, err := BuildConsultModeArgs(AgentTarget{Command: "pi", Model: "litellm/kimi-k2.7"}, "which option should win")
	if err != nil {
		t.Fatalf("BuildConsultModeArgs: %v", err)
	}
	want := []string{"-p", "--tools", "read,grep,find,ls", "--model", "litellm/kimi-k2.7", "which option should win"}
	if strings.Join(got.Args, "\x00") != strings.Join(want, "\x00") {
		t.Errorf("Args = %q, want %q", got.Args, want)
	}
}

func TestBuildConsultModeArgs_PromptSurvivesWithoutAModel(t *testing.T) {
	got, err := BuildConsultModeArgs(AgentTarget{Command: "claude"}, "which option should win")
	if err != nil {
		t.Fatalf("BuildConsultModeArgs: %v", err)
	}
	if !strings.Contains(strings.Join(got.Args, " "), "which option should win") {
		t.Errorf("Args = %q, want the prompt present", got.Args)
	}
}

// Tools stay on for the DA, but never every tool: a dialect that cannot be
// restricted to read-only access is refused, the same way BuildAnswerModeArgs
// refuses a dialect it cannot silence completely.
func TestBuildConsultModeArgs_RefusesDialectsThatCannotBeRestrictedToReadOnly(t *testing.T) {
	for _, command := range []string{"codex", "agy", "gemini", "copilot", "opencode", "some-other-cli"} {
		if _, err := BuildConsultModeArgs(AgentTarget{Command: command}, "q"); err == nil {
			t.Errorf("BuildConsultModeArgs(%q) returned no error, want a refusal", command)
		} else if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
			t.Errorf("%s: error %q lacks the dispatch-failure marker", command, err)
		} else if !strings.Contains(err.Error(), "da.agent") {
			t.Errorf("%s: error %q should name da.agent, not llm.agent", command, err)
		}
	}
}

// Asking the DA to read is not asking it to write. No dialect's argv may
// carry a permission-bypass flag, in any form.
func TestBuildConsultModeArgs_NeverBypassesPermissions(t *testing.T) {
	for _, command := range []string{"claude", "pi"} {
		got, err := BuildConsultModeArgs(AgentTarget{Command: command, Model: "m"}, "q")
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
