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

func TestRepoScopedBeatKey(t *testing.T) {
	t.Run("concatenates repo path and beat id", func(t *testing.T) {
		got := RepoScopedBeatKey("beat-1", "/repos/a")
		want := "/repos/a::beat-1"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("works with empty repo path", func(t *testing.T) {
		got := RepoScopedBeatKey("beat-1", "")
		want := "::beat-1"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

func TestBuildRetakeShippingIndex(t *testing.T) {
	t.Run("includes only running terminals", func(t *testing.T) {
		terminals := []RetakeTerminal{
			{SessionID: "s1", BeatID: "b1", RepoPath: "/r/a", Status: "running"},
			{SessionID: "s2", BeatID: "b2", RepoPath: "/r/a", Status: "completed"},
			{SessionID: "s3", BeatID: "b3", RepoPath: "/r/b", Status: "running"},
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

	t.Run("builds shipping index entries per repo for duplicate beat ids", func(t *testing.T) {
		terminals := []RetakeTerminal{
			{SessionID: "session-a", BeatID: "same-id", RepoPath: "/repos/a", Status: "running", StartedAt: "2026-03-17T09:00:00Z"},
			{SessionID: "session-b", BeatID: "same-id", RepoPath: "/repos/b", Status: "running", StartedAt: "2026-03-17T09:05:00Z"},
		}

		idx := BuildRetakeShippingIndex(terminals)
		if idx[RepoScopedBeatKey("same-id", "/repos/a")] != "session-a" {
			t.Error("expected session-a for repo a")
		}
		if idx[RepoScopedBeatKey("same-id", "/repos/b")] != "session-b" {
			t.Error("expected session-b for repo b")
		}
	})
}

func TestFindRunningTerminalForBeat(t *testing.T) {
	t.Run("reuses only the running session from the same repo when beat ids collide", func(t *testing.T) {
		terminals := []RetakeTerminal{
			{SessionID: "session-a", BeatID: "foolery-6428", RepoPath: "/repos/a", Status: "running", StartedAt: "2026-03-17T09:00:00Z"},
			{SessionID: "session-b", BeatID: "foolery-6428", RepoPath: "/repos/b", Status: "running", StartedAt: "2026-03-17T09:05:00Z"},
		}

		found := FindRunningTerminalForBeat(terminals, "foolery-6428", "/repos/b")
		if found == nil || found.SessionID != "session-b" {
			t.Errorf("expected session-b, got %v", found)
		}

		found = FindRunningTerminalForBeat(terminals, "foolery-6428", "/repos/a")
		if found == nil || found.SessionID != "session-a" {
			t.Errorf("expected session-a, got %v", found)
		}
	})

	t.Run("returns nil when no matching terminal", func(t *testing.T) {
		terminals := []RetakeTerminal{
			{SessionID: "s1", BeatID: "b1", RepoPath: "/r/a", Status: "running"},
		}

		found := FindRunningTerminalForBeat(terminals, "b2", "/r/a")
		if found != nil {
			t.Errorf("expected nil, got %v", found)
		}
	})

	t.Run("returns nil when terminal is not running", func(t *testing.T) {
		terminals := []RetakeTerminal{
			{SessionID: "s1", BeatID: "b1", RepoPath: "/r/a", Status: "completed"},
		}

		found := FindRunningTerminalForBeat(terminals, "b1", "/r/a")
		if found != nil {
			t.Errorf("expected nil for non-running, got %v", found)
		}
	})
}

func TestBuildRetakeParentIndex(t *testing.T) {
	t.Run("maps beats with and without parents", func(t *testing.T) {
		beats := []RetakeBeat{
			{ID: "parent", RepoPath: "/r/a"},
			{ID: "child", Parent: "parent", RepoPath: "/r/a"},
		}

		idx := BuildRetakeParentIndex(beats)
		if idx["/r/a::parent"] != "" {
			t.Errorf("expected empty parent for parent beat, got %q", idx["/r/a::parent"])
		}
		if idx["/r/a::child"] != "/r/a::parent" {
			t.Errorf("expected parent scoped key, got %q", idx["/r/a::child"])
		}
	})
}

func TestHasRollingAncestor(t *testing.T) {
	t.Run("returns true when an ancestor is rolling", func(t *testing.T) {
		parentByBeatID := map[string]string{
			"/r/a::child":  "/r/a::parent",
			"/r/a::parent": "",
		}
		shipping := map[string]string{
			"/r/a::parent": "session-a",
		}

		beat := RetakeBeat{ID: "child", RepoPath: "/r/a"}
		if !HasRollingAncestor(beat, parentByBeatID, shipping) {
			t.Error("expected true when parent is rolling")
		}
	})

	t.Run("returns false when no ancestor is rolling", func(t *testing.T) {
		parentByBeatID := map[string]string{
			"/r/a::child":  "/r/a::parent",
			"/r/a::parent": "",
		}
		shipping := map[string]string{}

		beat := RetakeBeat{ID: "child", RepoPath: "/r/a"}
		if HasRollingAncestor(beat, parentByBeatID, shipping) {
			t.Error("expected false when no ancestor is rolling")
		}
	})

	t.Run("returns false for root beat", func(t *testing.T) {
		parentByBeatID := map[string]string{
			"/r/a::parent": "",
		}
		shipping := map[string]string{}

		beat := RetakeBeat{ID: "parent", RepoPath: "/r/a"}
		if HasRollingAncestor(beat, parentByBeatID, shipping) {
			t.Error("expected false for root beat")
		}
	})

	t.Run("returns false when ancestor is in different repo", func(t *testing.T) {
		// Repo-scoped keys isolate lookups across repos.
		parentByBeatID := map[string]string{
			"/repos/b::child":  "/repos/b::parent",
			"/repos/b::parent": "",
		}
		shipping := map[string]string{
			"/repos/a::parent": "session-a",
		}

		beat := RetakeBeat{ID: "child", RepoPath: "/repos/b"}
		if HasRollingAncestor(beat, parentByBeatID, shipping) {
			t.Error("expected false when rolling ancestor is in different repo")
		}
	})

	t.Run("returns true when rolling ancestor is in same repo", func(t *testing.T) {
		parentByBeatID := map[string]string{
			"/repos/b::child":  "/repos/b::parent",
			"/repos/b::parent": "",
		}
		shipping := map[string]string{
			"/repos/b::parent": "session-b",
		}

		beat := RetakeBeat{ID: "child", RepoPath: "/repos/b"}
		if !HasRollingAncestor(beat, parentByBeatID, shipping) {
			t.Error("expected true when rolling ancestor is in same repo")
		}
	})

	t.Run("handles deeply nested chains", func(t *testing.T) {
		parentByBeatID := map[string]string{
			"/r/a::c": "/r/a::b",
			"/r/a::b": "/r/a::a",
			"/r/a::a": "",
		}
		shipping := map[string]string{
			"/r/a::a": "session-a",
		}

		beat := RetakeBeat{ID: "c", RepoPath: "/r/a"}
		if !HasRollingAncestor(beat, parentByBeatID, shipping) {
			t.Error("expected true for deeply nested rolling ancestor")
		}
	})

	t.Run("cyclical parent chain does not hang", func(t *testing.T) {
		parentByBeatID := map[string]string{
			"/r/a::a": "/r/a::b",
			"/r/a::b": "/r/a::a",
		}
		shipping := map[string]string{}

		beat := RetakeBeat{ID: "a", RepoPath: "/r/a"}
		if HasRollingAncestor(beat, parentByBeatID, shipping) {
			t.Error("expected false for cyclical chain")
		}
	})
}
