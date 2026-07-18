package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/planning"
)

// runPlan shows the substrate-aware planning context for a topic: the vault
// notes the DA planner would automatically have in scope. This is the keystone
// seam made visible from the CLI — "you're about to plan X, here are your notes
// on it" — no hunting, no manual paste.
func runPlan(configPath string, args []string) error {
	var asJSON bool
	var topicWords []string
	for _, arg := range args {
		switch {
		case arg == "--json":
			asJSON = true
		case strings.HasPrefix(arg, "-"):
			return usagef("KERNL DISPATCH FAILURE: unknown plan flag %q%s — valid: --json",
				arg, didYouMean(arg, []string{"--json"}))
		default:
			topicWords = append(topicWords, arg)
		}
	}
	if len(topicWords) == 0 {
		return usagef("KERNL DISPATCH FAILURE: plan requires a topic — run: kernl plan \"caching strategy\"")
	}
	seed := strings.Join(topicWords, " ")

	cfg, err := loadCLIConfig(configPath)
	if err != nil {
		return err
	}
	a, err := app.NewApp(cfg)
	if err != nil {
		return fmt.Errorf("creating app: %w", err)
	}
	defer a.Close()

	notes, err := planning.BuildContext(context.Background(), a.Graph, seed, 8)
	if err != nil {
		return fmt.Errorf("building planning context: %w", err)
	}

	if asJSON {
		return json.NewEncoder(os.Stdout).Encode(newPlanOutput(seed, notes))
	}

	if len(notes) == 0 {
		fmt.Printf("No vault notes found relevant to %q yet.\n", seed)
		return nil
	}

	fmt.Printf("Planning context for %q — %d relevant note(s) the DA would have in scope:\n\n", seed, len(notes))
	for i, n := range notes {
		fmt.Printf("%d. %s  [%s]\n   %s\n", i+1, n.Title, n.Via, n.Snippet)
	}
	return nil
}

// planOutput is the machine contract for `kernl plan --json`: one camelCase
// object (not JSONL) so a single json.Unmarshal captures the whole answer.
type planOutput struct {
	Topic string     `json:"topic"`
	Notes []planNote `json:"notes"`
}

type planNote struct {
	Title   string `json:"title"`
	Via     string `json:"via"`
	Snippet string `json:"snippet"`
}

func newPlanOutput(topic string, notes []planning.ContextNote) planOutput {
	out := planOutput{Topic: topic, Notes: make([]planNote, 0, len(notes))}
	for _, n := range notes {
		out.Notes = append(out.Notes, planNote{Title: n.Title, Via: n.Via, Snippet: n.Snippet})
	}
	return out
}
