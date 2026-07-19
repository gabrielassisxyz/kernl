package main

import (
	"io"
	"strings"
	"testing"
)

// R2-001: a malformed invocation must fail with its real usage error at exit 2,
// even when no kernl.yaml is reachable. Before the fix these verbs built the API
// client (which loads kernl.yaml) in the dispatcher, BEFORE the subcommand
// validated its arguments, so from a directory with no config every usage error
// surfaced as a config-load error at exit 1. The client is now resolved lazily,
// on the first request, so validation runs first.
//
// The verbContext points configPath at a path that does not exist and sets no
// server/port, so any premature client resolution would fail with a config
// error at exit 1 — which is exactly the regression this pins.
func TestUsageErrorsBeatMissingConfig(t *testing.T) {
	const noConfig = "/nonexistent/definitely-not-here/kernl.yaml"

	cases := []struct {
		name string
		run  func(verbContext, []string) error
		args []string
	}{
		{"project create (no title)", runProject, []string{"create"}},
		{"project set (no id)", runProject, []string{"set", "--status", "done"}},
		{"note read (no path)", runNote, []string{"read"}},
		{"inbox process (unknown flag)", runInbox, []string{"process", "--nope", "x"}},
		{"ingest upload (no file)", runIngest, []string{"upload"}},
		{"settings set (bad section)", runSettings, []string{"set", "bogus"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := verbContext{configPath: noConfig, out: io.Discard}
			err := tc.run(v, tc.args)
			if err == nil {
				t.Fatalf("expected a usage error, got nil")
			}
			if code := exitCode(err); code != 2 {
				t.Errorf("want exit 2 (usage), got %d — config load masked the usage error: %v", code, err)
			}
			if strings.Contains(err.Error(), "reading config") {
				t.Errorf("the config-load error leaked instead of the usage error: %v", err)
			}
		})
	}
}
