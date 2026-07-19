package main

import (
	"context"
	"fmt"
	"strings"
)

var healthSubcommands = []string{"update-check"}

var healthCommand = commandMeta{
	Name:    "health",
	Summary: "Check that the server is up and answering (no subcommand needed)",
	Usage:   "kernl health [update-check] [--json]",
	Details: `The verb to reach for when something is wrong: it says whether a server is
reachable at the address this CLI would use, and which vault that server
has open. Plain 'kernl health' does the check; the subcommand below is the
GUI's update banner.

When no server answers, the error names the address that was tried and how
to start one — that failure is the useful answer, not a crash.

{{flags}}

Run 'kernl health update-check' for the app-update check.`,
	Flags: []commandFlag{
		{Name: "--json", Description: `Emit the API's {"status","vaultRoot","vaultLabel"} verbatim`},
	},
	Subs: []commandMeta{
		{
			Name:    "update-check",
			Summary: "Ask the server whether a newer kernl is available",
			Usage:   "kernl health update-check [--json]",
			Details: `Calls the same endpoint the GUI's update banner uses. That endpoint does
not contact any release feed yet — it answers {"status":"unknown","checked":false}.
Branch on "checked": false means "nobody looked", not "you are current".

{{flags}}`,
			Flags: []commandFlag{
				{Name: "--json", Description: `Emit the API's {"status","checked","detail"} verbatim`},
			},
		},
	},
}

func runHealth(v verbContext, args []string) error {
	// health has no mandatory subcommand: bare 'kernl health' is the check
	// itself, so only a non-flag first token is treated as a subcommand.
	sub := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		chosen, rest, err := requireSub("health", args, healthSubcommands)
		if err != nil {
			return err
		}
		sub, args = chosen, rest
	}
	asJSON, rest := parseBoolFlag(args, "--json")

	client, err := v.client()
	if err != nil {
		return err
	}
	if sub == "" {
		return healthStatus(v, client, asJSON, rest)
	}
	return healthUpdateCheck(v, client, asJSON, rest)
}

// healthStatusView is the subset of GET /api/health the summary prints.
type healthStatusView struct {
	Status     string `json:"status"`
	VaultRoot  string `json:"vaultRoot"`
	VaultLabel string `json:"vaultLabel"`
}

func healthStatus(v verbContext, c *apiClient, asJSON bool, args []string) error {
	if err := requireNoHealthArgs("health", "kernl health [--json]", args); err != nil {
		return err
	}

	raw, err := c.get(context.Background(), "/api/health")
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(v.stdout(), raw)
	}
	var status healthStatusView
	if err := decodeInto(raw, "GET /api/health", &status); err != nil {
		return err
	}
	fmt.Fprintf(v.stdout(), "%s  server %s\n", status.Status, c.baseURL)
	fmt.Fprintf(v.stdout(), "vault  %s\n", healthVaultLabel(status))
	return nil
}

func healthUpdateCheck(v verbContext, c *apiClient, asJSON bool, args []string) error {
	if err := requireNoHealthArgs("health update-check", "kernl health update-check [--json]", args); err != nil {
		return err
	}

	raw, err := c.get(context.Background(), "/api/app-update")
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(v.stdout(), raw)
	}
	var update struct {
		Status string `json:"status"`
	}
	if err := decodeInto(raw, "GET /api/app-update", &update); err != nil {
		return err
	}
	fmt.Fprintf(v.stdout(), "%s (the server does not check a release feed yet)\n", update.Status)
	return nil
}

func requireNoHealthArgs(verb, example string, args []string) error {
	if err := rejectUnknownFlags(verb, args); err != nil {
		return err
	}
	if len(args) > 0 {
		return usagef("KERNL DISPATCH FAILURE: %s takes no arguments, got %q — run: %s", verb, args[0], example)
	}
	return nil
}

// healthVaultLabel prefers the server's abbreviated label, falling back to the
// raw root: a server with no vault configured is worth saying out loud.
func healthVaultLabel(status healthStatusView) string {
	if label := strings.TrimSpace(status.VaultLabel); label != "" {
		return label
	}
	if root := strings.TrimSpace(status.VaultRoot); root != "" {
		return root
	}
	return "(none configured — set one with: kernl settings set vault --root <dir>)"
}
