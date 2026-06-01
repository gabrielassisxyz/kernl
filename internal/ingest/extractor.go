package ingest

import "context"

// Extractor interface enables calling out to a DA/LLM to extract structured data.
type Extractor interface {
	ExtractActions(ctx context.Context, content string) ([]Action, error)
}

// Action represents an intended ingestion step.
type Action struct {
	Type    string
	Title   string
	Payload string
}

// StubExtractor is a placeholder implementation.
type StubExtractor struct{}

// ExtractActions returns a stub Contradiction Callout action.
func (e *StubExtractor) ExtractActions(ctx context.Context, content string) ([]Action, error) {
	return []Action{
		{
			Type:    "Contradiction Callout",
			Title:   "Potential Conflict",
			Payload: "Found a contradiction in the text.",
		},
	}, nil
}
