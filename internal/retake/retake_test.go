package retake

import "testing"

func TestIsRetakeSourceState(t *testing.T) {
	t.Run("accepts shipped and legacy closed aliases", func(t *testing.T) {
		for _, s := range []string{"shipped", "closed", "done", "approved"} {
			if !IsRetakeSourceState(s) {
				t.Errorf("expected %q to be a retake source state", s)
			}
		}
	})

	t.Run("normalizes case and whitespace", func(t *testing.T) {
		if !IsRetakeSourceState("  SHIPPED ") {
			t.Error("expected '  SHIPPED ' to normalize to shipped")
		}
	})

	t.Run("rejects non-retake source states", func(t *testing.T) {
		for _, s := range []string{"ready_for_implementation", "abandoned", ""} {
			if IsRetakeSourceState(s) {
				t.Errorf("expected %q NOT to be a retake source state", s)
			}
		}
	})
}

func TestRepoScopedBeadKey(t *testing.T) {
	t.Run("concatenates repo path and bead id", func(t *testing.T) {
		got := RepoScopedBeadKey("bead-1", "/repos/a")
		want := "/repos/a::bead-1"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("works with empty repo path", func(t *testing.T) {
		got := RepoScopedBeadKey("bead-1", "")
		want := "::bead-1"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

func TestBuildRetakeShippingIndex(t *testing.T) {
	t.Run("includes only running terminals", func(t *testing.T) {
		terminals := []RetakeTerminal{
			{SessionID: "s1", BeadID: "b1", RepoPath: "/r/a", Status: "running"},
			{SessionID: "s2", BeadID: "b2", RepoPath: "/r/a", Status: "completed"},
			{SessionID: "s3", BeadID: "b3", RepoPath: "/r/b", Status: "running"},
		}

		idx := BuildRetakeShippingIndex(terminals)
		if len(idx) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(idx))
		}
		if idx["/r/a::b1"] != "s1" {
			t.Errorf("expected s1 for /r/a::b1, got %s", idx["/r/a::b1"])
		}
		if idx["/r/a::b2"] != "" {
			t.Error("expected completed terminal to be excluded")
		}
		if idx["/r/b::b3"] != "s3" {
			t.Errorf("expected s3 for /r/b::b3, got %s", idx["/r/b::b3"])
		}
	})

	t.Run("builds shipping index entries per repo for duplicate bead ids", func(t *testing.T) {
		terminals := []RetakeTerminal{
			{SessionID: "session-a", BeadID: "same-id", RepoPath: "/repos/a", Status: "running", StartedAt: "2026-03-17T09:00:00Z"},
			{SessionID: "session-b", BeadID: "same-id", RepoPath: "/repos/b", Status: "running", StartedAt: "2026-03-17T09:05:00Z"},
		}

		idx := BuildRetakeShippingIndex(terminals)
		if idx[RepoScopedBeadKey("same-id", "/repos/a")] != "session-a" {
			t.Error("expected session-a for repo a")
		}
		if idx[RepoScopedBeadKey("same-id", "/repos/b")] != "session-b" {
			t.Error("expected session-b for repo b")
		}
	})
}

func TestFindRunningTerminalForBead(t *testing.T) {
	t.Run("reuses only the running session from the same repo when bead ids collide", func(t *testing.T) {
		terminals := []RetakeTerminal{
			{SessionID: "session-a", BeadID: "kernl-6428", RepoPath: "/repos/a", Status: "running", StartedAt: "2026-03-17T09:00:00Z"},
			{SessionID: "session-b", BeadID: "kernl-6428", RepoPath: "/repos/b", Status: "running", StartedAt: "2026-03-17T09:05:00Z"},
		}

		found := FindRunningTerminalForBead(terminals, "kernl-6428", "/repos/b")
		if found == nil || found.SessionID != "session-b" {
			t.Errorf("expected session-b, got %v", found)
		}

		found = FindRunningTerminalForBead(terminals, "kernl-6428", "/repos/a")
		if found == nil || found.SessionID != "session-a" {
			t.Errorf("expected session-a, got %v", found)
		}
	})

	t.Run("returns nil when no matching terminal", func(t *testing.T) {
		terminals := []RetakeTerminal{
			{SessionID: "s1", BeadID: "b1", RepoPath: "/r/a", Status: "running"},
		}

		found := FindRunningTerminalForBead(terminals, "b2", "/r/a")
		if found != nil {
			t.Errorf("expected nil, got %v", found)
		}
	})

	t.Run("returns nil when terminal is not running", func(t *testing.T) {
		terminals := []RetakeTerminal{
			{SessionID: "s1", BeadID: "b1", RepoPath: "/r/a", Status: "completed"},
		}

		found := FindRunningTerminalForBead(terminals, "b1", "/r/a")
		if found != nil {
			t.Errorf("expected nil for non-running, got %v", found)
		}
	})
}

func TestBuildRetakeParentIndex(t *testing.T) {
	t.Run("maps beads with and without parents", func(t *testing.T) {
		beads := []RetakeBead{
			{ID: "parent", RepoPath: "/r/a"},
			{ID: "child", Parent: "parent", RepoPath: "/r/a"},
		}

		idx := BuildRetakeParentIndex(beads)
		if idx["/r/a::parent"] != "" {
			t.Errorf("expected empty parent for parent bead, got %q", idx["/r/a::parent"])
		}
		if idx["/r/a::child"] != "/r/a::parent" {
			t.Errorf("expected parent scoped key, got %q", idx["/r/a::child"])
		}
	})
}

func TestHasRollingAncestor(t *testing.T) {
	t.Run("returns true when an ancestor is rolling", func(t *testing.T) {
		parentByBeadID := map[string]string{
			"/r/a::child":  "/r/a::parent",
			"/r/a::parent": "",
		}
		shipping := map[string]string{
			"/r/a::parent": "session-a",
		}

		bead := RetakeBead{ID: "child", RepoPath: "/r/a"}
		if !HasRollingAncestor(bead, parentByBeadID, shipping) {
			t.Error("expected true when parent is rolling")
		}
	})

	t.Run("returns false when no ancestor is rolling", func(t *testing.T) {
		parentByBeadID := map[string]string{
			"/r/a::child":  "/r/a::parent",
			"/r/a::parent": "",
		}
		shipping := map[string]string{}

		bead := RetakeBead{ID: "child", RepoPath: "/r/a"}
		if HasRollingAncestor(bead, parentByBeadID, shipping) {
			t.Error("expected false when no ancestor is rolling")
		}
	})

	t.Run("returns false for root bead", func(t *testing.T) {
		parentByBeadID := map[string]string{
			"/r/a::parent": "",
		}
		shipping := map[string]string{}

		bead := RetakeBead{ID: "parent", RepoPath: "/r/a"}
		if HasRollingAncestor(bead, parentByBeadID, shipping) {
			t.Error("expected false for root bead")
		}
	})

	t.Run("returns false when ancestor is in different repo", func(t *testing.T) {
		// Repo-scoped keys isolate lookups across repos.
		parentByBeadID := map[string]string{
			"/repos/b::child":  "/repos/b::parent",
			"/repos/b::parent": "",
		}
		shipping := map[string]string{
			"/repos/a::parent": "session-a",
		}

		bead := RetakeBead{ID: "child", RepoPath: "/repos/b"}
		if HasRollingAncestor(bead, parentByBeadID, shipping) {
			t.Error("expected false when rolling ancestor is in different repo")
		}
	})

	t.Run("returns true when rolling ancestor is in same repo", func(t *testing.T) {
		parentByBeadID := map[string]string{
			"/repos/b::child":  "/repos/b::parent",
			"/repos/b::parent": "",
		}
		shipping := map[string]string{
			"/repos/b::parent": "session-b",
		}

		bead := RetakeBead{ID: "child", RepoPath: "/repos/b"}
		if !HasRollingAncestor(bead, parentByBeadID, shipping) {
			t.Error("expected true when rolling ancestor is in same repo")
		}
	})

	t.Run("handles deeply nested chains", func(t *testing.T) {
		parentByBeadID := map[string]string{
			"/r/a::c": "/r/a::b",
			"/r/a::b": "/r/a::a",
			"/r/a::a": "",
		}
		shipping := map[string]string{
			"/r/a::a": "session-a",
		}

		bead := RetakeBead{ID: "c", RepoPath: "/r/a"}
		if !HasRollingAncestor(bead, parentByBeadID, shipping) {
			t.Error("expected true for deeply nested rolling ancestor")
		}
	})

	t.Run("cyclical parent chain does not hang", func(t *testing.T) {
		parentByBeadID := map[string]string{
			"/r/a::a": "/r/a::b",
			"/r/a::b": "/r/a::a",
		}
		shipping := map[string]string{}

		bead := RetakeBead{ID: "a", RepoPath: "/r/a"}
		if HasRollingAncestor(bead, parentByBeadID, shipping) {
			t.Error("expected false for cyclical chain")
		}
	})
}
