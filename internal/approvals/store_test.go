package approvals

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(filepath.Join(t.TempDir(), "approvals"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return s
}

func mustCreate(t *testing.T, s *Store, req *ApprovalRequest) *ApprovalRequest {
	t.Helper()
	created, err := s.Create(req, 30*time.Minute)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return created
}

func TestCreateParksAPendingRequest(t *testing.T) {
	s := newTestStore(t)
	req := mustCreate(t, s, &ApprovalRequest{
		BeadID: "kernl-1", SessionID: "sess-1", ToolName: "Bash", Adapter: "claude",
	})

	if req.ID == "" {
		t.Fatal("Create must mint an id")
	}
	if req.Status != StatusPending || !req.Actionable {
		t.Errorf("a new request must be pending and actionable, got %s actionable=%v", req.Status, req.Actionable)
	}
	if req.ExpiresAt == "" {
		t.Error("a new request must carry a deadline, or a killed bridge leaves it pending forever")
	}
	if req.NotificationKey == "" {
		t.Error("a new request must carry a logical key")
	}

	got, err := s.Get(req.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ToolName != "Bash" || got.BeadID != "kernl-1" {
		t.Errorf("round trip lost fields: %+v", got)
	}
}

// The scaffolding this store replaced returned nil for an unknown id, which is
// how `kernl approval resolve apr-999` printed "Resolved" at exit 0 for an
// approval that never existed.
func TestDecideUnknownIDIsAnError(t *testing.T) {
	s := newTestStore(t)

	_, err := s.Decide("apr-999", ActionAllow, "")
	if err == nil {
		t.Fatal("answering an unknown approval must fail, not silently succeed")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound so the API can answer 404, got %v", err)
	}
}

func TestDecideRecordsTheAnswer(t *testing.T) {
	for _, tc := range []struct {
		action     Action
		wantStatus string
	}{
		{ActionAllow, StatusApproved},
		{ActionAllowAlways, StatusAlwaysApproved},
		{ActionDeny, StatusRejected},
		{ActionExpire, StatusExpired},
	} {
		t.Run(string(tc.action), func(t *testing.T) {
			s := newTestStore(t)
			req := mustCreate(t, s, &ApprovalRequest{SessionID: "sess-1", ToolName: "Write"})

			decided, err := s.Decide(req.ID, tc.action, "because")
			if err != nil {
				t.Fatalf("Decide: %v", err)
			}
			if decided.Status != tc.wantStatus {
				t.Errorf("want status %s, got %s", tc.wantStatus, decided.Status)
			}
			if decided.Actionable {
				t.Error("an answered approval is no longer actionable")
			}
			if decided.Reason != "because" {
				t.Errorf("the reason must survive, got %q", decided.Reason)
			}

			// The answer must be visible to another process, which only ever
			// reads the files.
			reread, err := s.Get(req.ID)
			if err != nil {
				t.Fatalf("Get after Decide: %v", err)
			}
			if reread.Status != tc.wantStatus {
				t.Errorf("a second reader saw %s, want %s", reread.Status, tc.wantStatus)
			}
		})
	}
}

func TestDecideTwiceIsRefused(t *testing.T) {
	s := newTestStore(t)
	req := mustCreate(t, s, &ApprovalRequest{ToolName: "Write"})

	if _, err := s.Decide(req.ID, ActionAllow, ""); err != nil {
		t.Fatalf("first Decide: %v", err)
	}
	_, err := s.Decide(req.ID, ActionDeny, "")
	if err == nil {
		t.Fatal("a second answer must be refused: the agent already acted on the first")
	}
	if !strings.Contains(err.Error(), "already") {
		t.Errorf("the error must say it was already answered, got %v", err)
	}
}

// A bridge killed with its run never writes a decision. Without a deadline the
// record would advertise itself as answerable forever, and a human answering it
// would be answering an agent that is no longer listening.
func TestPendingRequestExpiresOnItsOwn(t *testing.T) {
	s := newTestStore(t)
	req, err := s.Create(&ApprovalRequest{ToolName: "Bash"}, time.Millisecond)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	time.Sleep(5 * time.Millisecond)

	got, err := s.Get(req.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != StatusExpired {
		t.Errorf("want %s past the deadline, got %s", StatusExpired, got.Status)
	}
	if got.Actionable {
		t.Error("an expired approval must not invite an answer")
	}
	if _, err := s.Decide(req.ID, ActionAllow, ""); err == nil {
		t.Error("answering an expired approval must be refused")
	}
}

func TestListFiltersAndOrders(t *testing.T) {
	s := newTestStore(t)
	first := mustCreate(t, s, &ApprovalRequest{ToolName: "A", RepoPath: "/repo-a", CreatedAt: "2026-01-01T00:00:00Z"})
	second := mustCreate(t, s, &ApprovalRequest{ToolName: "B", RepoPath: "/repo-b", CreatedAt: "2026-01-02T00:00:00Z"})

	all, err := s.List(ApprovalFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("want 2 approvals, got %d", len(all))
	}
	if all[0].ID != first.ID {
		t.Error("the oldest must come first: it is the one blocked longest")
	}

	byRepo, err := s.List(ApprovalFilter{RepoPath: "/repo-b"})
	if err != nil {
		t.Fatalf("List by repo: %v", err)
	}
	if len(byRepo) != 1 || byRepo[0].ID != second.ID {
		t.Errorf("repo filter returned %+v", byRepo)
	}

	if _, err := s.Decide(first.ID, ActionAllow, ""); err != nil {
		t.Fatalf("Decide: %v", err)
	}
	active, err := s.List(ApprovalFilter{ActiveOnly: true})
	if err != nil {
		t.Fatalf("List active: %v", err)
	}
	if len(active) != 1 || active[0].ID != second.ID {
		t.Errorf("ActiveOnly must drop the answered one, got %+v", active)
	}
}

func TestRememberedAllowOnlyMatchesSameSessionAndTool(t *testing.T) {
	s := newTestStore(t)
	req := mustCreate(t, s, &ApprovalRequest{SessionID: "sess-1", ToolName: "Bash"})
	if _, err := s.Decide(req.ID, ActionAllowAlways, ""); err != nil {
		t.Fatalf("Decide: %v", err)
	}

	for _, tc := range []struct {
		name, session, tool string
		want                bool
	}{
		{"same session and tool", "sess-1", "Bash", true},
		{"other tool", "sess-1", "Write", false},
		{"other session", "sess-2", "Bash", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := s.RememberedAllow(tc.session, tc.tool)
			if err != nil {
				t.Fatalf("RememberedAllow: %v", err)
			}
			if got != tc.want {
				t.Errorf("want %v, got %v", tc.want, got)
			}
		})
	}
}

// A plain allow must not be remembered: the human answered one call, not every
// future call of the same shape.
func TestRememberedAllowIgnoresAPlainAllow(t *testing.T) {
	s := newTestStore(t)
	req := mustCreate(t, s, &ApprovalRequest{SessionID: "sess-1", ToolName: "Bash"})
	if _, err := s.Decide(req.ID, ActionAllow, ""); err != nil {
		t.Fatalf("Decide: %v", err)
	}

	got, err := s.RememberedAllow("sess-1", "Bash")
	if err != nil {
		t.Fatalf("RememberedAllow: %v", err)
	}
	if got {
		t.Error("a one-off allow must not silence the next prompt")
	}
}

// Approval ids arrive from an HTTP path segment and from a CLI argument, so a
// traversing id must be refused rather than resolved. The planted file is what
// makes this test fail when the check is removed: without it, a traversing id
// merely fails to find a file, which passes for the wrong reason.
func TestIDsThatCouldEscapeTheStoreAreRefused(t *testing.T) {
	root := t.TempDir()
	s, err := NewStore(filepath.Join(root, "approvals"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	planted := &ApprovalRequest{ID: "planted", ToolName: "Bash", Status: StatusPending, Actionable: true}
	if err := writeJSONAtomic(filepath.Join(root, "outside.request.json"), planted); err != nil {
		t.Fatalf("planting a file outside the store: %v", err)
	}

	if got, err := s.Get("../outside"); err == nil {
		t.Errorf("a traversing id resolved to a record outside the store: %+v", got)
	}
	if _, err := s.Decide("../outside", ActionAllow, ""); err == nil {
		t.Error("a traversing id must not be answerable")
	}

	for _, id := range []string{"a/b", "", "with space", "dot.dot"} {
		if _, err := s.Get(id); err == nil {
			t.Errorf("Get(%q) must be refused", id)
		}
		if _, err := s.Decide(id, ActionAllow, ""); err == nil {
			t.Errorf("Decide(%q) must be refused", id)
		}
	}
}

func TestListSkipsUnrelatedFiles(t *testing.T) {
	s := newTestStore(t)
	mustCreate(t, s, &ApprovalRequest{ToolName: "A"})
	if err := os.WriteFile(filepath.Join(s.Dir(), "notes.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	all, err := s.List(ApprovalFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("want 1 approval, got %d", len(all))
	}
}
