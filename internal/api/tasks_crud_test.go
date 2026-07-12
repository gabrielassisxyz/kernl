package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func listTasksViaAPI(t *testing.T, r http.Handler) []taskDTO {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/tasks", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list tasks: expected 200, got %d", w.Code)
	}
	var out []taskDTO
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func patchTaskViaAPI(t *testing.T, r http.Handler, id, body string, wantCode int) {
	t.Helper()
	req := httptest.NewRequest("PATCH", "/api/tasks/"+id, bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != wantCode {
		t.Fatalf("patch task %s: expected %d, got %d: %s", body, wantCode, w.Code, w.Body.String())
	}
}

func TestTaskTagsRoundTrip(t *testing.T) {
	a, _ := newCompanionTestApp(t)
	r := NewRouter(a)

	body, _ := json.Marshal(map[string]any{
		"title": "Swap the NAS disks",
		"tags":  []string{"homelab", "hardware"},
	})
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

	tasks := listTasksViaAPI(t, r)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if got := sortedTags(tasks[0].Tags); !slices.Equal(got, []string{"hardware", "homelab"}) {
		t.Errorf("tags = %v, want [hardware homelab]", got)
	}

	// A status-only patch must leave the tags alone: an omitted tags key means
	// "unchanged", never "clear".
	patchTaskViaAPI(t, r, created.ID, `{"status":"done"}`, http.StatusNoContent)
	tasks = listTasksViaAPI(t, r)
	if tasks[0].Status != "done" {
		t.Errorf("status = %q, want done", tasks[0].Status)
	}
	if len(tasks[0].Tags) != 2 {
		t.Errorf("status patch wiped tags: %v", tasks[0].Tags)
	}

	// A tags-only patch replaces the set and leaves the status alone.
	patchTaskViaAPI(t, r, created.ID, `{"tags":["storage"]}`, http.StatusNoContent)
	tasks = listTasksViaAPI(t, r)
	if !slices.Equal(tasks[0].Tags, []string{"storage"}) {
		t.Errorf("tags = %v, want [storage]", tasks[0].Tags)
	}
	if tasks[0].Status != "done" {
		t.Errorf("tags patch clobbered status: %q", tasks[0].Status)
	}

	// An explicit empty list is the way to clear every tag.
	patchTaskViaAPI(t, r, created.ID, `{"tags":[]}`, http.StatusNoContent)
	tasks = listTasksViaAPI(t, r)
	if len(tasks[0].Tags) != 0 {
		t.Errorf("tags = %v, want empty after explicit clear", tasks[0].Tags)
	}

	// An empty patch has nothing to do; a missing task is a 404.
	patchTaskViaAPI(t, r, created.ID, `{}`, http.StatusBadRequest)
	patchTaskViaAPI(t, r, "nope", `{"tags":["x"]}`, http.StatusNotFound)
}

func sortedTags(in []string) []string {
	out := slices.Clone(in)
	slices.Sort(out)
	return out
}
