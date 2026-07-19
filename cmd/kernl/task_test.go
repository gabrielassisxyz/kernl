package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// taskCall is one request the fake API saw, so a test can assert the wire
// contract (method, path, body) instead of only the printed output.
type taskCall struct {
	method string
	path   string
	query  string
	body   map[string]any
}

// fakeTaskAPI stands in for a running `kernl serve`: it records what the verb
// sent and replies with a canned status and body.
type fakeTaskAPI struct {
	t      *testing.T
	status int
	body   string
	calls  []taskCall
}

func (f *fakeTaskAPI) start() *httptest.Server {
	f.t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := taskCall{method: r.Method, path: r.URL.Path, query: r.URL.RawQuery}
		if raw, _ := io.ReadAll(r.Body); len(bytes.TrimSpace(raw)) > 0 {
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
	}))
	f.t.Cleanup(srv.Close)
	return srv
}

// runTaskVerb drives a task subcommand end-to-end against the fake API.
func runTaskVerb(t *testing.T, api *fakeTaskAPI, args ...string) (string, error) {
	t.Helper()
	srv := api.start()
	var out bytes.Buffer
	err := runTask(verbContext{server: srv.URL, out: &out}, args)
	return out.String(), err
}

const taskListBody = `[{"id":"tsk-1","title":"renew the domain","description":"",
  "status":"todo","projectId":"prj-1","tags":["ops"],"dueDate":"2026-08-01",
  "createdAt":"2026-07-01T10:00:00Z","updatedAt":"2026-07-01T10:00:00Z"}]`

func TestTaskListPrintsReadableSummary(t *testing.T) {
	api := &fakeTaskAPI{t: t, status: http.StatusOK, body: taskListBody}
	out, err := runTaskVerb(t, api, "list")
	if err != nil {
		t.Fatalf("task list failed: %v", err)
	}
	for _, want := range []string{"tsk-1", "todo", "renew the domain", "due 2026-08-01", "#ops", "1 task(s)"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q, got:\n%s", want, out)
		}
	}
	if len(api.calls) != 1 || api.calls[0].method != http.MethodGet || api.calls[0].path != "/api/tasks" {
		t.Fatalf("unexpected calls: %+v", api.calls)
	}
}

func TestTaskListScopesToProjectAndPassesJSONThrough(t *testing.T) {
	api := &fakeTaskAPI{t: t, status: http.StatusOK, body: taskListBody}
	out, err := runTaskVerb(t, api, "list", "--project", "prj-1", "--json")
	if err != nil {
		t.Fatalf("task list --json failed: %v", err)
	}
	if api.calls[0].query != "project=prj-1" {
		t.Errorf("--project must scope the query, got %q", api.calls[0].query)
	}
	// --json is the API's own camelCase body, not a re-rendered one.
	var decoded []map[string]any
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("--json output is not JSON: %v\n%s", err, out)
	}
	if decoded[0]["projectId"] != "prj-1" {
		t.Errorf("camelCase field lost in --json output: %v", decoded[0])
	}
}

func TestTaskCreateSendsCamelCaseBody(t *testing.T) {
	api := &fakeTaskAPI{t: t, status: http.StatusCreated, body: `{"id":"tsk-9"}`}
	out, err := runTaskVerb(t, api, "create", "renew the domain",
		"--project", "prj-1", "--due", "2026-08-01", "--tags", "ops, infra")
	if err != nil {
		t.Fatalf("task create failed: %v", err)
	}
	if !strings.Contains(out, "Created task tsk-9") {
		t.Errorf("create should confirm the new id, got: %s", out)
	}
	want := map[string]any{
		"title":     "renew the domain",
		"projectId": "prj-1",
		"dueDate":   "2026-08-01",
		"tags":      []any{"ops", "infra"},
	}
	if api.calls[0].method != http.MethodPost || api.calls[0].path != "/api/tasks" {
		t.Fatalf("wrong route: %+v", api.calls[0])
	}
	if !reflect.DeepEqual(api.calls[0].body, want) {
		t.Errorf("create body = %#v, want %#v", api.calls[0].body, want)
	}
}

func TestTaskSetPatchesOnlyTheFlagsGiven(t *testing.T) {
	api := &fakeTaskAPI{t: t, status: http.StatusNoContent}
	out, err := runTaskVerb(t, api, "set", "tsk-1", "--status", "done", "--tags", "")
	if err != nil {
		t.Fatalf("task set failed: %v", err)
	}
	if !strings.Contains(out, "Updated task tsk-1") {
		t.Errorf("set should confirm the update, got: %s", out)
	}
	call := api.calls[0]
	if call.method != http.MethodPatch || call.path != "/api/tasks/tsk-1" {
		t.Fatalf("wrong route: %+v", call)
	}
	// An omitted flag must not appear at all: the handler reads a present key
	// as "change this", so sending title would blank an untouched title.
	want := map[string]any{"status": "done", "tags": []any{}}
	if !reflect.DeepEqual(call.body, want) {
		t.Errorf("patch body = %#v, want %#v", call.body, want)
	}
}

func TestTaskSetJSONEmitsAckForEmpty204Body(t *testing.T) {
	api := &fakeTaskAPI{t: t, status: http.StatusNoContent}
	out, err := runTaskVerb(t, api, "set", "tsk-1", "--title", "new title", "--json")
	if err != nil {
		t.Fatalf("task set --json failed: %v", err)
	}
	var ack map[string]any
	if err := json.Unmarshal([]byte(out), &ack); err != nil {
		t.Fatalf("--json on a 204 must still emit JSON, got %q (%v)", out, err)
	}
	if ack["id"] != "tsk-1" || ack["updated"] != true {
		t.Errorf("ack = %v, want id/updated", ack)
	}
}

func TestTaskDeleteWithoutYesIssuesNoRequest(t *testing.T) {
	api := &fakeTaskAPI{t: t, status: http.StatusNoContent}
	out, err := runTaskVerb(t, api, "delete", "tsk-1")
	if err != nil {
		t.Fatalf("unconfirmed delete must succeed as a preview, got: %v", err)
	}
	if len(api.calls) != 0 {
		t.Fatalf("delete without --yes must not touch the server, got %+v", api.calls)
	}
	if !strings.Contains(out, "Would delete task tsk-1") || !strings.Contains(out, "--yes") {
		t.Errorf("preview must name the task and the confirmation flag, got: %s", out)
	}
}

func TestTaskDeleteWithYesIssuesDelete(t *testing.T) {
	api := &fakeTaskAPI{t: t, status: http.StatusNoContent}
	out, err := runTaskVerb(t, api, "delete", "tsk-1", "--yes")
	if err != nil {
		t.Fatalf("task delete --yes failed: %v", err)
	}
	if len(api.calls) != 1 || api.calls[0].method != http.MethodDelete || api.calls[0].path != "/api/tasks/tsk-1" {
		t.Fatalf("unexpected calls: %+v", api.calls)
	}
	if !strings.Contains(out, "Deleted task tsk-1") {
		t.Errorf("delete should confirm, got: %s", out)
	}
}

func TestTaskAPINotFoundIsAUsageError(t *testing.T) {
	api := &fakeTaskAPI{t: t, status: http.StatusNotFound, body: `{"error":"task not found"}`}
	_, err := runTaskVerb(t, api, "set", "tsk-missing", "--status", "done")
	if err == nil {
		t.Fatal("a 404 must not be reported as success")
	}
	if exitCode(err) != 2 {
		t.Errorf("4xx exits 2 (bad invocation), got %d: %v", exitCode(err), err)
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("error must be loud, got: %v", err)
	}
}

func TestTaskAPIServerErrorExitsOne(t *testing.T) {
	api := &fakeTaskAPI{t: t, status: http.StatusInternalServerError, body: `{"error":"boom"}`}
	_, err := runTaskVerb(t, api, "list")
	if err == nil || exitCode(err) != 1 {
		t.Fatalf("5xx must exit 1, got %d: %v", exitCode(err), err)
	}
}

func TestTaskUsageErrorsExitTwoWithoutTouchingTheServer(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no subcommand", nil, "requires a subcommand"},
		{"unknown subcommand", []string{"lst"}, "unknown task subcommand"},
		{"create without title", []string{"create", "--project", "prj-1"}, "requires a title"},
		{"set without id", []string{"set", "--status", "done"}, "requires a task ID"},
		{"set without fields", []string{"set", "tsk-1"}, "at least one field"},
		{"set with two ids", []string{"set", "tsk-1", "tsk-2", "--status", "done"}, "exactly one task ID"},
		{"delete without id", []string{"delete", "--yes"}, "requires a task ID"},
		{"unknown flag", []string{"list", "--projects", "prj-1"}, "unknown flag"},
		{"positional on list", []string{"list", "tsk-1"}, "no positional arguments"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// No server URL at all: a malformed invocation must be diagnosed
			// before anything tries to reach the backend.
			err := runTask(verbContext{server: "http://127.0.0.1:1", out: io.Discard}, tc.args)
			if err == nil {
				t.Fatalf("expected a usage error for %v", tc.args)
			}
			if exitCode(err) != 2 {
				t.Errorf("want exit 2, got %d: %v", exitCode(err), err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error should mention %q, got: %v", tc.want, err)
			}
		})
	}
}
