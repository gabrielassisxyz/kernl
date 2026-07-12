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
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
)

func createProjectViaAPI(t *testing.T, r http.Handler, title string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"title": title})
	req := httptest.NewRequest("POST", "/api/projects", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create project: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	return resp.ID
}

func listProjectsViaAPI(t *testing.T, r http.Handler) []projectDTO {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/projects", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list projects: expected 200, got %d", w.Code)
	}
	var out []projectDTO
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// A companion note must not be tagged with the kind of the entity it describes.
// Tags are the user's subjects — a "project" tag would sit in the tag surface as
// if the user had chosen it, and every companion note would share it. The note's
// kind is already carried by its folder and its describes edge.
//
// The body assertion guards the original UAT finding that produced this test: a
// tag must never be a literal "#project" hashtag appended to the body, which
// never reached the tag index and read as noise.
func TestCompanionNoteCarriesNoMachineTag(t *testing.T) {
	a, vault := newCompanionTestApp(t)
	r := NewRouter(a)

	createProjectViaAPI(t, r, "Frontmatter Tags")

	data, err := os.ReadFile(filepath.Join(vault, "projects", "frontmatter-tags.md"))
	if err != nil {
		t.Fatalf("companion file: %v", err)
	}
	content := string(data)
	fm := strings.SplitN(content, "---\n", 3)
	if len(fm) < 3 {
		t.Fatalf("companion file has no frontmatter:\n%s", content)
	}
	if strings.Contains(fm[1], "- project") {
		t.Errorf("companion frontmatter carries the machine tag \"project\":\n%s", fm[1])
	}
	if strings.Contains(fm[2], "#project") {
		t.Errorf("body still carries a literal #project hashtag:\n%s", fm[2])
	}
}

func TestPatchProjectTitleAndDescription(t *testing.T) {
	a, _ := newCompanionTestApp(t)
	r := NewRouter(a)
	id := createProjectViaAPI(t, r, "Old Title")

	// Description-only patch preserves the title.
	body := []byte(`{"description":"the missing description"}`)
	req := httptest.NewRequest("PATCH", "/api/projects/"+id, bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("patch description: expected 204, got %d: %s", w.Code, w.Body.String())
	}

	// Title patch preserves the description.
	body = []byte(`{"title":"New Title"}`)
	req = httptest.NewRequest("PATCH", "/api/projects/"+id, bytes.NewReader(body))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("patch title: expected 204, got %d: %s", w.Code, w.Body.String())
	}

	projects := listProjectsViaAPI(t, r)
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	if projects[0].Title != "New Title" {
		t.Errorf("title = %q, want New Title", projects[0].Title)
	}
	if projects[0].Description != "the missing description" {
		t.Errorf("description = %q, want preserved", projects[0].Description)
	}

	// Empty patch → 400; empty title → 400.
	for _, payload := range []string{`{}`, `{"title":"  "}`} {
		req = httptest.NewRequest("PATCH", "/api/projects/"+id, strings.NewReader(payload))
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("patch %s: expected 400, got %d", payload, w.Code)
		}
	}
}

func TestProjectTagsRoundTrip(t *testing.T) {
	a, _ := newCompanionTestApp(t)
	r := NewRouter(a)

	body, _ := json.Marshal(map[string]any{
		"title": "Homelab rebuild",
		"tags":  []string{"homelab"},
	})
	req := httptest.NewRequest("POST", "/api/projects", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create project: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	if got := listProjectsViaAPI(t, r); len(got) != 1 || !slices.Equal(got[0].Tags, []string{"homelab"}) {
		t.Fatalf("tags after create = %v, want [homelab]", got)
	}

	patchProject := func(payload string, wantCode int) {
		t.Helper()
		req := httptest.NewRequest("PATCH", "/api/projects/"+created.ID, strings.NewReader(payload))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != wantCode {
			t.Fatalf("patch %s: expected %d, got %d: %s", payload, wantCode, w.Code, w.Body.String())
		}
	}

	// A title-only patch must leave the tags alone.
	patchProject(`{"title":"Homelab rebuild v2"}`, http.StatusNoContent)
	got := listProjectsViaAPI(t, r)
	if got[0].Title != "Homelab rebuild v2" {
		t.Errorf("title = %q, want updated", got[0].Title)
	}
	if !slices.Equal(got[0].Tags, []string{"homelab"}) {
		t.Errorf("title patch wiped tags: %v", got[0].Tags)
	}

	// Tags patched alongside a title: both land, neither clobbers the other.
	patchProject(`{"title":"Homelab","tags":["homelab","infra"]}`, http.StatusNoContent)
	got = listProjectsViaAPI(t, r)
	if got[0].Title != "Homelab" {
		t.Errorf("title = %q, want Homelab", got[0].Title)
	}
	if tags := sortedTags(got[0].Tags); !slices.Equal(tags, []string{"homelab", "infra"}) {
		t.Errorf("tags = %v, want [homelab infra]", tags)
	}

	// An explicit empty list clears every tag.
	patchProject(`{"tags":[]}`, http.StatusNoContent)
	if got := listProjectsViaAPI(t, r); len(got[0].Tags) != 0 {
		t.Errorf("tags = %v, want empty after explicit clear", got[0].Tags)
	}
}

func TestDeleteProjectRemovesCompanion(t *testing.T) {
	a, vault := newCompanionTestApp(t)
	r := NewRouter(a)
	ctx := context.Background()
	id := createProjectViaAPI(t, r, "Doomed Project")

	companionFile := filepath.Join(vault, "projects", "doomed-project.md")
	if _, err := os.Stat(companionFile); err != nil {
		t.Fatalf("companion file should exist before delete: %v", err)
	}

	req := httptest.NewRequest("DELETE", "/api/projects/"+id, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d: %s", w.Code, w.Body.String())
	}

	if got := listProjectsViaAPI(t, r); len(got) != 0 {
		t.Errorf("expected 0 projects after delete, got %d", len(got))
	}
	if _, err := os.Stat(companionFile); !os.IsNotExist(err) {
		t.Error("companion file should be removed with the project")
	}
	var liveNotes int
	_ = a.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT COUNT(*) FROM nodes WHERE type = 'note' AND deleted_at IS NULL`).Scan(&liveNotes)
	})
	if liveNotes != 0 {
		t.Errorf("companion note node should be deleted, %d live notes remain", liveNotes)
	}

	// Deleting again → 404.
	req = httptest.NewRequest("DELETE", "/api/projects/"+id, nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("second delete: expected 404, got %d", w.Code)
	}
}
