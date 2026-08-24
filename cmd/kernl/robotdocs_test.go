package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRobotGuideCoversContractEssentials(t *testing.T) {
	guide := renderRobotGuide()
	for _, needle := range []string{
		"KERNL DISPATCH FAILURE", "capabilities --json", "exit", "--yes",
		"NO_COLOR", "epic list --json", "doctor --json", "KERNL_LOG_LEVEL",
	} {
		if !strings.Contains(guide, needle) {
			t.Errorf("robot guide missing %q", needle)
		}
	}
	// Every top-level verb must appear.
	for _, c := range commandTable {
		if !strings.Contains(guide, c.Name) {
			t.Errorf("robot guide missing verb %q", c.Name)
		}
	}
}

// TestRobotGuideJSONSurfaceMatchesSearchEntry guards the one hand-written line
// in the robot guide's JSON read surface. It names a verb by hand, so a rename
// of the table entry it describes would leave the guide stale with nothing to
// notice - the exact defect this test pins against.
func TestRobotGuideJSONSurfaceMatchesSearchEntry(t *testing.T) {
	cmd := findCommand(commandTable, "search")
	if cmd == nil {
		t.Fatal(`no "search" command in the table`)
	}
	var shape string
	for _, f := range cmd.Flags {
		if f.Name == "--json" {
			shape = f.Description
		}
	}
	if shape == "" {
		t.Fatal(`search entry must declare a --json flag`)
	}

	guide := renderRobotGuide()
	if !strings.Contains(guide, "kernl search --json <topic>") {
		t.Errorf("robot guide JSON surface must name kernl search, got: %q", guide)
	}
	for _, want := range []string{`{"topic","notes":[{"id","title","via","snippet","path"}]}`} {
		if !strings.Contains(shape, want) {
			t.Errorf("search --json flag must document %s, got: %q", want, shape)
		}
		if !strings.Contains(guide, want) {
			t.Errorf("robot guide JSON surface must advertise %s", want)
		}
	}
}

func TestRobotDocsUnknownTopicHinted(t *testing.T) {
	err := runRobotDocs(&bytes.Buffer{}, []string{"guid"})
	if err == nil || !strings.Contains(err.Error(), `did you mean "guide"?`) {
		t.Fatalf("expected hint for guid, got: %v", err)
	}
}
