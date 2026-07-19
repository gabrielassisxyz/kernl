package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
)

// beadAPISubcommandNames is the half of `bead` the REST API serves. `run` is
// absent on purpose — it drives a local agent and is handled in bead.go — but
// every diagnostic below suggests against the full beadSubcommands set, so a
// user who typos `run` is still pointed at it.
var beadAPISubcommandNames = []string{"list", "get", "create", "set", "close", "mark-terminal", "rollback", "refine-scope"}

// beadAPISubcommands documents the API half of `bead`; help.go splices it into
// the `bead` entry next to `run`.
var beadAPISubcommands = []commandMeta{
	{
		Name:    "list",
		Summary: "List every bead in the registered repo",
		Usage:   "kernl bead list [--json]",
		Details: `The API serves the whole tracker: GET /api/beads takes no filters, so
there is nothing to scope by here. Filter downstream (--json | jq).`,
	},
	{
		Name:    "get",
		Summary: "Show one bead",
		Usage:   "kernl bead get <bead-id> [--json]",
		Details: `--json emits the API's full bead object; without it, the fields worth
reading on a terminal are printed.`,
	},
	{
		Name:    "create",
		Summary: "Create a bead",
		Usage:   "kernl bead create <title> [--type <t>] [--priority <n>] [--labels <a,b>] [--json]",
		Details: `Text flags: --description, --type, --assignee, --due, --acceptance,
--notes, --parent <bead-id>, --profile <id>, --workflow <id>.
Integers: --priority, --estimate. List: --labels <a,b,c>.
--json emits the created bead verbatim.

Invariants are not settable here: they are structured objects, not a flag.

Example:
  kernl bead create "wire the SSE reconnect" --parent ep-3 --priority 2`,
	},
	{
		Name:    "set",
		Summary: "Change fields of an existing bead",
		Usage:   "kernl bead set <bead-id> [--title <t>] [--state <s>] [--priority <n>] [--json]",
		Details: `At least one field is required; only the flags you pass are sent.

Text flags: --title, --description, --type, --state, --assignee, --due,
--acceptance, --notes, --parent, --profile.
Integers: --priority, --estimate.
Labels: --labels adds, --set-labels replaces the whole set, --remove-labels drops.
--json emits the updated bead verbatim.`,
	},
	{
		Name:    "close",
		Summary: "Close a bead, recording why",
		Usage:   "kernl bead close <bead-id> [--reason <text>] --yes [--json]",
		Details: `Requires --yes: closing a bead is what unblocks its dependents, so the
orchestrator may immediately dispatch agents and create worktrees for them.
Without --yes the close is described and nothing is sent.
--reason <text> is stored on the bead.`,
	},
	{
		Name:    "mark-terminal",
		Summary: "Force a bead into a terminal state, skipping its workflow",
		Usage:   "kernl bead mark-terminal <bead-id> --state <state> [--reason <text>] --yes [--json]",
		Details: `Requires --yes: this abandons every workflow stage the bead had left —
review, integration, shipment — without running them. Use it to correct a
bead the orchestrator cannot finish, not to advance one.
--state is required; --reason is appended to the bead's notes.`,
	},
	{
		Name:    "rollback",
		Summary: "Rewind a bead to an earlier workflow state",
		Usage:   "kernl bead rollback <bead-id> --state <state> [--reason <text>] --yes [--json]",
		Details: `Requires --yes: rewinding discards the progress recorded after the target
state, and re-running those stages re-spawns agents.
--state is required; --reason records why the correction was needed.

The bd backend does not implement rewind — against a bd-backed server this
route fails loud. The verb exists for backends that do.`,
	},
	{
		Name:    "refine-scope",
		Summary: "Rewrite a bead's description, notes or acceptance criteria",
		Usage:   "kernl bead refine-scope <bead-id> [--description <t>] [--notes <t>] [--acceptance <t>] [--json]",
		Details: `At least one of the three is required. Each one you pass replaces that
field wholesale.`,
	},
}

// beadView is the subset of the API's bead object the terminal listing shows.
// --json passes the server's own body through, so this never becomes a second
// wire contract.
type beadView struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	State    string   `json:"state"`
	Title    string   `json:"title"`
	Priority int      `json:"priority"`
	Assignee string   `json:"assignee"`
	ParentID string   `json:"parentId"`
	Labels   []string `json:"labels"`
}

func runBeadAPI(v verbContext, args []string) error {
	if len(args) == 0 {
		return usagef("KERNL DISPATCH FAILURE: bead requires a subcommand — valid: %s. Run: kernl bead --help",
			strings.Join(beadSubcommands, ", "))
	}
	sub, rest, err := requireSub("bead", args, beadAPISubcommandNames)
	if err != nil {
		// Suggest against the full set: `kernl bead rn` must find `run`, which
		// this file does not serve.
		return usagef("KERNL DISPATCH FAILURE: unknown bead subcommand %q%s — valid: %s. Run: kernl bead --help",
			args[0], didYouMean(args[0], beadSubcommands), strings.Join(beadSubcommands, ", "))
	}
	asJSON, rest := parseBoolFlag(rest, "--json")
	switch sub {
	case "list":
		return runBeadList(v, asJSON, rest)
	case "get":
		return runBeadGet(v, asJSON, rest)
	case "create":
		return runBeadCreate(v, asJSON, rest)
	case "set":
		return runBeadSet(v, asJSON, rest)
	case "close":
		return runBeadClose(v, asJSON, rest)
	case "mark-terminal":
		return runBeadTerminal(v, asJSON, rest, "mark-terminal")
	case "rollback":
		return runBeadTerminal(v, asJSON, rest, "rollback")
	default:
		return runBeadRefineScope(v, asJSON, rest)
	}
}

func runBeadList(v verbContext, asJSON bool, args []string) error {
	if err := rejectUnknownFlags("bead list", args); err != nil {
		return err
	}
	if len(args) > 0 {
		return usagef("KERNL DISPATCH FAILURE: bead list takes no arguments, got %q — GET /api/beads has no filters; pipe --json through jq instead", args[0])
	}
	raw, err := beadAPICall(v, func(ctx context.Context, c *apiClient) (json.RawMessage, error) {
		return c.get(ctx, "/api/beads")
	})
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(v.stdout(), raw)
	}
	return printBeadList(v.stdout(), raw)
}

func runBeadGet(v verbContext, asJSON bool, args []string) error {
	if err := rejectUnknownFlags("bead get", args); err != nil {
		return err
	}
	id, err := singleBeadID("bead get", args)
	if err != nil {
		return err
	}
	raw, err := beadAPICall(v, func(ctx context.Context, c *apiClient) (json.RawMessage, error) {
		return c.get(ctx, "/api/beads/"+url.PathEscape(id))
	})
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(v.stdout(), raw)
	}
	return printBeadDetail(v.stdout(), raw)
}

func runBeadCreate(v verbContext, asJSON bool, args []string) error {
	body, rest, err := beadFieldBody(args, beadCreateFields)
	if err != nil {
		return err
	}
	if err := rejectUnknownFlags("bead create", rest); err != nil {
		return err
	}
	if len(rest) == 0 {
		return usagef(`KERNL DISPATCH FAILURE: bead create requires a title — run: kernl bead create "<title>" [--parent <epic-id>]`)
	}
	// Unquoted multi-word titles are the common shell slip, and the title is
	// echoed back on success, so joining is safe and is what was meant.
	body["title"] = strings.Join(rest, " ")

	raw, err := beadAPICall(v, func(ctx context.Context, c *apiClient) (json.RawMessage, error) {
		return c.post(ctx, "/api/beads", body)
	})
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(v.stdout(), raw)
	}
	var created beadView
	if err := decodeInto(raw, "POST /api/beads", &created); err != nil {
		return err
	}
	fmt.Fprintf(v.stdout(), "Created bead %s\n", created.ID)
	return nil
}

func runBeadSet(v verbContext, asJSON bool, args []string) error {
	body, rest, err := beadFieldBody(args, beadUpdateFields)
	if err != nil {
		return err
	}
	if err := rejectUnknownFlags("bead set", rest); err != nil {
		return err
	}
	id, err := singleBeadID("bead set", rest)
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return usagef("KERNL DISPATCH FAILURE: bead set needs at least one field to change — run: kernl bead set --help")
	}
	raw, err := beadAPICall(v, func(ctx context.Context, c *apiClient) (json.RawMessage, error) {
		return c.patch(ctx, "/api/beads/"+url.PathEscape(id), body)
	})
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(v.stdout(), raw)
	}
	fmt.Fprintf(v.stdout(), "Updated bead %s\n", id)
	return nil
}

func runBeadClose(v verbContext, asJSON bool, args []string) error {
	confirmed, rest := parseBoolFlag(args, "--yes")
	reason, _, rest, err := takeFlag(rest, "--reason")
	if err != nil {
		return err
	}
	if err := rejectUnknownFlags("bead close", rest); err != nil {
		return err
	}
	id, err := singleBeadID("bead close", rest)
	if err != nil {
		return err
	}
	// Preview without contacting the server: an unconfirmed state change must
	// not depend on the server being reachable to be safe.
	if !confirmed {
		fmt.Fprintf(v.stdout(), "Would close bead %s%s, which unblocks its dependents. Re-run with --yes to confirm.\n", id, beadReasonSuffix(reason))
		return nil
	}
	raw, err := beadAPICall(v, func(ctx context.Context, c *apiClient) (json.RawMessage, error) {
		return c.post(ctx, "/api/beads/"+url.PathEscape(id)+"/close", map[string]any{"reason": reason})
	})
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(v.stdout(), raw)
	}
	var closed struct {
		State string `json:"state"`
	}
	if err := decodeInto(raw, "POST /api/beads/{id}/close", &closed); err != nil {
		return err
	}
	fmt.Fprintf(v.stdout(), "Closed bead %s (state %s)\n", id, closed.State)
	return nil
}

// runBeadTerminal serves mark-terminal and rollback: same payload, same gate,
// different route and different blast radius in the preview text.
func runBeadTerminal(v verbContext, asJSON bool, args []string, sub string) error {
	confirmed, rest := parseBoolFlag(args, "--yes")
	state, _, rest, err := takeFlag(rest, "--state")
	if err != nil {
		return err
	}
	reason, _, rest, err := takeFlag(rest, "--reason")
	if err != nil {
		return err
	}
	if err := rejectUnknownFlags("bead "+sub, rest); err != nil {
		return err
	}
	id, err := singleBeadID("bead "+sub, rest)
	if err != nil {
		return err
	}
	if state == "" {
		return usagef("KERNL DISPATCH FAILURE: bead %s requires a target state — run: kernl bead %s %s --state <state> --yes", sub, sub, id)
	}
	if !confirmed {
		fmt.Fprintf(v.stdout(), "Would %s bead %s to state %q%s, %s. Re-run with --yes to confirm.\n",
			sub, id, state, beadReasonSuffix(reason), beadTerminalBlastRadius(sub))
		return nil
	}
	body := map[string]any{"targetState": state, "reason": reason}
	raw, err := beadAPICall(v, func(ctx context.Context, c *apiClient) (json.RawMessage, error) {
		return c.post(ctx, "/api/beads/"+url.PathEscape(id)+"/"+sub, body)
	})
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(v.stdout(), raw)
	}
	fmt.Fprintf(v.stdout(), "Bead %s moved to %s\n", id, state)
	return nil
}

func runBeadRefineScope(v verbContext, asJSON bool, args []string) error {
	body, rest, err := beadFieldBody(args, []beadFlagField{
		{"--description", "description", beadFieldText},
		{"--notes", "notes", beadFieldText},
		{"--acceptance", "acceptance", beadFieldText},
	})
	if err != nil {
		return err
	}
	if err := rejectUnknownFlags("bead refine-scope", rest); err != nil {
		return err
	}
	id, err := singleBeadID("bead refine-scope", rest)
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return usagef("KERNL DISPATCH FAILURE: bead refine-scope needs at least one of --description, --notes, --acceptance — run: kernl bead refine-scope --help")
	}
	raw, err := beadAPICall(v, func(ctx context.Context, c *apiClient) (json.RawMessage, error) {
		return c.post(ctx, "/api/beads/"+url.PathEscape(id)+"/refine-scope", body)
	})
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(v.stdout(), raw)
	}
	fmt.Fprintf(v.stdout(), "Refined scope of bead %s\n", id)
	return nil
}

// beadAPICall builds the client only after the invocation has been validated,
// so a malformed command is diagnosed without needing a running server.
func beadAPICall(v verbContext, call func(context.Context, *apiClient) (json.RawMessage, error)) (json.RawMessage, error) {
	c, err := v.client()
	if err != nil {
		return nil, err
	}
	return call(context.Background(), c)
}

// beadFieldKind is how a flag's value is encoded. The tracker types some of
// its fields as arrays and some as integers, and either one sent as the string
// the user typed is rejected by the JSON decoder.
type beadFieldKind int

const (
	beadFieldText beadFieldKind = iota
	beadFieldList
	beadFieldInt
)

// beadFlagField maps one CLI flag onto one field of the route's payload.
type beadFlagField struct {
	flag  string
	field string
	kind  beadFieldKind
}

var beadCreateFields = []beadFlagField{
	{"--description", "description", beadFieldText}, {"--type", "type", beadFieldText},
	{"--assignee", "assignee", beadFieldText}, {"--due", "due", beadFieldText},
	{"--acceptance", "acceptance", beadFieldText}, {"--notes", "notes", beadFieldText},
	{"--parent", "parentId", beadFieldText}, {"--profile", "profileId", beadFieldText},
	{"--workflow", "workflowId", beadFieldText}, {"--labels", "labels", beadFieldList},
	{"--priority", "priority", beadFieldInt}, {"--estimate", "estimate", beadFieldInt},
}

var beadUpdateFields = []beadFlagField{
	{"--title", "title", beadFieldText}, {"--description", "description", beadFieldText},
	{"--type", "type", beadFieldText}, {"--state", "state", beadFieldText},
	{"--assignee", "assignee", beadFieldText}, {"--due", "due", beadFieldText},
	{"--acceptance", "acceptance", beadFieldText}, {"--notes", "notes", beadFieldText},
	{"--parent", "parentId", beadFieldText}, {"--profile", "profileId", beadFieldText},
	{"--labels", "labels", beadFieldList}, {"--set-labels", "setLabels", beadFieldList},
	{"--remove-labels", "removeLabels", beadFieldList},
	{"--priority", "priority", beadFieldInt}, {"--estimate", "estimate", beadFieldInt},
}

// beadFieldBody strips every known field flag off args and returns the payload
// plus the leftover positional arguments. Presence, not emptiness, decides
// inclusion, so an omitted flag leaves the field alone.
func beadFieldBody(args []string, fields []beadFlagField) (map[string]any, []string, error) {
	body := map[string]any{}
	rest := args
	for _, f := range fields {
		value, present, remaining, err := takeFlag(rest, f.flag)
		if err != nil {
			return nil, nil, err
		}
		rest = remaining
		if !present {
			continue
		}
		switch f.kind {
		case beadFieldList:
			body[f.field] = splitBeadLabels(value)
		case beadFieldInt:
			n, convErr := strconv.Atoi(strings.TrimSpace(value))
			if convErr != nil {
				return nil, nil, usagef("KERNL DISPATCH FAILURE: %s requires an integer, got %q — example: %s 2", f.flag, value, f.flag)
			}
			body[f.field] = n
		default:
			body[f.field] = value
		}
	}
	return body, rest, nil
}

// splitBeadLabels always returns a non-nil slice: a nil slice marshals to null,
// which the tracker reads as "omitted" rather than "clear these".
func splitBeadLabels(raw string) []string {
	labels := []string{}
	for _, l := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(l); trimmed != "" {
			labels = append(labels, trimmed)
		}
	}
	return labels
}

func singleBeadID(verb string, args []string) (string, error) {
	if len(args) == 0 {
		return "", usagef("KERNL DISPATCH FAILURE: %s requires a bead ID — run: kernl %s <bead-id>. List them with: kernl bead list", verb, verb)
	}
	if len(args) > 1 {
		return "", usagef("KERNL DISPATCH FAILURE: %s takes exactly one bead ID, got %d (%s) — run: kernl %s --help",
			verb, len(args), strings.Join(args, ", "), verb)
	}
	return args[0], nil
}

func beadReasonSuffix(reason string) string {
	if reason == "" {
		return ""
	}
	return fmt.Sprintf(" with reason %q", reason)
}

func beadTerminalBlastRadius(sub string) string {
	if sub == "rollback" {
		return "discarding the progress recorded after that state"
	}
	return "skipping every workflow stage it had left"
}

func printBeadList(w io.Writer, raw json.RawMessage) error {
	var beads []beadView
	if err := decodeInto(raw, "GET /api/beads", &beads); err != nil {
		return err
	}
	if len(beads) == 0 {
		fmt.Fprintln(w, "No beads. Create one with: kernl bead create \"<title>\"")
		return nil
	}
	for _, b := range beads {
		fmt.Fprintf(w, "%-20s [%-14s] %-8s %s%s\n", b.ID, b.State, b.Type, b.Title, beadAnnotations(b))
	}
	fmt.Fprintf(w, "\n%d bead(s)\n", len(beads))
	return nil
}

func beadAnnotations(b beadView) string {
	var parts []string
	if b.ParentID != "" {
		parts = append(parts, "parent "+b.ParentID)
	}
	if b.Assignee != "" {
		parts = append(parts, "@"+b.Assignee)
	}
	for _, l := range b.Labels {
		parts = append(parts, "#"+l)
	}
	if len(parts) == 0 {
		return ""
	}
	return "  (" + strings.Join(parts, ", ") + ")"
}

func printBeadDetail(w io.Writer, raw json.RawMessage) error {
	var b beadView
	if err := decodeInto(raw, "GET /api/beads/{id}", &b); err != nil {
		return err
	}
	fmt.Fprintf(w, "%s  %s\n", b.ID, b.Title)
	fmt.Fprintf(w, "  type %s · state %s · priority %d%s\n", b.Type, b.State, b.Priority, beadAnnotations(b))
	return nil
}
