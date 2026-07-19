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

// sessionCall is one request the fake API saw, so a test can assert the wire
// contract (method, path, body) instead of only the printed output.
type sessionCall struct {
	method string
	path   string
	body   map[string]any
}

// fakeSessionAPI stands in for a running `kernl serve`.
type fakeSessionAPI struct {
	t      *testing.T
	status int
	body   string
	calls  []sessionCall
}

func (f *fakeSessionAPI) start() *httptest.Server {
	f.t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// EscapedPath, not Path: Path is already decoded, which would hide a
		// verb that interpolated an id into the URL without escaping it.
		call := sessionCall{method: r.Method, path: r.URL.EscapedPath()}
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

func runSessionVerb(t *testing.T, api *fakeSessionAPI, args ...string) (string, error) {
	t.Helper()
	srv := api.start()
	var out bytes.Buffer
	err := runSession(verbContext{server: srv.URL, out: &out}, args)
	return out.String(), err
}

func TestSessionNudgeSendsPresetAndReportsServerStatus(t *testing.T) {
	api := &fakeSessionAPI{t: t, status: http.StatusAccepted,
		body: `{"status":"dispatched","sessionId":"sess-2"}`}
	out, err := runSessionVerb(t, api, "nudge", "sess-2", "--preset", "advance_status")
	if err != nil {
		t.Fatalf("session nudge failed: %v", err)
	}
	call := api.calls[0]
	if call.method != http.MethodPost || call.path != "/api/sessions/sess-2/nudge" {
		t.Fatalf("wrong route: %+v", call)
	}
	if !reflect.DeepEqual(call.body, map[string]any{"preset": "advance_status"}) {
		t.Errorf("nudge body = %#v", call.body)
	}
	if !strings.Contains(out, "Nudged session sess-2: dispatched") {
		t.Errorf("nudge should report the server's own status, got: %s", out)
	}
}

func TestSessionNudgeOmitsFlagsNotGiven(t *testing.T) {
	api := &fakeSessionAPI{t: t, status: http.StatusAccepted, body: `{"status":"dispatched"}`}
	if _, err := runSessionVerb(t, api, "nudge", "sess/2"); err != nil {
		t.Fatalf("session nudge failed: %v", err)
	}
	// An absent preset must not be sent: the handler owns the default.
	if len(api.calls[0].body) != 0 {
		t.Errorf("bare nudge must send an empty body, got %#v", api.calls[0].body)
	}
	if api.calls[0].path != "/api/sessions/sess%2F2/nudge" {
		t.Errorf("session id must be escaped into the path, got %q", api.calls[0].path)
	}
}

func TestSessionNudgeCustomPromptOverridesTemplate(t *testing.T) {
	api := &fakeSessionAPI{t: t, status: http.StatusAccepted, body: `{"status":"dispatched"}`}
	if _, err := runSessionVerb(t, api, "nudge", "sess-2", "--prompt", "status?"); err != nil {
		t.Fatalf("session nudge --prompt failed: %v", err)
	}
	if !reflect.DeepEqual(api.calls[0].body, map[string]any{"prompt": "status?"}) {
		t.Errorf("nudge body = %#v", api.calls[0].body)
	}
}

const sessionPromptsBody = `{"beadId":"kn-42","opencodeSessionId":"oc-9","running":true,
  "generic":"Status on kn-42?","advance_status":"Move kn-42 forward.\nThen report."}`

func TestSessionNudgePromptsPrintsBothTemplates(t *testing.T) {
	api := &fakeSessionAPI{t: t, status: http.StatusOK, body: sessionPromptsBody}
	out, err := runSessionVerb(t, api, "nudge-prompts", "sess-2")
	if err != nil {
		t.Fatalf("session nudge-prompts failed: %v", err)
	}
	if api.calls[0].method != http.MethodGet || api.calls[0].path != "/api/sessions/sess-2/nudge-prompts" {
		t.Fatalf("wrong route: %+v", api.calls[0])
	}
	for _, want := range []string{
		"session sess-2", "bead kn-42", "agent session oc-9",
		"running (a nudge will be refused until it finishes)",
		"generic:", "Status on kn-42?", "advance_status:", "Then report.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("prompts output missing %q, got:\n%s", want, out)
		}
	}
}

func TestSessionNudgePromptsJSONPassesServerBodyThrough(t *testing.T) {
	api := &fakeSessionAPI{t: t, status: http.StatusOK, body: sessionPromptsBody}
	out, err := runSessionVerb(t, api, "nudge-prompts", "sess-2", "--json")
	if err != nil {
		t.Fatalf("session nudge-prompts --json failed: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("--json output is not JSON: %v\n%s", err, out)
	}
	if decoded["opencodeSessionId"] != "oc-9" {
		t.Errorf("camelCase field lost in --json output: %v", decoded)
	}
}

func TestSessionNudgeJSONEmitsAckForEmptyBody(t *testing.T) {
	api := &fakeSessionAPI{t: t, status: http.StatusAccepted}
	out, err := runSessionVerb(t, api, "nudge", "sess-2", "--json")
	if err != nil {
		t.Fatalf("session nudge --json failed: %v", err)
	}
	var ack map[string]any
	if err := json.Unmarshal([]byte(out), &ack); err != nil {
		t.Fatalf("--json on an empty body must still emit JSON, got %q (%v)", out, err)
	}
	if ack["id"] != "sess-2" || ack["nudged"] != true {
		t.Errorf("ack = %v, want id/nudged", ack)
	}
}

func TestSessionAPINotFoundIsAUsageError(t *testing.T) {
	api := &fakeSessionAPI{t: t, status: http.StatusNotFound, body: `{"error":"unknown session"}`}
	_, err := runSessionVerb(t, api, "nudge", "sess-missing")
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

func TestSessionAPIServerErrorExitsOne(t *testing.T) {
	api := &fakeSessionAPI{t: t, status: http.StatusInternalServerError, body: `{"error":"boom"}`}
	_, err := runSessionVerb(t, api, "nudge-prompts", "sess-2")
	if err == nil || exitCode(err) != 1 {
		t.Fatalf("5xx must exit 1, got %d: %v", exitCode(err), err)
	}
}

func TestSessionUsageErrorsExitTwoWithoutTouchingTheServer(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no subcommand", nil, "requires a subcommand"},
		{"unknown subcommand", []string{"nudges"}, "unknown session subcommand"},
		{"nudge without id", []string{"nudge", "--preset", "generic"}, "requires a session ID"},
		{"nudge with two ids", []string{"nudge", "a", "b"}, "exactly one session ID"},
		{"unknown preset", []string{"nudge", "sess-2", "--preset", "advance-status"}, "unknown nudge preset"},
		{"empty prompt", []string{"nudge", "sess-2", "--prompt", "  "}, "--prompt needs text"},
		{"prompts without id", []string{"nudge-prompts"}, "requires a session ID"},
		{"unknown flag", []string{"nudge", "sess-2", "--presets", "generic"}, "unknown flag"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := runSession(verbContext{server: "http://127.0.0.1:1", out: io.Discard}, tc.args)
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
