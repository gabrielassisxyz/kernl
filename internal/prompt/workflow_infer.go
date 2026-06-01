package prompt

import (
	"fmt"
)

// WorkflowInferInput represents the inputs for the workflow inference prompt.
type WorkflowInferInput struct {
	Title       string
	Description string
	Shapes      []string
}

// RenderWorkflowInfer returns a prompt string instructing the LLM to choose the best workflow shape.
func RenderWorkflowInfer(in WorkflowInferInput) string {
	shapesStr := ""
	for _, s := range in.Shapes {
		shapesStr += fmt.Sprintf("- %s\n", s)
	}

	return fmt.Sprintf(`You are the kernl workflow inference engine. Pick the best workflow shape based on the epic title and description.
Available shapes:
%s
Return ONLY the ID of the chosen workflow on the first line, and a one-sentence rationale on the second line.

Epic Title: %s
Epic Description: %s`, shapesStr, in.Title, in.Description)
}
