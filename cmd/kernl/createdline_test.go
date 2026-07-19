package main

import (
	"regexp"
	"strings"
	"testing"
)

// createdShape pins the one format every creation confirmation follows: a
// quoted name before an id in parentheses at the end of the line.
var createdShape = regexp.MustCompile(`^[A-Za-z ]+ "[^"]*"( .+)? \([^()]+\)$`)

func TestCreatedLineLeadsWithTheNameAndEndsWithTheID(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{
			name: "task",
			got:  createdLine("Created task", "renew the domain", "", "01912f3e-7c4a-7b3d-9f21-4e8a1c2b5d70"),
			want: `Created task "renew the domain" (01912f3e-7c4a-7b3d-9f21-4e8a1c2b5d70)`,
		},
		{
			name: "project",
			got:  createdLine("Created project", "Rebuild the backups", "", "prj-1"),
			want: `Created project "Rebuild the backups" (prj-1)`,
		},
		{
			name: "memory claim",
			got:  createdLine("Added claim", "coffee after 4pm ruins my sleep", `under "habits"`, "clm-1"),
			want: `Added claim "coffee after 4pm ruins my sleep" under "habits" (clm-1)`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("got  %s\nwant %s", tc.got, tc.want)
			}
			if !createdShape.MatchString(tc.got) {
				t.Errorf("does not match the agreed shape: %s", tc.got)
			}
		})
	}
}

// One line, so grep and awk keep working on the output. Structured consumers
// use --json, which these verbs leave untouched.
func TestCreatedLineStaysOnOneLine(t *testing.T) {
	line := createdLine("Created task", "a title\nwith a newline", "", "id-1")
	if strings.Count(line, "\n") != 0 {
		t.Errorf("a title with a newline must not break the line: %q", line)
	}
}
