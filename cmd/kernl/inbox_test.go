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

// inboxCall is what the fake API saw, so a test can assert the CLI sent
// the right method, path and payload rather than only reading its output.
type inboxCall struct {
	method string
	path   string
	query  string
	body   map[string]any
}

// fakeInboxAPI stands in for a running `kernl serve`: it records every request
// and replies with canned JSON, keeping these tests off the host and network.
func fakeInboxAPI(t *testing.T, status int, response string) (*httptest.Server, *[]inboxCall) {
	t.Helper()
	var seen []inboxCall
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := inboxCall{method: r.Method, path: r.URL.Path, query: r.URL.RawQuery}
		if raw, _ := io.ReadAll(r.Body); len(raw) > 0 {
			_ = json.Unmarshal(raw, &rec.body)
		}
		seen = append(seen, rec)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, response)
	}))
	t.Cleanup(ts.Close)
	return ts, &seen
}

// driveInbox runs one inbox invocation against the fake server.
func driveInbox(t *testing.T, ts *httptest.Server, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := runInbox(verbContext{server: ts.URL, out: &out}, args)
	return out.String(), err
}

func TestInboxListPending(t *testing.T) {
	ts, seen := fakeInboxAPI(t, http.StatusOK,
		`[{"id":"cap-1","type":"CLI","title":"call the accountant","hasPrep":true}]`)

	out, err := driveInbox(t, ts, "list")
	if err != nil {
		t.Fatalf("inbox list: %v", err)
	}
	if (*seen)[0].method != http.MethodGet || (*seen)[0].path != "/api/inbox/pending" {
		t.Fatalf("expected GET /api/inbox/pending, got %s %s", (*seen)[0].method, (*seen)[0].path)
	}
	if !strings.Contains(out, "cap-1") || !strings.Contains(out, "[prep]") {
		t.Fatalf("pending row missing from output: %q", out)
	}
}

func TestInboxListProcessed(t *testing.T) {
	ts, seen := fakeInboxAPI(t, http.StatusOK,
		`[{"captureId":"cap-2","title":"receipt","became":[{"type":"note"},{"type":"task"}]}]`)

	out, err := driveInbox(t, ts, "list", "--processed")
	if err != nil {
		t.Fatalf("inbox list --processed: %v", err)
	}
	if (*seen)[0].path != "/api/inbox/processed" {
		t.Fatalf("expected /api/inbox/processed, got %s", (*seen)[0].path)
	}
	if !strings.Contains(out, "note+task") {
		t.Fatalf("expected the fan-out types in output, got %q", out)
	}
}

func TestInboxListJSONPassesResponseThrough(t *testing.T) {
	ts, _ := fakeInboxAPI(t, http.StatusOK, `[{"id":"cap-1","title":"t"}]`)

	out, err := driveInbox(t, ts, "list", "--json")
	if err != nil {
		t.Fatalf("inbox list --json: %v", err)
	}
	var items []map[string]any
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		t.Fatalf("--json output is not the API's JSON: %v (%q)", err, out)
	}
}

func TestInboxAddPostsBodyText(t *testing.T) {
	ts, seen := fakeInboxAPI(t, http.StatusCreated, `{"id":"cap-9"}`)

	out, err := driveInbox(t, ts, "add", "call", "the", "accountant")
	if err != nil {
		t.Fatalf("inbox add: %v", err)
	}
	req := (*seen)[0]
	if req.method != http.MethodPost || req.path != "/api/inbox" {
		t.Fatalf("expected POST /api/inbox, got %s %s", req.method, req.path)
	}
	if req.body["body"] != "call the accountant" {
		t.Fatalf("capture text not sent verbatim: %#v", req.body)
	}
	if !strings.Contains(out, "cap-9") {
		t.Fatalf("expected the new id in output, got %q", out)
	}
}

func TestInboxProcessSendsActionWithoutBody(t *testing.T) {
	ts, seen := fakeInboxAPI(t, http.StatusOK, `{"status":"ok"}`)

	if _, err := driveInbox(t, ts, "process", "cap-1",
		"--target", "task", "--title", "Call", "--tags", "money, admin", "--due", "2026-08-01"); err != nil {
		t.Fatalf("inbox process: %v", err)
	}
	req := (*seen)[0]
	if req.method != http.MethodPost || req.path != "/api/inbox/cap-1/process" {
		t.Fatalf("expected POST /api/inbox/cap-1/process, got %s %s", req.method, req.path)
	}
	actions, ok := req.body["actions"].([]any)
	if !ok || len(actions) != 1 {
		t.Fatalf("expected exactly one action, got %#v", req.body)
	}
	action := actions[0].(map[string]any)
	if _, present := action["body"]; present {
		t.Fatal("the CLI must never send a capture body: it is the primary source")
	}
	if action["target"] != "task" || action["title"] != "Call" || action["dueDate"] != "2026-08-01" {
		t.Fatalf("action fields wrong: %#v", action)
	}
	if tags := action["tags"].([]any); tags[0] != "money" || tags[1] != "admin" {
		t.Fatalf("tags not split and trimmed: %#v", tags)
	}
}

func TestInboxProcessRequiresTarget(t *testing.T) {
	ts, seen := fakeInboxAPI(t, http.StatusOK, `{"status":"ok"}`)

	_, err := driveInbox(t, ts, "process", "cap-1")
	if exitCode(err) != 2 || !strings.Contains(err.Error(), "--target") {
		t.Fatalf("expected a usage error naming --target, got %v", err)
	}
	if len(*seen) != 0 {
		t.Fatal("a rejected invocation must not reach the server")
	}
}

func TestInboxConvertPostsAction(t *testing.T) {
	ts, seen := fakeInboxAPI(t, http.StatusOK, `{"status":"ok"}`)

	if _, err := driveInbox(t, ts, "convert", "cap-3", "bookmark"); err != nil {
		t.Fatalf("inbox convert: %v", err)
	}
	req := (*seen)[0]
	if req.path != "/api/inbox/cap-3/convert" || req.body["action"] != "bookmark" {
		t.Fatalf("unexpected convert request: %s %#v", req.path, req.body)
	}
}

func TestInboxReopenPostsToCapture(t *testing.T) {
	ts, seen := fakeInboxAPI(t, http.StatusOK, `{"status":"ok"}`)

	out, err := driveInbox(t, ts, "reopen", "cap-4", "--yes")
	if err != nil {
		t.Fatalf("inbox reopen: %v", err)
	}
	if (*seen)[0].method != http.MethodPost || (*seen)[0].path != "/api/inbox/cap-4/reopen" {
		t.Fatalf("unexpected reopen request: %s %s", (*seen)[0].method, (*seen)[0].path)
	}
	if !strings.Contains(out, "Reopened cap-4") {
		t.Fatalf("unexpected output: %q", out)
	}
}

// R2-006: reopen deletes the derived node — without --yes it must preview and
// write nothing, like sweep / epic abort / inbox batch apply.
func TestInboxReopenWithoutYesPreviewsAndWritesNothing(t *testing.T) {
	ts, seen := fakeInboxAPI(t, http.StatusOK, `{"status":"ok"}`)

	out, err := driveInbox(t, ts, "reopen", "cap-4")
	requireRefusedWithoutYes(t, err, "inbox reopen")
	if len(*seen) != 0 {
		t.Fatalf("reopen without --yes must not touch the server, hit %+v", *seen)
	}
	if !strings.Contains(out, "Would reopen cap-4") || !strings.Contains(out, "--yes") {
		t.Errorf("preview must name the capture and the confirmation flag, got: %q", out)
	}
}

func TestInboxClassifyPostsOnce(t *testing.T) {
	ts, seen := fakeInboxAPI(t, http.StatusOK, `{"status":"ok"}`)

	if _, err := driveInbox(t, ts, "classify"); err != nil {
		t.Fatalf("inbox classify: %v", err)
	}
	if (*seen)[0].method != http.MethodPost || (*seen)[0].path != "/api/inbox/classify" {
		t.Fatalf("unexpected classify request: %s %s", (*seen)[0].method, (*seen)[0].path)
	}
}

// An unconfigured LLM is a 503 server-side; the CLI must surface it, not
// swallow it into a success.
func TestInboxClassifySurfacesUnconfiguredLLM(t *testing.T) {
	ts, _ := fakeInboxAPI(t, http.StatusServiceUnavailable,
		`KERNL DISPATCH FAILURE: cannot classify — no LLM provider configured`)

	_, err := driveInbox(t, ts, "classify")
	if err == nil || !strings.Contains(err.Error(), "no LLM provider configured") {
		t.Fatalf("expected the server's loud failure to surface, got %v", err)
	}
}

func TestInboxAutoClassifyWithoutArgumentOnlyReads(t *testing.T) {
	ts, seen := fakeInboxAPI(t, http.StatusOK, `{"enabled":true,"llmConfigured":true}`)

	out, err := driveInbox(t, ts, "auto-classify")
	if err != nil {
		t.Fatalf("inbox auto-classify: %v", err)
	}
	for _, req := range *seen {
		if req.method != http.MethodGet {
			t.Fatalf("a bare read must never write, saw %s %s", req.method, req.path)
		}
	}
	if !strings.Contains(out, "auto-classify: on") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestInboxAutoClassifyOffPutsDisabled(t *testing.T) {
	ts, seen := fakeInboxAPI(t, http.StatusOK, `{"enabled":false}`)

	if _, err := driveInbox(t, ts, "auto-classify", "off"); err != nil {
		t.Fatalf("inbox auto-classify off: %v", err)
	}
	req := (*seen)[0]
	if req.method != http.MethodPut || req.path != "/api/inbox/auto-classify" {
		t.Fatalf("unexpected request: %s %s", req.method, req.path)
	}
	if req.body["enabled"] != false {
		t.Fatalf("expected enabled=false, got %#v", req.body)
	}
}

func TestInboxAutoClassifyRejectsUnknownValue(t *testing.T) {
	ts, seen := fakeInboxAPI(t, http.StatusOK, `{}`)

	_, err := driveInbox(t, ts, "auto-classify", "onn")
	if exitCode(err) != 2 {
		t.Fatalf("expected a usage error, got %v", err)
	}
	if len(*seen) != 0 {
		t.Fatal("a rejected value must not reach the server")
	}
}

func TestInboxPrepGeneratesAndShows(t *testing.T) {
	ts, seen := fakeInboxAPI(t, http.StatusOK, `{"id":"n1","title":"Prep","body":"context"}`)

	out, err := driveInbox(t, ts, "prep", "cap-5")
	if err != nil {
		t.Fatalf("inbox prep: %v", err)
	}
	if (*seen)[0].method != http.MethodPost || (*seen)[0].path != "/api/inbox/cap-5/prep" {
		t.Fatalf("unexpected prep request: %s %s", (*seen)[0].method, (*seen)[0].path)
	}
	if !strings.Contains(out, "Prep") || !strings.Contains(out, "context") {
		t.Fatalf("unexpected output: %q", out)
	}

	if _, err := driveInbox(t, ts, "prep", "--show", "cap-5"); err != nil {
		t.Fatalf("inbox prep --show: %v", err)
	}
	if (*seen)[1].method != http.MethodGet {
		t.Fatalf("--show must read, saw %s", (*seen)[1].method)
	}
}

func TestInboxRollupsPrintsDays(t *testing.T) {
	ts, seen := fakeInboxAPI(t, http.StatusOK,
		`{"rollups":[{"date":"2026-07-18","count":3,"captures":[]}]}`)

	out, err := driveInbox(t, ts, "rollups")
	if err != nil {
		t.Fatalf("inbox rollups: %v", err)
	}
	if (*seen)[0].path != "/api/inbox/rollups" {
		t.Fatalf("unexpected path: %s", (*seen)[0].path)
	}
	if !strings.Contains(out, "2026-07-18") || !strings.Contains(out, "3 captures") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestInboxBatchLogQueriesBatchID(t *testing.T) {
	ts, seen := fakeInboxAPI(t, http.StatusOK,
		`{"batchId":"b1","source":"whatsapp","contextTitle":"chat","rawEntries":[{"sequence":1,"body":"first\nsecond"}]}`)

	out, err := driveInbox(t, ts, "batch-log", "b 1")
	if err != nil {
		t.Fatalf("inbox batch-log: %v", err)
	}
	if (*seen)[0].query != "batchId=b+1" {
		t.Fatalf("batch id not query-escaped: %q", (*seen)[0].query)
	}
	if !strings.Contains(out, "whatsapp") || !strings.Contains(out, "first") || strings.Contains(out, "second") {
		t.Fatalf("expected the first line of each entry, got %q", out)
	}
}

func TestInboxNotFoundExitsTwo(t *testing.T) {
	// A 404 on a route where the id is the whole request is still a bad
	// invocation: the caller named something that does not exist.
	ts, _ := fakeInboxAPI(t, http.StatusNotFound, `no such capture`)

	_, err := driveInbox(t, ts, "reopen", "cap-nope", "--yes")
	if exitCode(err) != 2 {
		t.Fatalf("a 404 must exit 2 (bad invocation), got %d: %v", exitCode(err), err)
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Fatalf("expected a loud failure, got %v", err)
	}
}

func TestInboxPrepShowTreatsMissingPrepAsAnAnswer(t *testing.T) {
	// 'prep --show' asks whether a briefing exists. "It does not" answers the
	// question, so it exits 0 — the 404 underneath is not a mis-invocation.
	ts, _ := fakeInboxAPI(t, http.StatusNotFound, `no prep`)

	out, err := driveInbox(t, ts, "prep", "--show", "cap-nope")
	if err != nil {
		t.Fatalf("missing prep must not be an error, got: %v", err)
	}
	if !strings.Contains(out, "No prep for cap-nope yet") {
		t.Errorf("expected an explicit no-prep line, got %q", out)
	}
}

func TestInboxPrepShowJSONEmitsNullWhenMissing(t *testing.T) {
	// A script reading --json needs a parseable document, not empty output.
	ts, _ := fakeInboxAPI(t, http.StatusNotFound, `no prep`)

	out, err := driveInbox(t, ts, "prep", "--show", "--json", "cap-nope")
	if err != nil {
		t.Fatalf("missing prep must not be an error, got: %v", err)
	}
	var doc struct {
		Prep *string `json:"prep"`
	}
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &doc); jsonErr != nil {
		t.Fatalf("--json output is not valid JSON (%v): %q", jsonErr, out)
	}
	if doc.Prep != nil {
		t.Errorf("expected a null prep, got %v", *doc.Prep)
	}
}

func TestInboxUnknownSubcommandSuggests(t *testing.T) {
	err := runInbox(verbContext{}, []string{"lst"})
	if exitCode(err) != 2 || !strings.Contains(err.Error(), "list") {
		t.Fatalf("expected a did-you-mean toward list, got %v", err)
	}
}
