package runstate

import "testing"

func TestStoreRoundTripsWorktreeAndAgentRecords(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	if err := s.SetWorktree("e", "c1", "/tmp/wt/e/c1"); err != nil {
		t.Fatalf("SetWorktree: %v", err)
	}
	wt, ok := s.Worktree("e", "c1")
	if !ok || wt != "/tmp/wt/e/c1" {
		t.Errorf("Worktree = %q,%v", wt, ok)
	}
	s.RecordAgent("c1", "implementing", AgentRecord{AgentID: "opencode", SessionID: "term-1", Status: "running"})
	rec, ok := s.AgentRecord("c1", "implementing")
	if !ok || rec.SessionID != "term-1" {
		t.Errorf("AgentRecord = %+v,%v", rec, ok)
	}
}
