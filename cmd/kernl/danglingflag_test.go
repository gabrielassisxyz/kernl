package main

import (
	"strings"
	"testing"
)

// A dangling flag value used to send every verb to root `kernl --help`, which
// lists the verbs and none of their flags — so it could not answer the question
// the caller had just got wrong. The hint names the owning verb now.
func TestTakeFlagDanglingValuePointsAtTheOwningVerb(t *testing.T) {
	_, _, _, err := takeFlag("task list", []string{"--project"}, "--project")
	if exitCode(err) != 2 {
		t.Fatalf("a dangling flag must exit 2, got %d: %v", exitCode(err), err)
	}
	if !strings.Contains(err.Error(), "run: kernl task list --help") {
		t.Errorf("hint must name the owning verb, got: %v", err)
	}
	if strings.Contains(err.Error(), "run: kernl --help") {
		t.Errorf("hint must not fall back to root help, got: %v", err)
	}
}

// Every verb path passed to takeFlag has to be one `kernl <path> --help`
// actually resolves, or the recovery hint sends the caller nowhere. This walks
// the whole command tree and checks each hint against it.
func TestEveryDanglingFlagHintNamesAResolvableHelpPath(t *testing.T) {
	known := map[string]bool{}
	var walk func(prefix string, cmds []commandMeta)
	walk = func(prefix string, cmds []commandMeta) {
		for _, c := range cmds {
			path := strings.TrimSpace(prefix + " " + c.Name)
			known[path] = true
			walk(path, c.Subs)
		}
	}
	walk("", commandTable)

	// The verb strings the code hands to takeFlag, one per owning subcommand.
	for _, verb := range []string{
		"approval resolve", "bead close", "bead set", "bead create",
		"bead refine-scope", "epic events", "graph search", "graph related",
		"inbox process", "inbox batch", "ingest paste", "ingest source",
		"ingest trigger", "memory claims", "memory add-claim", "memory refute",
		"note write", "note suggest", "note apply-hunks", "project create",
		"project set", "session nudge", "settings set", "task list",
		"task create", "task set",
	} {
		if !known[verb] {
			t.Errorf("takeFlag points at %q, which is not a command in the tree", verb)
		}
	}
}
