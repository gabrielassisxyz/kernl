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

// memoryCall is what the fake API saw, so a test can assert the CLI issued the
// right request instead of only checking what it printed.
type memoryCall struct {
	method string
	path   string
	query  string
	body   map[string]any
}

// fakeMemoryAPI stands in for a running `kernl serve`: it records every call
// and answers with canned JSON.
type fakeMemoryAPI struct {
	t      *testing.T
	server *httptest.Server
	calls  []memoryCall
	status int
	body   string
}

func newFakeMemoryAPI(t *testing.T, status int, body string) *fakeMemoryAPI {
	t.Helper()
	f := &fakeMemoryAPI{t: t, status: status, body: body}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeMemoryAPI) serve(w http.ResponseWriter, r *http.Request) {
	call := memoryCall{method: r.Method, path: r.URL.Path, query: r.URL.RawQuery}
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

func (f *fakeMemoryAPI) run(args ...string) (string, error) {
	var out bytes.Buffer
	err := runMemory(verbContext{server: f.server.URL, out: &out}, args)
	return out.String(), err
}

func (f *fakeMemoryAPI) only(t *testing.T) memoryCall {
	t.Helper()
	if len(f.calls) != 1 {
		t.Fatalf("want exactly 1 API call, got %d: %+v", len(f.calls), f.calls)
	}
	return f.calls[0]
}

func TestMemoryTopicsListsAndPassesJSONThrough(t *testing.T) {
	body := `{"topics":["deploys","backups"]}`

	fake := newFakeMemoryAPI(t, http.StatusOK, body)
	out, err := fake.run("topics")
	if err != nil {
		t.Fatalf("memory topics: %v", err)
	}
	if call := fake.only(t); call.method != http.MethodGet || call.path != "/api/memory/topics" {
		t.Errorf("want GET /api/memory/topics, got %s %s", call.method, call.path)
	}
	for _, want := range []string{"deploys", "backups", "2 topic(s)"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q, got: %s", want, out)
		}
	}

	jsonFake := newFakeMemoryAPI(t, http.StatusOK, body)
	out, err = jsonFake.run("topics", "--json")
	if err != nil {
		t.Fatalf("memory topics --json: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("--json output must be the API body verbatim, got %q: %v", out, err)
	}
	if topics, ok := decoded["topics"].([]any); !ok || len(topics) != 2 {
		t.Errorf("--json lost fields: %v", decoded)
	}
}

func TestMemoryTopicsEmptyTeachesNextStep(t *testing.T) {
	fake := newFakeMemoryAPI(t, http.StatusOK, `{"topics":[]}`)
	out, err := fake.run("topics")
	if err != nil {
		t.Fatalf("memory topics: %v", err)
	}
	if !strings.Contains(out, "kernl memory add-claim") {
		t.Errorf("empty topics must point at add-claim, got: %s", out)
	}
}

func TestMemoryClaimsSendsTopicAsAnEscapedQuery(t *testing.T) {
	// The route emits camelCase (TestMemoryClaimsJSONContract pins it server
	// side). Decoding the wrong spelling fails silently — zero-valued fields
	// print as blank columns, never as an error — so the listing is asserted
	// against a body in the shape the server actually sends.
	body := `{"claims":[{"id":"c1","statement":"deploy from tags","subject":"deploys","source":"user","confidence":0.9}]}`
	fake := newFakeMemoryAPI(t, http.StatusOK, body)
	out, err := fake.run("claims", "--topic", "deploys & rollbacks")
	if err != nil {
		t.Fatalf("memory claims: %v", err)
	}

	call := fake.only(t)
	if call.method != http.MethodGet || call.path != "/api/memory/claims" {
		t.Fatalf("want GET /api/memory/claims, got %s %s", call.method, call.path)
	}
	if call.query != "topic=deploys+%26+rollbacks" {
		t.Errorf("topic must be query-escaped, got %q", call.query)
	}
	for _, want := range []string{"c1", "deploy from tags", "via user", "confidence 0.90"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q, got: %s", want, out)
		}
	}
}

func TestMemoryClaimsWithoutTopicIsAUsageError(t *testing.T) {
	fake := newFakeMemoryAPI(t, http.StatusOK, `{"claims":[]}`)
	_, err := fake.run("claims")
	if err == nil || !strings.Contains(err.Error(), "requires --topic") {
		t.Fatalf("missing --topic must fail loud, got: %v", err)
	}
	if exitCode(err) != 2 {
		t.Errorf("missing flag is a usage error, got exit %d", exitCode(err))
	}
	if len(fake.calls) != 0 {
		t.Errorf("a rejected invocation must not reach the API: %+v", fake.calls)
	}
}

func TestMemoryTelosReportsTheInjectionFootprint(t *testing.T) {
	body := `{"notes":[{"id":"n1","title":"Telos","body":"x","path":"telos.md"}],
	          "injection":{"bytes":4096,"capBytes":4096,"truncated":true}}`
	fake := newFakeMemoryAPI(t, http.StatusOK, body)
	out, err := fake.run("telos")
	if err != nil {
		t.Fatalf("memory telos: %v", err)
	}
	if call := fake.only(t); call.path != "/api/memory/telos" {
		t.Errorf("want GET /api/memory/telos, got %s %s", call.method, call.path)
	}
	for _, want := range []string{"Telos", "telos.md", "4096/4096", "TRUNCATED"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q, got: %s", want, out)
		}
	}
}

func TestMemoryAddClaimPostsSubjectAndJoinedStatement(t *testing.T) {
	fake := newFakeMemoryAPI(t, http.StatusCreated, `{"id":"c9"}`)
	out, err := fake.run("add-claim", "--subject", "deploys", "releases", "come", "from", "tags")
	if err != nil {
		t.Fatalf("memory add-claim: %v", err)
	}

	call := fake.only(t)
	if call.method != http.MethodPost || call.path != "/api/memory/claims" {
		t.Fatalf("want POST /api/memory/claims, got %s %s", call.method, call.path)
	}
	if call.body["subject"] != "deploys" || call.body["statement"] != "releases come from tags" {
		t.Errorf("unexpected payload: %v", call.body)
	}
	if !strings.Contains(out, "c9") {
		t.Errorf("add-claim must print the new id, got: %s", out)
	}
}

func TestMemoryAddClaimRequiresSubjectAndStatement(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no subject", []string{"add-claim", "a statement"}, "requires --subject"},
		{"no statement", []string{"add-claim", "--subject", "deploys"}, "requires a statement"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeMemoryAPI(t, http.StatusCreated, `{"id":"c9"}`)
			_, err := fake.run(tc.args...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got: %v", tc.want, err)
			}
			if exitCode(err) != 2 {
				t.Errorf("want usage error, got exit %d", exitCode(err))
			}
			if len(fake.calls) != 0 {
				t.Errorf("a rejected invocation must not reach the API: %+v", fake.calls)
			}
		})
	}
}

func TestMemoryRefutePostsTheReasonToTheEscapedID(t *testing.T) {
	fake := newFakeMemoryAPI(t, http.StatusOK, `{"status":"refuted","id":"r1"}`)
	out, err := fake.run("refute", "claim/1", "--reason", "superseded")
	if err != nil {
		t.Fatalf("memory refute: %v", err)
	}

	call := fake.only(t)
	if call.method != http.MethodPost || call.path != "/api/memory/claims/claim/1/refute" {
		t.Fatalf("want POST on the escaped claim path, got %s %s", call.method, call.path)
	}
	if call.body["reason"] != "superseded" {
		t.Errorf("reason not sent: %v", call.body)
	}
	if !strings.Contains(out, "Refuted claim claim/1") {
		t.Errorf("refute must confirm what it refuted, got: %s", out)
	}
}

func TestMemoryRefuteRequiresExactlyOneClaimID(t *testing.T) {
	fake := newFakeMemoryAPI(t, http.StatusOK, `{}`)
	_, err := fake.run("refute")
	if err == nil || !strings.Contains(err.Error(), "requires a claim ID") {
		t.Fatalf("missing id must fail loud, got: %v", err)
	}
	if _, err := fake.run("refute", "c1", "c2"); err == nil || !strings.Contains(err.Error(), "exactly one claim ID") {
		t.Fatalf("two ids must fail loud, got: %v", err)
	}
	if len(fake.calls) != 0 {
		t.Errorf("a rejected invocation must not reach the API: %+v", fake.calls)
	}
}

func TestMemoryAPIErrorsMapToExitCodes(t *testing.T) {
	notFound := newFakeMemoryAPI(t, http.StatusNotFound, `{"error":"claim not found"}`)
	_, err := notFound.run("refute", "ghost")
	if err == nil || exitCode(err) != 2 {
		t.Fatalf("a 404 must exit 2, got exit %d from %v", exitCode(err), err)
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("API errors must carry the marker, got: %v", err)
	}

	broken := newFakeMemoryAPI(t, http.StatusInternalServerError, `{"error":"boom"}`)
	if _, err := broken.run("topics"); err == nil || exitCode(err) != 1 {
		t.Fatalf("a 500 must exit 1, got exit %d from %v", exitCode(err), err)
	}
}

func TestMemoryUnknownSubcommandHints(t *testing.T) {
	err := runMemory(verbContext{server: "http://127.0.0.1:1", out: io.Discard}, []string{"topic"})
	if err == nil || !strings.Contains(err.Error(), `did you mean "topics"?`) {
		t.Fatalf("typo'd subcommand must hint, got: %v", err)
	}
	if exitCode(err) != 2 {
		t.Errorf("want usage error, got exit %d", exitCode(err))
	}
}
