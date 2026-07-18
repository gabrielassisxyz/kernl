package main

import (
	"fmt"
	"strings"
)

// commandMeta is the single source of truth for the CLI surface: dispatch,
// help, and (later) capabilities/robot-docs all read from this table so they
// cannot drift from each other.
type commandMeta struct {
	Name    string
	Summary string
	Usage   string
	Details string
	Subs    []commandMeta
}

var commandTable = []commandMeta{
	{
		Name:    "serve",
		Summary: "Start the HTTP API server (add --no-orchestrator for GUI-only)",
		Usage:   "kernl [--config <path>] [--port <port>] serve [--no-orchestrator]",
		Details: `Flags:
  --port, -p         Server port (default: from kernl.yaml, or 8080)
  --no-orchestrator  Serve only the GUI/graph/notes; do not require bd

The server hosts the web GUI, the REST API and the SSE event streams.`,
	},
	{
		Name:    "doctor",
		Summary: "Run system checks (env, binaries, config)",
		Usage:   "kernl [--config <path>] doctor",
		Details: `Checks that bd, the agent CLI, Go and the config file are usable.
Exit code is non-zero when a required check fails.`,
	},
	{
		Name:    "epic",
		Summary: "Manage epics (bead graphs)",
		Usage:   "kernl epic <list|run|merge|abort> [args...]",
		Details: `Run 'kernl epic <subcommand> --help' for details on each.`,
		Subs: []commandMeta{
			{
				Name:    "list",
				Summary: "List epics with child counts and state",
				Usage:   "kernl epic list [--json]",
				Details: `Flags:
  --json  Emit {"epics":[{"id","title","children","state"}]} on stdout`,
			},
			{
				Name:    "run",
				Summary: "Execute an epic's bead graph in parallel",
				Usage:   "kernl epic run [--workflow <path>] [--autonomous] [--interactive] <epic-id>",
				Details: `Flags:
  --workflow <path>  Use a custom workflow YAML instead of the default profile
  --autonomous       Let the DA infer the workflow shape without prompting
  --interactive      Confirm the inferred workflow shape before running`,
			},
			{
				Name:    "merge",
				Summary: "(Re-)run only the epic-level integration stages",
				Usage:   "kernl epic merge <epic-id>",
				Details: `Drives the epic bead through integration -> integration_review -> shipment.
Use it to recover a blocked epic; it does not run the children.`,
			},
			{
				Name:    "abort",
				Summary: "Close an epic and its children; clean worktrees and agent state",
				Usage:   "kernl epic abort [--dry-run] <epic-id> --yes",
				Details: `Destructive: closes the epic and every child bead, removes their worktrees
and purges agent state. Requires --yes; use --dry-run to preview.`,
			},
		},
	},
	{
		Name:    "bead",
		Summary: "Manage individual beads",
		Usage:   "kernl bead run <bead-id>",
		Subs: []commandMeta{
			{
				Name:    "run",
				Summary: "Drive a single bead through its workflow",
				Usage:   "kernl bead run <bead-id>",
			},
		},
	},
	{
		Name:    "sweep",
		Summary: "Close epics whose PRs are merged in master",
		Usage:   "kernl sweep [--yes | --dry-run] [--repo <path>] [--failure-threshold <n>] [--backoff-minutes <a,b,...>] [--stale-warn-days <n>]",
		Details: `Lists epics awaiting PR review, asks gh whether each PR merged, and closes
the merged ones (epic + children) in the tracker.

Without --yes this is a dry-run preview: nothing is closed.

Flags:
  --yes                     Actually close the merged epics
  --dry-run                 Preview without closing (default behavior)
  --repo <path>             Repo to sweep (default: current directory)
  --failure-threshold <n>   Consecutive failures before backing off
  --backoff-minutes <list>  Comma-separated backoff schedule, e.g. 5,15,60
  --stale-warn-days <n>     Warn when a PR is open longer than n days`,
	},
	{
		Name:    "bookmark",
		Summary: "Manage bookmarks",
		Usage:   "kernl bookmark <add|import> [args...]",
		Subs: []commandMeta{
			{
				Name:    "add",
				Summary: "Add a bookmark by URL (archives the page HTML)",
				Usage:   "kernl bookmark add <url>",
			},
			{
				Name:    "import",
				Summary: "Bulk-import bookmarks from an export file",
				Usage:   "kernl bookmark import <pocket|pinboard> <file>",
			},
		},
	},
	{
		Name:    "capture",
		Summary: "Capture a quick note/idea into the inbox (text arg or stdin)",
		Usage:   "kernl capture [--] <text> | echo <text> | kernl capture",
		Details: `Examples:
  kernl capture "call the accountant tomorrow"
  echo "idea: robot mode for the CLI" | kernl capture
  kernl capture -- --help   (captures the literal text "--help")`,
	},
	{
		Name:    "plan",
		Summary: "Show the vault notes relevant to a topic (substrate-aware planning)",
		Usage:   "kernl plan [--json] <topic>",
		Details: `Flags:
  --json  Emit {"topic","notes":[{"title","via","snippet"}]} on stdout

Example:
  kernl plan "caching strategy"`,
	},
	{
		Name:    "version",
		Summary: "Print version and build information",
		Usage:   "kernl version [--json]",
		Details: `Flags:
  --json  Emit {"version","commit","built","go"} on stdout`,
	},
}

func findCommand(table []commandMeta, name string) *commandMeta {
	for i := range table {
		if table[i].Name == name {
			return &table[i]
		}
	}
	return nil
}

// helpTopic reports whether args request help, and for which topic path.
// It fires on a leading "help" verb or on a --help/-h token anywhere before
// a literal "--" (end-of-flags sentinel).
func helpTopic(args []string) ([]string, bool) {
	if len(args) > 0 && args[0] == "help" {
		return args[1:], true
	}
	var topic []string
	for _, a := range args {
		if a == "--" {
			return nil, false
		}
		if a == "--help" || a == "-h" {
			return topic, true
		}
		if !strings.HasPrefix(a, "-") && len(topic) < 2 {
			topic = append(topic, a)
		}
	}
	return nil, false
}

// printHelpFor renders help for a topic path resolved against commandTable.
// An empty topic prints the root help.
func printHelpFor(topic []string) error {
	if len(topic) == 0 {
		return helpFn()
	}
	cmd := findCommand(commandTable, topic[0])
	if cmd == nil {
		return usagef("KERNL DISPATCH FAILURE: no help topic %q — valid topics: %s. Run: kernl --help",
			topic[0], strings.Join(commandNames(), ", "))
	}
	qualified := "kernl " + cmd.Name
	if len(topic) > 1 {
		sub := findCommand(cmd.Subs, topic[1])
		if sub == nil {
			return usagef("KERNL DISPATCH FAILURE: no help topic %q under %q — valid: %s. Run: kernl %s --help",
				topic[1], cmd.Name, strings.Join(subNames(cmd), ", "), cmd.Name)
		}
		qualified = qualified + " " + sub.Name
		cmd = sub
	}
	fmt.Println(renderCommandHelp(qualified, cmd))
	return nil
}

func renderCommandHelp(qualified string, cmd *commandMeta) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s — %s\n\nUsage:\n  %s\n", qualified, cmd.Summary, cmd.Usage)
	if len(cmd.Subs) > 0 {
		b.WriteString("\nSubcommands:\n")
		for _, s := range cmd.Subs {
			fmt.Fprintf(&b, "  %-10s %s\n", s.Name, s.Summary)
		}
	}
	if cmd.Details != "" {
		b.WriteString("\n" + cmd.Details + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func commandNames() []string {
	names := make([]string, 0, len(commandTable))
	for _, c := range commandTable {
		names = append(names, c.Name)
	}
	return names
}

func subNames(cmd *commandMeta) []string {
	names := make([]string, 0, len(cmd.Subs))
	for _, s := range cmd.Subs {
		names = append(names, s.Name)
	}
	return names
}
