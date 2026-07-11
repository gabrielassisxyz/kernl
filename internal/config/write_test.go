package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const writableConfig = `# kernl.yaml — test fixture
settings:
  agents:
    opencode:
      command: opencode

registry:
  repos:
    - path: /tmp/repo

# The LLM powers chat, ingest, and note AI features.
llm:
  provider: openai
  model: kimi-k2.7
  api_key: sk-existing

vault:
  root: /tmp/vault
`

func writeFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kernl.yaml")
	if err := os.WriteFile(path, []byte(writableConfig), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	return path
}

func TestApplyPreservesCommentsAndUnknownFields(t *testing.T) {
	path := writeFixture(t)

	err := Apply(path, []Update{{Path: []string{"llm", "model"}, Value: "kimi-k2.8"}})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading result: %v", err)
	}
	got := string(raw)

	if !strings.Contains(got, "# kernl.yaml — test fixture") {
		t.Error("head comment was dropped")
	}
	if !strings.Contains(got, "# The LLM powers chat, ingest, and note AI features.") {
		t.Error("section comment was dropped")
	}
	if !strings.Contains(got, "model: kimi-k2.8") {
		t.Errorf("model was not updated:\n%s", got)
	}
	if !strings.Contains(got, "api_key: sk-existing") {
		t.Error("untouched sibling field was lost")
	}
	if !strings.Contains(got, "command: opencode") {
		t.Error("unrelated section was lost")
	}
}

func TestApplyCreatesMissingSection(t *testing.T) {
	path := writeFixture(t)

	err := Apply(path, []Update{
		{Path: []string{"inbox", "auto_prep"}, Value: true},
		{Path: []string{"inbox", "da_subdir"}, Value: "DA"},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load after Apply: %v", err)
	}
	if !cfg.Inbox.AutoPrep {
		t.Error("inbox.auto_prep was not created")
	}
	if cfg.Inbox.DASubdir != "DA" {
		t.Errorf("inbox.da_subdir = %q, want DA", cfg.Inbox.DASubdir)
	}
}

func TestApplyWritesIntsAndSlices(t *testing.T) {
	path := writeFixture(t)

	err := Apply(path, []Update{
		{Path: []string{"orchestrator", "maxConcurrentBeads"}, Value: 9},
		{Path: []string{"sweep", "backoff_minutes"}, Value: []int{2, 8, 30}},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load after Apply: %v", err)
	}
	if cfg.Orchestrator.MaxConcurrentBeads != 9 {
		t.Errorf("maxConcurrentBeads = %d, want 9", cfg.Orchestrator.MaxConcurrentBeads)
	}
	if len(cfg.Sweep.BackoffMinutes) != 3 || cfg.Sweep.BackoffMinutes[2] != 30 {
		t.Errorf("backoff_minutes = %v, want [2 8 30]", cfg.Sweep.BackoffMinutes)
	}
}

func TestApplyIsAtomicOnParseFailure(t *testing.T) {
	path := writeFixture(t)

	// settings.agents must stay a mapping; assigning a scalar over it would
	// produce a config that no longer loads.
	err := Apply(path, []Update{{Path: []string{"settings", "agents"}, Value: "not-a-mapping"}})
	if err == nil {
		t.Fatal("expected Apply to reject a config that breaks the schema")
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading result: %v", err)
	}
	if string(raw) != writableConfig {
		t.Error("original config was modified despite the rejected write")
	}
}

func TestApplyLeavesNoTempFiles(t *testing.T) {
	path := writeFixture(t)

	if err := Apply(path, []Update{{Path: []string{"server", "port"}, Value: 9090}}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("reading dir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Errorf("temp file left behind: %s", entry.Name())
		}
	}
}
