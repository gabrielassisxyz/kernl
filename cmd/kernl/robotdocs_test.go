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

func TestRobotDocsUnknownTopicHinted(t *testing.T) {
	err := runRobotDocs(&bytes.Buffer{}, []string{"guid"})
	if err == nil || !strings.Contains(err.Error(), `did you mean "guide"?`) {
		t.Fatalf("expected hint for guid, got: %v", err)
	}
}
