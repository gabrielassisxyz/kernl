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
	Flags   []commandFlag
	// FlagsHeading overrides the "Flags:" line above the block. `inbox batch`
	// needs it to say the flags cover all three of its subcommands - a fact
	// about scope, not decoration, which would have been lost by forcing every
	// block to the same heading.
	FlagsHeading string
	Subs         []commandMeta
}

// commandFlag is one flag, described once. The "Flags:" block of a help page is
// RENDERED from this - it is not written by hand into Details - so `--help` and
// `capabilities --json` cannot disagree about what a flag is or takes.
//
// WHY it works this way. Before this, the flags existed only as prose inside
// Details: an agent invited to pin against the machine contract could see 22
// verbs and none of their 125 flags, and the two descriptions could drift apart
// silently. Putting the structured data next to the prose would have been the
// same defect with extra steps - two sources for one fact. So the prose became
// the derived artifact and this became the source.
//
// Value is the placeholder shown after the flag ("<n>", "<path>"); empty means
// the flag is a boolean. Description is one line; use Continuation for the rare
// flag whose explanation genuinely needs a second one.
type commandFlag struct {
	Name         string
	Alias        string
	Value        string
	Description  string
	Continuation []string
}

var commandTable = []commandMeta{
	{
		// First in the table on purpose: it is the answer to the question an
		// agent or a returning human asks first, so it should be the first
		// thing `kernl --help` offers.
		Name:    "triage",
		Summary: "What needs attention right now, in one call",
		Usage:   "kernl triage [--json]",
		Details: `Answers "what should I do now" without five separate reads. Reports, in
the order you would act on them: captures waiting to be processed, ingest
reviews, beads in flight, beads ready to start, open tasks, approvals, and
whether the server is healthy. Each section names the command that shows
the rest.

A section that cannot be read says so and names why; it does not report
zero. "Nothing pending" and "could not ask" are different answers, and
conflating them is how a caller concludes no judgment gate is waiting.

Exit code is non-zero only when NOTHING could be read - a partial report
is a successful triage.

{{flags}}`,
		Flags: []commandFlag{
			{Name: "--json", Description: "Emit the whole report as one document, every section carrying",
				Continuation: []string{"{available, reason, count, items, command}"}},
		},
	},
	{
		Name:    "serve",
		Summary: "Start the HTTP API server (add --no-orchestrator for GUI-only)",
		Usage:   "kernl [--config <path>] [--port <port>] serve [--no-orchestrator]",
		Details: `{{flags}}

The server hosts the web GUI, the REST API and the SSE event streams.`,
		Flags: []commandFlag{
			{Name: "--port", Alias: "-p", Description: "Server port (default: from kernl.yaml, or 8080)"},
			{Name: "--no-orchestrator", Description: "Serve only the GUI/graph/notes; do not require bd"},
		},
	},
	{
		Name:    "doctor",
		Summary: "Run system checks (env, binaries, config)",
		Usage:   "kernl [--config <path>] doctor [--json]",
		Details: `Checks that the binaries the config names (each repo's tracker, each
configured agent), Go and the config file itself are usable.
Exit code is non-zero when a required check fails.

{{flags}}`,
		Flags: []commandFlag{
			{Name: "--json", Description: `Emit {"ok","checks":[...],"recommendedAction"} on stdout`},
		},
	},
	{
		Name:    "epic",
		Summary: "Manage epics (bead graphs)",
		Usage:   "kernl epic <list|run|merge|abort|events|sessions> [args...]",
		Details: `Run 'kernl epic <subcommand> --help' for details on each.

'events' and 'sessions' read an epic's history from a running 'kernl serve';
the others drive the orchestrator locally.`,
		Subs: append([]commandMeta{
			{
				Name:    "list",
				Summary: "List epics with child counts and state",
				Usage:   "kernl epic list [--json] [--repo <path|name>]",
				Details: "{{flags}}",
				Flags: []commandFlag{
					{Name: "--json", Description: `Emit {"epics":[{"id","title","children","state"}]} on stdout`},
					{Name: "--repo", Value: "<path|name>", Description: "Which registered repo to act on; defaults to the working directory's repo, required when it matches none or more than one is registered"},
				},
			},
			{
				Name:    "run",
				Summary: "Execute an epic's bead graph in parallel",
				Usage:   "kernl epic run [--workflow <path>] [--autonomous] [--interactive] [--stop-before-shipment] [--repo <path|name>] <epic-id>",
				Details: "{{flags}}",
				Flags: []commandFlag{
					{Name: "--workflow", Value: "<path>", Description: "Use a custom workflow YAML instead of the default profile"},
					{Name: "--autonomous", Description: "Let the DA infer the workflow shape without prompting"},
					{Name: "--interactive", Description: "With --autonomous: confirm the inferred shape first"},
					{Name: "--stop-before-shipment", Alias: "--dry-run", Description: "Run children and the epic's own integration for real; only shipment is withheld",
						Continuation: []string{
							"integration commits a real merge onto the epic branch, so this consumes that",
							"stage - a later real run can find nothing left to merge and get stuck. --dry-run",
							"is accepted as an alias, but that name overstates what the flag withholds.",
						}},
					{Name: "--repo", Value: "<path|name>", Description: "Which registered repo to act on; defaults to the working directory's repo, required when it matches none or more than one is registered"},
				},
			},
			{
				Name:    "merge",
				Summary: "(Re-)run only the epic-level integration stages",
				Usage:   "kernl epic merge [--stop-before-shipment] [--repo <path|name>] <epic-id>",
				Details: `Drives the epic bead through integration -> integration_review -> shipment.
Use it to recover a blocked epic; it does not run the children.

Shipment pushes to the remote declared in registry.repos[].shipment.allowedRemotes
and refuses any other. With no allow-list configured it refuses to push at all;
use --stop-before-shipment to run integration for real and withhold only shipment.

{{flags}}`,
				Flags: []commandFlag{
					{Name: "--stop-before-shipment", Alias: "--dry-run", Description: "Run integration and integration_review for real; only shipment is withheld",
						Continuation: []string{
							"integration commits a real merge onto the epic branch, so this consumes that",
							"stage - a later real run can find nothing left to merge and get stuck. --dry-run",
							"is accepted as an alias, but that name overstates what the flag withholds.",
						}},
					{Name: "--repo", Value: "<path|name>", Description: "Which registered repo to act on; defaults to the working directory's repo, required when it matches none or more than one is registered"},
				},
			},
			{
				Name:    "abort",
				Summary: "Close an epic and its children; clean worktrees and agent state",
				Usage:   "kernl epic abort [--dry-run] [--repo <path|name>] <epic-id> --yes",
				Details: `Destructive: closes the epic and every child bead, removes their worktrees
and purges agent state. Requires --yes; use --dry-run to preview.

{{flags}}`,
				Flags: []commandFlag{
					{Name: "--yes", Description: "Confirm the close; without it nothing is changed"},
					{Name: "--dry-run", Description: "Preview what would be closed and removed"},
					{Name: "--repo", Value: "<path|name>", Description: "Which registered repo to act on; defaults to the working directory's repo, required when it matches none or more than one is registered"},
				},
			},
		}, epicAPISubcommands...),
	},
	{
		Name:    "bead",
		Summary: "Manage individual beads (run one locally, or read and edit the tracker)",
		Usage:   "kernl bead <run|list|get|create|set|close|rollback|refine-scope|mark-terminal> [args...]",
		Details: `'bead run' drives an agent in a local worktree and needs the orchestrator
toolchain. Every other subcommand reads or edits the tracker through a
running 'kernl serve'.`,
		Subs: append([]commandMeta{
			{
				Name:    "run",
				Summary: "Drive a single bead through its workflow",
				Usage:   "kernl bead run [--dry-run] [--repo <path|name>] <bead-id>",
				Details: `--dry-run validates everything it can without writing anything, then stops
before the first change - the same refusal a real run would give, none of
its side effects. 'kernl epic run --stop-before-shipment' means something
weaker: it still dispatches and commits for real, withholding only shipment.

{{flags}}`,
				Flags: []commandFlag{
					{Name: "--dry-run", Description: "Validate without dispatching; stop before the first write"},
					{Name: "--repo", Value: "<path|name>", Description: "Which registered repo to act on; defaults to the working directory's repo, required when it matches none or more than one is registered"},
				},
			},
		}, beadAPISubcommands...),
	},
	{
		Name:    "sweep",
		Summary: "Close epics whose PRs are merged",
		Usage:   "kernl sweep [--yes | --dry-run] [--repo <path>] [--failure-threshold <n>] [--backoff-minutes <a,b,...>] [--stale-warn-days <n>]",
		Details: `Lists epics awaiting PR review, asks gh whether each PR merged, and closes
the merged ones (epic + children) in the tracker.

Without --yes this is a dry-run preview: nothing is closed.

{{flags}}`,
		Flags: []commandFlag{
			{Name: "--yes", Description: "Actually close the merged epics"},
			{Name: "--dry-run", Description: "Preview without closing (default behavior)"},
			{Name: "--repo", Value: "<path>", Description: "Repo to sweep (default: current directory)"},
			{Name: "--failure-threshold", Value: "<n>", Description: "Consecutive failures before backing off"},
			{Name: "--backoff-minutes", Value: "<list>", Description: "Comma-separated backoff schedule, e.g. 5,15,60"},
			{Name: "--stale-warn-days", Value: "<n>", Description: "Warn when a PR is open longer than n days"},
		},
	},
	orchestratorCommand,
	{
		Name:    "bookmark",
		Summary: "Manage bookmarks",
		Usage:   "kernl bookmark <add|import|retitle|rm> [args...]",
		Subs: []commandMeta{
			{
				Name:    "add",
				Summary: "Add a bookmark by URL (archives the page HTML)",
				Usage:   "kernl bookmark add [--title <title>] <url>",
				Details: `The title is taken from the archived page (og:title, then <title>,
then <h1>). Pass --title to set it yourself; extraction never
overwrites a title you supplied. If the page cannot be reached and
no --title was given, the URL stands in until you run retitle.

{{flags}}`,
				Flags: []commandFlag{
					{Name: "--title", Value: "<title>", Description: "Set the title instead of extracting it from the page"},
				},
			},
			{
				Name:    "import",
				Summary: "Bulk-import bookmarks from an export file",
				Usage:   "kernl bookmark import <pocket|pinboard> <file>",
			},
			{
				Name:    "retitle",
				Summary: "Change a bookmark's title",
				Usage:   "kernl bookmark retitle <id> <title>",
				Details: `Repairs bookmarks whose title was never extracted, including the
ones stored as their own URL. Does not re-fetch the page.`,
			},
			{
				Name:    "rm",
				Summary: "Delete a bookmark",
				Usage:   "kernl bookmark rm <id>",
				Details: `Deletes the bookmark and any generated companion note that describes it.
The companion markdown file is removed with the graph node so the vault
watcher cannot adopt or revive an orphaned note on the next pass.`,
			},
		},
	},
	{
		Name:    "capture",
		Summary: "Capture a quick note/idea into the inbox (text arg or stdin)",
		Usage:   "kernl capture [--json] [--] <text> | echo <text> | kernl capture",
		Details: `Prints the created capture's ID; --json (first arg only) emits
{"id","status"} instead.

Examples:
  kernl capture "call the accountant tomorrow"
  echo "idea: robot mode for the CLI" | kernl capture
  kernl capture -- --help   (captures the literal text "--help")`,
	},
	{
		Name:    "plan",
		Summary: "Show the vault notes relevant to a topic (substrate-aware planning)",
		Usage:   "kernl plan [--json] [--limit <n>] [--for-linking] [--link-budget <n>] <topic>",
		Details: `{{flags}}

Example:
  kernl plan "caching strategy"`,
		Flags: []commandFlag{
			{Name: "--json", Description: `Emit {"topic","notes":[{"id","title","via","snippet","path"}]} on stdout`,
				Continuation: []string{`path is null for via=claim (a claim has no file on disk)`}},
			{Name: "--limit", Value: "<n>", Description: "Maximum notes to return (default: 8)"},
			{Name: "--for-linking", Description: "Score the way link suggestion does, weighing a term by the",
				Continuation: []string{
					"rank FTS5 gives it so a long note stops matching every seed.",
					"The seed there is a whole note, not a question. Exists so the",
					"mechanism can be measured without writing a note to see it.",
				}},
			{Name: "--link-budget", Value: "<n>", Description: "Slots reserved for notes reached by an EDGE from a content hit,",
				Continuation: []string{
					"overriding planning.linkBudget in kernl.yaml for this call only.",
					"0 turns the expansion off. The reservation comes out of --limit,",
					"never on top of it, so it must be smaller than --limit.",
					"Only --for-linking uses it; the question path never expands.",
					"Exists to sweep the curve: the right budget grows as the graph",
					"densifies, so the config value ages and has to be re-measured.",
				}},
		},
	},
	{
		Name:    "capabilities",
		Summary: "Print the machine-readable CLI contract (JSON)",
		Usage:   "kernl capabilities [--json]",
		Details: `Emits every verb, flag, env var and exit code as JSON, plus a
contractVersion agents can pin against. Output is JSON with or without
the flag.`,
	},
	{
		Name:    "robot-docs",
		Summary: "Print the agent handbook (paste-ready, generated from metadata)",
		Usage:   "kernl robot-docs guide",
	},
	{
		Name:    "version",
		Summary: "Print version and build information",
		Usage:   "kernl version [--json]",
		Details: "{{flags}}",
		Flags: []commandFlag{
			{Name: "--json", Description: `Emit {"version","commit","built","go"} on stdout`},
		},
	},
	// GUI-parity verbs declare their own metadata next to their
	// implementation, so one file owns a verb's dispatch, help and tests.
	taskCommand,
	projectCommand,
	noteCommand,
	inboxCommand,
	memoryCommand,
	graphCommand,
	settingsCommand,
	healthCommand,
	approvalCommand,
	sessionCommand,
	ingestCommand,
}

func findCommand(table []commandMeta, name string) *commandMeta {
	for i := range table {
		if table[i].Name == name {
			return &table[i]
		}
	}
	return nil
}

// maxTopicDepth is how deep the command tree goes: `ingest queue resolve` and
// `inbox batch apply` are three tokens. Collecting fewer would silently answer a
// question about a leaf with its parent's page; collecting extra is harmless,
// since printHelpFor stops descending once a command has no sub-verbs.
const maxTopicDepth = 3

// helpTopic reports whether args request help, and for which topic path.
// It fires on a leading "help" verb, on "<verb> help [sub]" for verbs that
// have sub-verbs, or on a --help/-h token anywhere before a literal "--"
// (end-of-flags sentinel).
func helpTopic(args []string) ([]string, bool) {
	if len(args) > 0 && args[0] == "help" {
		return args[1:], true
	}
	if len(args) >= 2 && args[1] == "help" {
		if cmd := findCommand(commandTable, args[0]); cmd != nil && len(cmd.Subs) > 0 {
			topic := []string{args[0]}
			for _, a := range args[2:] {
				if strings.HasPrefix(a, "-") || len(topic) >= maxTopicDepth {
					break
				}
				topic = append(topic, a)
			}
			return topic, true
		}
	}
	var topic []string
	for _, a := range args {
		if a == "--" {
			return nil, false
		}
		if a == "--help" || a == "-h" {
			return topic, true
		}
		if !strings.HasPrefix(a, "-") && len(topic) < maxTopicDepth {
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
		return usagef("KERNL DISPATCH FAILURE: no help topic %q - valid topics: %s. Run: kernl --help",
			topic[0], strings.Join(commandNames(), ", "))
	}
	qualified := "kernl " + cmd.Name
	// Descend as far as the tree goes, not one level: `ingest queue resolve`
	// and `inbox batch apply` are real commands, and stopping at the parent
	// would answer a question about the child with the parent's page.
	//
	// Extra tokens under a command WITHOUT sub-verbs are the user's own args
	// (e.g. `kernl capture quick note --help`), not a topic path - show the
	// command's help rather than erroring on the user's text.
	for _, name := range topic[1:] {
		if len(cmd.Subs) == 0 {
			break
		}
		sub := findCommand(cmd.Subs, name)
		if sub == nil {
			return usagef("KERNL DISPATCH FAILURE: no help topic %q under %q - valid: %s. Run: kernl %s --help",
				name, cmd.Name, strings.Join(subNames(cmd), ", "), qualified[len("kernl "):])
		}
		qualified = qualified + " " + sub.Name
		cmd = sub
	}
	fmt.Println(renderCommandHelp(qualified, cmd))
	return nil
}

func renderCommandHelp(qualified string, cmd *commandMeta) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s - %s\n\nUsage:\n  %s\n", qualified, cmd.Summary, cmd.Usage)
	if len(cmd.Subs) > 0 {
		b.WriteString("\nSubcommands:\n")
		for _, s := range cmd.Subs {
			fmt.Fprintf(&b, "  %-14s %s\n", s.Name, s.Summary)
		}
	}
	if cmd.Details != "" {
		b.WriteString("\n" + expandFlagsPlaceholder(cmd) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// flagsPlaceholder marks where in a command's Details the rendered flag block
// goes. It is an explicit token rather than an appended section because the
// block sits mid-page - prose above it, an Example below - and moving it would
// change every help page this migration was required not to change.
const flagsPlaceholder = "{{flags}}"

// expandFlagsPlaceholder substitutes the rendered flag block into Details.
// TestEveryCommandWithFlagsRendersThem pins the two halves together, so a
// command can never carry flags it does not show or a token it cannot fill.
func expandFlagsPlaceholder(cmd *commandMeta) string {
	if !strings.Contains(cmd.Details, flagsPlaceholder) {
		return cmd.Details
	}
	heading := cmd.FlagsHeading
	if heading == "" {
		heading = "Flags:"
	}
	return strings.Replace(cmd.Details, flagsPlaceholder, heading+renderFlagBlock(cmd.Flags), 1)
}

// renderFlagBlock lays the flags out the way they were laid out by hand: each
// label padded to the width of the widest, then two spaces, then the
// description. Deriving the column instead of eyeballing it is a side benefit  -
// adding a long flag now re-aligns its neighbours automatically.
func renderFlagBlock(flags []commandFlag) string {
	width := 0
	for _, f := range flags {
		if n := len(flagLabel(f)); n > width {
			width = n
		}
	}
	var b strings.Builder
	for _, f := range flags {
		fmt.Fprintf(&b, "\n  %-*s  %s", width, flagLabel(f), f.Description)
		for _, line := range f.Continuation {
			// Continuations align under the description, not the flag.
			fmt.Fprintf(&b, "\n  %-*s  %s", width, "", line)
		}
	}
	return b.String()
}

func flagLabel(f commandFlag) string {
	label := f.Name
	if f.Alias != "" {
		label += ", " + f.Alias
	}
	if f.Value != "" {
		label += " " + f.Value
	}
	return label
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

// subcommandFlagOwners maps every sub-verb flag to the invocation that owns
// it, so a flag typed at the root (e.g. `kernl --dry-run`) can be redirected
// to where it actually works.
var subcommandFlagOwners = map[string]string{
	"--dry-run":           "kernl sweep --dry-run",
	"--yes":               "kernl sweep --yes",
	"--repo":              "kernl sweep --repo <path>",
	"--failure-threshold": "kernl sweep --failure-threshold <n>",
	"--backoff-minutes":   "kernl sweep --backoff-minutes <a,b,...>",
	"--stale-warn-days":   "kernl sweep --stale-warn-days <n>",
	"--workflow":          "kernl epic run --workflow <path> <epic-id>",
	"--autonomous":        "kernl epic run --autonomous <epic-id>",
	"--interactive":       "kernl epic run --interactive <epic-id>",
	// --json is near-universal now (every read verb has it), so the useful hint
	// is where it goes - after the subcommand - not a stale list of four verbs.
	"--json": "any read verb, placed after the subcommand: e.g. kernl task list --json",
}

// rootFlagHint builds the recovery hint for an unknown flag at the root:
// first a did-you-mean over global flags, then over sub-verb flags (naming
// the owning invocation), so a typo'd subcommand flag is never a dead end.
func rootFlagHint(flag string) string {
	// Exact sub-verb flag at the wrong scope is the most common case and
	// must hit before any fuzzy matching (suggest() only returns dist>=1).
	if owner, ok := subcommandFlagOwners[flag]; ok {
		return fmt.Sprintf(" - %s belongs to a subcommand: %s", flag, owner)
	}
	if hint := didYouMean(flag, globalFlagNames); hint != "" {
		return hint
	}
	owners := make([]string, 0, len(subcommandFlagOwners))
	for f := range subcommandFlagOwners {
		owners = append(owners, f)
	}
	if match := suggest(flag, owners); match != "" {
		return fmt.Sprintf(" - did you mean %q? It belongs to: %s", match, subcommandFlagOwners[match])
	}
	return ""
}

// verbAliasHints maps verbs agents reach for out of habit (bd/git idioms) to
// the kernl invocation they almost certainly meant.
var verbAliasHints = map[string]string{
	// "what is going on / what do I do now" all mean triage. They used to
	// answer `kernl epic list`, which sent someone asking what to work on to an
	// orchestrator listing that omits captures, tasks and approvals entirely.
	"status": "kernl triage",
	"ready":  "kernl triage",
	"next":   "kernl triage",
	"todo":   "kernl triage",
	"list":   "kernl epic list",
	"ls":     "kernl epic list",
	"run":    "kernl epic run <epic-id>",
	"check":  "kernl doctor",
	"init":   "kernl doctor",
}
