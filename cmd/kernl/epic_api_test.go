package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// epicAPIRequest is what the fake API saw, so a test can assert the CLI hit the
// right route rather than only checking what it printed.
type epicAPIRequest struct {
	method string
	path   string
	query  string
}

// fakeEpicAPI stands in for a running `kernl serve`. The events route is an SSE
// stream, so the fake writes its frames and then holds the connection open the
// way the real hub does — that is exactly the condition the drain must survive.
type fakeEpicAPI struct {
	server *httptest.Server
	calls  []epicAPIRequest
	status int
	body   string
	frames []string
}

func newFakeEpicAPI(t *testing.T, f *fakeEpicAPI) *fakeEpicAPI {
	t.Helper()
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeEpicAPI) serve(w http.ResponseWriter, r *http.Request) {
	f.calls = append(f.calls, epicAPIRequest{method: r.Method, path: r.URL.EscapedPath(), query: r.URL.RawQuery})

	if f.status >= 400 {
		w.WriteHeader(f.status)
		_, _ = w.Write([]byte(f.body))
		return
	}
	if !strings.HasSuffix(r.URL.Path, "/events") {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(f.body))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	flusher, ok := w.(http.Flusher)
	if !ok {
		panic("httptest response must support flushing")
	}
	for _, frame := range f.frames {
		fmt.Fprintf(w, "data: %s\n\n", frame)
	}
	flusher.Flush()
	<-r.Context().Done()
}

func (f *fakeEpicAPI) run(args ...string) (string, error) {
	var out bytes.Buffer
	err := runEpicAPI(verbContext{server: f.server.URL, out: &out}, args)
	return out.String(), err
}

func TestEpicAPISessionsPrintsSummary(t *testing.T) {
	fake := newFakeEpicAPI(t, &fakeEpicAPI{
		status: http.StatusOK,
		body:   `[{"sessionId":"s-1","beadId":"kn-1","exited":false}]`,
	})

	out, err := fake.run("sessions", "ep-2")
	if err != nil {
		t.Fatalf("epic sessions: %v", err)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("want 1 call, got %+v", fake.calls)
	}
	if call := fake.calls[0]; call.method != http.MethodGet || call.path != "/api/epics/ep-2/sessions" {
		t.Errorf("want GET /api/epics/ep-2/sessions, got %s %s", call.method, call.path)
	}
	for _, want := range []string{"s-1", "kn-1", "running", "1 session(s)"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q:\n%s", want, out)
		}
	}
}

func TestEpicAPISessionsJSONPassesResponseThrough(t *testing.T) {
	fake := newFakeEpicAPI(t, &fakeEpicAPI{status: http.StatusOK, body: `[{"sessionId":"s-1"}]`})

	out, err := fake.run("sessions", "ep-2", "--json")
	if err != nil {
		t.Fatalf("epic sessions --json: %v", err)
	}
	var decoded []map[string]any
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("--json output is not JSON: %v\n%s", err, out)
	}
	if decoded[0]["sessionId"] != "s-1" {
		t.Errorf("server body not passed through: %s", out)
	}
}

func TestEpicAPISessionsEscapesTheID(t *testing.T) {
	fake := newFakeEpicAPI(t, &fakeEpicAPI{status: http.StatusOK, body: `[]`})

	if _, err := fake.run("sessions", "ep 2/x"); err != nil {
		t.Fatalf("epic sessions: %v", err)
	}
	if got := fake.calls[0].path; got != "/api/epics/ep%202%2Fx/sessions" {
		t.Errorf("id not path-escaped, got %q", got)
	}
}

// The default drain exists because the route never ends: it must return once
// the replayed buffer stops producing, without --follow and without a timeout.
func TestEpicAPIEventsDrainsTheReplayBuffer(t *testing.T) {
	fake := newFakeEpicAPI(t, &fakeEpicAPI{
		status: http.StatusOK,
		frames: []string{
			`{"type":"BeadStateChanged","epicId":"ep-2","beadId":"kn-1","detail":"open -> implementing","time":1750000000}`,
			`{"type":"SessionStarted","epicId":"ep-2","beadId":"kn-1","sessionId":"s-1","time":1750000001}`,
		},
	})

	out, err := fake.run("events", "ep-2")
	if err != nil {
		t.Fatalf("epic events: %v", err)
	}
	if call := fake.calls[0]; call.method != http.MethodGet || call.path != "/api/epics/ep-2/events" {
		t.Errorf("want GET /api/epics/ep-2/events, got %s %s", call.method, call.path)
	}
	for _, want := range []string{"BeadStateChanged", "open -> implementing", "SessionStarted", "session s-1"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestEpicAPIEventsJSONEmitsOneObjectPerLine(t *testing.T) {
	fake := newFakeEpicAPI(t, &fakeEpicAPI{
		status: http.StatusOK,
		frames: []string{`{"type":"WaveAdvanced","epicId":"ep-2"}`, `{"type":"SessionError","epicId":"ep-2"}`},
	})

	out, err := fake.run("events", "ep-2", "--json")
	if err != nil {
		t.Fatalf("epic events --json: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("want one JSON object per line, got %d:\n%s", len(lines), out)
	}
	for _, line := range lines {
		var decoded map[string]any
		if err := json.Unmarshal([]byte(line), &decoded); err != nil {
			t.Fatalf("line is not JSON: %v (%s)", err, line)
		}
	}
}

func TestEpicAPIEventsStopsAtTheLimit(t *testing.T) {
	fake := newFakeEpicAPI(t, &fakeEpicAPI{
		status: http.StatusOK,
		frames: []string{`{"type":"WaveAdvanced"}`, `{"type":"SessionStarted"}`, `{"type":"SessionError"}`},
	})

	out, err := fake.run("events", "ep-2", "--limit", "1", "--json")
	if err != nil {
		t.Fatalf("epic events --limit: %v", err)
	}
	if lines := strings.Split(strings.TrimSpace(out), "\n"); len(lines) != 1 {
		t.Fatalf("--limit 1 should print one event, got %d:\n%s", len(lines), out)
	}
}

func TestEpicAPIEventsEmptyStreamSaysSo(t *testing.T) {
	fake := newFakeEpicAPI(t, &fakeEpicAPI{status: http.StatusOK})

	out, err := fake.run("events", "ep-2")
	if err != nil {
		t.Fatalf("epic events: %v", err)
	}
	if !strings.Contains(out, "No events for this epic") {
		t.Errorf("an empty stream should explain itself, got:\n%s", out)
	}
}

func TestEpicAPIEventsRejectsBadBounds(t *testing.T) {
	cases := [][]string{
		{"events", "ep-2", "--limit", "zero"},
		{"events", "ep-2", "--timeout", "soon"},
		{"events", "ep-2", "--tail"},
		{"events"},
		{"events", "ep-1", "ep-2"},
	}
	for _, args := range cases {
		fake := newFakeEpicAPI(t, &fakeEpicAPI{status: http.StatusOK})
		if _, err := fake.run(args...); exitCode(err) != 2 {
			t.Errorf("%v: want exit 2, got %v (%d)", args, err, exitCode(err))
		}
		if len(fake.calls) != 0 {
			t.Errorf("%v reached the server despite being invalid: %+v", args, fake.calls)
		}
	}
}

func TestEpicAPINotFoundExitsTwo(t *testing.T) {
	for _, sub := range []string{"sessions", "events"} {
		fake := newFakeEpicAPI(t, &fakeEpicAPI{status: http.StatusNotFound, body: `{"error":"epic ep-404 not found"}`})
		_, err := fake.run(sub, "ep-404")
		if exitCode(err) != 2 {
			t.Errorf("epic %s: want exit 2 for a 404, got %v (%d)", sub, err, exitCode(err))
		}
	}
}

func TestEpicAPISubcommandsAreDocumented(t *testing.T) {
	documented := map[string]bool{}
	for _, meta := range epicAPISubcommands {
		documented[meta.Name] = true
		if meta.Summary == "" || meta.Usage == "" {
			t.Errorf("epic %s needs a summary and a usage line", meta.Name)
		}
	}
	for _, name := range []string{"events", "sessions"} {
		if !documented[name] {
			t.Errorf("epic %s is dispatched but not documented", name)
		}
	}
}
