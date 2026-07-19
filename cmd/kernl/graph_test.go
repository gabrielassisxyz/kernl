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

// graphCall is what the fake API saw, so a test can assert the CLI issued the
// right request instead of only checking what it printed.
type graphCall struct {
	method string
	path   string
	query  string
}

// fakeGraphAPI stands in for a running `kernl serve`: it records every call and
// answers with canned JSON.
type fakeGraphAPI struct {
	server *httptest.Server
	calls  []graphCall
	status int
	body   string
}

func newFakeGraphAPI(t *testing.T, status int, body string) *fakeGraphAPI {
	t.Helper()
	f := &fakeGraphAPI{status: status, body: body}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeGraphAPI) serve(w http.ResponseWriter, r *http.Request) {
	f.calls = append(f.calls, graphCall{method: r.Method, path: r.URL.Path, query: r.URL.RawQuery})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(f.status)
	if f.body != "" {
		_, _ = io.WriteString(w, f.body)
	}
}

func (f *fakeGraphAPI) run(args ...string) (string, error) {
	var out bytes.Buffer
	err := runGraph(verbContext{server: f.server.URL, out: &out}, args)
	return out.String(), err
}

func (f *fakeGraphAPI) only(t *testing.T) graphCall {
	t.Helper()
	if len(f.calls) != 1 {
		t.Fatalf("want exactly 1 API call, got %d: %+v", len(f.calls), f.calls)
	}
	return f.calls[0]
}

func TestGraphNodesListsAndPassesJSONThrough(t *testing.T) {
	body := `[{"id":"n1","title":"Backups","type":"note"},{"id":"n2","title":"Ship v1","type":"task"}]`

	fake := newFakeGraphAPI(t, http.StatusOK, body)
	out, err := fake.run("nodes")
	if err != nil {
		t.Fatalf("graph nodes: %v", err)
	}
	call := fake.only(t)
	if call.method != http.MethodGet || call.path != "/api/nodes" {
		t.Errorf("want GET /api/nodes, got %s %s", call.method, call.path)
	}
	if call.query != "" {
		t.Errorf("the route takes no parameters; got query %q", call.query)
	}
	for _, want := range []string{"n1", "Backups", "note", "2 node(s)"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q, got: %s", want, out)
		}
	}

	jsonFake := newFakeGraphAPI(t, http.StatusOK, body)
	out, err = jsonFake.run("nodes", "--json")
	if err != nil {
		t.Fatalf("graph nodes --json: %v", err)
	}
	var decoded []map[string]any
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("--json output must be the API body verbatim, got %q: %v", out, err)
	}
	if len(decoded) != 2 || decoded[1]["type"] != "task" {
		t.Errorf("--json lost fields: %v", decoded)
	}
}

// The route answers `null` (not `[]`) for an empty graph; the listing must
// still teach the next step rather than printing nothing.
func TestGraphNodesEmptyTeachesNextStep(t *testing.T) {
	fake := newFakeGraphAPI(t, http.StatusOK, `null`)
	out, err := fake.run("nodes")
	if err != nil {
		t.Fatalf("graph nodes: %v", err)
	}
	if !strings.Contains(out, "kernl capture") {
		t.Errorf("empty graph must point somewhere, got: %s", out)
	}
}

func TestGraphSearchSendsQueryTypeAndLimit(t *testing.T) {
	fake := newFakeGraphAPI(t, http.StatusOK, `[{"id":"n1","title":"Backups","type":"note"}]`)
	out, err := fake.run("search", "--type", "note", "--limit", "5", "back", "ups")
	if err != nil {
		t.Fatalf("graph search: %v", err)
	}

	call := fake.only(t)
	if call.method != http.MethodGet || call.path != "/api/nodes/search" {
		t.Fatalf("want GET /api/nodes/search, got %s %s", call.method, call.path)
	}
	if call.query != "limit=5&q=back+ups&type=note" {
		t.Errorf("unexpected query %q", call.query)
	}
	if !strings.Contains(out, "Backups") {
		t.Errorf("search must print its hits, got: %s", out)
	}
}

func TestGraphSearchOmitsFlagsNotGiven(t *testing.T) {
	fake := newFakeGraphAPI(t, http.StatusOK, `[]`)
	out, err := fake.run("search", "back")
	if err != nil {
		t.Fatalf("graph search: %v", err)
	}
	if call := fake.only(t); call.query != "q=back" {
		t.Errorf("omitted flags must not be sent, got query %q", call.query)
	}
	if !strings.Contains(out, "No matches") {
		t.Errorf("empty result must say so, got: %s", out)
	}
}

func TestGraphSearchRequiresAQuery(t *testing.T) {
	fake := newFakeGraphAPI(t, http.StatusOK, `[]`)
	_, err := fake.run("search", "--type", "note")
	if err == nil || !strings.Contains(err.Error(), "requires a query") {
		t.Fatalf("missing query must fail loud, got: %v", err)
	}
	if exitCode(err) != 2 {
		t.Errorf("missing argument is a usage error, got exit %d", exitCode(err))
	}
	if len(fake.calls) != 0 {
		t.Errorf("a rejected invocation must not reach the API: %+v", fake.calls)
	}
}

func TestGraphLimitMustBeAPositiveInteger(t *testing.T) {
	for _, bad := range []string{"abc", "0", "-3"} {
		fake := newFakeGraphAPI(t, http.StatusOK, `[]`)
		_, err := fake.run("search", "--limit", bad, "back")
		if err == nil || !strings.Contains(err.Error(), "--limit must be a positive integer") {
			t.Fatalf("--limit %q must fail loud, got: %v", bad, err)
		}
		if exitCode(err) != 2 {
			t.Errorf("want usage error, got exit %d", exitCode(err))
		}
		if len(fake.calls) != 0 {
			t.Errorf("a rejected invocation must not reach the API: %+v", fake.calls)
		}
	}
}

func TestGraphRelatedEscapesTheIDAndForwardsTheLimit(t *testing.T) {
	fake := newFakeGraphAPI(t, http.StatusOK, `[{"id":"n2","title":"Ship v1","type":"task"}]`)
	out, err := fake.run("related", "node/1", "--limit", "3")
	if err != nil {
		t.Fatalf("graph related: %v", err)
	}

	call := fake.only(t)
	if call.path != "/api/nodes/node/1/related" {
		t.Fatalf("want the escaped related path, got %s", call.path)
	}
	if call.query != "limit=3" {
		t.Errorf("limit not forwarded, got query %q", call.query)
	}
	if !strings.Contains(out, "Ship v1") {
		t.Errorf("related must print its hits, got: %s", out)
	}
}

func TestGraphBriefingPrintsTitleAndBody(t *testing.T) {
	fake := newFakeGraphAPI(t, http.StatusOK, `{"id":"n1","title":"Prep: backups","body":"Do the thing."}`)
	out, err := fake.run("briefing", "n1")
	if err != nil {
		t.Fatalf("graph briefing: %v", err)
	}
	if call := fake.only(t); call.path != "/api/nodes/n1/briefing" {
		t.Fatalf("want GET /api/nodes/n1/briefing, got %s", call.path)
	}
	for _, want := range []string{"Prep: backups", "Do the thing."} {
		if !strings.Contains(out, want) {
			t.Errorf("briefing missing %q, got: %s", want, out)
		}
	}
}

func TestGraphEdgesPrintsTheConnections(t *testing.T) {
	fake := newFakeGraphAPI(t, http.StatusOK, `[{"id":"e1","src":"n1","dst":"n2","label":"part_of"}]`)
	out, err := fake.run("edges")
	if err != nil {
		t.Fatalf("graph edges: %v", err)
	}
	if call := fake.only(t); call.path != "/api/edges" {
		t.Fatalf("want GET /api/edges, got %s", call.path)
	}
	for _, want := range []string{"n1", "part_of", "n2", "1 edge(s)"} {
		if !strings.Contains(out, want) {
			t.Errorf("edge listing missing %q, got: %s", want, out)
		}
	}

	empty := newFakeGraphAPI(t, http.StatusOK, `[]`)
	if out, err := empty.run("edges"); err != nil || !strings.Contains(out, "No edges") {
		t.Errorf("empty edges must say so, got %q / %v", out, err)
	}
}

func TestGraphAPIErrorsMapToExitCodes(t *testing.T) {
	// A node the DA never briefed answers 404 — a 4xx is about the invocation.
	notFound := newFakeGraphAPI(t, http.StatusNotFound, "no briefing\n")
	_, err := notFound.run("briefing", "ghost")
	if err == nil || exitCode(err) != 2 {
		t.Fatalf("a 404 must exit 2, got exit %d from %v", exitCode(err), err)
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("API errors must carry the marker, got: %v", err)
	}

	broken := newFakeGraphAPI(t, http.StatusInternalServerError, `{"error":"boom"}`)
	if _, err := broken.run("nodes"); err == nil || exitCode(err) != 1 {
		t.Fatalf("a 500 must exit 1, got exit %d from %v", exitCode(err), err)
	}
}

func TestGraphUnknownSubcommandHints(t *testing.T) {
	err := runGraph(verbContext{server: "http://127.0.0.1:1", out: io.Discard}, []string{"node"})
	if err == nil || !strings.Contains(err.Error(), `did you mean "nodes"?`) {
		t.Fatalf("typo'd subcommand must hint, got: %v", err)
	}
	if exitCode(err) != 2 {
		t.Errorf("want usage error, got exit %d", exitCode(err))
	}
}

func TestGraphReadOnlyVerbsRejectStrayPositionalArgs(t *testing.T) {
	for _, args := range [][]string{{"nodes", "extra"}, {"edges", "extra"}} {
		fake := newFakeGraphAPI(t, http.StatusOK, `[]`)
		_, err := fake.run(args...)
		if err == nil || !strings.Contains(err.Error(), "takes no positional arguments") {
			t.Fatalf("%v must fail loud, got: %v", args, err)
		}
		if len(fake.calls) != 0 {
			t.Errorf("a rejected invocation must not reach the API: %+v", fake.calls)
		}
	}
}
