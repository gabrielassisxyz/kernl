package api

import (
	"fmt"
	"net/http"
	"testing"
)

// A closed task is work that was abandoned, not work that was finished and not
// work still outstanding. Counting it either way misreports the project: as
// done it inflates progress with something nobody did, and as outstanding it
// leaves a project that has nothing left to do permanently short of complete.
func TestClosedTasksLeaveTheProjectProgressAltogether(t *testing.T) {
	a, _ := newCompanionTestApp(t)
	r := NewRouter(a)

	pid := createProjectViaAPI(t, r, "Rewire the study")
	mk := func(title, status string) string {
		return createTaskViaAPI(t, r,
			fmt.Sprintf(`{"title":%q,"projectId":%q,"status":%q}`, title, pid, status),
			http.StatusCreated)
	}
	mk("Measure the alcove", "done")
	mk("Order the shelves", "todo")
	closed := mk("Rent a router", "todo")
	patchTaskViaAPI(t, r, closed, `{"status":"closed"}`, http.StatusNoContent)

	p := findProject(t, listProjectsViaAPI(t, r), pid)
	if p.TaskCount != 2 {
		t.Fatalf("task count: want 2 (the closed one is out), got %d", p.TaskCount)
	}
	if p.DoneCount != 1 {
		t.Fatalf("done count: want 1, got %d", p.DoneCount)
	}
}

// The consequence that makes the rule worth having: a project whose only
// remaining work was called off reads as finished, not as stuck at 50%.
func TestAProjectWhoseRestWasCalledOffReadsAsComplete(t *testing.T) {
	a, _ := newCompanionTestApp(t)
	r := NewRouter(a)

	pid := createProjectViaAPI(t, r, "Rewire the study")
	done := createTaskViaAPI(t, r, fmt.Sprintf(`{"title":"Measure","projectId":%q,"status":"done"}`, pid), http.StatusCreated)
	abandoned := createTaskViaAPI(t, r, fmt.Sprintf(`{"title":"Rent a router","projectId":%q}`, pid), http.StatusCreated)
	_ = done
	patchTaskViaAPI(t, r, abandoned, `{"status":"closed"}`, http.StatusNoContent)

	p := findProject(t, listProjectsViaAPI(t, r), pid)
	if p.DoneCount != p.TaskCount || p.TaskCount != 1 {
		t.Fatalf("want 1/1, got %d/%d", p.DoneCount, p.TaskCount)
	}
}

// Creating with a project id that names nothing left the task pointing at a
// project no screen can resolve. PATCH refused that from the moment it could
// move a task between projects; create accepted it, so the same bad value was
// rejected or stored depending only on which verb reached the server.
func TestCreatingATaskRefusesAProjectIdThatNamesNothing(t *testing.T) {
	a, _ := newCompanionTestApp(t)
	r := NewRouter(a)

	createTaskViaAPI(t, r, `{"title":"Rent a router","projectId":"019f0000-0000-7000-8000-000000000000"}`,
		http.StatusBadRequest)

	if tasks := listTasksViaAPI(t, r); len(tasks) != 0 {
		t.Fatalf("the refused task was stored anyway: %d task(s) exist", len(tasks))
	}
}

func TestCreatingATaskStillAcceptsNoProjectAtAll(t *testing.T) {
	a, _ := newCompanionTestApp(t)
	r := NewRouter(a)

	createTaskViaAPI(t, r, `{"title":"Rent a router"}`, http.StatusCreated)
	if tasks := listTasksViaAPI(t, r); len(tasks) != 1 {
		t.Fatalf("an unassigned task must still be creatable, got %d", len(tasks))
	}
}
