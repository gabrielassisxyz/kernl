package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/vault/frontmatter"
	"github.com/gabrielassisxyz/kernl/internal/vault/layout"
	"github.com/gabrielassisxyz/kernl/internal/vault/reconcile"
)

func deleteTaskViaAPI(t *testing.T, r http.Handler, id string, wantCode int) {
	t.Helper()
	req := httptest.NewRequest("DELETE", "/api/tasks/"+id, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != wantCode {
		t.Fatalf("delete task %s: expected %d, got %d: %s", id, wantCode, w.Code, w.Body.String())
	}
}

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

func TestTaskTitleRoundTrip(t *testing.T) {
	a, _ := newCompanionTestApp(t)
	r := NewRouter(a)
	id := createTaskViaAPI(t, r, `{"title":"Old title"}`, http.StatusCreated)

	// A title-only patch must be accepted - it must NOT hit the all-nil guard.
	patchTaskViaAPI(t, r, id, `{"title":"New title"}`, http.StatusNoContent)
	tasks := listTasksViaAPI(t, r)
	if len(tasks) != 1 || tasks[0].Title != "New title" {
		t.Fatalf("title = %q, want \"New title\"", tasks[0].Title)
	}

	// A blank title is rejected - dropping the title is not a legitimate edit.
	patchTaskViaAPI(t, r, id, `{"title":"   "}`, http.StatusBadRequest)

	// A retitle of a missing task is a 404.
	patchTaskViaAPI(t, r, "nope", `{"title":"x"}`, http.StatusNotFound)
}

func TestTaskCompanionMetadataFollowsCreateAndPatch(t *testing.T) {
	a, vault := newCompanionTestApp(t)
	r := NewRouter(a)
	id := createTaskViaAPI(t, r, `{"title":"Alpha","tags":["urgent","later"]}`, http.StatusCreated)
	relPath := companionPath(t, a, id)
	fullPath := filepath.Join(vault, filepath.FromSlash(relPath))

	created, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("read created companion: %v", err)
	}
	fm, err := frontmatter.Parse(created)
	if err != nil {
		t.Fatalf("parse created companion: %v", err)
	}
	if fm.Title != "Alpha" || !slices.Equal(fm.Tags, []string{"task", "urgent", "later"}) {
		t.Fatalf("created frontmatter = title %q tags %v", fm.Title, fm.Tags)
	}

	custom := []byte("---\n# discarded on managed rewrite\nid: " + fm.ID + "\ntitle: Hand-edited title\ncustom: discarded\ntags:\n  - task\n  - manual\n---\nBody stays byte-for-byte.\n")
	if err := os.WriteFile(fullPath, custom, 0o644); err != nil {
		t.Fatalf("write custom companion: %v", err)
	}

	patchTaskViaAPI(t, r, id, `{"title":"Beta: renamed","description":"Updated","tags":["blocked"]}`, http.StatusNoContent)
	updated, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("read updated companion: %v", err)
	}
	want := []byte("---\nid: " + fm.ID + "\ntitle: 'Beta: renamed'\ndescription: Updated\ntags:\n    - task\n    - blocked\n---\nBody stays byte-for-byte.\n")
	if !bytes.Equal(updated, want) {
		t.Fatalf("companion was not canonically re-rendered:\ngot:  %q\nwant: %q", updated, want)
	}

	var storedHash string
	var describesCount int
	if err := a.Graph.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		if err := tx.QueryRow(`SELECT content_hash FROM note_paths WHERE uuid = ?`, fm.ID).Scan(&storedHash); err != nil {
			return err
		}
		return tx.QueryRow(`SELECT COUNT(*) FROM edges WHERE dst = ? AND label = 'describes'`, id).Scan(&describesCount)
	}); err != nil {
		t.Fatalf("read companion graph state: %v", err)
	}
	if storedHash != reconcile.HashBytes(updated) {
		t.Errorf("stored hash %q does not match rewritten bytes", storedHash)
	}
	if describesCount != 1 {
		t.Errorf("describes edges = %d, want 1", describesCount)
	}
	if got := companionPath(t, a, id); got != relPath {
		t.Errorf("rename moved companion from %q to %q", relPath, got)
	}
}

func TestTaskCompanionTagsDeduplicateTaskMarker(t *testing.T) {
	a, vault := newCompanionTestApp(t)
	r := NewRouter(a)
	id := createTaskViaAPI(t, r, `{"title":"Tagged","tags":["task","task","#urgent"," urgent ","   ","#urgent"]}`, http.StatusCreated)
	data, err := os.ReadFile(filepath.Join(vault, filepath.FromSlash(companionPath(t, a, id))))
	if err != nil {
		t.Fatal(err)
	}
	fm, err := frontmatter.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"task", "#urgent", " urgent ", "   "}
	if !slices.Equal(fm.Tags, want) {
		t.Fatalf("companion tags = %q, want %q", fm.Tags, want)
	}
	tasks := listTasksViaAPI(t, r)
	if !slices.Equal(sortedTags(tasks[0].Tags), sortedTags(want)) {
		t.Fatalf("task tags %q differ from companion tags %q", tasks[0].Tags, fm.Tags)
	}
}

func TestTaskTagsOnlyPatchRewritesCompanion(t *testing.T) {
	a, vault := newCompanionTestApp(t)
	r := NewRouter(a)
	id := createTaskViaAPI(t, r, `{"title":"Alpha","tags":["urgent"]}`, http.StatusCreated)
	fullPath := filepath.Join(vault, filepath.FromSlash(companionPath(t, a, id)))

	// A tags-only PATCH is the bead's own reproduction, and the combined PATCH
	// covered elsewhere satisfies the sync condition through its title alone, so
	// without this the tags arm of that condition guards nothing.
	patchTaskViaAPI(t, r, id, `{"tags":["blocked"]}`, http.StatusNoContent)
	updated, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("read updated companion: %v", err)
	}
	fm, err := frontmatter.Parse(updated)
	if err != nil {
		t.Fatalf("parse updated companion: %v", err)
	}
	if !slices.Equal(fm.Tags, []string{"task", "blocked"}) {
		t.Fatalf("companion tags = %v, want [task blocked]", fm.Tags)
	}
	if fm.Title != "Alpha" {
		t.Errorf("tags-only patch changed title to %q", fm.Title)
	}
	var storedHash string
	if err := a.Graph.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT content_hash FROM note_paths WHERE uuid = ?`, fm.ID).Scan(&storedHash)
	}); err != nil {
		t.Fatalf("read companion hash: %v", err)
	}
	if storedHash != reconcile.HashBytes(updated) {
		t.Errorf("stored hash %q does not match rewritten bytes", storedHash)
	}
}

func TestTaskRenameUsesDescribesEdgeWithDuplicateTitles(t *testing.T) {
	a, vault := newCompanionTestApp(t)
	r := NewRouter(a)
	first := createTaskViaAPI(t, r, `{"title":"Same title"}`, http.StatusCreated)
	second := createTaskViaAPI(t, r, `{"title":"Same title"}`, http.StatusCreated)
	firstPath := companionPath(t, a, first)
	secondPath := companionPath(t, a, second)
	firstBefore, err := os.ReadFile(filepath.Join(vault, filepath.FromSlash(firstPath)))
	if err != nil {
		t.Fatal(err)
	}

	patchTaskViaAPI(t, r, second, `{"title":"Only second changes"}`, http.StatusNoContent)
	firstAfter, err := os.ReadFile(filepath.Join(vault, filepath.FromSlash(firstPath)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstAfter, firstBefore) {
		t.Fatal("renaming second task rewrote first task companion")
	}
	secondBytes, err := os.ReadFile(filepath.Join(vault, filepath.FromSlash(secondPath)))
	if err != nil {
		t.Fatal(err)
	}
	fm, err := frontmatter.Parse(secondBytes)
	if err != nil || fm.Title != "Only second changes" {
		t.Fatalf("second title = %q, parse error = %v", fm.Title, err)
	}
}

func TestDeleteTaskRemovesCompanion(t *testing.T) {
	a, vault := newCompanionTestApp(t)
	r := NewRouter(a)
	ctx := context.Background()
	id := createTaskViaAPI(t, r, `{"title":"Doomed Task"}`, http.StatusCreated)

	companionFile := filepath.Join(vault, filepath.FromSlash(layout.TasksFolder), "doomed-task.md")
	if _, err := os.Stat(companionFile); err != nil {
		t.Fatalf("companion file should exist before delete: %v", err)
	}

	deleteTaskViaAPI(t, r, id, http.StatusNoContent)

	if got := listTasksViaAPI(t, r); len(got) != 0 {
		t.Errorf("expected 0 tasks after delete, got %d", len(got))
	}
	if _, err := os.Stat(companionFile); !os.IsNotExist(err) {
		t.Error("companion file should be removed with the task")
	}
	var liveNotes int
	_ = a.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT COUNT(*) FROM nodes WHERE type = 'note' AND deleted_at IS NULL`).Scan(&liveNotes)
	})
	if liveNotes != 0 {
		t.Errorf("companion note node should be deleted, %d live notes remain", liveNotes)
	}

	// Deleting again → 404.
	deleteTaskViaAPI(t, r, id, http.StatusNotFound)
}

func sortedTags(in []string) []string {
	out := slices.Clone(in)
	slices.Sort(out)
	return out
}
