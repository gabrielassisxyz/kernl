//go:build integration

package integration

import "testing"

func TestHarnessBootsConfigAndTempRepo(t *testing.T) {
	h := NewHarness(t)
	defer h.Cleanup()

	if h.Config.Settings.Agents["opencode"].Command == "" {
		t.Fatal("expected opencode agent in integration fixture config")
	}
	if h.RepoPath == "" {
		t.Fatal("expected harness to create a temp git repo with .beads/")
	}
}
