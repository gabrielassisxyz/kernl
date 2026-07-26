package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// The proportions the filter exists for. A backlog with any history is mostly
// history: the import this was built for carries 258 finished entries against 55
// open ones, and a listing that shows both shows the open work four rows deep in
// a screen of archive. Testing with three tasks would prove the filter runs, not
// that it solves anything.
const (
	openTaskCount = 55
	doneTaskCount = 258
)

// importScaleTaskList renders a task array the way the API does, newest first,
// with the done entries interleaved rather than grouped: a filter that happens
// to work on a sorted body and not on a shuffled one is a filter that works by
// accident.
func importScaleTaskList() string {
	var b strings.Builder
	b.WriteByte('[')
	open, done := 0, 0
	for i := 0; open < openTaskCount || done < doneTaskCount; i++ {
		status, title := "done", fmt.Sprintf("archived decision %d", done)
		if open < openTaskCount && (done >= doneTaskCount || i%5 == 0) {
			status, title = "todo", fmt.Sprintf("open item %d", open)
			open++
		} else {
			done++
		}
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"id":"tsk-%d","title":%q,"description":"context for %s",`+
			`"status":%q,"projectId":"","tags":[],"dueDate":"",`+
			`"createdAt":"2026-07-01T10:00:00Z","updatedAt":"2026-07-02T10:00:00Z"}`,
			i, title, title, status)
	}
	b.WriteByte(']')
	return b.String()
}

func importScaleAPI(t *testing.T) *fakeTaskAPI {
	return &fakeTaskAPI{t: t, status: http.StatusOK, body: importScaleTaskList()}
}

// countTaskLines counts printed task rows, which are the lines carrying an id.
func countTaskLines(out string) int {
	n := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "tsk-") {
			n++
		}
	}
	return n
}

func TestTaskListLeavesDoneOutAtImportScale(t *testing.T) {
	api := importScaleAPI(t)
	out, err := runTaskVerb(t, api, "list")
	if err != nil {
		t.Fatalf("task list failed: %v", err)
	}
	if got := countTaskLines(out); got != openTaskCount {
		t.Errorf("default listing printed %d rows, want the %d open ones", got, openTaskCount)
	}
	if strings.Contains(out, "archived decision") {
		t.Error("a done task reached the default listing")
	}
	// The count line has to admit the omission, or the listing reads as the
	// whole truth about how much work is left.
	wantFooter := fmt.Sprintf("%d task(s), %d done hidden", openTaskCount, doneTaskCount)
	if !strings.Contains(out, wantFooter) {
		t.Errorf("footer must report what was hidden, want %q, got:\n%s", wantFooter, lastLines(out, 3))
	}
	// The server keeps answering with every status: the board and the web list
	// need them in one response, so the narrowing is the CLI's alone.
	if api.calls[0].query != "" {
		t.Errorf("the filter must not become a server query, got %q", api.calls[0].query)
	}
}

func TestTaskListJSONDropsDoneAndKeepsEveryFieldOfWhatRemains(t *testing.T) {
	api := importScaleAPI(t)
	out, err := runTaskVerb(t, api, "list", "--json")
	if err != nil {
		t.Fatalf("task list --json failed: %v", err)
	}
	var decoded []map[string]any
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("--json output is not JSON: %v", err)
	}
	if len(decoded) != openTaskCount {
		t.Errorf("--json returned %d tasks, want %d: the filter applies to both renderings or the two disagree about what a task list is",
			len(decoded), openTaskCount)
	}
	// Dropping entries is the whole change; dropping fields would break the
	// agents that read this document.
	for _, field := range []string{"id", "title", "description", "status", "projectId", "tags", "dueDate", "createdAt", "updatedAt"} {
		if _, ok := decoded[0][field]; !ok {
			t.Errorf("--json lost the %q field, so the element shape changed", field)
		}
	}
}

func TestTaskListAllKeepsEveryStatus(t *testing.T) {
	api := importScaleAPI(t)
	out, err := runTaskVerb(t, api, "list", "--all")
	if err != nil {
		t.Fatalf("task list --all failed: %v", err)
	}
	if got := countTaskLines(out); got != openTaskCount+doneTaskCount {
		t.Errorf("--all printed %d rows, want %d", got, openTaskCount+doneTaskCount)
	}
	if strings.Contains(out, "done hidden") {
		t.Error("--all hides nothing, so it must not claim to")
	}
}

func TestTaskListStatusSelectsExactlyThatStatus(t *testing.T) {
	for _, tc := range []struct {
		status string
		want   int
	}{
		{"done", doneTaskCount},
		{"todo", openTaskCount},
		{"in_progress", 0},
	} {
		t.Run(tc.status, func(t *testing.T) {
			api := importScaleAPI(t)
			out, err := runTaskVerb(t, api, "list", "--status", tc.status)
			if err != nil {
				t.Fatalf("task list --status %s failed: %v", tc.status, err)
			}
			if got := countTaskLines(out); got != tc.want {
				t.Errorf("--status %s printed %d rows, want %d", tc.status, got, tc.want)
			}
		})
	}
}

// The one case where an empty result is not an empty backlog. "No tasks" would
// send the reader off to create work that is already recorded.
func TestTaskListSeparatesNoOpenTasksFromNoTasks(t *testing.T) {
	allDone := &fakeTaskAPI{t: t, status: http.StatusOK, body: `[
		{"id":"tsk-1","title":"shipped","status":"done","projectId":"","tags":[],"dueDate":"",
		 "createdAt":"2026-07-01T10:00:00Z","updatedAt":"2026-07-01T10:00:00Z"}]`}
	out, err := runTaskVerb(t, allDone, "list")
	if err != nil {
		t.Fatalf("task list failed: %v", err)
	}
	if !strings.Contains(out, "No open tasks") || !strings.Contains(out, "--all") {
		t.Errorf("an all-done backlog must say so and point at --all, got: %q", out)
	}

	empty := &fakeTaskAPI{t: t, status: http.StatusOK, body: `[]`}
	out, err = runTaskVerb(t, empty, "list")
	if err != nil {
		t.Fatalf("task list failed: %v", err)
	}
	if !strings.Contains(out, "No tasks.") {
		t.Errorf("an empty backlog keeps the create hint, got: %q", out)
	}
}

func lastLines(out string, n int) string {
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
