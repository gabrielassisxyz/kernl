package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// runTaskVerbSplit is runTaskVerb with the two streams kept apart, which is the
// whole point of the tests below: what stdout carries and what stderr carries
// are different contracts, and a helper that merges them cannot tell them apart.
func runTaskVerbSplit(t *testing.T, api *fakeTaskAPI, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	srv := api.start()
	var out, errOut bytes.Buffer
	err = runTask(verbContext{server: srv.URL, out: &out, errOut: &errOut}, args)
	return out.String(), errOut.String(), err
}

// A filtered listing that matches nothing must not be worded like an empty
// vault. The import-scale body holds 313 tasks and not one in_progress, so the
// old "No tasks. Create one with:" was a false statement to the reader and an
// invitation to create a duplicate to an agent.
func TestTaskListEmptyUnderStatusFilterSaysWhatItFiltered(t *testing.T) {
	api := importScaleAPI(t)
	out, _, err := runTaskVerbSplit(t, api, "list", "--status", "in_progress")
	if err != nil {
		t.Fatalf("task list failed: %v", err)
	}
	if strings.Contains(out, "Create one with") {
		t.Errorf("a filtered miss claimed the vault is empty, got:\n%s", out)
	}
	for _, want := range []string{"in_progress", "313"} {
		if !strings.Contains(out, want) {
			t.Errorf("empty line missing %q, got:\n%s", want, out)
		}
	}
}

// The genuinely empty case keeps its old wording: there is nothing to widen to,
// so pointing at --all would send the reader after tasks that do not exist.
func TestTaskListEmptyWithoutFilterStillOffersCreate(t *testing.T) {
	api := &fakeTaskAPI{t: t, status: http.StatusOK, body: `[]`}
	out, _, err := runTaskVerbSplit(t, api, "list")
	if err != nil {
		t.Fatalf("task list failed: %v", err)
	}
	if !strings.Contains(out, "Create one with") {
		t.Errorf("empty vault should offer create, got:\n%s", out)
	}
}

// --json drops the done entries like the human listing does, so it owes the same
// disclosure. It goes to stderr because stdout has to stay an array another tool
// can parse whole; a machine reading only stdout would otherwise have no way to
// learn that 258 rows were withheld short of re-running with --all.
func TestTaskListJSONReportsHiddenCountOnStderr(t *testing.T) {
	api := importScaleAPI(t)
	out, errOut, err := runTaskVerbSplit(t, api, "list", "--json")
	if err != nil {
		t.Fatalf("task list --json failed: %v", err)
	}

	var got []map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("stdout is not a JSON array: %v\n%s", err, out)
	}
	if len(got) != openTaskCount {
		t.Errorf("stdout carried %d tasks, want the %d open ones", len(got), openTaskCount)
	}
	for _, want := range []string{"258", "--all"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr note missing %q, got: %q", want, errOut)
		}
	}
}

// Nothing hidden, nothing said: the note is a disclosure, not a banner, and a
// caller piping stderr should not have to filter noise out of a complete result.
func TestTaskListJSONStaysSilentWhenNothingIsHidden(t *testing.T) {
	api := importScaleAPI(t)
	_, errOut, err := runTaskVerbSplit(t, api, "list", "--all", "--json")
	if err != nil {
		t.Fatalf("task list --all --json failed: %v", err)
	}
	if errOut != "" {
		t.Errorf("stderr should be empty when no task was hidden, got: %q", errOut)
	}
}
