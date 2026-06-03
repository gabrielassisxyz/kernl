package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/chat"
)

// allowedActionTypes is the set of action types the review queue understands.
// These map to the resolve handler in resolve.go and the UI buttons in
// web/components/ingest/IngestItem.vue.
var allowedActionTypes = map[string]struct{}{
	"Create Page":               {},
	"Update":                    {},
	"Add Contradiction Callout": {},
	"Deep Research":             {},
	"Skip":                      {},
}

const extractorSystemPrompt = `You analyze a note or document and extract a short list of structured ingestion actions for a knowledge base review queue.

Return ONLY a JSON array (no prose, no markdown fences). Each element must be an object with exactly these fields:
  "type":    one of "Create Page", "Update", "Add Contradiction Callout", "Deep Research", "Skip"
  "title":   a short human-readable title (a few words)
  "payload": the relevant snippet or text the action applies to

Rules:
- Choose "Create Page" for genuinely new concepts worth their own page.
- Choose "Update" when the content extends or revises an existing topic.
- Choose "Add Contradiction Callout" when the content conflicts with itself or with established knowledge.
- Choose "Deep Research" when a claim needs verification or further investigation.
- Choose "Skip" when nothing actionable is present.
- Keep the list small (at most 5 actions). If nothing is actionable, return an empty array [].`

// LLMExtractor implements Extractor by asking an LLM to extract structured
// ingest actions from content.
type LLMExtractor struct {
	llm chat.LLMClient
}

// NewLLMExtractor builds an LLMExtractor from a chat.LLMClient.
func NewLLMExtractor(llm chat.LLMClient) *LLMExtractor {
	return &LLMExtractor{llm: llm}
}

// llmAction mirrors the JSON shape we ask the LLM to produce.
type llmAction struct {
	Type    string `json:"type"`
	Title   string `json:"title"`
	Payload string `json:"payload"`
}

// ExtractActions asks the LLM to produce structured actions. It tolerates
// markdown fences and surrounding prose. If the LLM returns nothing usable,
// it returns an empty slice and no error so ProcessFile creates no reviews.
func (e *LLMExtractor) ExtractActions(ctx context.Context, content string) ([]Action, error) {
	prompt := fmt.Sprintf("%s\n\nDocument:\n%s", extractorSystemPrompt, content)

	resp, err := e.llm.Chat(ctx, []chat.Message{{Role: "user", Content: prompt}}, nil)
	if err != nil {
		return nil, fmt.Errorf("llm extract: %w", err)
	}

	raw := extractJSONArray(resp.Content)
	if raw == "" {
		return []Action{}, nil
	}

	var parsed []llmAction
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		// Unparseable output is treated as "nothing usable" rather than an error,
		// so a malformed response never blocks ingestion.
		return []Action{}, nil
	}

	actions := make([]Action, 0, len(parsed))
	for _, a := range parsed {
		t := strings.TrimSpace(a.Type)
		if _, ok := allowedActionTypes[t]; !ok {
			continue
		}
		actions = append(actions, Action{
			Type:    t,
			Title:   strings.TrimSpace(a.Title),
			Payload: a.Payload,
		})
	}

	return actions, nil
}

// extractJSONArray pulls the first balanced JSON array out of an LLM response,
// tolerating markdown code fences and surrounding prose. It returns "" when no
// array is found.
func extractJSONArray(content string) string {
	s := strings.TrimSpace(content)
	if s == "" {
		return ""
	}

	// Strip markdown code fences if present (```json ... ``` or ``` ... ```).
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
		if i := strings.IndexByte(s, '\n'); i >= 0 {
			// Drop an optional language hint on the fence line (e.g. "json").
			if !strings.Contains(s[:i], "[") {
				s = s[i+1:]
			}
		}
		if i := strings.LastIndex(s, "```"); i >= 0 {
			s = s[:i]
		}
		s = strings.TrimSpace(s)
	}

	start := strings.IndexByte(s, '[')
	if start < 0 {
		return ""
	}

	// Walk forward to find the matching closing bracket, respecting strings.
	depth := 0
	inStr := false
	escaped := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inStr {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}

	return ""
}
