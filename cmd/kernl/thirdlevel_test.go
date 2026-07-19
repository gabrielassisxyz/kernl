package main

import (
	"strings"
	"testing"
)

// thirdLevel is the set of commands that live two levels under a verb. They
// were plain string slices, invisible to capabilities and to help, until they
// were modelled as commandMeta.Subs.
var thirdLevel = [][]string{
	{"ingest", "queue", "list"},
	{"ingest", "queue", "resolve"},
	{"ingest", "queue", "merge-plan"},
	{"inbox", "batch", "analyze"},
	{"inbox", "batch", "preview"},
	{"inbox", "batch", "apply"},
}

func TestThirdLevelCommandsAreInTheCommandTree(t *testing.T) {
	for _, path := range thirdLevel {
		cmd := findCommand(commandTable, path[0])
		if cmd == nil {
			t.Fatalf("no verb %q", path[0])
		}
		for _, name := range path[1:] {
			cmd = findCommand(cmd.Subs, name)
			if cmd == nil {
				t.Fatalf("%s is not in the command tree", strings.Join(path, " "))
			}
		}
		if cmd.Usage == "" || cmd.Summary == "" {
			t.Errorf("%s has no usage or summary to expose", strings.Join(path, " "))
		}
	}
}

// capabilities is generated from commandTable, so being in the tree is what
// puts a command in the machine surface. This pins the depth the walk reaches.
func TestCapabilitiesExposesThirdLevelCommands(t *testing.T) {
	caps := capabilityCommands(commandTable)
	for _, path := range thirdLevel {
		cur := caps
		var found *capabilityCommand
		for _, name := range path {
			found = nil
			for i := range cur {
				if cur[i].Name == name {
					found = &cur[i]
					break
				}
			}
			if found == nil {
				t.Fatalf("capabilities is missing %s", strings.Join(path, " "))
			}
			cur = found.Subcommands
		}
	}
}

// Help used to descend exactly one level, so `ingest queue resolve --help`
// answered with the queue topic — the parent's page for a question about the
// child, with nothing signalling the substitution.
func TestHelpResolvesTheFullTopicPath(t *testing.T) {
	for _, path := range thirdLevel {
		topic, ok := helpTopic(append(append([]string{}, path...), "--help"))
		if !ok {
			t.Fatalf("%v --help was not recognised as a help request", path)
		}
		if len(topic) != len(path) {
			t.Errorf("topic truncated for %v: got %v", path, topic)
		}
	}
}

// A verb with no sub-verbs must still treat trailing words as the caller's own
// text, not as a topic path that could fail to resolve.
func TestHelpUnderALeafVerbIgnoresTrailingWords(t *testing.T) {
	if err := printHelpFor([]string{"capture", "quick", "note"}); err != nil {
		t.Fatalf("trailing words under a leaf verb must not error: %v", err)
	}
}

// Dispatch reads its subcommand list from the same Subs the tree exposes, so
// the two cannot disagree about which subcommands exist.
func TestDispatchListsMatchTheCommandTree(t *testing.T) {
	queue := findCommand(ingestCommand.Subs, "queue")
	if got, want := strings.Join(ingestQueueSubcommands, ","), strings.Join(subNames(queue), ","); got != want {
		t.Errorf("ingest queue dispatch list %q != tree %q", got, want)
	}
	if got, want := strings.Join(inboxBatchSubcommands, ","), strings.Join(subNames(&inboxBatchCommand), ","); got != want {
		t.Errorf("inbox batch dispatch list %q != tree %q", got, want)
	}
}
