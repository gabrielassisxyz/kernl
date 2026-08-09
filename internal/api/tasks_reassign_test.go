package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/graph"
)

func createTaskInProject(t *testing.T, r http.Handler, title, projectID string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"title": title, "projectId": projectID})
	req := httptest.NewRequest("POST", "/api/tasks", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create task: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	return created.ID
}

// partOfTargets reads the edges, which are the canonical side of the
// relationship, rather than the projectId attr the DTO reports. The whole risk
// in reassignment is that the two stop agreeing.
func partOfTargets(t *testing.T, a *app.App, taskID string) []string {
	t.Helper()
	var out []string
	err := a.Graph.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		rows, err := tx.Query(`SELECT dst FROM edges WHERE src = ? AND label = 'part_of' ORDER BY dst`, taskID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var dst string
			if err := rows.Scan(&dst); err != nil {
				return err
			}
			out = append(out, dst)
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatalf("read part_of edges: %v", err)
	}
	return out
}

func taskByID(t *testing.T, tasks []taskDTO, id string) taskDTO {
	t.Helper()
	for _, task := range tasks {
		if task.ID == id {
			return task
		}
	}
	t.Fatalf("task %s not in the listing", id)
	return taskDTO{}
}

func TestTaskReassignMovesAttrAndEdgeTogether(t *testing.T) {
	a, _ := newCompanionTestApp(t)
	r := NewRouter(a)

	from := createProjectViaAPI(t, r, "homelab")
	to := createProjectViaAPI(t, r, "kernl")
	id := createTaskInProject(t, r, "Swap the NAS disks", from)

	patchTaskViaAPI(t, r, id, `{"projectId":"`+to+`"}`, http.StatusNoContent)

	if got := taskByID(t, listTasksViaAPI(t, r), id).ProjectID; got != to {
		t.Errorf("projectId = %q, want %q", got, to)
	}
	// Exactly one, at the new project: a reassignment that only adds leaves the
	// task a member of both, and the old project keeps counting it.
	if got := partOfTargets(t, a, id); len(got) != 1 || got[0] != to {
		t.Errorf("part_of targets = %v, want exactly [%s]", got, to)
	}
}

func TestTaskReassignFiltersByTheNewProject(t *testing.T) {
	a, _ := newCompanionTestApp(t)
	r := NewRouter(a)

	from := createProjectViaAPI(t, r, "homelab")
	to := createProjectViaAPI(t, r, "kernl")
	id := createTaskInProject(t, r, "Swap the NAS disks", from)
	patchTaskViaAPI(t, r, id, `{"projectId":"`+to+`"}`, http.StatusNoContent)

	req := httptest.NewRequest("GET", "/api/tasks?project="+from, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var stillThere []taskDTO
	if err := json.Unmarshal(w.Body.Bytes(), &stillThere); err != nil {
		t.Fatal(err)
	}
	if len(stillThere) != 0 {
		t.Errorf("the old project still lists %d task(s)", len(stillThere))
	}
}

func TestTaskCanBeUnassigned(t *testing.T) {
	a, _ := newCompanionTestApp(t)
	r := NewRouter(a)

	from := createProjectViaAPI(t, r, "homelab")
	id := createTaskInProject(t, r, "Swap the NAS disks", from)

	patchTaskViaAPI(t, r, id, `{"projectId":""}`, http.StatusNoContent)

	if got := taskByID(t, listTasksViaAPI(t, r), id).ProjectID; got != "" {
		t.Errorf("projectId = %q, want empty", got)
	}
	if got := partOfTargets(t, a, id); len(got) != 0 {
		t.Errorf("part_of targets = %v, want none", got)
	}
}

func TestTaskReassignRejectsAnUnknownProject(t *testing.T) {
	a, _ := newCompanionTestApp(t)
	r := NewRouter(a)

	from := createProjectViaAPI(t, r, "homelab")
	id := createTaskInProject(t, r, "Swap the NAS disks", from)

	patchTaskViaAPI(t, r, id, `{"projectId":"019f0000-0000-7000-8000-000000000000"}`, http.StatusBadRequest)

	// The rejection has to leave the task where it was, edges included.
	if got := taskByID(t, listTasksViaAPI(t, r), id).ProjectID; got != from {
		t.Errorf("projectId = %q after a rejected patch, want %q", got, from)
	}
	if got := partOfTargets(t, a, id); len(got) != 1 || got[0] != from {
		t.Errorf("part_of targets = %v after a rejected patch, want [%s]", got, from)
	}
}

// Reassignment goes through the node chokepoint, which reconciles the tag set
// against the struct it is handed: a task loaded without its tags is a task
// about to lose them.
func TestTaskReassignKeepsTagsAndDueDate(t *testing.T) {
	a, _ := newCompanionTestApp(t)
	r := NewRouter(a)

	from := createProjectViaAPI(t, r, "homelab")
	to := createProjectViaAPI(t, r, "kernl")
	id := createTaskInProject(t, r, "Swap the NAS disks", from)
	patchTaskViaAPI(t, r, id, `{"tags":["hardware","storage"],"dueDate":"2026-09-01"}`, http.StatusNoContent)

	patchTaskViaAPI(t, r, id, `{"projectId":"`+to+`"}`, http.StatusNoContent)

	got := taskByID(t, listTasksViaAPI(t, r), id)
	if len(got.Tags) != 2 {
		t.Errorf("tags = %v, want two", got.Tags)
	}
	if got.DueDate != "2026-09-01" {
		t.Errorf("dueDate = %q, want 2026-09-01", got.DueDate)
	}
}
