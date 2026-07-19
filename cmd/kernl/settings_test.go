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

// settingsCall records one request the CLI issued, so a test can assert the
// whole-section merge sent the untouched fields back unchanged.
type settingsCall struct {
	method string
	path   string
	body   map[string]any
}

// fakeSettingsAPI answers GET /api/settings with a fixed snapshot and every
// PUT with the same snapshot, which is what the real handler does.
type fakeSettingsAPI struct {
	t      *testing.T
	server *httptest.Server
	calls  []settingsCall
	status int
	body   string
	// readFails makes the GET fail too. By default reads succeed, so a test of
	// a failing write still gets past the read-modify-write's first call.
	readFails bool
}

const settingsFixture = `{
  "configPath": "/home/u/kernl.yaml",
  "writable": true,
  "restartPending": ["llm.model"],
  "llm": {"provider":"openai","model":"gpt-4o-mini","endpoint":"http://127.0.0.1:4000","apiKeySet":true},
  "vault": {"root":"/home/u/vault","coalesceWindowMs":250,"moveWindowMs":500,"rescanIntervalSec":60},
  "inbox": {"autoPrep":false,"daSubdir":"DA"},
  "runtime": {"serverPort":8080,"worktreeRoot":"/home/u/wt","maxConcurrentBeads":4,
    "runStatePath":"/home/u/.kernl/state","stageRetryAttempts":2,"sweepIntervalSec":300,
    "prStaleWarnDays":3,"sweepFailureLimit":5,"sweepBackoffMinutes":[5,15,60]}
}`

func newFakeSettingsAPI(t *testing.T, status int, body string) *fakeSettingsAPI {
	t.Helper()
	f := &fakeSettingsAPI{t: t, status: status, body: body}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeSettingsAPI) serve(w http.ResponseWriter, r *http.Request) {
	call := settingsCall{method: r.Method, path: r.URL.Path}
	if raw, err := io.ReadAll(r.Body); err == nil && len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &call.body); err != nil {
			f.t.Errorf("request body is not JSON: %s", raw)
		}
	}
	f.calls = append(f.calls, call)

	if r.Method == http.MethodGet && !f.readFails {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, settingsFixture)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(f.status)
	if f.body != "" {
		_, _ = io.WriteString(w, f.body)
	}
}

func (f *fakeSettingsAPI) run(args ...string) (string, error) {
	var out bytes.Buffer
	err := runSettings(verbContext{server: f.server.URL, out: &out}, args)
	return out.String(), err
}

func (f *fakeSettingsAPI) lastCall(t *testing.T) settingsCall {
	t.Helper()
	if len(f.calls) == 0 {
		t.Fatal("want at least one API call, got none")
	}
	return f.calls[len(f.calls)-1]
}

func TestSettingsGetPrintsEverySectionAndJSON(t *testing.T) {
	fake := newFakeSettingsAPI(t, http.StatusOK, settingsFixture)
	out, err := fake.run("get")
	if err != nil {
		t.Fatalf("settings get: %v", err)
	}
	if call := fake.lastCall(t); call.method != http.MethodGet || call.path != "/api/settings" {
		t.Errorf("want GET /api/settings, got %s %s", call.method, call.path)
	}
	for _, want := range []string{"/home/u/kernl.yaml", "llm", "vault", "inbox", "runtime", "gpt-4o-mini", "5,15,60"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q, got: %s", want, out)
		}
	}
	if !strings.Contains(out, "restart") && !strings.Contains(out, "NOT active") {
		t.Errorf("a pending restart must be surfaced, got: %s", out)
	}

	jsonFake := newFakeSettingsAPI(t, http.StatusOK, settingsFixture)
	out, err = jsonFake.run("get", "--json")
	if err != nil {
		t.Fatalf("settings get --json: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("--json output must be the API body verbatim, got %q: %v", out, err)
	}
	if decoded["configPath"] != "/home/u/kernl.yaml" {
		t.Errorf("--json lost fields: %v", decoded)
	}
}

func TestSettingsSetMergesOntoTheCurrentSection(t *testing.T) {
	cases := []struct {
		name string
		args []string
		path string
		want map[string]any
	}{
		{
			"llm keeps the untouched fields",
			[]string{"set", "llm", "--model", "kimi-k2"},
			"/api/settings/llm",
			map[string]any{"provider": "openai", "model": "kimi-k2", "endpoint": "http://127.0.0.1:4000"},
		},
		{
			"inbox toggles auto-prep",
			[]string{"set", "inbox", "--auto-prep", "true"},
			"/api/settings/inbox",
			map[string]any{"autoPrep": true, "daSubdir": "DA"},
		},
		{
			"vault edits one window",
			[]string{"set", "vault", "--move-window-ms", "900"},
			"/api/settings/vault",
			map[string]any{"root": "/home/u/vault", "coalesceWindowMs": float64(250),
				"moveWindowMs": float64(900), "rescanIntervalSec": float64(60)},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeSettingsAPI(t, http.StatusOK, settingsFixture)
			if _, err := fake.run(tc.args...); err != nil {
				t.Fatalf("settings set: %v", err)
			}
			call := fake.lastCall(t)
			if call.method != http.MethodPut || call.path != tc.path {
				t.Fatalf("want PUT %s, got %s %s", tc.path, call.method, call.path)
			}
			for key, want := range tc.want {
				if call.body[key] != want {
					t.Errorf("%s = %v, want %v (full body: %v)", key, call.body[key], want, call.body)
				}
			}
		})
	}
}

func TestSettingsSetRuntimeSendsTheWholeSection(t *testing.T) {
	fake := newFakeSettingsAPI(t, http.StatusOK, settingsFixture)
	out, err := fake.run("set", "runtime", "--max-concurrent-beads", "8", "--sweep-backoff-minutes", "5, 30")
	if err != nil {
		t.Fatalf("settings set runtime: %v", err)
	}
	body := fake.lastCall(t).body
	if body["maxConcurrentBeads"] != float64(8) || body["serverPort"] != float64(8080) {
		t.Errorf("runtime merge lost fields: %v", body)
	}
	backoff, ok := body["sweepBackoffMinutes"].([]any)
	if !ok || len(backoff) != 2 || backoff[1] != float64(30) {
		t.Errorf("backoff must split and trim, got: %v", body["sweepBackoffMinutes"])
	}
	if !strings.Contains(out, "restart") {
		t.Errorf("a write must say it is not applied until restart, got: %s", out)
	}
}

func TestSettingsSetAPIKeyNeverReachesStdout(t *testing.T) {
	const secret = "sk-do-not-print-me"
	fake := newFakeSettingsAPI(t, http.StatusOK, settingsFixture)
	out, err := fake.run("set", "llm", "--api-key", secret)
	if err != nil {
		t.Fatalf("settings set llm --api-key: %v", err)
	}
	if fake.lastCall(t).body["apiKey"] != secret {
		t.Errorf("the key must reach the server: %v", fake.lastCall(t).body)
	}
	if strings.Contains(out, secret) {
		t.Fatalf("the API key must never be echoed to stdout, got: %s", out)
	}
	if !strings.Contains(out, "apiKey=set") {
		t.Errorf("output should report that a key exists without printing it, got: %s", out)
	}
}

func TestSettingsSetOmittedAPIKeyIsNotSent(t *testing.T) {
	fake := newFakeSettingsAPI(t, http.StatusOK, settingsFixture)
	if _, err := fake.run("set", "llm", "--provider", "ollama", "--model", "llama3"); err != nil {
		t.Fatalf("settings set llm: %v", err)
	}
	if _, sent := fake.lastCall(t).body["apiKey"]; sent {
		t.Errorf("an omitted --api-key must not clear the stored credential: %v", fake.lastCall(t).body)
	}
}

func TestSettingsSetWithNoFieldsIsAUsageError(t *testing.T) {
	fake := newFakeSettingsAPI(t, http.StatusOK, settingsFixture)
	_, err := fake.run("set", "vault")
	if err == nil || !strings.Contains(err.Error(), "changes nothing") {
		t.Fatalf("an empty write must fail loud, got: %v", err)
	}
	if exitCode(err) != 2 {
		t.Errorf("want usage error, got exit %d", exitCode(err))
	}
	for _, call := range fake.calls {
		if call.method == http.MethodPut {
			t.Fatalf("a rejected invocation must not write: %+v", fake.calls)
		}
	}
}

func TestSettingsSetRejectsNonNumericValues(t *testing.T) {
	fake := newFakeSettingsAPI(t, http.StatusOK, settingsFixture)
	_, err := fake.run("set", "runtime", "--server-port", "eighty")
	if err == nil || !strings.Contains(err.Error(), "whole number") {
		t.Fatalf("a non-numeric value must fail loud, got: %v", err)
	}
	if exitCode(err) != 2 {
		t.Errorf("want usage error, got exit %d", exitCode(err))
	}
}

func TestSettingsAPIErrorsMapToExitCodes(t *testing.T) {
	rejected := newFakeSettingsAPI(t, http.StatusNotFound, `{"error":"no such section"}`)
	_, err := rejected.run("set", "inbox", "--da-subdir", "DA2")
	if err == nil || exitCode(err) != 2 {
		t.Fatalf("a 404 must exit 2, got exit %d from %v", exitCode(err), err)
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("API errors must carry the marker, got: %v", err)
	}

	broken := newFakeSettingsAPI(t, http.StatusInternalServerError, `{"error":"boom"}`)
	broken.readFails = true
	if _, err := broken.run("get"); err == nil || exitCode(err) != 1 {
		t.Fatalf("a 500 must exit 1, got exit %d from %v", exitCode(err), err)
	}
}

func TestSettingsUnknownSubcommandHints(t *testing.T) {
	err := runSettings(verbContext{server: "http://127.0.0.1:1", out: io.Discard}, []string{"st"})
	if err == nil || !strings.Contains(err.Error(), `did you mean "set"?`) {
		t.Fatalf("typo'd subcommand must hint, got: %v", err)
	}
	if exitCode(err) != 2 {
		t.Errorf("want usage error, got exit %d", exitCode(err))
	}
}

func TestSettingsSetUnknownSectionIsAUsageError(t *testing.T) {
	fake := newFakeSettingsAPI(t, http.StatusOK, settingsFixture)
	_, err := fake.run("set", "vaults", "--root", "/tmp")
	if err == nil || !strings.Contains(err.Error(), "vault") {
		t.Fatalf("unknown section must name the valid ones, got: %v", err)
	}
	if exitCode(err) != 2 {
		t.Errorf("want usage error, got exit %d", exitCode(err))
	}
}
