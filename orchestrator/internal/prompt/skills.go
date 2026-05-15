package prompt

import "fmt"

func BuildSkillPrompt(beatID, state string) string {
	prompt := fmt.Sprintf("# %s\n\n", state)
	prompt += fmt.Sprintf("bd show %s\n", beatID)
	prompt += "bd sync\n"
	prompt += fmt.Sprintf("Current state: %s\n\n", state)
	prompt += "## Authority Boundary\n"
	prompt += "Complete exactly one workflow action, then stop.\n"
	return prompt
}