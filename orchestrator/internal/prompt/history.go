package prompt

func BuildDebugPrompt(beatID string, sessionEvents []string) string {
	prompt := "## Session Debug\n"
	prompt += "Beat: " + beatID + "\n"
	for _, e := range sessionEvents {
		prompt += "- " + e + "\n"
	}
	return prompt
}