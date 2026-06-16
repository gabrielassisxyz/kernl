package prompt

import (
	"encoding/json"
	"fmt"
)

func SummarizeToolInput(input map[string]any) string {
	priorityKeys := []string{"command", "filePath", "file_path", "pattern"}

	for _, k := range priorityKeys {
		if v, ok := input[k]; ok {
			return truncate(fmt.Sprintf("%s: %v", k, v), 200)
		}
	}

	data, err := json.Marshal(input)
	if err != nil {
		return "<invalid input>"
	}
	return truncate(string(data), 200)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
