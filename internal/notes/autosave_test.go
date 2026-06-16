package notes

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCheckConflict(t *testing.T) {
	dir := t.TempDir()
	fullPath := filepath.Join(dir, "test.md")

	err := CheckConflict(fullPath, "")
	if err != nil {
		t.Fatalf("expected no error for non-existent file, got %v", err)
	}

	err = os.WriteFile(fullPath, []byte("content"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	info, _ := os.Stat(fullPath)
	clientTime := info.ModTime().Format(time.RFC3339)

	err = CheckConflict(fullPath, clientTime)
	if err != nil {
		t.Fatalf("expected no error for matching time, got %v", err)
	}

	// simulate old client time
	oldTime := info.ModTime().Add(-2 * time.Second).Format(time.RFC3339)
	err = CheckConflict(fullPath, oldTime)
	if err != ErrConflict {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}
