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
	"time"
)

// approvalCall is one request the fake API saw, so a test can assert the wire
// contract (method, path, body) instead of only the printed output.
type approvalCall struct {
	method string
	path   string
	body   map[string]any
}

// fakeApprovalAPI stands in for a running `kernl serve`: it records what the
// verb sent and replies with a canned status and body.
type fakeApprovalAPI struct {
	t      *testing.T
	status int
	body   string
	calls  []approvalCall
}

func (f *fakeApprovalAPI) start() *httptest.Server {
	f.t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// EscapedPath, not Path: Path is already decoded, which would hide a
		// verb that interpolated an id into the URL without escaping it.
		call := approvalCall{method: r.Method, path: r.URL.EscapedPath()}
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

func runApprovalVerb(t *testing.T, api *fakeApprovalAPI, args ...string) (string, error) {
	t.Helper()
	srv := api.start()
	var out bytes.Buffer
	err := runApproval(verbContext{server: srv.URL, out: &out}, args)
	return out.String(), err
}

func approvalListBody(t *testing.T) string {
	t.Helper()
	// Relative to now so the age rendering is asserted, not the clock.
	old := time.Now().Add(-3 * time.Hour).UTC().Format(time.RFC3339)
	recent := time.Now().Add(-2 * time.Minute).UTC().Format(time.RFC3339)
	return `[
	  {"id":"apr-done","status":"approved","createdAt":"` + recent + `","toolName":"Read",
	   "beadId":"kn-1","actionable":false,"supportedActions":[]},
	  {"id":"apr-wait","status":"pending","createdAt":"` + old + `","toolName":"Bash(rm -rf build)",
	   "beadId":"kn-42","sessionId":"sess-7","adapter":"opencode","repoPath":"/tmp/repo",
	   "actionable":true,"supportedActions":["accept","decline"]}
	]`
}

func TestApprovalListLeadsWithWhatIsWaiting(t *testing.T) {
	api := &fakeApprovalAPI{t: t, status: http.StatusOK, body: approvalListBody(t)}
	out, err := runApprovalVerb(t, api, "list")
	if err != nil {
		t.Fatalf("approval list failed: %v", err)
	}
	if len(api.calls) != 1 || api.calls[0].method != http.MethodGet || api.calls[0].path != "/api/approvals" {
		t.Fatalf("unexpected calls: %+v", api.calls)
	}
	// What is waiting, for what, since when — plus the command that ends it.
	for _, want := range []string{
		"apr-wait", "pending", "3h ago", "Bash(rm -rf build)",
		"bead kn-42", "session sess-7", "/tmp/repo",
		"kernl approval resolve apr-wait --action accept --session sess-7 --yes",
		"2 approval(s), 1 waiting on you",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("listing missing %q, got:\n%s", want, out)
		}
	}
	// Actionable first, regardless of the order the API returned.
	if strings.Index(out, "apr-wait") > strings.Index(out, "apr-done") {
		t.Errorf("what needs a decision must come first, got:\n%s", out)
	}
}

func TestApprovalListEmptySaysSo(t *testing.T) {
	api := &fakeApprovalAPI{t: t, status: http.StatusOK, body: `[]`}
	out, err := runApprovalVerb(t, api, "list")
	if err != nil {
		t.Fatalf("approval list failed: %v", err)
	}
	if !strings.Contains(out, "Nothing waiting on you") {
		t.Errorf("empty listing should be explicit, got: %q", out)
	}
}

func TestApprovalListJSONPassesServerBodyThrough(t *testing.T) {
	api := &fakeApprovalAPI{t: t, status: http.StatusOK, body: approvalListBody(t)}
	out, err := runApprovalVerb(t, api, "list", "--json")
	if err != nil {
		t.Fatalf("approval list --json failed: %v", err)
	}
	var decoded []map[string]any
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("--json output is not JSON: %v\n%s", err, out)
	}
	if decoded[0]["supportedActions"] == nil {
		t.Errorf("camelCase field lost in --json output: %v", decoded[0])
	}
}

func TestApprovalResolveGrantRequiresYes(t *testing.T) {
	api := &fakeApprovalAPI{t: t, status: http.StatusOK, body: `{}`}
	out, err := runApprovalVerb(t, api, "resolve", "apr-7", "--action", "approve")
	if err != nil {
		t.Fatalf("unconfirmed grant must succeed as a preview, got: %v", err)
	}
	if len(api.calls) != 0 {
		t.Fatalf("approving without --yes must not touch the server, got %+v", api.calls)
	}
	if !strings.Contains(out, "apr-7") || !strings.Contains(out, "--yes") {
		t.Errorf("preview must name the approval and the confirmation flag, got: %s", out)
	}
}

func TestApprovalResolveApproveWithYesPostsAction(t *testing.T) {
	api := &fakeApprovalAPI{t: t, status: http.StatusOK, body: `{}`}
	out, err := runApprovalVerb(t, api, "resolve", "apr 7", "--action", "approve", "--yes")
	if err != nil {
		t.Fatalf("approval resolve failed: %v", err)
	}
	call := api.calls[0]
	// The id is escaped into the path, not concatenated raw.
	if call.method != http.MethodPost || call.path != "/api/approvals/apr%207/actions" {
		t.Fatalf("wrong route: %+v", call)
	}
	if !reflect.DeepEqual(call.body, map[string]any{"action": "approve"}) {
		t.Errorf("action body = %#v", call.body)
	}
	if !strings.Contains(out, "Resolved approval apr 7") {
		t.Errorf("resolve should confirm, got: %s", out)
	}
}

// The backend answers 501 until the judgment-gate capture flow exists. The verb
// must surface that as a loud, non-zero failure — never the old fabricated
// "Resolved" for an id the server never saw. This is the regression guard for
// the honest-facade fix.
func TestApprovalResolveSurfacesNotImplemented(t *testing.T) {
	api := &fakeApprovalAPI{t: t, status: http.StatusNotImplemented,
		body: `{"error":"approvals are not implemented yet","implemented":false}`}
	out, err := runApprovalVerb(t, api, "resolve", "apr-999", "--action", "approve", "--yes")
	if err == nil {
		t.Fatalf("a 501 must be a non-nil error, got success with output: %q", out)
	}
	if exitCode(err) != 1 {
		t.Errorf("501 is a server-side failure — want exit 1, got %d", exitCode(err))
	}
	if strings.Contains(out, "Resolved") {
		t.Errorf("must not fabricate a resolution, got: %q", out)
	}
	if !strings.Contains(err.Error(), "not implemented") {
		t.Errorf("error should carry the server's not-implemented message, got: %v", err)
	}
}

func TestApprovalListSurfacesNotImplemented(t *testing.T) {
	api := &fakeApprovalAPI{t: t, status: http.StatusNotImplemented,
		body: `{"error":"approvals are not implemented yet","implemented":false}`}
	out, err := runApprovalVerb(t, api, "list")
	if err == nil {
		t.Fatalf("list against a 501 must fail, got success with output: %q", out)
	}
	if strings.Contains(out, "Nothing waiting") {
		t.Errorf("must not report an idle gate when the gate is unbuilt, got: %q", out)
	}
}

func TestApprovalResolveRejectNeedsNoConfirmation(t *testing.T) {
	api := &fakeApprovalAPI{t: t, status: http.StatusOK, body: `{}`}
	// Declining only stops work, so it is not gated — asserted so the
	// asymmetry cannot be "tidied away" without a failing test.
	if _, err := runApprovalVerb(t, api, "resolve", "apr-7", "--action", "reject"); err != nil {
		t.Fatalf("approval resolve --action reject failed: %v", err)
	}
	if len(api.calls) != 1 || api.calls[0].path != "/api/approvals/apr-7/actions" {
		t.Fatalf("reject should go straight out, got %+v", api.calls)
	}
}

func TestApprovalResolveSessionUsesTerminalRoute(t *testing.T) {
	api := &fakeApprovalAPI{t: t, status: http.StatusOK, body: `{}`}
	out, err := runApprovalVerb(t, api, "resolve", "apr-7", "--action", "always_approve", "--session", "sess/2", "--yes")
	if err != nil {
		t.Fatalf("session-scoped resolve failed: %v", err)
	}
	call := api.calls[0]
	if call.path != "/api/terminal/sess%2F2/approvals/apr-7" {
		t.Fatalf("session id must be escaped into the terminal route, got %q", call.path)
	}
	if !reflect.DeepEqual(call.body, map[string]any{"action": "always_approve"}) {
		t.Errorf("action body = %#v", call.body)
	}
	if !strings.Contains(out, "in session sess/2") {
		t.Errorf("confirmation should name the session, got: %s", out)
	}
}

func TestApprovalResolveJSONEmitsAckForEmptyBody(t *testing.T) {
	api := &fakeApprovalAPI{t: t, status: http.StatusNoContent}
	out, err := runApprovalVerb(t, api, "resolve", "apr-7", "--action", "reject", "--json")
	if err != nil {
		t.Fatalf("approval resolve --json failed: %v", err)
	}
	var ack map[string]any
	if err := json.Unmarshal([]byte(out), &ack); err != nil {
		t.Fatalf("--json on an empty body must still emit JSON, got %q (%v)", out, err)
	}
	if ack["id"] != "apr-7" || ack["resolved"] != true {
		t.Errorf("ack = %v, want id/resolved", ack)
	}
}

func TestApprovalAPINotFoundIsAUsageError(t *testing.T) {
	api := &fakeApprovalAPI{t: t, status: http.StatusNotFound, body: `{"error":"approval not found"}`}
	_, err := runApprovalVerb(t, api, "resolve", "apr-missing", "--action", "reject")
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

func TestApprovalAPIServerErrorExitsOne(t *testing.T) {
	api := &fakeApprovalAPI{t: t, status: http.StatusInternalServerError, body: `{"error":"boom"}`}
	_, err := runApprovalVerb(t, api, "list")
	if err == nil || exitCode(err) != 1 {
		t.Fatalf("5xx must exit 1, got %d: %v", exitCode(err), err)
	}
}

func TestApprovalUsageErrorsExitTwoWithoutTouchingTheServer(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no subcommand", nil, "requires a subcommand"},
		{"unknown subcommand", []string{"resolv"}, "unknown approval subcommand"},
		{"list with positional", []string{"list", "apr-1"}, "no positional arguments"},
		{"resolve without id", []string{"resolve", "--action", "approve"}, "requires an approval ID"},
		{"resolve with two ids", []string{"resolve", "a", "b", "--action", "reject"}, "exactly one approval ID"},
		{"resolve without action", []string{"resolve", "apr-1"}, "requires --action"},
		{"session action on the gate route", []string{"resolve", "apr-1", "--action", "accept"}, "unknown approval action"},
		{"gate action on the session route", []string{"resolve", "apr-1", "--action", "approve", "--session", "s1"}, "unknown approval action"},
		{"unknown flag", []string{"list", "--pending"}, "unknown flag"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// No reachable server: a malformed invocation must be diagnosed
			// before anything tries to reach the backend.
			err := runApproval(verbContext{server: "http://127.0.0.1:1", out: io.Discard}, tc.args)
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

// The hint is a copy-paste command, so it has to name the route the approval
// actually lives on — a session id alone does not make it session-scoped.
func TestApprovalResolveHintFollowsTheAdvertisedActions(t *testing.T) {
	gate := approvalView{ID: "apr-1", SessionID: "sess-7", SupportedActions: []string{"approve", "reject"}}
	if hint := approvalResolveHint(gate); strings.Contains(hint, "--session") ||
		!strings.Contains(hint, "--action approve") {
		t.Errorf("gate approval hint = %q", hint)
	}
	prompt := approvalView{ID: "apr-2", SessionID: "sess-7", SupportedActions: []string{"accept", "decline"}}
	if hint := approvalResolveHint(prompt); !strings.Contains(hint, "--action accept --session sess-7") {
		t.Errorf("session-scoped hint = %q", hint)
	}
}

func TestApprovalActionErrorNamesTheRoutesVocabulary(t *testing.T) {
	err := checkApprovalAction("accept", "")
	if err == nil || !strings.Contains(err.Error(), "approve, reject") {
		t.Errorf("gate-route error must list the gate actions, got: %v", err)
	}
	err = checkApprovalAction("approve", "sess-1")
	if err == nil || !strings.Contains(err.Error(), "accept, always_approve, decline") {
		t.Errorf("session-route error must list the session actions, got: %v", err)
	}
}
