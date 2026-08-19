package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/planning"
)

// planArgs is the parsed surface of `kernl plan`: the topic seed, the note
// limit, and whether the caller wants the machine-readable JSON contract.
type planArgs struct {
	asJSON bool
	limit  int
	topic  string
}

// parsePlanArgs turns the plan verb's args into planArgs. The limit defaults
// to 8 and is only overridden by an explicit --limit; a non-positive or
// non-numeric value is refused rather than silently falling back, because a
// typo that quietly measures the wrong depth is exactly the failure the flag
// exists to prevent.
func parsePlanArgs(args []string) (planArgs, error) {
	p := planArgs{limit: 8}
	var topicWords []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--json":
			p.asJSON = true
		case arg == "--limit":
			if i+1 >= len(args) {
				return p, usagef("KERNL DISPATCH FAILURE: --limit requires a value - run: kernl plan --help")
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil {
				return p, usagef("KERNL DISPATCH FAILURE: --limit needs a positive integer, got %q", args[i])
			}
			if n <= 0 {
				return p, usagef("KERNL DISPATCH FAILURE: --limit must be positive, got %d", n)
			}
			p.limit = n
		case strings.HasPrefix(arg, "-"):
			return p, usagef("KERNL DISPATCH FAILURE: unknown plan flag %q%s - valid: --json, --limit <n>",
				arg, didYouMean(arg, []string{"--json", "--limit"}))
		default:
			topicWords = append(topicWords, arg)
		}
	}
	p.topic = strings.Join(topicWords, " ")
	return p, nil
}

// runPlan shows the substrate-aware planning context for a topic: the vault
// notes the DA planner would automatically have in scope. This is the keystone
// seam made visible from the CLI - "you're about to plan X, here are your notes
// on it" - no hunting, no manual paste.
func runPlan(configPath string, args []string) error {
	pa, err := parsePlanArgs(args)
	if err != nil {
		return err
	}
	if pa.topic == "" {
		return usagef("KERNL DISPATCH FAILURE: plan requires a topic - run: kernl plan \"caching strategy\"")
	}
	seed := pa.topic

	cfg, err := loadCLIConfig(configPath)
	if err != nil {
		return err
	}
	a, err := app.NewApp(cfg)
	if err != nil {
		return wrapLoud("creating app", err)
	}
	defer a.Close()

	notes, err := planning.BuildContext(context.Background(), a.Graph, seed, pa.limit)
	if err != nil {
		return fmt.Errorf("building planning context: %w", err)
	}

	if pa.asJSON {
		return json.NewEncoder(os.Stdout).Encode(newPlanOutput(seed, notes))
	}

	if len(notes) == 0 {
		fmt.Printf("No vault notes found relevant to %q yet.\n", seed)
		return nil
	}

	fmt.Printf("Planning context for %q - %d relevant note(s) the DA would have in scope:\n\n", seed, len(notes))
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
	ID      string  `json:"id"`
	Title   string  `json:"title"`
	Via     string  `json:"via"`
	Snippet string  `json:"snippet"`
	Path    *string `json:"path"` // null for via=claim: a claim has no file on disk
}

func newPlanOutput(topic string, notes []planning.ContextNote) planOutput {
	out := planOutput{Topic: topic, Notes: make([]planNote, 0, len(notes))}
	for _, n := range notes {
		out.Notes = append(out.Notes, planNote{
			ID: n.ID, Title: n.Title, Via: n.Via, Snippet: n.Snippet, Path: n.Path,
		})
	}
	return out
}
