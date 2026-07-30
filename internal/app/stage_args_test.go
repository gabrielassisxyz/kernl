package app

import (
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/adapter"
)

func joined(args []string) string { return strings.Join(args, " ") }

func contains(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

// The regression this whole file exists for: every agent used to be dispatched
// with opencode's argument shape, so claude and codex died on unknown flags
// before ever reading their prompt.
func TestBuildStageArgs_ClaudeGetsNoOpencodeFlags(t *testing.T) {
	cmd, args := BuildStageArgs(
		adapter.AgentTarget{Command: "claude", Model: "opus"},
		nil, "arch-c9k", "/wt", "", "do the thing")

	if cmd != "claude" {
		t.Fatalf("command = %q, want claude", cmd)
	}
	for _, forbidden := range []string{"--dir", "--title", "-s"} {
		if contains(args, forbidden) {
			t.Errorf("claude argv carries opencode flag %s: %s", forbidden, joined(args))
		}
	}
	if !contains(args, "-p") || !contains(args, "do the thing") {
		t.Errorf("claude argv lost its prompt: %s", joined(args))
	}
	if !contains(args, "--model") || !contains(args, "opus") {
		t.Errorf("claude argv lost its model: %s", joined(args))
	}
}

func TestBuildStageArgs_CodexUsesExecAndItsOwnModelFlag(t *testing.T) {
	_, args := BuildStageArgs(
		adapter.AgentTarget{Command: "codex", Model: "gpt-5-codex"},
		nil, "arch-c9k", "/wt", "", "review the diff")

	if len(args) < 2 || args[0] != "exec" || args[1] != "review the diff" {
		t.Fatalf("codex argv must start with `exec <prompt>`: %s", joined(args))
	}
	// codex takes -m; --model is an opencode-ism and would abort the run.
	if contains(args, "--model") {
		t.Errorf("codex argv carries --model instead of -m: %s", joined(args))
	}
	if !contains(args, "-m") || !contains(args, "gpt-5-codex") {
		t.Errorf("codex argv lost its model: %s", joined(args))
	}
}

// opencode still needs --dir: it resolves the project by git discovery from the
// launch directory, not from the process working directory.
func TestBuildStageArgs_OpencodeKeepsStageFlagsAndTrailingPrompt(t *testing.T) {
	_, args := BuildStageArgs(
		adapter.AgentTarget{Command: "opencode", Model: "litellm/kimi"},
		nil, "arch-c9k", "/wt", "ses_1", "implement it")

	if got := args[len(args)-1]; got != "implement it" {
		t.Fatalf("prompt must stay the trailing positional, got %q in %s", got, joined(args))
	}
	for _, want := range []string{"--dir", "/wt", "-s", "ses_1", "--title", "kernl:arch-c9k"} {
		if !contains(args, want) {
			t.Errorf("opencode argv missing %q: %s", want, joined(args))
		}
	}
}

// Operator flags from settings.agents.<id>.args must never displace the prompt.
func TestBuildStageArgs_ExtraArgsNeverDisplaceThePrompt(t *testing.T) {
	_, oc := BuildStageArgs(
		adapter.AgentTarget{Command: "opencode"},
		[]string{"--agent", "build"}, "b", "/wt", "", "the prompt")
	if got := oc[len(oc)-1]; got != "the prompt" {
		t.Errorf("opencode: extra args displaced the prompt: %s", joined(oc))
	}
	if !contains(oc, "--agent") {
		t.Errorf("opencode: extra args dropped: %s", joined(oc))
	}

	_, cx := BuildStageArgs(
		adapter.AgentTarget{Command: "codex"},
		[]string{"--cd", "/wt"}, "b", "/wt", "", "the prompt")
	if cx[1] != "the prompt" {
		t.Errorf("codex: extra args displaced the prompt: %s", joined(cx))
	}
	if !contains(cx, "--cd") {
		t.Errorf("codex: extra args dropped: %s", joined(cx))
	}
}
