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

// beadAPIRequest is what the fake API saw, so a test can assert the CLI issued
// the right request instead of only checking what it printed.
type beadAPIRequest struct {
	method string
	path   string
	query  string
	body   map[string]any
}

// fakeBeadAPI stands in for a running `kernl serve`: it records every call and
// answers with canned JSON.
type fakeBeadAPI struct {
	t      *testing.T
	server *httptest.Server
	calls  []beadAPIRequest
	status int
	body   string
}

func newFakeBeadAPI(t *testing.T, status int, body string) *fakeBeadAPI {
	t.Helper()
	f := &fakeBeadAPI{t: t, status: status, body: body}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeBeadAPI) serve(w http.ResponseWriter, r *http.Request) {
	call := beadAPIRequest{method: r.Method, path: r.URL.EscapedPath(), query: r.URL.RawQuery}
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

func (f *fakeBeadAPI) run(args ...string) (string, error) {
	var out bytes.Buffer
	err := runBeadAPI(verbContext{server: f.server.URL, out: &out}, args)
	return out.String(), err
}

func (f *fakeBeadAPI) only(t *testing.T) beadAPIRequest {
	t.Helper()
	if len(f.calls) != 1 {
		t.Fatalf("want exactly 1 API call, got %d: %+v", len(f.calls), f.calls)
	}
	return f.calls[0]
}

func TestBeadAPIListPrintsSummary(t *testing.T) {
	body := `[{"id":"kn-1","type":"task","state":"open","title":"Wire SSE","labels":["cli"],"parentId":"ep-2"}]`
	fake := newFakeBeadAPI(t, http.StatusOK, body)

	out, err := fake.run("list")
	if err != nil {
		t.Fatalf("bead list: %v", err)
	}
	call := fake.only(t)
	if call.method != http.MethodGet || call.path != "/api/beads" || call.query != "" {
		t.Errorf("want GET /api/beads with no query, got %s %s?%s", call.method, call.path, call.query)
	}
	for _, want := range []string{"kn-1", "Wire SSE", "parent ep-2", "#cli", "1 bead(s)"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q:\n%s", want, out)
		}
	}
}

func TestBeadAPIListJSONPassesResponseThrough(t *testing.T) {
	fake := newFakeBeadAPI(t, http.StatusOK, `[{"id":"kn-1","title":"Wire SSE"}]`)

	out, err := fake.run("list", "--json")
	if err != nil {
		t.Fatalf("bead list --json: %v", err)
	}
	var decoded []map[string]any
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("--json output is not JSON: %v\n%s", err, out)
	}
	if decoded[0]["title"] != "Wire SSE" {
		t.Errorf("server body not passed through: %s", out)
	}
}

func TestBeadAPIGetEscapesTheID(t *testing.T) {
	fake := newFakeBeadAPI(t, http.StatusOK, `{"id":"kn 1/x","title":"Odd","type":"task","state":"open"}`)

	out, err := fake.run("get", "kn 1/x")
	if err != nil {
		t.Fatalf("bead get: %v", err)
	}
	if call := fake.only(t); call.path != "/api/beads/kn%201%2Fx" {
		t.Errorf("id not path-escaped, got %q", call.path)
	}
	if !strings.Contains(out, "Odd") {
		t.Errorf("detail missing the title:\n%s", out)
	}
}

func TestBeadAPICreateSendsTypedFields(t *testing.T) {
	fake := newFakeBeadAPI(t, http.StatusCreated, `{"id":"kn-9"}`)

	out, err := fake.run("create", "Wire", "the", "reconnect",
		"--parent", "ep-2", "--priority", "2", "--labels", "cli, sse", "--type", "task")
	if err != nil {
		t.Fatalf("bead create: %v", err)
	}
	call := fake.only(t)
	if call.method != http.MethodPost || call.path != "/api/beads" {
		t.Errorf("want POST /api/beads, got %s %s", call.method, call.path)
	}
	if call.body["title"] != "Wire the reconnect" {
		t.Errorf("unquoted title not joined: %v", call.body["title"])
	}
	if call.body["priority"] != float64(2) {
		t.Errorf("priority must be sent as a number, got %#v", call.body["priority"])
	}
	labels, _ := call.body["labels"].([]any)
	if len(labels) != 2 || labels[0] != "cli" || labels[1] != "sse" {
		t.Errorf("labels not split and trimmed: %#v", call.body["labels"])
	}
	if !strings.Contains(out, "Created bead kn-9") {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestBeadAPICreateRejectsNonIntegerPriority(t *testing.T) {
	fake := newFakeBeadAPI(t, http.StatusCreated, `{"id":"kn-9"}`)

	_, err := fake.run("create", "Wire", "--priority", "high")
	if exitCode(err) != 2 {
		t.Fatalf("want exit 2 for a non-integer priority, got %v (%d)", err, exitCode(err))
	}
	if len(fake.calls) != 0 {
		t.Errorf("a rejected invocation must not reach the server: %+v", fake.calls)
	}
}

func TestBeadAPISetPatchesOnlyThePassedFields(t *testing.T) {
	fake := newFakeBeadAPI(t, http.StatusOK, `{"id":"kn-1"}`)

	out, err := fake.run("set", "kn-1", "--state", "in_progress", "--remove-labels", "stale")
	if err != nil {
		t.Fatalf("bead set: %v", err)
	}
	call := fake.only(t)
	if call.method != http.MethodPatch || call.path != "/api/beads/kn-1" {
		t.Errorf("want PATCH /api/beads/kn-1, got %s %s", call.method, call.path)
	}
	if call.body["state"] != "in_progress" {
		t.Errorf("state not sent: %#v", call.body)
	}
	if _, present := call.body["title"]; present {
		t.Errorf("an unpassed field must not be sent: %#v", call.body)
	}
	if !strings.Contains(out, "Updated bead kn-1") {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestBeadAPISetRequiresAField(t *testing.T) {
	fake := newFakeBeadAPI(t, http.StatusOK, `{}`)

	_, err := fake.run("set", "kn-1")
	if exitCode(err) != 2 {
		t.Fatalf("want exit 2 with no field to change, got %v (%d)", err, exitCode(err))
	}
	if len(fake.calls) != 0 {
		t.Errorf("nothing should be sent: %+v", fake.calls)
	}
}

func TestBeadAPICloseSendsTheReasonWithYes(t *testing.T) {
	fake := newFakeBeadAPI(t, http.StatusOK, `{"state":"closed","reason":"shipped"}`)

	out, err := fake.run("close", "kn-1", "--reason", "shipped", "--yes")
	if err != nil {
		t.Fatalf("bead close: %v", err)
	}
	call := fake.only(t)
	if call.method != http.MethodPost || call.path != "/api/beads/kn-1/close" {
		t.Errorf("want POST /api/beads/kn-1/close, got %s %s", call.method, call.path)
	}
	if call.body["reason"] != "shipped" {
		t.Errorf("reason not sent: %#v", call.body)
	}
	if !strings.Contains(out, "Closed bead kn-1") {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestBeadAPIMarkTerminalAndRollbackSendTargetState(t *testing.T) {
	for _, sub := range []string{"mark-terminal", "rollback"} {
		fake := newFakeBeadAPI(t, http.StatusOK, `{"status":"ok"}`)
		if _, err := fake.run(sub, "kn-1", "--state", "closed", "--reason", "wedged", "--yes"); err != nil {
			t.Fatalf("bead %s: %v", sub, err)
		}
		call := fake.only(t)
		if call.path != "/api/beads/kn-1/"+sub {
			t.Errorf("want /api/beads/kn-1/%s, got %s", sub, call.path)
		}
		if call.body["targetState"] != "closed" || call.body["reason"] != "wedged" {
			t.Errorf("bead %s payload: %#v", sub, call.body)
		}
	}
}

func TestBeadAPIMarkTerminalRequiresTargetState(t *testing.T) {
	fake := newFakeBeadAPI(t, http.StatusOK, `{}`)

	_, err := fake.run("mark-terminal", "kn-1", "--yes")
	if exitCode(err) != 2 {
		t.Fatalf("want exit 2 without --state, got %v (%d)", err, exitCode(err))
	}
	if len(fake.calls) != 0 {
		t.Errorf("nothing should be sent: %+v", fake.calls)
	}
}

// The gated verbs must be inert without --yes, and inert without needing the
// server at all: the preview is what makes them safe to type by accident.
func TestBeadAPIGatedVerbsSendNothingWithoutYes(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"close", "kn-1"}, "Would close bead kn-1"},
		{[]string{"mark-terminal", "kn-1", "--state", "closed"}, "Would mark-terminal bead kn-1"},
		{[]string{"rollback", "kn-1", "--state", "implementing"}, "Would rollback bead kn-1"},
	}
	for _, tc := range cases {
		fake := newFakeBeadAPI(t, http.StatusOK, `{}`)
		out, err := fake.run(tc.args...)
		if err != nil {
			t.Fatalf("%v: %v", tc.args, err)
		}
		if len(fake.calls) != 0 {
			t.Errorf("%v reached the server without --yes: %+v", tc.args, fake.calls)
		}
		if !strings.Contains(out, tc.want) || !strings.Contains(out, "--yes") {
			t.Errorf("%v preview should name the change and the flag, got:\n%s", tc.args, out)
		}
	}
}

func TestBeadAPIRefineScopeRequiresOneField(t *testing.T) {
	fake := newFakeBeadAPI(t, http.StatusOK, `{"id":"kn-1"}`)

	if _, err := fake.run("refine-scope", "kn-1"); exitCode(err) != 2 {
		t.Fatalf("want exit 2 with no field, got %v (%d)", err, exitCode(err))
	}

	fake = newFakeBeadAPI(t, http.StatusOK, `{"id":"kn-1"}`)
	out, err := fake.run("refine-scope", "kn-1", "--acceptance", "tests green")
	if err != nil {
		t.Fatalf("bead refine-scope: %v", err)
	}
	call := fake.only(t)
	if call.method != http.MethodPost || call.path != "/api/beads/kn-1/refine-scope" {
		t.Errorf("want POST /api/beads/kn-1/refine-scope, got %s %s", call.method, call.path)
	}
	if call.body["acceptance"] != "tests green" {
		t.Errorf("acceptance not sent: %#v", call.body)
	}
	if !strings.Contains(out, "Refined scope of bead kn-1") {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestBeadAPINotFoundExitsTwo(t *testing.T) {
	fake := newFakeBeadAPI(t, http.StatusNotFound, `{"error":"bead kn-404 not found"}`)

	_, err := fake.run("get", "kn-404")
	if exitCode(err) != 2 {
		t.Fatalf("want exit 2 for a 404, got %v (%d)", err, exitCode(err))
	}
	if !strings.Contains(err.Error(), "kn-404") {
		t.Errorf("error should name the missing bead: %v", err)
	}
}

// A typo of `run` must be diagnosed against the whole verb, even though `run`
// is dispatched elsewhere and this file never serves it.
func TestBeadAPIUnknownSubcommandSuggestsRun(t *testing.T) {
	fake := newFakeBeadAPI(t, http.StatusOK, `{}`)

	_, err := fake.run("rn", "kn-1")
	if exitCode(err) != 2 {
		t.Fatalf("want exit 2 for an unknown subcommand, got %v (%d)", err, exitCode(err))
	}
	if !strings.Contains(err.Error(), `did you mean "run"`) {
		t.Errorf("error should suggest run: %v", err)
	}
	if !strings.Contains(err.Error(), "refine-scope") {
		t.Errorf("error should list the full valid set: %v", err)
	}
}

func TestBeadAPIUnknownFlagExitsTwo(t *testing.T) {
	fake := newFakeBeadAPI(t, http.StatusOK, `[]`)

	_, err := fake.run("list", "--status", "open")
	if exitCode(err) != 2 {
		t.Fatalf("want exit 2 for an unknown flag, got %v (%d)", err, exitCode(err))
	}
	if len(fake.calls) != 0 {
		t.Errorf("nothing should be sent: %+v", fake.calls)
	}
}

// Every shipped subcommand needs a help entry: the entries are what help.go
// splices into the `bead` command, so a missing one is an invisible verb.
func TestBeadAPISubcommandsAreDocumented(t *testing.T) {
	documented := map[string]bool{}
	for _, meta := range beadAPISubcommands {
		documented[meta.Name] = true
		if meta.Summary == "" || meta.Usage == "" {
			t.Errorf("bead %s needs a summary and a usage line", meta.Name)
		}
	}
	for _, name := range beadAPISubcommandNames {
		if !documented[name] {
			t.Errorf("bead %s is dispatched but not documented", name)
		}
	}
}
