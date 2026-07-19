package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
)

var inboxCommand = commandMeta{
	Name:    "inbox",
	Summary: "Triage the capture inbox (list, add, process, classify)",
	Usage:   "kernl inbox <list|add|process|convert|reopen|classify|auto-classify|prep|rollups|batch-log|batch> [args...]",
	Details: `Every inbox verb talks to a running 'kernl serve' over the same REST API the
web inbox uses. 'kernl capture' is the offline path: it writes the graph
directly and needs no server.

A capture's body is never rewritten here — the CLI sends only what a capture
becomes (target, title, tags); the body travels untouched from the capture.

Run 'kernl inbox <subcommand> --help' for details on each.`,
	Subs: []commandMeta{
		{
			Name:    "list",
			Summary: "List pending captures (or the processed ones)",
			Usage:   "kernl inbox list [--processed] [--json]",
			Details: `Flags:
  --processed  List captures that already left the queue, and what they became
  --json       Emit the API response verbatim (camelCase)`,
		},
		{
			Name:    "add",
			Summary: "Add a capture through the running server (text arg or stdin)",
			Usage:   "kernl inbox add [--json] [--] <text> | echo <text> | kernl inbox add",
			Details: `Prints the new capture's ID; --json (first argument only) emits {"id"}.

Examples:
  kernl inbox add "call the accountant tomorrow"
  echo "idea: robot mode" | kernl inbox add
  kernl inbox add -- --help   (adds the literal text "--help")`,
		},
		{
			Name:    "process",
			Summary: "Turn a capture into a node (note, task, project, bookmark, discard)",
			Usage:   "kernl inbox process <capture-id> --target <note|task|project|bookmark|update|discard> [flags]",
			Details: `Flags:
  --target <t>       What the capture becomes (required)
  --title <text>     Title for the created node (default: the capture's own)
  --project-id <id>  File a task under an existing project
  --tags <a,b,c>     Comma-separated tags
  --due <YYYY-MM-DD> Due date; on --target task only
  --link-to <id>     Link the created node to an existing node
  --target-note <id> With --target update: the note to merge into
  --json             Emit the API response verbatim

One invocation sends one action. A capture that fans out into several nodes
at once is a GUI/API flow; here, reopen and process again.`,
		},
		{
			Name:    "convert",
			Summary: "Legacy single-action triage of a capture",
			Usage:   "kernl inbox convert <capture-id> <note|task|project|bookmark|discard> [--json]",
			Details: `Kept because the API keeps it. Prefer 'kernl inbox process', which carries a
title, tags, a project and a due date.`,
		},
		{
			Name:    "reopen",
			Summary: "Undo a process: delete the derived node, requeue the capture",
			Usage:   "kernl inbox reopen <capture-id> [--json]",
		},
		{
			Name:    "classify",
			Summary: "Run one LLM classification pass over the unclassified pending captures",
			Usage:   "kernl inbox classify [--json]",
			Details: `Fails loud when no LLM provider is configured — Fix: set llm.provider in
kernl.yaml, then restart the server.`,
		},
		{
			Name:    "auto-classify",
			Summary: "Read or flip the background auto-classify switch",
			Usage:   "kernl inbox auto-classify [on|off] [--json]",
			Details: `With no argument it only reads the current state and reports whether an LLM
is configured at all. The switch is session-only: it resets to the config
default when the server restarts.`,
		},
		{
			Name:    "prep",
			Summary: "Generate (or show) the DA briefing note for a capture",
			Usage:   "kernl inbox prep <capture-id> [--show] [--json]",
			Details: `Flags:
  --show  Read the existing prep note instead of generating one (exit 2 if none)

Generating needs an LLM provider; --show does not.`,
		},
		{
			Name:    "rollups",
			Summary: "Show captures grouped by the day they were created",
			Usage:   "kernl inbox rollups [--json]",
		},
		{
			Name:    "batch-log",
			Summary: "Show the raw and final segments of an imported batch",
			Usage:   "kernl inbox batch-log <batch-id> [--json]",
		},
		inboxBatchCommand,
	},
}

var inboxSubcommands = []string{
	"list", "add", "process", "convert", "reopen",
	"classify", "auto-classify", "prep", "rollups", "batch-log", "batch",
}

func runInbox(v verbContext, args []string) error {
	sub, rest, err := requireSub("inbox", args, inboxSubcommands)
	if err != nil {
		return err
	}
	// add parses its own flags: its payload is free text that may legitimately
	// look like a flag, exactly as `kernl capture` handles it.
	if sub == "add" {
		return inboxAdd(v, rest)
	}

	asJSON, rest := parseBoolFlag(rest, "--json")
	client, err := v.client()
	if err != nil {
		return err
	}
	ctx := context.Background()

	switch sub {
	case "list":
		return inboxList(ctx, v, client, asJSON, rest)
	case "process":
		return inboxProcess(ctx, v, client, asJSON, rest)
	case "convert":
		return inboxConvert(ctx, v, client, asJSON, rest)
	case "reopen":
		return inboxSimplePost(ctx, v, client, asJSON, "reopen", rest, "Reopened")
	case "classify":
		return inboxClassify(ctx, v, client, asJSON, rest)
	case "auto-classify":
		return inboxAutoClassify(ctx, v, client, asJSON, rest)
	case "prep":
		return inboxPrep(ctx, v, client, asJSON, rest)
	case "rollups":
		return inboxRollups(ctx, v, client, asJSON, rest)
	case "batch":
		return inboxBatch(ctx, v, client, asJSON, rest)
	}
	return inboxBatchLog(ctx, v, client, asJSON, rest)
}

// takeFlags pulls several value flags in one pass so a verb's own body stays
// about the request it builds rather than about argument plumbing.
func takeFlags(args []string, names ...string) (map[string]string, []string, error) {
	values := make(map[string]string, len(names))
	rest := args
	for _, name := range names {
		value, present, remaining, err := takeFlag(rest, name)
		if err != nil {
			return nil, nil, err
		}
		if present {
			values[name] = value
		}
		rest = remaining
	}
	return values, rest, nil
}

// requireID pulls the single positional id a verb needs, rejecting both a
// missing one and extra positionals that would otherwise be ignored silently.
func requireID(sub string, args []string) (string, error) {
	if err := rejectUnknownFlags("inbox "+sub, args); err != nil {
		return "", err
	}
	if len(args) == 0 {
		return "", usagef("KERNL DISPATCH FAILURE: inbox %s requires a capture id — run: kernl inbox %s --help", sub, sub)
	}
	if len(args) > 1 {
		return "", usagef("KERNL DISPATCH FAILURE: inbox %s takes exactly one capture id, got %d — run: kernl inbox %s --help", sub, len(args), sub)
	}
	return args[0], nil
}

func inboxList(ctx context.Context, v verbContext, c *apiClient, asJSON bool, args []string) error {
	processed, args := parseBoolFlag(args, "--processed")
	if err := rejectUnknownFlags("inbox list", args); err != nil {
		return err
	}
	if len(args) > 0 {
		return usagef("KERNL DISPATCH FAILURE: inbox list takes no arguments, got %q — run: kernl inbox list --help", args[0])
	}
	route := "/api/inbox/pending"
	if processed {
		route = "/api/inbox/processed"
	}
	raw, err := c.get(ctx, route)
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(v.stdout(), raw)
	}
	if processed {
		return printProcessed(v.stdout(), raw, route)
	}
	return printPending(v.stdout(), raw, route)
}

func inboxAdd(v verbContext, args []string) error {
	if len(args) > 0 && args[0] == "--json" {
		return inboxAddText(v, strings.Join(args[1:], " "), true)
	}
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	return inboxAddText(v, strings.Join(args, " "), false)
}

func inboxAddText(v verbContext, text string, asJSON bool) error {
	text = strings.TrimSpace(text)
	if text == "" {
		read, err := io.ReadAll(os.Stdin)
		if err != nil {
			return wrapLoud("reading capture text from stdin", err)
		}
		text = strings.TrimSpace(string(read))
	}
	if text == "" {
		return usagef("KERNL DISPATCH FAILURE: capture text cannot be empty — pass text as an argument or via stdin. Run: kernl inbox add --help")
	}
	client, err := v.client()
	if err != nil {
		return err
	}
	raw, err := client.post(context.Background(), "/api/inbox", map[string]string{"body": text})
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(v.stdout(), raw)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := decodeInto(raw, "POST /api/inbox", &created); err != nil {
		return err
	}
	_, err = fmt.Fprintf(v.stdout(), "Captured %s.\n", created.ID)
	return err
}

func inboxProcess(ctx context.Context, v verbContext, c *apiClient, asJSON bool, args []string) error {
	flags, args, err := takeFlags(args,
		"--target", "--title", "--project-id", "--tags", "--due", "--link-to", "--target-note")
	if err != nil {
		return err
	}
	id, err := requireID("process", args)
	if err != nil {
		return err
	}
	if strings.TrimSpace(flags["--target"]) == "" {
		return usagef("KERNL DISPATCH FAILURE: inbox process requires --target — valid: note, task, project, bookmark, update, discard. Run: kernl inbox process --help")
	}
	body := map[string]any{"actions": []any{buildCaptureAction(flags)}}
	if target := flags["--target-note"]; target != "" {
		body["targetNoteId"] = target
	}
	raw, err := c.post(ctx, "/api/inbox/"+url.PathEscape(id)+"/process", body)
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(v.stdout(), raw)
	}
	_, err = fmt.Fprintf(v.stdout(), "Processed %s as %s.\n", id, flags["--target"])
	return err
}

// buildCaptureAction maps the process flags onto one wire CaptureAction. The
// body field is deliberately never set: the capture's own body is the primary
// source, and the server fills it in from the capture itself.
func buildCaptureAction(flags map[string]string) map[string]any {
	action := map[string]any{"target": flags["--target"]}
	for flag, field := range map[string]string{
		"--title":      "title",
		"--project-id": "projectId",
		"--due":        "dueDate",
		"--link-to":    "linkTo",
	} {
		if value := flags[flag]; value != "" {
			action[field] = value
		}
	}
	if raw := strings.TrimSpace(flags["--tags"]); raw != "" {
		tags := strings.Split(raw, ",")
		for i := range tags {
			tags[i] = strings.TrimSpace(tags[i])
		}
		action["tags"] = tags
	}
	return action
}

func inboxConvert(ctx context.Context, v verbContext, c *apiClient, asJSON bool, args []string) error {
	if err := rejectUnknownFlags("inbox convert", args); err != nil {
		return err
	}
	if len(args) != 2 {
		return usagef("KERNL DISPATCH FAILURE: inbox convert requires <capture-id> <action> — valid actions: note, task, project, bookmark, discard. Run: kernl inbox convert --help")
	}
	id, action := args[0], args[1]
	raw, err := c.post(ctx, "/api/inbox/"+url.PathEscape(id)+"/convert", map[string]string{"action": action})
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(v.stdout(), raw)
	}
	_, err = fmt.Fprintf(v.stdout(), "Converted %s to %s.\n", id, action)
	return err
}

// inboxSimplePost covers the id-only POSTs whose response carries no data
// beyond {"status":"ok"}.
func inboxSimplePost(ctx context.Context, v verbContext, c *apiClient, asJSON bool, sub string, args []string, done string) error {
	id, err := requireID(sub, args)
	if err != nil {
		return err
	}
	raw, err := c.post(ctx, "/api/inbox/"+url.PathEscape(id)+"/"+sub, nil)
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(v.stdout(), raw)
	}
	_, err = fmt.Fprintf(v.stdout(), "%s %s.\n", done, id)
	return err
}

func inboxClassify(ctx context.Context, v verbContext, c *apiClient, asJSON bool, args []string) error {
	if err := rejectUnknownFlags("inbox classify", args); err != nil {
		return err
	}
	if len(args) > 0 {
		return usagef("KERNL DISPATCH FAILURE: inbox classify takes no arguments, got %q — run: kernl inbox classify --help", args[0])
	}
	raw, err := c.post(ctx, "/api/inbox/classify", nil)
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(v.stdout(), raw)
	}
	_, err = fmt.Fprintln(v.stdout(), "Classification pass complete.")
	return err
}

func inboxAutoClassify(ctx context.Context, v verbContext, c *apiClient, asJSON bool, args []string) error {
	if err := rejectUnknownFlags("inbox auto-classify", args); err != nil {
		return err
	}
	raw, err := autoClassifyRequest(ctx, c, args)
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(v.stdout(), raw)
	}
	var state struct {
		Enabled       bool `json:"enabled"`
		LLMConfigured bool `json:"llmConfigured"`
	}
	if err := decodeInto(raw, "/api/inbox/auto-classify", &state); err != nil {
		return err
	}
	_, err = fmt.Fprintf(v.stdout(), "auto-classify: %s (llm configured: %t)\n",
		onOff(state.Enabled), state.LLMConfigured)
	return err
}

// autoClassifyRequest keeps the read path a read: with no argument the verb
// must never write, so a bare invocation cannot flip the switch by accident.
func autoClassifyRequest(ctx context.Context, c *apiClient, args []string) (json.RawMessage, error) {
	if len(args) == 0 {
		return c.get(ctx, "/api/inbox/auto-classify")
	}
	if len(args) > 1 {
		return nil, usagef("KERNL DISPATCH FAILURE: inbox auto-classify takes at most one argument — valid: on, off. Run: kernl inbox auto-classify --help")
	}
	switch args[0] {
	case "on":
		return c.put(ctx, "/api/inbox/auto-classify", map[string]bool{"enabled": true})
	case "off":
		return c.put(ctx, "/api/inbox/auto-classify", map[string]bool{"enabled": false})
	}
	return nil, usagef("KERNL DISPATCH FAILURE: unknown auto-classify value %q%s — valid: on, off. Run: kernl inbox auto-classify --help",
		args[0], didYouMean(args[0], []string{"on", "off"}))
}

func onOff(enabled bool) string {
	if enabled {
		return "on"
	}
	return "off"
}

func inboxPrep(ctx context.Context, v verbContext, c *apiClient, asJSON bool, args []string) error {
	show, args := parseBoolFlag(args, "--show")
	id, err := requireID("prep", args)
	if err != nil {
		return err
	}
	route := "/api/inbox/" + url.PathEscape(id) + "/prep"
	// --show must stay a pure read: generating a briefing costs an LLM call.
	var raw json.RawMessage
	if show {
		raw, err = c.get(ctx, route)
	} else {
		raw, err = c.post(ctx, route, nil)
	}
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(v.stdout(), raw)
	}
	var note struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if err := decodeInto(raw, route, &note); err != nil {
		return err
	}
	_, err = fmt.Fprintf(v.stdout(), "%s\n\n%s\n", note.Title, note.Body)
	return err
}

func inboxRollups(ctx context.Context, v verbContext, c *apiClient, asJSON bool, args []string) error {
	if err := rejectUnknownFlags("inbox rollups", args); err != nil {
		return err
	}
	if len(args) > 0 {
		return usagef("KERNL DISPATCH FAILURE: inbox rollups takes no arguments, got %q — run: kernl inbox rollups --help", args[0])
	}
	raw, err := c.get(ctx, "/api/inbox/rollups")
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(v.stdout(), raw)
	}
	var payload struct {
		Rollups []struct {
			Date  string `json:"date"`
			Count int    `json:"count"`
		} `json:"rollups"`
	}
	if err := decodeInto(raw, "GET /api/inbox/rollups", &payload); err != nil {
		return err
	}
	for _, day := range payload.Rollups {
		if _, err := fmt.Fprintf(v.stdout(), "%s  %d captures\n", day.Date, day.Count); err != nil {
			return err
		}
	}
	return nil
}

func inboxBatchLog(ctx context.Context, v verbContext, c *apiClient, asJSON bool, args []string) error {
	if err := rejectUnknownFlags("inbox batch-log", args); err != nil {
		return err
	}
	if len(args) != 1 {
		return usagef("KERNL DISPATCH FAILURE: inbox batch-log requires exactly one batch id — run: kernl inbox batch-log --help")
	}
	raw, err := c.get(ctx, "/api/inbox/batch-log?batchId="+url.QueryEscape(args[0]))
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(v.stdout(), raw)
	}
	var log struct {
		BatchID      string `json:"batchId"`
		Source       string `json:"source"`
		ContextTitle string `json:"contextTitle"`
		RawEntries   []struct {
			Sequence int    `json:"sequence"`
			Body     string `json:"body"`
		} `json:"rawEntries"`
	}
	if err := decodeInto(raw, "GET /api/inbox/batch-log", &log); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(v.stdout(), "%s  %s  %s\n", log.BatchID, log.Source, log.ContextTitle); err != nil {
		return err
	}
	for _, entry := range log.RawEntries {
		if _, err := fmt.Fprintf(v.stdout(), "  %d  %s\n", entry.Sequence, firstLine(entry.Body)); err != nil {
			return err
		}
	}
	return nil
}

func printPending(w io.Writer, raw json.RawMessage, route string) error {
	var items []struct {
		ID      string `json:"id"`
		Title   string `json:"title"`
		Type    string `json:"type"`
		HasPrep bool   `json:"hasPrep"`
	}
	if err := decodeInto(raw, "GET "+route, &items); err != nil {
		return err
	}
	for _, item := range items {
		prep := ""
		if item.HasPrep {
			prep = "  [prep]"
		}
		if _, err := fmt.Fprintf(w, "%s  %-8s %s%s\n", item.ID, item.Type, item.Title, prep); err != nil {
			return err
		}
	}
	return nil
}

func printProcessed(w io.Writer, raw json.RawMessage, route string) error {
	var items []struct {
		CaptureID string `json:"captureId"`
		Title     string `json:"title"`
		Discarded bool   `json:"discarded"`
		Became    []struct {
			Type string `json:"type"`
		} `json:"became"`
	}
	if err := decodeInto(raw, "GET "+route, &items); err != nil {
		return err
	}
	for _, item := range items {
		became := "discarded"
		if !item.Discarded {
			types := make([]string, 0, len(item.Became))
			for _, node := range item.Became {
				types = append(types, node.Type)
			}
			became = strings.Join(types, "+")
		}
		if _, err := fmt.Fprintf(w, "%s  %-12s %s\n", item.CaptureID, became, item.Title); err != nil {
			return err
		}
	}
	return nil
}

func firstLine(body string) string {
	if i := strings.IndexByte(body, '\n'); i >= 0 {
		return body[:i]
	}
	return body
}
