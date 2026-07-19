package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
)

var taskSubcommands = []string{"list", "create", "set", "delete"}

var taskCommand = commandMeta{
	Name:    "task",
	Summary: "Manage tasks (the GUI's task board, from the shell)",
	Usage:   "kernl task <list|create|set|delete> [args...]",
	Details: `Talks to a running server over the REST API, so 'kernl serve' must be up
(or point elsewhere with --server <url> / KERNL_SERVER).

Run 'kernl task <subcommand> --help' for details on each.`,
	Subs: []commandMeta{
		{
			Name:    "list",
			Summary: "List tasks, optionally scoped to one project",
			Usage:   "kernl task list [--project <project-id>] [--json]",
			Details: `Flags:
  --project <id>  Only tasks belonging to that project
  --json          Emit the API's task array verbatim (camelCase)`,
		},
		{
			Name:    "create",
			Summary: "Create a task",
			Usage:   "kernl task create <title> [--title <t>] [--project <id>] [--status <status>] [--description <text>] [--tags <a,b>] [--due <YYYY-MM-DD>] [--json]",
			Details: `The title comes from the positional argument or --title, never both. An
unquoted multi-word positional title is joined into one string; the success
line quotes what was stored, so a swallowed word is visible.

Flags:
  --title <t>          The title, as an alternative to the positional form
  --project <id>       Attach the task to a project (creates the part_of edge)
  --status <status>    Initial status (server default when omitted)
  --description <text> Long-form body
  --tags <a,b,c>       Comma-separated tags
  --due <YYYY-MM-DD>   Due date, a calendar day (no time, no timezone)
  --json               Emit {"id"} on stdout

Example:
  kernl task create "renew the domain" --project prj-1 --due 2026-08-01`,
		},
		{
			Name:    "set",
			Summary: "Change fields of an existing task",
			Usage:   "kernl task set <task-id> [--title <text>] [--status <status>] [--tags <a,b>] [--due <YYYY-MM-DD>] [--json]",
			Details: `At least one field is required. Only the flags you pass are touched;
passing an empty value clears the field: --tags "" removes every tag and
--due "" removes the due date.

Flags:
  --title <text>      New title (cannot be empty)
  --status <status>   New status (cannot be empty)
  --tags <a,b,c>      Replace the tag set; "" clears it
  --due <YYYY-MM-DD>  Replace the due date; "" clears it
  --json              Emit {"id","updated"} on stdout`,
		},
		{
			Name:    "delete",
			Summary: "Delete a task and its companion note",
			Usage:   "kernl task delete <task-id> --yes [--json]",
			Details: `Destructive: removes the task node and the companion note that was
created with it, file included. Requires --yes; without it the task that
would be deleted is printed and nothing is sent to the server.`,
		},
	},
}

// taskView mirrors the fields of the API's task DTO that the human-readable
// listing prints. It is deliberately a subset: --json passes the server's own
// body through, so this struct never becomes a second wire contract.
type taskView struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Status    string   `json:"status"`
	ProjectID string   `json:"projectId"`
	Tags      []string `json:"tags"`
	DueDate   string   `json:"dueDate"`
}

func runTask(v verbContext, args []string) error {
	sub, rest, err := requireSub("task", args, taskSubcommands)
	if err != nil {
		return err
	}
	asJSON, rest := parseBoolFlag(rest, "--json")
	switch sub {
	case "list":
		return runTaskList(v, asJSON, rest)
	case "create":
		return runTaskCreate(v, asJSON, rest)
	case "set":
		return runTaskSet(v, asJSON, rest)
	default:
		return runTaskDelete(v, asJSON, rest)
	}
}

func runTaskList(v verbContext, asJSON bool, args []string) error {
	project, _, rest, err := takeFlag("task list", args, "--project")
	if err != nil {
		return err
	}
	if err := rejectUnknownFlags("task list", rest); err != nil {
		return err
	}
	if len(rest) > 0 {
		return usagef("KERNL DISPATCH FAILURE: task list takes no positional arguments, got %q — run: kernl task list --help", rest[0])
	}

	path := "/api/tasks"
	if project != "" {
		path += "?project=" + url.QueryEscape(project)
	}
	raw, err := requestTask(v, func(ctx context.Context, c *apiClient) (json.RawMessage, error) {
		return c.get(ctx, path)
	})
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(v.stdout(), raw)
	}
	return printTaskList(v.stdout(), raw)
}

func runTaskCreate(v verbContext, asJSON bool, args []string) error {
	body, err := taskCreateBody("task create", args)
	if err != nil {
		return err
	}
	raw, err := requestTask(v, func(ctx context.Context, c *apiClient) (json.RawMessage, error) {
		return c.post(ctx, "/api/tasks", body)
	})
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(v.stdout(), raw)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := decodeInto(raw, "POST /api/tasks", &created); err != nil {
		return err
	}
	title, _ := body["title"].(string)
	fmt.Fprintln(v.stdout(), createdLine("Created task", title, "", created.ID))
	return nil
}

func runTaskSet(v verbContext, asJSON bool, args []string) error {
	body, rest, err := taskPatchBody("task set", args)
	if err != nil {
		return err
	}
	if err := rejectUnknownFlags("task set", rest); err != nil {
		return err
	}
	id, err := singleTaskID("task set", rest)
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return usagef("KERNL DISPATCH FAILURE: task set needs at least one field to change — valid: --title, --status, --tags, --due. Run: kernl task set --help")
	}

	raw, err := requestTask(v, func(ctx context.Context, c *apiClient) (json.RawMessage, error) {
		return c.patch(ctx, "/api/tasks/"+url.PathEscape(id), body)
	})
	if err != nil {
		return err
	}
	if asJSON {
		return emitTaskAck(v.stdout(), raw, id, "updated")
	}
	fmt.Fprintf(v.stdout(), "Updated task %s\n", id)
	return nil
}

func runTaskDelete(v verbContext, asJSON bool, args []string) error {
	confirmed, rest := parseBoolFlag(args, "--yes")
	if err := rejectUnknownFlags("task delete", rest); err != nil {
		return err
	}
	id, err := singleTaskID("task delete", rest)
	if err != nil {
		return err
	}
	// Preview without contacting the server at all: an unconfirmed destructive
	// invocation must not depend on the server being reachable to be safe.
	if !confirmed {
		if asJSON {
			return emitJSON(v.stdout(), json.RawMessage(fmt.Sprintf(`{"id":%q,"deleted":false,"wouldDelete":true}`, id)))
		}
		fmt.Fprintf(v.stdout(), "Would delete task %s and its companion note. Re-run with --yes to confirm.\n", id)
		return nil
	}

	raw, err := requestTask(v, func(ctx context.Context, c *apiClient) (json.RawMessage, error) {
		return c.delete(ctx, "/api/tasks/"+url.PathEscape(id))
	})
	if err != nil {
		return err
	}
	if asJSON {
		return emitTaskAck(v.stdout(), raw, id, "deleted")
	}
	fmt.Fprintf(v.stdout(), "Deleted task %s\n", id)
	return nil
}

// requestTask builds the client only after the invocation has been validated,
// so a malformed command is diagnosed without needing a loadable config or a
// running server.
func requestTask(v verbContext, call func(context.Context, *apiClient) (json.RawMessage, error)) (json.RawMessage, error) {
	c, err := v.client()
	if err != nil {
		return nil, err
	}
	return call(context.Background(), c)
}

// taskCreateBody maps create flags onto the POST /api/tasks payload. Only flags
// the caller passed are included, so the server keeps ownership of the defaults.
func taskCreateBody(verb string, args []string) (map[string]any, error) {
	body := map[string]any{}
	rest := args
	for _, f := range []struct{ flag, field string }{
		{"--description", "description"},
		{"--status", "status"},
		{"--project", "projectId"},
		{"--due", "dueDate"},
	} {
		value, present, remaining, err := takeFlag(verb, rest, f.flag)
		if err != nil {
			return nil, err
		}
		rest = remaining
		if present {
			body[f.field] = value
		}
	}
	tags, present, rest, err := takeFlag(verb, rest, "--tags")
	if err != nil {
		return nil, err
	}
	if present {
		body["tags"] = splitTaskTags(tags)
	}
	title, hasTitle, rest, err := takeFlag(verb, rest, "--title")
	if err != nil {
		return nil, err
	}
	if err := rejectUnknownFlags("task create", rest); err != nil {
		return nil, err
	}
	resolved, err := taskCreateTitle(title, hasTitle, rest)
	if err != nil {
		return nil, err
	}
	body["title"] = resolved
	return body, nil
}

// taskCreateTitle resolves the title from --title or the positional args,
// refusing both at once the way project create does — silently preferring one
// would hide a typo'd flag.
//
// Unquoted multi-word titles are the common shell slip, and joining them is what
// the caller meant, so the join stays as interactive forgiveness. It is safe to
// keep now that --title gives an unambiguous alternative and the joined title is
// echoed back on success; before that the join's only stated safety net —
// "the title is echoed back anyway" — did not exist, since the verb printed the
// id alone.
func taskCreateTitle(title string, hasTitle bool, rest []string) (string, error) {
	if hasTitle && len(rest) > 0 {
		return "", usagef("KERNL DISPATCH FAILURE: task create got a title both positionally (%q) and via --title (%q) — pass only one",
			strings.Join(rest, " "), title)
	}
	if hasTitle {
		if strings.TrimSpace(title) == "" {
			return "", usagef(`KERNL DISPATCH FAILURE: task create got an empty --title — run: kernl task create --title "<title>"`)
		}
		return title, nil
	}
	if len(rest) == 0 {
		return "", usagef(`KERNL DISPATCH FAILURE: task create requires a title — run: kernl task create "<title>" [--project <id>]`)
	}
	return strings.Join(rest, " "), nil
}

// taskPatchBody maps set flags onto the PATCH payload and returns the leftover
// positional args. Presence, not emptiness, decides inclusion: the handler
// reads an absent key as "leave alone" and an empty value as "clear".
func taskPatchBody(verb string, args []string) (map[string]any, []string, error) {
	body := map[string]any{}
	rest := args
	for _, f := range []struct{ flag, field string }{
		{"--title", "title"},
		{"--status", "status"},
		{"--due", "dueDate"},
	} {
		value, present, remaining, err := takeFlag(verb, rest, f.flag)
		if err != nil {
			return nil, nil, err
		}
		rest = remaining
		if present {
			body[f.field] = value
		}
	}
	tags, present, rest, err := takeFlag(verb, rest, "--tags")
	if err != nil {
		return nil, nil, err
	}
	if present {
		body["tags"] = splitTaskTags(tags)
	}
	return body, rest, nil
}

// splitTaskTags always returns a non-nil slice: the handler distinguishes an
// omitted tags key from an empty array, and a nil slice would marshal to null,
// which reads as "omitted" and would silently fail to clear the tags.
func splitTaskTags(raw string) []string {
	tags := []string{}
	for _, t := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(t); trimmed != "" {
			tags = append(tags, trimmed)
		}
	}
	return tags
}

func singleTaskID(verb string, args []string) (string, error) {
	if len(args) == 0 {
		return "", usagef("KERNL DISPATCH FAILURE: %s requires a task ID — run: kernl %s <task-id>. List them with: kernl task list", verb, verb)
	}
	if len(args) > 1 {
		return "", usagef("KERNL DISPATCH FAILURE: %s takes exactly one task ID, got %d (%s) — run: kernl %s --help",
			verb, len(args), strings.Join(args, ", "), verb)
	}
	return args[0], nil
}

// emitTaskAck keeps --json parseable for the routes that answer 204 with no
// body: a script piping into jq must never receive an empty document.
func emitTaskAck(w io.Writer, raw json.RawMessage, id, action string) error {
	if len(bytes.TrimSpace(raw)) > 0 {
		return emitJSON(w, raw)
	}
	ack, err := json.Marshal(map[string]any{"id": id, action: true})
	if err != nil {
		return wrapLoud("encoding the "+action+" acknowledgement", err)
	}
	return emitJSON(w, ack)
}

func printTaskList(w io.Writer, raw json.RawMessage) error {
	var tasks []taskView
	if err := decodeInto(raw, "GET /api/tasks", &tasks); err != nil {
		return err
	}
	if len(tasks) == 0 {
		fmt.Fprintln(w, "No tasks. Create one with: kernl task create \"<title>\"")
		return nil
	}
	for _, t := range tasks {
		fmt.Fprintf(w, "%-24s [%-11s] %s%s\n", t.ID, t.Status, t.Title, taskAnnotations(t))
	}
	fmt.Fprintf(w, "\n%d task(s)\n", len(tasks))
	return nil
}

func taskAnnotations(t taskView) string {
	var parts []string
	if t.DueDate != "" {
		parts = append(parts, "due "+t.DueDate)
	}
	if t.ProjectID != "" {
		parts = append(parts, "project "+t.ProjectID)
	}
	for _, tag := range t.Tags {
		parts = append(parts, "#"+tag)
	}
	if len(parts) == 0 {
		return ""
	}
	return "  (" + strings.Join(parts, ", ") + ")"
}
