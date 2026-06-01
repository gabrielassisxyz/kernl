package notes

import (
	"encoding/json"
)

type DiffHunk struct {
	ID      string `json:"id"`
	Line    int    `json:"line"`
	Action  string `json:"action"` // "add", "remove", "replace"
	Content string `json:"content"`
}

func ParseDiff(payload []byte) ([]DiffHunk, error) {
	var hunks []DiffHunk
	err := json.Unmarshal(payload, &hunks)
	return hunks, err
}
