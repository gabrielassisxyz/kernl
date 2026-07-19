package main

import (
	"fmt"
	"io"
	"strings"
)

// runRobotDocs prints the agent-facing handbook, generated from the same
// metadata as dispatch/help/capabilities so it never goes stale by hand.
func runRobotDocs(w io.Writer, args []string) error {
	if len(args) > 0 && args[0] != "guide" {
		return usagef("KERNL DISPATCH FAILURE: unknown robot-docs topic %q%s — valid: guide",
			args[0], didYouMean(args[0], []string{"guide"}))
	}
	fmt.Fprintln(w, renderRobotGuide())
	return nil
}

func renderRobotGuide() string {
	var b strings.Builder
	b.WriteString(`# kernl — agent handbook

Read the machine contract first: kernl capabilities --json
(verbs, flags, env vars, exit codes, contractVersion).

## Ground rules
- stdout is data, stderr is diagnostics. All errors carry the greppable
  marker KERNL DISPATCH FAILURE and exit non-zero.
- Exit codes: 0 success · 1 runtime/internal error · 2 usage error, which
  includes a destructive invocation refused for want of --yes.
- --help/-h/help work on every verb and sub-verb, never run work, exit 0.
- NO_COLOR=1, TERM=dumb or CI=true guarantee ANSI-free output.
- Destructive verbs are gated, and a refused one EXITS 2 while printing the
  preview: task/note/project delete, inbox reopen, inbox batch apply,
  bead close, bead mark-terminal, bead rollback, approval resolve, epic abort.
  Never read exit 0 from one of these as "it happened" — re-run with --yes.
- 'sweep' is the deliberate exception and exits 0: its default mode is a
  read-only scan announced on stderr, not a mutation you asked for and lost.

## JSON read surfaces
`)
	for _, line := range []string{
		"kernl epic list --json      {\"epics\":[{\"id\",\"title\",\"children\",\"state\"}]}",
		"kernl plan --json <topic>   {\"topic\",\"notes\":[{\"title\",\"via\",\"snippet\"}]}",
		"kernl doctor --json         {\"ok\",\"checks\":[...],\"recommendedAction\"}",
		"kernl version --json        {\"version\",\"commit\",\"built\",\"go\"}",
		"kernl capabilities          full contract, always JSON",
	} {
		fmt.Fprintf(&b, "  %s\n", line)
	}
	b.WriteString("\n## Verbs\n")
	for _, c := range commandTable {
		fmt.Fprintf(&b, "  %-12s %s\n", c.Name, c.Summary)
		for _, s := range c.Subs {
			fmt.Fprintf(&b, "    %s\n", s.Usage)
		}
	}
	b.WriteString("\n## Environment\n")
	for _, e := range capabilityEnvVars {
		fmt.Fprintf(&b, "  %-28s %s\n", e.Name, e.Description)
	}
	b.WriteString(`
## Recipes
  # preflight, machine-readable
  kernl doctor --json | jq -r .recommendedAction
  # capture from a pipe (text starting with '-' needs the sentinel)
  echo "idea" | kernl capture
  kernl capture -- "--weird-text"
  # what would sweep close? then actually close it
  kernl sweep --dry-run
  kernl sweep --yes`)
	return b.String()
}
