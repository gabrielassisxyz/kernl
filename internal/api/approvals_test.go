package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/approvals"
)

func approvalTestApp(t *testing.T) (*app.App, *approvals.Store) {
	t.Helper()
	store, err := approvals.NewStore(filepath.Join(t.TempDir(), "approvals"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	a := testApp()
	a.Approvals = store
	return a, store
}

func parkGate(t *testing.T, store *approvals.Store, session, tool string) *approvals.ApprovalRequest {
	t.Helper()
	req, err := store.Create(&approvals.ApprovalRequest{
		SessionID: session, ToolName: tool, BeadID: "kernl-1", RepoPath: "/repo",
		SupportedActions: approvals.SessionActions,
	}, 30*time.Minute)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return req
}

func post(t *testing.T, r http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestListApprovalsServesTheParkedGates(t *testing.T) {
	a, store := approvalTestApp(t)
	parkGate(t, store, "sess-1", "Bash")
	r := NewRouter(a)

	req := httptest.NewRequest("GET", "/api/approvals", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", w.Code, w.Body.String())
	}
	// A bare array: the CLI decodes this straight into a slice, and an object
	// wrapper would break `kernl approval list` without breaking any test here
	// unless this one asserts the shape.
	var got []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("body is not a JSON array: %v (%s)", err, w.Body.String())
	}
	if len(got) != 1 || got[0]["toolName"] != "Bash" {
		t.Fatalf("body = %s", w.Body.String())
	}
	if got[0]["actionable"] != true || got[0]["status"] != "pending" {
		t.Errorf("a parked gate must list as pending and actionable: %s", w.Body.String())
	}
}

func TestListApprovalsIsAnEmptyArrayNotNull(t *testing.T) {
	a, _ := approvalTestApp(t)
	req := httptest.NewRequest("GET", "/api/approvals", nil)
	w := httptest.NewRecorder()
	NewRouter(a).ServeHTTP(w, req)

	if strings.TrimSpace(w.Body.String()) != "[]" {
		t.Errorf("an idle gate must serve [], got %q", w.Body.String())
	}
}

func TestResolveApprovalAppliesTheGateVocabulary(t *testing.T) {
	for _, tc := range []struct{ action, wantStatus string }{
		{"approve", "approved"},
		{"reject", "rejected"},
	} {
		t.Run(tc.action, func(t *testing.T) {
			a, store := approvalTestApp(t)
			gate := parkGate(t, store, "sess-1", "Bash")

			w := post(t, NewRouter(a), "/api/approvals/"+gate.ID+"/actions", `{"action":"`+tc.action+`"}`)
			if w.Code != http.StatusOK {
				t.Fatalf("want 200, got %d (%s)", w.Code, w.Body.String())
			}

			after, err := store.Get(gate.ID)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if after.Status != tc.wantStatus {
				t.Errorf("want %s, got %s", tc.wantStatus, after.Status)
			}
		})
	}
}

// The regression this whole subsystem exists to prevent: POST used to report
// success for an id that never existed, so `kernl approval resolve apr-999`
// printed "Resolved" at exit 0.
func TestResolveUnknownApprovalIs404(t *testing.T) {
	a, _ := approvalTestApp(t)

	w := post(t, NewRouter(a), "/api/approvals/apr-999/actions", `{"action":"approve"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 for an approval that never existed, got %d (%s)", w.Code, w.Body.String())
	}
}

func TestResolveAlreadyAnsweredIs409(t *testing.T) {
	a, store := approvalTestApp(t)
	gate := parkGate(t, store, "sess-1", "Bash")
	r := NewRouter(a)

	if w := post(t, r, "/api/approvals/"+gate.ID+"/actions", `{"action":"approve"}`); w.Code != http.StatusOK {
		t.Fatalf("first answer: %d (%s)", w.Code, w.Body.String())
	}
	w := post(t, r, "/api/approvals/"+gate.ID+"/actions", `{"action":"reject"}`)
	if w.Code != http.StatusConflict {
		t.Errorf("want 409 for a second answer, got %d (%s)", w.Code, w.Body.String())
	}
}

// Each route accepts only the vocabulary it advertises. A session word sent to
// the gate route means the caller expected different semantics than this route
// implements, and acting on it anyway hides that.
func TestEachRouteRefusesTheOtherVocabulary(t *testing.T) {
	a, store := approvalTestApp(t)
	gate := parkGate(t, store, "sess-1", "Bash")
	r := NewRouter(a)

	w := post(t, r, "/api/approvals/"+gate.ID+"/actions", `{"action":"always_approve"}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("the gate route must refuse a session action, got %d", w.Code)
	}
	w = post(t, r, "/api/terminal/sess-1/approvals/"+gate.ID, `{"action":"approve"}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("the session route must refuse a gate action, got %d", w.Code)
	}
}

func TestSessionRouteAnswersWithTheSessionVocabulary(t *testing.T) {
	a, store := approvalTestApp(t)
	gate := parkGate(t, store, "sess-1", "Bash")

	w := post(t, NewRouter(a), "/api/terminal/sess-1/approvals/"+gate.ID, `{"action":"always_approve"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", w.Code, w.Body.String())
	}

	remembered, err := store.RememberedAllow("sess-1", "Bash")
	if err != nil {
		t.Fatalf("RememberedAllow: %v", err)
	}
	if !remembered {
		t.Error("always_approve must silence the next prompt of the same shape")
	}
}

// The session segment must be load-bearing: without this check a mistyped
// session id would answer a gate belonging to another run.
func TestSessionRouteRefusesAnApprovalFromAnotherSession(t *testing.T) {
	a, store := approvalTestApp(t)
	gate := parkGate(t, store, "sess-1", "Bash")

	w := post(t, NewRouter(a), "/api/terminal/sess-OTHER/approvals/"+gate.ID, `{"action":"accept"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d (%s)", w.Code, w.Body.String())
	}

	after, err := store.Get(gate.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if after.Status != "pending" {
		t.Errorf("the gate must be untouched, got %s", after.Status)
	}
}

// An App with no store cannot report anything, and "cannot report" must not
// look like "nothing is pending".
func TestRoutesWithoutAStoreStillReport501(t *testing.T) {
	a := testApp()
	a.Approvals = nil
	r := NewRouter(a)

	for _, c := range []struct{ method, path string }{
		{"GET", "/api/approvals"},
		{"POST", "/api/approvals/apr-1/actions"},
		{"POST", "/api/terminal/sess-1/approvals/apr-1"},
	} {
		req := httptest.NewRequest(c.method, c.path, strings.NewReader(`{"action":"approve"}`))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotImplemented {
			t.Errorf("%s %s: want 501, got %d", c.method, c.path, w.Code)
		}
		var body struct {
			Implemented bool `json:"implemented"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if body.Implemented {
			t.Errorf("%s %s: must report implemented=false", c.method, c.path)
		}
	}
}
