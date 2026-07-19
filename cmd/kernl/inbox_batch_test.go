package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const batchFixture = "[1/2/26, 09:00] Me: buy milk\n[1/2/26, 09:01] Me: call the vet\n"

// writeBatchFixture puts the export in a file so the test drives --file rather
// than stdin, which a hermetic test cannot own.
func writeBatchFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "chat.txt")
	if err := os.WriteFile(path, []byte(batchFixture), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestInboxBatchPreviewSendsTextVerbatim(t *testing.T) {
	ts, seen := fakeInboxAPI(t, http.StatusOK,
		`{"source":"whatsapp","separator":"whatsapp","suggestedContextTitle":"chat","segments":[{"sequence":1,"body":"buy milk"},{"sequence":2,"body":"call the vet"}]}`)

	out, err := driveInbox(t, ts, "batch", "preview", "--file", writeBatchFixture(t), "--split", "whatsapp")
	if err != nil {
		t.Fatalf("inbox batch preview: %v", err)
	}
	req := (*seen)[0]
	if req.method != http.MethodPost || req.path != "/api/inbox/batch/preview" {
		t.Fatalf("expected POST /api/inbox/batch/preview, got %s %s", req.method, req.path)
	}
	if req.body["text"] != batchFixture {
		t.Fatalf("the pasted text must travel untouched, got %#v", req.body["text"])
	}
	if req.body["splitMode"] != "whatsapp" {
		t.Fatalf("unexpected split mode: %#v", req.body)
	}
	if !strings.Contains(out, "buy milk") || !strings.Contains(out, "2 segment(s)") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestInboxBatchAnalyzeHitsAnalyzeRoute(t *testing.T) {
	ts, seen := fakeInboxAPI(t, http.StatusOK,
		`{"source":"text","separator":"lines","suggestedContextTitle":"errands","segments":[{"sequence":1,"body":"buy milk"}]}`)

	if _, err := driveInbox(t, ts, "batch", "analyze", "--file", writeBatchFixture(t), "--title", "errands"); err != nil {
		t.Fatalf("inbox batch analyze: %v", err)
	}
	req := (*seen)[0]
	if req.path != "/api/inbox/batch/analyze" || req.body["contextTitle"] != "errands" {
		t.Fatalf("unexpected analyze request: %s %#v", req.path, req.body)
	}
}

// The write path must never be reached by accident: an apply without --yes is
// answered by the preview route, which creates nothing.
func TestInboxBatchApplyWithoutYesOnlyPreviews(t *testing.T) {
	ts, seen := fakeInboxAPI(t, http.StatusOK,
		`{"source":"whatsapp","separator":"whatsapp","segments":[{"sequence":1,"body":"buy milk"}]}`)

	out, err := driveInbox(t, ts, "batch", "apply", "--file", writeBatchFixture(t))
	requireRefusedWithoutYes(t, err, "inbox batch apply")
	if (*seen)[0].path != "/api/inbox/batch/preview" {
		t.Fatalf("an unconfirmed apply must not write, it hit %s", (*seen)[0].path)
	}
	if !strings.Contains(out, "--yes") {
		t.Fatalf("expected the confirmation hint, got %q", out)
	}
}

// R2-006: under --json, an unconfirmed apply must state applied:false so a
// caller cannot read the preview body as a completed apply.
func TestInboxBatchApplyWithoutYesJSONSaysNotApplied(t *testing.T) {
	ts, seen := fakeInboxAPI(t, http.StatusOK,
		`{"source":"whatsapp","separator":"whatsapp","segments":[{"sequence":1,"body":"buy milk"}]}`)

	out, err := driveInbox(t, ts, "batch", "apply", "--json", "--file", writeBatchFixture(t))
	requireRefusedWithoutYes(t, err, "inbox batch apply")
	if (*seen)[0].path != "/api/inbox/batch/preview" {
		t.Fatalf("an unconfirmed apply must not write, it hit %s", (*seen)[0].path)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("--json output must parse, got %q (%v)", out, err)
	}
	if got["applied"] != false || got["wouldApply"] != true {
		t.Errorf("unconfirmed apply must report applied:false, wouldApply:true — got %v", got)
	}
}

func TestInboxBatchApplySendsNoClientRewrittenSegments(t *testing.T) {
	ts, seen := fakeInboxAPI(t, http.StatusCreated, `{"batchId":"b-1","ids":["cap-1","cap-2"]}`)

	out, err := driveInbox(t, ts, "batch", "apply", "--yes", "--file", writeBatchFixture(t))
	if err != nil {
		t.Fatalf("inbox batch apply --yes: %v", err)
	}
	req := (*seen)[0]
	if req.path != "/api/inbox/batch" {
		t.Fatalf("expected POST /api/inbox/batch, got %s", req.path)
	}
	for _, field := range []string{"rawSegments", "finalSegments"} {
		if _, present := req.body[field]; present {
			t.Fatalf("the CLI must never send %s: capture bodies are the primary source (%#v)", field, req.body)
		}
	}
	if req.body["text"] != batchFixture {
		t.Fatalf("the pasted text must travel untouched, got %#v", req.body["text"])
	}
	if !strings.Contains(out, "b-1") || !strings.Contains(out, "2 capture(s)") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestInboxBatchRejectsPositionalText(t *testing.T) {
	ts, seen := fakeInboxAPI(t, http.StatusOK, `{}`)

	_, err := driveInbox(t, ts, "batch", "preview", "buy milk")
	if exitCode(err) != 2 || !strings.Contains(err.Error(), "--file") {
		t.Fatalf("expected a usage error pointing at --file, got %v", err)
	}
	if len(*seen) != 0 {
		t.Fatal("a rejected invocation must not reach the server")
	}
}

func TestInboxBatchUnknownSubcommandSuggests(t *testing.T) {
	ts, _ := fakeInboxAPI(t, http.StatusOK, `{}`)

	_, err := driveInbox(t, ts, "batch", "previw")
	if exitCode(err) != 2 || !strings.Contains(err.Error(), "preview") {
		t.Fatalf("expected a did-you-mean toward preview, got %v", err)
	}
}
