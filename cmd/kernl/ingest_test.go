package main

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ingestCall is what the fake API saw, so a test can assert the CLI sent the
// right method, path and payload rather than only reading its output.
type ingestCall struct {
	method      string
	path        string
	contentType string
	body        map[string]any
	rawBody     []byte
}

// fakeIngestAPI stands in for a running `kernl serve`: it records every request
// and replies with canned JSON, keeping these tests off the host and network.
func fakeIngestAPI(t *testing.T, status int, response string) (*httptest.Server, *[]ingestCall) {
	t.Helper()
	var seen []ingestCall
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := ingestCall{method: r.Method, path: r.URL.Path, contentType: r.Header.Get("Content-Type")}
		rec.rawBody, _ = io.ReadAll(r.Body)
		if len(rec.rawBody) > 0 {
			_ = json.Unmarshal(rec.rawBody, &rec.body)
		}
		seen = append(seen, rec)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, response)
	}))
	t.Cleanup(ts.Close)
	return ts, &seen
}

// driveIngest runs one ingest invocation against the fake server.
func driveIngest(t *testing.T, ts *httptest.Server, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := runIngest(verbContext{server: ts.URL, out: &out}, args)
	return out.String(), err
}

func TestIngestPasteSendsTextAndTitle(t *testing.T) {
	ts, seen := fakeIngestAPI(t, http.StatusAccepted, "")

	out, err := driveIngest(t, ts, "paste", "--title", "Meeting", "raw", "notes")
	if err != nil {
		t.Fatalf("ingest paste: %v", err)
	}
	req := (*seen)[0]
	if req.method != http.MethodPost || req.path != "/api/ingest/paste" {
		t.Fatalf("expected POST /api/ingest/paste, got %s %s", req.method, req.path)
	}
	if req.body["text"] != "raw notes" || req.body["title"] != "Meeting" {
		t.Fatalf("unexpected paste payload: %#v", req.body)
	}
	if !strings.Contains(out, "queue list") {
		t.Fatalf("expected the follow-up hint, got %q", out)
	}
}

func TestIngestPasteSentinelKeepsFlagLookingText(t *testing.T) {
	ts, seen := fakeIngestAPI(t, http.StatusAccepted, "")

	if _, err := driveIngest(t, ts, "paste", "--", "--title", "is text"); err != nil {
		t.Fatalf("ingest paste --: %v", err)
	}
	if body := (*seen)[0].body; body["text"] != "--title is text" || body["title"] != nil {
		t.Fatalf("text after -- must stay text: %#v", body)
	}
}

func TestIngestPasteJSONEmitsAckForEmptyBody(t *testing.T) {
	ts, _ := fakeIngestAPI(t, http.StatusAccepted, "")

	out, err := driveIngest(t, ts, "paste", "--json", "some text")
	if err != nil {
		t.Fatalf("ingest paste --json: %v", err)
	}
	var ack map[string]string
	if err := json.Unmarshal([]byte(out), &ack); err != nil || ack["status"] != "accepted" {
		t.Fatalf("--json must stay parseable on a 202 with no body, got %q (%v)", out, err)
	}
}

func TestIngestUploadSendsMultipartFileField(t *testing.T) {
	ts, seen := fakeIngestAPI(t, http.StatusAccepted, "")
	path := filepath.Join(t.TempDir(), "notes.md")
	if err := os.WriteFile(path, []byte("# heading\n\nbody"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	if _, err := driveIngest(t, ts, "upload", path); err != nil {
		t.Fatalf("ingest upload: %v", err)
	}
	req := (*seen)[0]
	if req.path != "/api/ingest/upload" {
		t.Fatalf("expected /api/ingest/upload, got %s", req.path)
	}
	mediaType, params, err := mime.ParseMediaType(req.contentType)
	if err != nil || mediaType != "multipart/form-data" {
		t.Fatalf("expected multipart/form-data, got %q (%v)", req.contentType, err)
	}
	part, err := multipart.NewReader(bytes.NewReader(req.rawBody), params["boundary"]).NextPart()
	if err != nil {
		t.Fatalf("reading the uploaded part: %v", err)
	}
	if part.FormName() != "file" || part.FileName() != "notes.md" {
		t.Fatalf("expected field \"file\" named notes.md, got %q/%q", part.FormName(), part.FileName())
	}
	content, _ := io.ReadAll(part)
	if string(content) != "# heading\n\nbody" {
		t.Fatalf("file content not sent verbatim: %q", content)
	}
}

func TestIngestUploadMissingFileFailsBeforeTheServer(t *testing.T) {
	ts, seen := fakeIngestAPI(t, http.StatusAccepted, "")

	_, err := driveIngest(t, ts, "upload", filepath.Join(t.TempDir(), "absent.md"))
	if err == nil || !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Fatalf("expected a loud read failure, got %v", err)
	}
	if len(*seen) != 0 {
		t.Fatal("an unreadable file must not reach the server")
	}
}

func TestIngestSourcePostsURLAndPrintsSourceNode(t *testing.T) {
	ts, seen := fakeIngestAPI(t, http.StatusAccepted,
		`{"sourceNodeId":"bk-1","title":"A page","kind":"article"}`)

	out, err := driveIngest(t, ts, "source", "https://example.com/a", "--kind", "article")
	if err != nil {
		t.Fatalf("ingest source: %v", err)
	}
	req := (*seen)[0]
	if req.path != "/api/ingest/source" || req.body["url"] != "https://example.com/a" || req.body["kind"] != "article" {
		t.Fatalf("unexpected source request: %s %#v", req.path, req.body)
	}
	if !strings.Contains(out, "bk-1") {
		t.Fatalf("expected the source node id in output, got %q", out)
	}
}

func TestIngestTriggerSendsSnakeCasePath(t *testing.T) {
	ts, seen := fakeIngestAPI(t, http.StatusAccepted, "")

	if _, err := driveIngest(t, ts, "trigger", "/vault/inbox/a.md", "--node", "nd-1"); err != nil {
		t.Fatalf("ingest trigger: %v", err)
	}
	req := (*seen)[0]
	if req.path != "/api/ingest/trigger" {
		t.Fatalf("expected /api/ingest/trigger, got %s", req.path)
	}
	if req.body["file_path"] != "/vault/inbox/a.md" || req.body["node_id"] != "nd-1" {
		t.Fatalf("trigger payload must use the handler's snake_case keys: %#v", req.body)
	}
}

func TestIngestQueueListPrintsReviews(t *testing.T) {
	ts, seen := fakeIngestAPI(t, http.StatusOK,
		`[{"ID":"rv-1","Title":"Kubernetes notes","Action":"Create Page"}]`)

	out, err := driveIngest(t, ts, "queue", "list")
	if err != nil {
		t.Fatalf("ingest queue list: %v", err)
	}
	if (*seen)[0].method != http.MethodGet || (*seen)[0].path != "/api/ingest/queue" {
		t.Fatalf("unexpected queue request: %s %s", (*seen)[0].method, (*seen)[0].path)
	}
	if !strings.Contains(out, "rv-1") || !strings.Contains(out, "Kubernetes notes") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestIngestQueueResolveMapsActionAndEscapesID(t *testing.T) {
	ts, seen := fakeIngestAPI(t, http.StatusOK, "")

	if _, err := driveIngest(t, ts, "queue", "resolve", "rv 1",
		"--action", "create-page"); err != nil {
		t.Fatalf("ingest queue resolve: %v", err)
	}
	req := (*seen)[0]
	if req.path != "/api/ingest/queue/rv 1/resolve" {
		t.Fatalf("review id not path-escaped: %q", req.path)
	}
	if req.body["action"] != "Create Page" {
		t.Fatalf("expected the API's action literal, got %#v", req.body)
	}
}

func TestIngestQueueResolveRejectsUnknownAction(t *testing.T) {
	ts, seen := fakeIngestAPI(t, http.StatusOK, "")

	_, err := driveIngest(t, ts, "queue", "resolve", "rv-1", "--action", "creat-page")
	if exitCode(err) != 2 || !strings.Contains(err.Error(), "create-page") {
		t.Fatalf("expected a did-you-mean toward create-page, got %v", err)
	}
	if len(*seen) != 0 {
		t.Fatal("a rejected action must not reach the server")
	}
}

// A forgotten --action used to mean "Skip", which deletes the review. The verb
// must refuse rather than pick the destructive branch for the caller.
func TestIngestQueueResolveRequiresAction(t *testing.T) {
	ts, seen := fakeIngestAPI(t, http.StatusOK, "")

	_, err := driveIngest(t, ts, "queue", "resolve", "rv-1")
	if exitCode(err) != 2 || !strings.Contains(err.Error(), "requires --action") {
		t.Fatalf("expected exit 2 demanding --action, got %d: %v", exitCode(err), err)
	}
	if len(*seen) != 0 {
		t.Fatal("a missing action must not reach the server")
	}
}

// The shell token is 'discard' — the wire value stays "Skip", which the GUI sends.
func TestIngestQueueResolveDiscardSendsSkip(t *testing.T) {
	ts, seen := fakeIngestAPI(t, http.StatusOK, "")

	if _, err := driveIngest(t, ts, "queue", "resolve", "rv-1", "--action", "discard"); err != nil {
		t.Fatalf("ingest queue resolve --action discard: %v", err)
	}
	if len(*seen) != 1 || (*seen)[0].body["action"] != "Skip" {
		t.Fatalf("expected the wire value Skip, got %#v", *seen)
	}
}

// 'skip' is gone with no alias — it read as "leave it for later" while deleting.
func TestIngestQueueResolveRejectsRetiredSkipToken(t *testing.T) {
	ts, seen := fakeIngestAPI(t, http.StatusOK, "")

	_, err := driveIngest(t, ts, "queue", "resolve", "rv-1", "--action", "skip")
	if exitCode(err) != 2 || !strings.Contains(err.Error(), "discard") {
		t.Fatalf("expected exit 2 pointing at discard, got %d: %v", exitCode(err), err)
	}
	if len(*seen) != 0 {
		t.Fatal("a rejected action must not reach the server")
	}
}

func TestIngestQueueMergePlanSummarizesHunks(t *testing.T) {
	ts, seen := fakeIngestAPI(t, http.StatusOK,
		`{"targetNoteId":"nt-1","targetTitle":"Kubernetes","hunks":[{"text":"a"},{"text":"b"}]}`)

	out, err := driveIngest(t, ts, "queue", "merge-plan", "rv-1")
	if err != nil {
		t.Fatalf("ingest queue merge-plan: %v", err)
	}
	if (*seen)[0].path != "/api/ingest/queue/rv-1/merge-plan" {
		t.Fatalf("unexpected path: %s", (*seen)[0].path)
	}
	if !strings.Contains(out, "2 hunk(s)") {
		t.Fatalf("unexpected output: %q", out)
	}
}

// An unconfigured LLM is a 503 server-side; the CLI must surface it verbatim,
// not swallow it into a generic message.
func TestIngestSurfacesUnconfiguredLLM(t *testing.T) {
	ts, _ := fakeIngestAPI(t, http.StatusServiceUnavailable,
		"ingest requires an LLM provider; set llm.provider in kernl.yaml")

	_, err := driveIngest(t, ts, "queue", "resolve", "rv-1", "--action", "create-page")
	if err == nil || !strings.Contains(err.Error(), "set llm.provider in kernl.yaml") {
		t.Fatalf("expected the server's own message to surface, got %v", err)
	}
}

func TestIngestNotFoundExitsTwo(t *testing.T) {
	ts, _ := fakeIngestAPI(t, http.StatusNotFound, "no such review")

	_, err := driveIngest(t, ts, "queue", "merge-plan", "rv-nope")
	if exitCode(err) != 2 {
		t.Fatalf("a 404 must exit 2 (bad invocation), got %d: %v", exitCode(err), err)
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Fatalf("expected a loud failure, got %v", err)
	}
}

func TestIngestUnknownSubcommandSuggests(t *testing.T) {
	err := runIngest(verbContext{}, []string{"que"})
	if exitCode(err) != 2 || !strings.Contains(err.Error(), "queue") {
		t.Fatalf("expected a did-you-mean toward queue, got %v", err)
	}
}
