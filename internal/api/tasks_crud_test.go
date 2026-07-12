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

// The `sys/` namespace belongs to the machine. A user who could author one
// could forge a capture back into the inbox queue by tagging it sys/pending, so
// the API refuses it on the way in rather than filtering it on the way out.
func TestSystemTagsRejectedAtAPI(t *testing.T) {
	a, _ := newCompanionTestApp(t)
	r := NewRouter(a)

	post := func(path string, body map[string]any) int {
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", path, bytes.NewReader(raw))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}

	if code := post("/api/tasks", map[string]any{
		"title": "Sneaky", "tags": []string{"homelab", "sys/pending"},
	}); code != http.StatusBadRequest {
		t.Errorf("create task with sys/ tag: expected 400, got %d", code)
	}
	if code := post("/api/projects", map[string]any{
		"title": "Sneaky", "tags": []string{"sys/audit"},
	}); code != http.StatusBadRequest {
		t.Errorf("create project with sys/ tag: expected 400, got %d", code)
	}

	// And on the way through a patch, where the tag set is replaced wholesale.
	id := createTaskViaAPI(t, r, "Legit")
	patchTaskViaAPI(t, r, id, `{"tags":["sys/triaged"]}`, http.StatusBadRequest)

	// A rejected patch must not have touched the tags it was replacing.
	tasks := listTasksViaAPI(t, r)
	if len(tasks) != 1 || len(tasks[0].Tags) != 1 || tasks[0].Tags[0] != "homelab" {
		t.Errorf("rejected patch disturbed the task's tags: %v", tasks[0].Tags)
	}

	// A subject that merely starts with "sys" is an ordinary user tag.
	if code := post("/api/tasks", map[string]any{
		"title": "Fine", "tags": []string{"sysadmin"},
	}); code != http.StatusCreated {
		t.Errorf("create task tagged \"sysadmin\": expected 201, got %d", code)
	}
}

// A tag name that breaks the `/` nesting convention is a client error: stored
// as-is it would be a tag no tree and no descendant query could ever reach.
func TestAPIRejectsMalformedTags(t *testing.T) {
	a, _ := newCompanionTestApp(t)
	r := NewRouter(a)

	post := func(path string, body map[string]any) int {
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", path, bytes.NewReader(raw))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}

	for _, tag := range []string{"/foo", "foo/", "foo//bar", "   "} {
		if code := post("/api/tasks", map[string]any{
			"title": "Malformed", "tags": []string{tag},
		}); code != http.StatusBadRequest {
			t.Errorf("create task with tag %q: expected 400, got %d", tag, code)
		}
		if code := post("/api/projects", map[string]any{
			"title": "Malformed", "tags": []string{tag},
		}); code != http.StatusBadRequest {
			t.Errorf("create project with tag %q: expected 400, got %d", tag, code)
		}
	}

	id := createTaskViaAPI(t, r, "Legit")
	patchTaskViaAPI(t, r, id, `{"tags":["foo//bar"]}`, http.StatusBadRequest)

	// Nesting itself is legal — that is the whole point of the convention.
	if code := post("/api/tasks", map[string]any{
		"title": "Nested", "tags": []string{"homelab/nas"},
	}); code != http.StatusCreated {
		t.Errorf("create task tagged \"homelab/nas\": expected 201, got %d", code)
	}
}

func createTaskViaAPI(t *testing.T, r http.Handler, title string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"title": title, "tags": []string{"homelab"}})
	req := httptest.NewRequest("POST", "/api/tasks", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create task: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	return resp.ID
}

func sortedTags(in []string) []string {
	out := slices.Clone(in)
	slices.Sort(out)
	return out
}
