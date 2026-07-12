package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func createTaskViaAPI(t *testing.T, r http.Handler, body string, wantCode int) string {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/tasks", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != wantCode {
		t.Fatalf("create task %s: expected %d, got %d: %s", body, wantCode, w.Code, w.Body.String())
	}
	if w.Code != http.StatusCreated {
		return ""
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	return created.ID
}

// The wire form of a due date is a calendar day under a camelCase key, so the
// Vue side can bind it straight to an <input type="date"> without a timezone
// conversion that could shift it a day.
func TestTaskDueDateAPIRoundTrip(t *testing.T) {
	a, _ := newCompanionTestApp(t)
	r := NewRouter(a)

	id := createTaskViaAPI(t, r, `{"title":"Read the plainenglish PDFs","dueDate":"2026-04-02"}`, http.StatusCreated)

	tasks := listTasksViaAPI(t, r)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].DueDate != "2026-04-02" {
		t.Errorf("dueDate = %q, want 2026-04-02", tasks[0].DueDate)
	}

	// The JSON key is camelCase, and a task with no due date reports an empty
	// string rather than a null the client would have to guard against.
	req := httptest.NewRequest("GET", "/api/tasks", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), `"dueDate":"2026-04-02"`) {
		t.Errorf("expected a camelCase dueDate in the payload: %s", w.Body.String())
	}

	// Move it, then clear it.
	patchTaskViaAPI(t, r, id, `{"dueDate":"2026-04-06"}`, http.StatusNoContent)
	if got := listTasksViaAPI(t, r)[0].DueDate; got != "2026-04-06" {
		t.Errorf("after patch: dueDate = %q, want 2026-04-06", got)
	}
	patchTaskViaAPI(t, r, id, `{"dueDate":""}`, http.StatusNoContent)
	if got := listTasksViaAPI(t, r)[0].DueDate; got != "" {
		t.Errorf("after clearing: dueDate = %q, want empty", got)
	}
}

// A due date the API cannot read is a 400, not a task quietly created without
// the deadline the caller asked for.
func TestTaskDueDateRejectsMalformedDates(t *testing.T) {
	a, _ := newCompanionTestApp(t)
	r := NewRouter(a)

	createTaskViaAPI(t, r, `{"title":"Bad date","dueDate":"tomorrow"}`, http.StatusBadRequest)
	createTaskViaAPI(t, r, `{"title":"Bad date","dueDate":"2026-04-02T00:00:00Z"}`, http.StatusBadRequest)

	id := createTaskViaAPI(t, r, `{"title":"Good"}`, http.StatusCreated)
	patchTaskViaAPI(t, r, id, `{"dueDate":"02/04/2026"}`, http.StatusBadRequest)
	patchTaskViaAPI(t, r, id, `{}`, http.StatusBadRequest)
}

// A due-date patch is partial like the others: it must not disturb the fields it
// does not name.
func TestTaskDueDatePatchLeavesTagsAndStatusAlone(t *testing.T) {
	a, _ := newCompanionTestApp(t)
	r := NewRouter(a)

	id := createTaskViaAPI(t, r, `{"title":"Swap the NAS disks","tags":["homelab"],"status":"in_progress"}`, http.StatusCreated)
	patchTaskViaAPI(t, r, id, `{"dueDate":"2026-04-03"}`, http.StatusNoContent)

	task := listTasksViaAPI(t, r)[0]
	if task.DueDate != "2026-04-03" {
		t.Errorf("dueDate = %q, want 2026-04-03", task.DueDate)
	}
	if task.Status != "in_progress" {
		t.Errorf("due-date patch clobbered status: %q", task.Status)
	}
	if len(task.Tags) != 1 || task.Tags[0] != "homelab" {
		t.Errorf("due-date patch clobbered tags: %v", task.Tags)
	}
}
