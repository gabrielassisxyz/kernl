package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// noteAPI is a stand-in for a running `kernl serve`: it records what the verb
// asked for so a test can assert the method, path and body the CLI produced.
type noteAPI struct {
	server   *httptest.Server
	requests []recordedRequest
}

type recordedRequest struct {
	method string
	path   string
	query  string
	body   string
}

func newNoteAPI(t *testing.T, respond func(w http.ResponseWriter, r *http.Request)) *noteAPI {
	t.Helper()
	api := &noteAPI{}
	api.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		api.requests = append(api.requests, recordedRequest{
			method: r.Method, path: r.URL.Path, query: r.URL.RawQuery, body: string(body),
		})
		respond(w, r)
	}))
	t.Cleanup(api.server.Close)
	return api
}

func (a *noteAPI) run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := runNote(verbContext{server: a.server.URL, out: &out}, args)
	return out.String(), err
}

func jsonResponse(payload string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, payload)
	}
}

func TestNoteListPrintsPathTypeAndTitle(t *testing.T) {
	api := newNoteAPI(t, jsonResponse(`[{"path":"a.md","id":"1","type":"note","title":"A"}]`))
	out, err := api.run(t, "list")
	if err != nil {
		t.Fatalf("note list: %v", err)
	}
	if api.requests[0].method != http.MethodGet || api.requests[0].path != "/api/vault/notes" {
		t.Fatalf("wrong call: %+v", api.requests[0])
	}
	if !strings.Contains(out, "a.md\tnote\tA") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestNoteListFilesUsesVaultList(t *testing.T) {
	api := newNoteAPI(t, jsonResponse(`{"files":["a.md","sub/b.md"]}`))
	out, err := api.run(t, "list", "--files")
	if err != nil {
		t.Fatalf("note list --files: %v", err)
	}
	if api.requests[0].path != "/api/vault/list" {
		t.Fatalf("wrong route: %+v", api.requests[0])
	}
	if !strings.Contains(out, "sub/b.md") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestNoteReadWritesRawBytes(t *testing.T) {
	api := newNoteAPI(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "# Title\nbody\n")
	})
	out, err := api.run(t, "read", "notes/x.md")
	if err != nil {
		t.Fatalf("note read: %v", err)
	}
	if out != "# Title\nbody\n" {
		t.Fatalf("read must pipe byte-exact, got %q", out)
	}
	if api.requests[0].query != "path=notes%2Fx.md" {
		t.Fatalf("path must travel as a query param, got %q", api.requests[0].query)
	}
}

func TestNoteReadJSONWrapsTheTextBody(t *testing.T) {
	api := newNoteAPI(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "hello")
	})
	out, err := api.run(t, "read", "x.md", "--json")
	if err != nil {
		t.Fatalf("note read --json: %v", err)
	}
	var decoded map[string]string
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("--json must emit JSON, got %q", out)
	}
	if decoded["content"] != "hello" || decoded["path"] != "x.md" {
		t.Fatalf("unexpected envelope: %v", decoded)
	}
}

func TestNoteWriteSendsFileBodyVerbatim(t *testing.T) {
	api := newNoteAPI(t, jsonResponse(`{"status":"saved"}`))
	local := filepath.Join(t.TempDir(), "draft.md")
	if err := os.WriteFile(local, []byte("# Draft\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := api.run(t, "write", "notes/x.md", "--file", local); err != nil {
		t.Fatalf("note write: %v", err)
	}
	req := api.requests[0]
	if req.method != http.MethodPost || req.path != "/api/vault/file" {
		t.Fatalf("wrong call: %+v", req)
	}
	// The markdown must arrive raw — JSON-encoding it would write a quoted
	// string into the vault.
	if req.body != "# Draft\n" {
		t.Fatalf("body must be verbatim markdown, got %q", req.body)
	}
}

func TestNoteWriteWithoutSourceOrStdinFailsLoud(t *testing.T) {
	api := newNoteAPI(t, jsonResponse(`{}`))
	_, err := api.run(t, "write", "notes/x.md", "--file", "  ")
	if err == nil || exitCode(err) != 2 {
		t.Fatalf("empty --file must be a usage error, got: %v", err)
	}
	if len(api.requests) != 0 {
		t.Fatalf("nothing must be written, got %d requests", len(api.requests))
	}
}

func TestNoteDeleteRequiresYes(t *testing.T) {
	api := newNoteAPI(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	out, err := api.run(t, "delete", "notes/x.md")
	requireRefusedWithoutYes(t, err, "note delete")
	if len(api.requests) != 0 {
		t.Fatalf("delete without --yes must not call the API, got %+v", api.requests)
	}
	if !strings.Contains(out, "Would delete notes/x.md") {
		t.Fatalf("preview must name the file, got %q", out)
	}
}

func TestNoteDeleteWithYesIssuesDelete(t *testing.T) {
	api := newNoteAPI(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	out, err := api.run(t, "delete", "notes/x.md", "--yes")
	if err != nil {
		t.Fatalf("note delete --yes: %v", err)
	}
	if api.requests[0].method != http.MethodDelete || api.requests[0].path != "/api/vault/file" {
		t.Fatalf("wrong call: %+v", api.requests[0])
	}
	if !strings.Contains(out, "Deleted notes/x.md") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestNoteTagsListsTagAndCount(t *testing.T) {
	api := newNoteAPI(t, jsonResponse(`{"work":{"files":["a.md","b.md"]},"idea":{"files":["c.md"]}}`))
	out, err := api.run(t, "tags")
	if err != nil {
		t.Fatalf("note tags: %v", err)
	}
	if api.requests[0].path != "/api/notes/tags" {
		t.Fatalf("wrong route: %+v", api.requests[0])
	}
	// Sorted, so repeated runs diff cleanly.
	if !strings.HasPrefix(out, "idea\t1\nwork\t2\n") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestNoteSuggestSendsPathAndInstruction(t *testing.T) {
	api := newNoteAPI(t, jsonResponse(`{"hunks":[{"id":"h1","from":0,"to":4,"content":"new"}]}`))
	out, err := api.run(t, "suggest", "x.md", "--instruction", "tighten it")
	if err != nil {
		t.Fatalf("note suggest: %v", err)
	}
	if api.requests[0].path != "/api/notes/suggest" {
		t.Fatalf("wrong route: %+v", api.requests[0])
	}
	var sent map[string]string
	if err := json.Unmarshal([]byte(api.requests[0].body), &sent); err != nil {
		t.Fatalf("body must be JSON: %v", err)
	}
	if sent["path"] != "x.md" || sent["instruction"] != "tighten it" {
		t.Fatalf("unexpected request body: %v", sent)
	}
	if !strings.Contains(out, "h1 [0:4]") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestNoteSuggestWithoutInstructionIsUsageError(t *testing.T) {
	api := newNoteAPI(t, jsonResponse(`{}`))
	_, err := api.run(t, "suggest", "x.md")
	if err == nil || exitCode(err) != 2 {
		t.Fatalf("missing --instruction must exit 2, got: %v", err)
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Fatalf("errors must carry the marker, got: %v", err)
	}
}

func TestNoteApplyHunksAcceptsASuggestResponse(t *testing.T) {
	api := newNoteAPI(t, jsonResponse(`{"status":"applied","last_modified":"2026-07-18T10:00:00Z"}`))
	local := filepath.Join(t.TempDir(), "hunks.json")
	if err := os.WriteFile(local, []byte(`{"hunks":[{"id":"h1","from":0,"to":4,"content":"new"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := api.run(t, "apply-hunks", "x.md", "--hunks", local)
	if err != nil {
		t.Fatalf("note apply-hunks: %v", err)
	}
	if api.requests[0].path != "/api/notes/apply-hunks" {
		t.Fatalf("wrong route: %+v", api.requests[0])
	}
	var sent struct {
		Path  string `json:"path"`
		Hunks []struct {
			ID string `json:"id"`
		} `json:"hunks"`
	}
	if err := json.Unmarshal([]byte(api.requests[0].body), &sent); err != nil {
		t.Fatalf("body must be JSON: %v", err)
	}
	if sent.Path != "x.md" || len(sent.Hunks) != 1 || sent.Hunks[0].ID != "h1" {
		t.Fatalf("hunks must survive the round-trip, got %+v", sent)
	}
	if !strings.Contains(out, "Applied 1 hunk(s) to x.md") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestNoteAPI404ExitsTwo(t *testing.T) {
	api := newNoteAPI(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no such file", http.StatusNotFound)
	})
	_, err := api.run(t, "read", "missing.md")
	if err == nil {
		t.Fatal("a 404 must surface as an error")
	}
	if exitCode(err) != 2 {
		t.Fatalf("a 4xx is an invocation problem (exit 2), got %d: %v", exitCode(err), err)
	}
}

func TestNoteUnknownSubcommandHints(t *testing.T) {
	err := runNote(verbContext{server: "http://127.0.0.1:1"}, []string{"lst"})
	if err == nil || !strings.Contains(err.Error(), `did you mean "list"?`) {
		t.Fatalf("typo'd subcommand must hint, got: %v", err)
	}
	if exitCode(err) != 2 {
		t.Fatalf("want usage error, got %d", exitCode(err))
	}
}
