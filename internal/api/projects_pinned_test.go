package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func patchProjectViaAPI(t *testing.T, r http.Handler, id, body string, wantCode int) {
	t.Helper()
	req := httptest.NewRequest("PATCH", "/api/projects/"+id, bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != wantCode {
		t.Fatalf("patch project %s: expected %d, got %d: %s", body, wantCode, w.Code, w.Body.String())
	}
}

func findProject(t *testing.T, projects []projectDTO, id string) projectDTO {
	t.Helper()
	for _, p := range projects {
		if p.ID == id {
			return p
		}
	}
	t.Fatalf("project %s not in the listing", id)
	return projectDTO{}
}

func TestProjectPinnedDefaultsToFalse(t *testing.T) {
	a, _ := newCompanionTestApp(t)
	r := NewRouter(a)

	id := createProjectViaAPI(t, r, "Rewire the study")
	if findProject(t, listProjectsViaAPI(t, r), id).Pinned {
		t.Fatal("a project nobody pinned came back pinned")
	}
}

func TestProjectPinnedRoundTrips(t *testing.T) {
	a, _ := newCompanionTestApp(t)
	r := NewRouter(a)

	id := createProjectViaAPI(t, r, "Rewire the study")
	patchProjectViaAPI(t, r, id, `{"pinned":true}`, http.StatusNoContent)
	if !findProject(t, listProjectsViaAPI(t, r), id).Pinned {
		t.Fatal("pinned did not survive the round trip")
	}

	patchProjectViaAPI(t, r, id, `{"pinned":false}`, http.StatusNoContent)
	if findProject(t, listProjectsViaAPI(t, r), id).Pinned {
		t.Fatal("unpinning left the project pinned")
	}
}

// Pinning writes one attr key with json_set while tags go through the node
// chokepoint's read-modify-write. Either one dropping the other's field is the
// failure this guards, and it would only show up after both had been used.
func TestProjectPinnedSurvivesUnrelatedEdits(t *testing.T) {
	a, _ := newCompanionTestApp(t)
	r := NewRouter(a)

	id := createProjectViaAPI(t, r, "Rewire the study")
	patchProjectViaAPI(t, r, id, `{"pinned":true}`, http.StatusNoContent)
	patchProjectViaAPI(t, r, id, `{"tags":["home","electrical"]}`, http.StatusNoContent)
	patchProjectViaAPI(t, r, id, `{"status":"paused"}`, http.StatusNoContent)
	patchProjectViaAPI(t, r, id, `{"title":"Rewire the whole study"}`, http.StatusNoContent)

	got := findProject(t, listProjectsViaAPI(t, r), id)
	if !got.Pinned {
		t.Error("an unrelated edit cleared pinned")
	}
	if got.Status != "paused" {
		t.Errorf("status = %q, want paused", got.Status)
	}
	if len(got.Tags) != 2 {
		t.Errorf("tags = %v, want two", got.Tags)
	}
}

func TestProjectPinnedAtCreate(t *testing.T) {
	a, _ := newCompanionTestApp(t)
	r := NewRouter(a)

	body, _ := json.Marshal(map[string]any{"title": "Rewire the study", "pinned": true})
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
	if !findProject(t, listProjectsViaAPI(t, r), created.ID).Pinned {
		t.Fatal("a project created pinned came back unpinned")
	}
}

// An empty patch body has to stay an error now that a fourth field can carry it.
func TestProjectPatchStillRejectsAnEmptyBody(t *testing.T) {
	a, _ := newCompanionTestApp(t)
	r := NewRouter(a)

	id := createProjectViaAPI(t, r, "Rewire the study")
	patchProjectViaAPI(t, r, id, `{}`, http.StatusBadRequest)
}
