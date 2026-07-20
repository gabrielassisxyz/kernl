package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// triageRoutes serves one canned body per path. A path left out of the map
// answers 500, which is how the degradation tests below simulate one route
// being down while the others are fine.
func triageServer(t *testing.T, routes map[string]string) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := routes[r.URL.Path]
		w.Header().Set("Content-Type", "application/json")
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `{"error":"boom"}`)
			return
		}
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(ts.Close)
	return ts
}

// The fixtures below are deliberately spelled the way the SERVER spells them —
// inboxItemDTO and nodes.IngestReview — not the way triage happens to read them.
// encoding/json matches field names case-insensitively and silently ignores
// what it cannot place, so a decoder that names the wrong field does not fail:
// it returns a zero value. The first cut of this command guessed `body` and
// `proposedAction`, both wrong, and the only symptom against the live server
// was a listing of ids with blank titles.
func fullTriageRoutes() map[string]string {
	return map[string]string{
		"/api/inbox/pending": `[{"id":"cap-1","title":"buy milk\nand eggs","subtitle":"raw"},{"id":"cap-2","title":"","subtitle":"call the bank"}]`,
		"/api/ingest/queue":  `[{"id":"rv-1","title":"Paper on caching","action":"create-page"}]`,
		"/api/tasks":         `[{"id":"t-1","title":"Write the handoff","status":"open"},{"id":"t-2","title":"Old thing","status":"done"}]`,
		"/api/approvals":     `[{"id":"apr-1","summary":"Merge epic z4","state":"pending"}]`,
		"/api/health":        `{"status":"ok"}`,
		"/api/beads": `[{"id":"kn-1","title":"Running now","state":"implementation","assignee":"agent","priority":2},
		                {"id":"kn-2","title":"Free to start","state":"ready_for_implementation","priority":3},
		                {"id":"kn-3","title":"Already done","state":"closed","priority":1}]`,
	}
}

func runTriageAgainst(t *testing.T, ts *httptest.Server, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := runTriage(verbContext{server: ts.URL, out: &out}, args)
	return out.String(), err
}

func TestTriageReportsEverySliceInOneCall(t *testing.T) {
	out, err := runTriageAgainst(t, triageServer(t, fullTriageRoutes()), "--json")
	if err != nil {
		t.Fatalf("triage with every route healthy must succeed: %v", err)
	}
	var got triageReport
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("--json must emit one parseable document, got %q (%v)", out, err)
	}

	if got.Captures.Count != 2 || got.Captures.Items[0].ID != "cap-1" {
		t.Errorf("captures slice wrong: %+v", got.Captures)
	}
	// The title is the capture's first line: a multi-line title must not smear
	// across the report.
	if got.Captures.Items[0].Title != "buy milk" {
		t.Errorf("capture title must be the first line, got %q", got.Captures.Items[0].Title)
	}
	// An unclassified capture has no derived title yet; falling back to the
	// subtitle beats printing a bare uuid the caller cannot recognise.
	if got.Captures.Items[1].Title != "call the bank" {
		t.Errorf("a capture with no title must fall back to its subtitle, got %q", got.Captures.Items[1].Title)
	}
	if got.Ingest.Count != 1 || got.Ingest.Items[0].State != "create-page" {
		t.Errorf("ingest slice wrong: %+v", got.Ingest)
	}
	// A done task is not open work; counting it would inflate the one number a
	// caller uses to decide whether there is anything to do.
	if got.Tasks.Count != 1 || got.Tasks.Items[0].ID != "t-1" {
		t.Errorf("tasks slice must exclude done tasks: %+v", got.Tasks)
	}
	if got.Running.Count != 1 || got.Running.Items[0].ID != "kn-1" {
		t.Errorf("running slice wrong: %+v", got.Running)
	}
	if got.Ready.Count != 1 || got.Ready.Items[0].ID != "kn-2" {
		t.Errorf("ready slice must hold the unassigned ready_for_* bead: %+v", got.Ready)
	}
	if got.Health.Status != "ok" {
		t.Errorf("health slice wrong: %+v", got.Health)
	}
	// Every slice names the command that shows the rest — the whole point of a
	// mega-command is that its output tells you where to go next.
	for name, cmd := range map[string]string{
		"captures": got.Captures.Command, "ingest": got.Ingest.Command,
		"running": got.Running.Command, "ready": got.Ready.Command,
		"tasks": got.Tasks.Command, "approvals": got.Approvals.Command,
	} {
		if !strings.HasPrefix(cmd, "kernl ") {
			t.Errorf("%s slice must carry a runnable follow-up command, got %q", name, cmd)
		}
	}
}

// The finding this whole command waited on: approvals answers 501 today, and
// rendering that as "0 pending" would tell the human that no judgment gate is
// waiting on them — the one thing triage must never get wrong.
func TestTriageNeverReportsAnUnreadableSliceAsZero(t *testing.T) {
	routes := fullTriageRoutes()
	delete(routes, "/api/approvals") // now answers 500, standing in for the 501
	out, err := runTriageAgainst(t, triageServer(t, routes), "--json")
	if err != nil {
		t.Fatalf("one dead route must not fail the whole triage: %v", err)
	}
	var got triageReport
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("report must stay parseable: %v", err)
	}
	if got.Approvals.Available {
		t.Fatal("an unreadable approvals route must not be reported as available")
	}
	if got.Approvals.Reason == "" {
		t.Error("an unavailable slice must say why")
	}
	if !got.Captures.Available || got.Captures.Count != 2 {
		t.Error("the healthy slices must still answer when one route is down")
	}

	human, err := runTriageAgainst(t, triageServer(t, routes))
	if err != nil {
		t.Fatalf("human output must degrade the same way: %v", err)
	}
	if !strings.Contains(human, "approvals waiting: unavailable") {
		t.Errorf("human output must mark the slice unavailable, got:\n%s", human)
	}
	if strings.Contains(human, "approvals waiting: none") {
		t.Error("an unreadable slice must never render as 'none'")
	}
}

// A partial report is a successful triage: exiting non-zero because one route
// is unhappy would make the command useless in exactly the degraded conditions
// you most want to run it in.
func TestTriagePartialReportStillSucceeds(t *testing.T) {
	routes := map[string]string{"/api/health": `{"status":"ok"}`}
	if _, err := runTriageAgainst(t, triageServer(t, routes), "--json"); err != nil {
		t.Fatalf("a report with one live slice must exit 0, got: %v", err)
	}
}

func TestTriageFailsOnlyWhenNothingAnswers(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(ts.Close)

	out, err := runTriageAgainst(t, ts, "--json")
	if err == nil {
		t.Fatal("a triage that could read nothing must not exit 0")
	}
	if !strings.Contains(err.Error(), "kernl serve") {
		t.Errorf("the failure must name the likely cause, got: %v", err)
	}
	// The report was already written; main must not print past it.
	if !strings.Contains(out, `"available": false`) {
		t.Error("the report must still be emitted so the caller sees which slices failed")
	}
	var reported alreadyReported
	if !errors.As(err, &reported) {
		t.Error("the failure must be marked already-reported to avoid a duplicate message")
	}
}

// A fresh agent ran `kernl triage` from a directory with no kernl.yaml — the
// natural first move, since triage is the first verb in the help and says it
// reports what needs attention. It got six identical "run kernl from the
// directory containing kernl.yaml, or pass --config <path>" lines: one cause
// reported six times, and a remedy that cannot work, because there is no
// kernl.yaml anywhere to point --config at. The flag that works is --server,
// which the message never named.
func TestTriageWithNoServerAddressFailsOnceAndNamesTheFlagThatWorks(t *testing.T) {
	var out bytes.Buffer
	// No server, no config path: nothing to resolve an address from.
	err := runTriage(verbContext{out: &out, configPath: "/nonexistent/kernl.yaml"}, nil)
	if err == nil {
		t.Fatal("triage with no resolvable server must fail")
	}
	if exitCode(err) != 2 {
		t.Errorf("an unusable invocation must exit 2, got %d", exitCode(err))
	}
	msg := err.Error()
	if !strings.Contains(msg, "--server") {
		t.Errorf("the fix must name --server, the flag that actually works here: %s", msg)
	}
	if strings.Count(msg, "Fix:") != 1 {
		t.Errorf("exactly one remedy, not a correct one stacked on a wrong one: %s", msg)
	}
	// Reported once, not once per section: the sections never ran.
	if out.Len() != 0 {
		t.Errorf("no per-section report should be written when the address never resolved, got: %s", out.String())
	}
}

// Truncating a list is good manners; truncating an error removes the reason it
// was printed. The remedy comes last in these messages, so a naive cut always
// takes the actionable half — and mid-word, leaving "--config <path-to-ker…".
func TestTriageReasonKeepsTheRemedyAndCutsOnWords(t *testing.T) {
	long := "KERNL DISPATCH FAILURE: " + strings.Repeat("diagnosis words that go on and on ", 12) +
		"— Fix: pass --server <url> or set KERNL_SERVER"
	got := triageReason(errors.New(long))

	if !strings.Contains(got, "--server <url>") {
		t.Errorf("the remedy must survive truncation intact, got: %q", got)
	}
	if !strings.Contains(got, "…") {
		t.Errorf("an over-long reason should show that it was cut, got: %q", got)
	}
	for _, fragment := range []string{"<url", "--serv"} {
		if strings.HasSuffix(strings.TrimSuffix(got, "… "), fragment) {
			t.Errorf("cut left an unusable fragment: %q", got)
		}
	}
}

func TestTriageRejectsArgumentsAndUnknownFlags(t *testing.T) {
	ts := triageServer(t, fullTriageRoutes())
	for _, args := range [][]string{{"extra"}, {"--nope"}} {
		_, err := runTriageAgainst(t, ts, args...)
		if err == nil {
			t.Fatalf("triage %v must be refused", args)
		}
		if exitCode(err) != 2 {
			t.Errorf("triage %v must exit 2, got %d", args, exitCode(err))
		}
	}
}

// The alias map is what makes the first thing an agent guesses work: `kernl
// status` used to answer `kernl epic list`, which omits captures, tasks and
// approvals entirely.
func TestWhatDoINowAliasesPointAtTriage(t *testing.T) {
	for _, verb := range []string{"status", "next", "ready", "todo"} {
		if got := verbAliasHints[verb]; got != "kernl triage" {
			t.Errorf("%q must redirect to kernl triage, got %q", verb, got)
		}
	}
}
