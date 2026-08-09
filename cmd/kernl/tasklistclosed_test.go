package main

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// A called-off task is not outstanding work, so the default listing owes it the
// same treatment it gives a finished one: out of the way, and said out loud.
const closedTaskCount = 7

func mixedTerminalTaskList() string {
	var b strings.Builder
	b.WriteByte('[')
	for i := 0; i < 4+closedTaskCount; i++ {
		status, title := "todo", fmt.Sprintf("open item %d", i)
		if i >= 4 {
			status, title = "closed", fmt.Sprintf("called off %d", i-4)
		}
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"id":"tsk-%d","title":%q,"description":"",`+
			`"status":%q,"projectId":"","tags":[],"dueDate":"",`+
			`"createdAt":"2026-07-01T10:00:00Z","updatedAt":"2026-07-02T10:00:00Z"}`,
			i, title, status)
	}
	b.WriteByte(']')
	return b.String()
}

func mixedTerminalAPI(t *testing.T) *fakeTaskAPI {
	return &fakeTaskAPI{t: t, status: http.StatusOK, body: mixedTerminalTaskList()}
}

func TestTaskListLeavesCalledOffWorkOut(t *testing.T) {
	out, err := runTaskVerb(t, mixedTerminalAPI(t), "list")
	if err != nil {
		t.Fatalf("task list failed: %v", err)
	}
	if got := countTaskLines(out); got != 4 {
		t.Errorf("default listing printed %d rows, want the 4 open ones", got)
	}
	if strings.Contains(out, "called off") {
		t.Error("a closed task reached the default listing")
	}
	// The footer names both terminal states, because saying "done hidden" over
	// a pile of abandoned work would be a count nobody could reconcile.
	if !strings.Contains(out, fmt.Sprintf("%d done or closed hidden", closedTaskCount)) {
		t.Errorf("footer must report what was hidden, got:\n%s", lastLines(out, 3))
	}
}

func TestTaskListStatusClosedSelectsExactlyThose(t *testing.T) {
	out, err := runTaskVerb(t, mixedTerminalAPI(t), "list", "--status", "closed")
	if err != nil {
		t.Fatalf("task list --status closed failed: %v", err)
	}
	if got := countTaskLines(out); got != closedTaskCount {
		t.Errorf("--status closed printed %d rows, want %d", got, closedTaskCount)
	}
	if strings.Contains(out, "open item") {
		t.Error("--status closed listed a task that is not closed")
	}
}

func TestTaskListAllKeepsCalledOffWork(t *testing.T) {
	out, err := runTaskVerb(t, mixedTerminalAPI(t), "list", "--all")
	if err != nil {
		t.Fatalf("task list --all failed: %v", err)
	}
	if got := countTaskLines(out); got != 4+closedTaskCount {
		t.Errorf("--all printed %d rows, want %d", got, 4+closedTaskCount)
	}
}
