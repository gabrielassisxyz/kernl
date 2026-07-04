package suggestlog

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLogAppendsJSONL(t *testing.T) {
	root := t.TempDir()

	if err := Log(root, Edit{Surface: "inbox", Field: "target", Original: "note", Edited: "task"}); err != nil {
		t.Fatal(err)
	}
	if err := Log(root, Edit{Surface: "inbox", Field: "projectId", Original: "", Edited: "p1"}); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(filepath.Join(root, "DA", "suggestion-edits.jsonl"))
	if err != nil {
		t.Fatalf("log file not created: %v", err)
	}
	defer f.Close()

	var lines []Edit
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var e Edit
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			t.Fatalf("line is not valid JSON: %v", err)
		}
		lines = append(lines, e)
	}
	if len(lines) != 2 {
		t.Fatalf("expected 2 appended records, got %d", len(lines))
	}
	if lines[0].Edited != "task" || lines[1].Edited != "p1" {
		t.Errorf("records out of order or wrong: %+v", lines)
	}
	if lines[0].Timestamp.IsZero() {
		t.Error("timestamp should be auto-filled")
	}
}

func TestLogNoOpWithoutVault(t *testing.T) {
	if err := Log("", Edit{Surface: "inbox"}); err != nil {
		t.Errorf("empty vault root should be a no-op, got %v", err)
	}
}
