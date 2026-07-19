package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// projectCall is what the fake API saw, so a test can assert the CLI issued
// the right request instead of only checking what it printed.
type projectCall struct {
	method string
	path   string
	// escaped is the raw, still-percent-encoded request path, so a test can tell
	// an escaped id from a raw one (r.URL.Path decodes both back to the same
	// string).
	escaped string
	body    map[string]any
}

// fakeProjectAPI stands in for a running `kernl serve`: it records every call
// and answers with canned JSON.
type fakeProjectAPI struct {
	t      *testing.T
	server *httptest.Server
	calls  []projectCall
	status int
	body   string
}

func newFakeProjectAPI(t *testing.T, status int, body string) *fakeProjectAPI {
	t.Helper()
	f := &fakeProjectAPI{t: t, status: status, body: body}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeProjectAPI) serve(w http.ResponseWriter, r *http.Request) {
	call := projectCall{method: r.Method, path: r.URL.Path, escaped: r.URL.EscapedPath()}
	if raw, err := io.ReadAll(r.Body); err == nil && len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &call.body); err != nil {
			f.t.Errorf("request body is not JSON: %s", raw)
		}
	}
	f.calls = append(f.calls, call)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(f.status)
	if f.body != "" {
		_, _ = io.WriteString(w, f.body)
	}
}

// run drives a project verb end-to-end through runProject against the fake.
func (f *fakeProjectAPI) run(args ...string) (string, error) {
	var out bytes.Buffer
	err := runProject(verbContext{server: f.server.URL, out: &out}, args)
	return out.String(), err
}

func (f *fakeProjectAPI) only(t *testing.T) projectCall {
	t.Helper()
	if len(f.calls) != 1 {
		t.Fatalf("want exactly 1 API call, got %d: %+v", len(f.calls), f.calls)
	}
	return f.calls[0]
}

func TestProjectListPrintsSummaryAndJSON(t *testing.T) {
	body := `[{"id":"p1","title":"Homelab","status":"active","taskCount":4,"doneCount":1}]`

	fake := newFakeProjectAPI(t, http.StatusOK, body)
	out, err := fake.run("list")
	if err != nil {
		t.Fatalf("project list: %v", err)
	}
	if call := fake.only(t); call.method != http.MethodGet || call.path != "/api/projects" {
		t.Errorf("want GET /api/projects, got %s %s", call.method, call.path)
	}
	for _, want := range []string{"p1", "Homelab", "active", "1/4"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q, got: %s", want, out)
		}
	}

	jsonFake := newFakeProjectAPI(t, http.StatusOK, body)
	out, err = jsonFake.run("list", "--json")
	if err != nil {
		t.Fatalf("project list --json: %v", err)
	}
	var decoded []map[string]any
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("--json output must be the API body verbatim, got %q: %v", out, err)
	}
	if len(decoded) != 1 || decoded[0]["taskCount"] != float64(4) {
		t.Errorf("--json lost fields: %v", decoded)
	}
}

func TestProjectListEmptyTeachesNextStep(t *testing.T) {
	fake := newFakeProjectAPI(t, http.StatusOK, `[]`)
	out, err := fake.run("list")
	if err != nil {
		t.Fatalf("project list: %v", err)
	}
	if !strings.Contains(out, "kernl project create") {
		t.Errorf("empty list must point at the create verb, got: %s", out)
	}
}

func TestProjectCreatePostsOnlyTheFlagsGiven(t *testing.T) {
	fake := newFakeProjectAPI(t, http.StatusCreated, `{"id":"p9"}`)
	out, err := fake.run("create", "--tags", "home, infra", "Rebuild backups")
	if err != nil {
		t.Fatalf("project create: %v", err)
	}

	call := fake.only(t)
	if call.method != http.MethodPost || call.path != "/api/projects" {
		t.Fatalf("want POST /api/projects, got %s %s", call.method, call.path)
	}
	if call.body["title"] != "Rebuild backups" {
		t.Errorf("title not sent: %v", call.body)
	}
	if _, sent := call.body["status"]; sent {
		t.Errorf("an omitted flag must not be sent at all: %v", call.body)
	}
	tags, ok := call.body["tags"].([]any)
	if !ok || len(tags) != 2 || tags[0] != "home" || tags[1] != "infra" {
		t.Errorf("tags must split and trim, got: %v", call.body["tags"])
	}
	if !strings.Contains(out, "p9") {
		t.Errorf("create must print the new id, got: %s", out)
	}
}

func TestProjectCreateRequiresATitle(t *testing.T) {
	fake := newFakeProjectAPI(t, http.StatusCreated, `{"id":"p9"}`)
	_, err := fake.run("create", "--status", "active")
	if err == nil || !strings.Contains(err.Error(), "requires a title") {
		t.Fatalf("missing title must fail loud, got: %v", err)
	}
	if exitCode(err) != 2 {
		t.Errorf("missing argument is a usage error, got exit %d", exitCode(err))
	}
	if len(fake.calls) != 0 {
		t.Errorf("a rejected invocation must not reach the API: %+v", fake.calls)
	}
}

func TestProjectSetPatchesOnlyTheFieldsGiven(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want map[string]any
	}{
		{"status only", []string{"set", "--status", "done", "p1"}, map[string]any{"status": "done"}},
		{"clearing tags", []string{"set", "--tags", "", "p1"}, map[string]any{"tags": []any{}}},
		{"title and description", []string{"set", "--title", "New", "--description", "", "p1"},
			map[string]any{"title": "New", "description": ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeProjectAPI(t, http.StatusNoContent, "")
			if _, err := fake.run(tc.args...); err != nil {
				t.Fatalf("project set: %v", err)
			}
			call := fake.only(t)
			if call.method != http.MethodPatch || call.path != "/api/projects/p1" {
				t.Fatalf("want PATCH /api/projects/p1, got %s %s", call.method, call.path)
			}
			gotJSON, _ := json.Marshal(call.body)
			wantJSON, _ := json.Marshal(tc.want)
			if string(gotJSON) != string(wantJSON) {
				t.Errorf("patch body = %s, want %s", gotJSON, wantJSON)
			}
		})
	}
}

func TestProjectSetWithNoFieldsIsAUsageError(t *testing.T) {
	fake := newFakeProjectAPI(t, http.StatusNoContent, "")
	_, err := fake.run("set", "p1")
	if err == nil || !strings.Contains(err.Error(), "changes nothing") {
		t.Fatalf("empty patch must fail loud, got: %v", err)
	}
	if exitCode(err) != 2 {
		t.Errorf("want usage error, got exit %d", exitCode(err))
	}
	if len(fake.calls) != 0 {
		t.Errorf("no API call should be made: %+v", fake.calls)
	}
}

func TestProjectSetJSONEmitsAConfirmationForA204(t *testing.T) {
	fake := newFakeProjectAPI(t, http.StatusNoContent, "")
	out, err := fake.run("set", "--json", "--status", "done", "p1")
	if err != nil {
		t.Fatalf("project set --json: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("--json must stay parseable on an empty 204 body, got %q", out)
	}
	if decoded["id"] != "p1" || decoded["updated"] != true {
		t.Errorf("unexpected confirmation object: %v", decoded)
	}
}

func TestProjectDeleteWithYesIssuesTheDelete(t *testing.T) {
	fake := newFakeProjectAPI(t, http.StatusNoContent, "")
	out, err := fake.run("delete", "p1", "--yes")
	if err != nil {
		t.Fatalf("project delete: %v", err)
	}
	call := fake.only(t)
	if call.method != http.MethodDelete || call.path != "/api/projects/p1" {
		t.Fatalf("want DELETE /api/projects/p1, got %s %s", call.method, call.path)
	}
	if !strings.Contains(out, "companion note") {
		t.Errorf("delete output must name the companion note side effect, got: %s", out)
	}
}

func TestProjectDeleteWithoutYesPreviewsAndDeletesNothing(t *testing.T) {
	fake := newFakeProjectAPI(t, http.StatusOK, `[{"id":"p1","title":"Homelab","status":"active"}]`)
	out, err := fake.run("delete", "p1")
	if err != nil {
		t.Fatalf("preview must succeed, got: %v", err)
	}
	for _, call := range fake.calls {
		if call.method == http.MethodDelete {
			t.Fatalf("delete without --yes must not issue a DELETE: %+v", fake.calls)
		}
	}
	if !strings.Contains(out, "Would delete") || !strings.Contains(out, "--yes") {
		t.Errorf("preview must say what would go and how to confirm, got: %s", out)
	}
}

// R2-010: an id with URL-significant characters must be percent-escaped into
// the request path, not concatenated raw — otherwise a '/' or '?' in the id
// rewrites the endpoint the CLI hits (task delete already escapes; project
// delete/set regressed).
func TestProjectDeleteEscapesTheIDInThePath(t *testing.T) {
	fake := newFakeProjectAPI(t, http.StatusNoContent, "")
	if _, err := fake.run("delete", "a/b", "--yes"); err != nil {
		t.Fatalf("project delete: %v", err)
	}
	call := fake.only(t)
	if call.escaped != "/api/projects/a%2Fb" {
		t.Fatalf("id must be percent-escaped in the path, got %q", call.escaped)
	}
}

func TestProjectSetEscapesTheIDInThePath(t *testing.T) {
	fake := newFakeProjectAPI(t, http.StatusNoContent, "")
	if _, err := fake.run("set", "--status", "done", "a/b"); err != nil {
		t.Fatalf("project set: %v", err)
	}
	call := fake.only(t)
	if call.escaped != "/api/projects/a%2Fb" {
		t.Fatalf("id must be percent-escaped in the path, got %q", call.escaped)
	}
}

func TestProjectAPIErrorsMapToExitCodes(t *testing.T) {
	notFound := newFakeProjectAPI(t, http.StatusNotFound, `{"error":"project not found"}`)
	_, err := notFound.run("set", "--status", "done", "ghost")
	if err == nil || exitCode(err) != 2 {
		t.Fatalf("a 404 must exit 2, got exit %d from %v", exitCode(err), err)
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("API errors must carry the marker, got: %v", err)
	}

	broken := newFakeProjectAPI(t, http.StatusInternalServerError, `{"error":"boom"}`)
	if _, err := broken.run("list"); err == nil || exitCode(err) != 1 {
		t.Fatalf("a 500 must exit 1, got exit %d from %v", exitCode(err), err)
	}
}

func TestProjectUnknownSubcommandHints(t *testing.T) {
	err := runProject(verbContext{server: "http://127.0.0.1:1", out: io.Discard}, []string{"lst"})
	if err == nil || !strings.Contains(err.Error(), `did you mean "list"?`) {
		t.Fatalf("typo'd subcommand must hint, got: %v", err)
	}
	if exitCode(err) != 2 {
		t.Errorf("want usage error, got exit %d", exitCode(err))
	}
}
