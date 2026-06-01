package dispatch

import "testing"

func TestIsHardGate(t *testing.T) {
	if !IsHardGate("git push") {
		t.Errorf("git push should be a hard gate")
	}
	if !IsHardGate("gh pr merge") {
		t.Errorf("gh pr merge should be a hard gate")
	}
	if IsHardGate("ls -la") {
		t.Errorf("ls -la should not be a hard gate")
	}
}

func TestCanAutoApprove(t *testing.T) {
	if !CanAutoApprove(true, "git commit -m 'test'") {
		t.Errorf("git commit should be auto-approved in autonomous mode")
	}
	if CanAutoApprove(true, "git push") {
		t.Errorf("git push should NOT be auto-approved")
	}
	if CanAutoApprove(false, "git commit") {
		t.Errorf("git commit should NOT be auto-approved if not autonomous")
	}
}
