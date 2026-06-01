package ingest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManifestManager(t *testing.T) {
	tempDir := t.TempDir()
	mm := NewManifestManager(tempDir)

	err := mm.Load()
	if err != nil {
		t.Fatalf("Load should not fail on missing file: %v", err)
	}

	content := []byte("hello world")
	filePath := "notes/hello.md"

	if !mm.NeedsProcessing(filePath, content) {
		t.Fatal("Expected new file to need processing")
	}

	err = mm.MarkProcessed(filePath, content)
	if err != nil {
		t.Fatalf("Failed to mark processed: %v", err)
	}

	// Verify it saved
	manifestPath := filepath.Join(tempDir, "vault-llm", "manifest.json")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		t.Fatal("Manifest file was not created")
	}

	// Same hash should not need processing
	if mm.NeedsProcessing(filePath, content) {
		t.Fatal("Expected same file content NOT to need processing")
	}

	// Different hash should need processing
	newContent := []byte("hello world 2")
	if !mm.NeedsProcessing(filePath, newContent) {
		t.Fatal("Expected changed content to need processing")
	}
}
