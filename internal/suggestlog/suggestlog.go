// Package suggestlog records the moments a user overrides an LLM suggestion.
// When a classifier/extractor proposes something and the user edits it before
// accepting, the (original → edited) pair is appended to a JSONL log. Nothing
// consumes it yet; it is raw material for later prompt tuning, kept out of the
// graph so it never pollutes the knowledge base.
package suggestlog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// logRelPath is the vault-relative file the edits are appended to. It lives
// under the DA-owned subdir so it travels with the vault backup.
const logRelPath = "DA/suggestion-edits.jsonl"

// Edit is one override event. Surface identifies where it happened
// (e.g. "inbox", "ingest", "da-learned"); Field names what was changed
// (e.g. "target", "projectId", "statement").
type Edit struct {
	Timestamp time.Time `json:"ts"`
	Surface   string    `json:"surface"`
	Field     string    `json:"field"`
	Original  string    `json:"original"`
	Edited    string    `json:"edited"`
	// Context carries anything useful for tuning without a fixed schema
	// (e.g. the capture body). Optional.
	Context string `json:"context,omitempty"`
}

// Log appends an edit record. Best-effort by design: a logging failure must
// never break the user action that produced it, so callers ignore the error in
// practice. A no-op when vaultRoot is empty (headless/test contexts).
func Log(vaultRoot string, e Edit) error {
	if vaultRoot == "" {
		return nil
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}

	full := filepath.Join(vaultRoot, filepath.FromSlash(logRelPath))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}

	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	line = append(line, '\n')

	f, err := os.OpenFile(full, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(line)
	return err
}
