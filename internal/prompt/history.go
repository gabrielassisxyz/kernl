package prompt

func BuildDebugPrompt(beadID string, sessionEvents []string) string {
	prompt := "## Session Debug\n"
	prompt += "Bead: " + beadID + "\n"
	for _, e := range sessionEvents {
		prompt += "- " + e + "\n"
	}
	return prompt
}
