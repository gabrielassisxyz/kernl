package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/planning"
)

// runPlan shows the substrate-aware planning context for a topic: the vault
// notes the DA planner would automatically have in scope. This is the keystone
// seam made visible from the CLI — "you're about to plan X, here are your notes
// on it" — no hunting, no manual paste.
func runPlan(configPath string, args []string) error {
	if len(args) == 0 {
		return usagef("KERNL DISPATCH FAILURE: plan requires a topic — run: kernl plan \"caching strategy\"")
	}
	seed := strings.Join(args, " ")

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
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
