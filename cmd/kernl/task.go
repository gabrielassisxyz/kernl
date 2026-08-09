package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

var taskSubcommands = []string{"list", "create", "set", "delete"}

var taskCommand = commandMeta{
	Name:    "task",
	Summary: "Manage tasks (the GUI's task board, from the shell)",
	Usage:   "kernl task <list|create|set|delete> [args...]",
	Details: `Talks to a running server over the REST API, so 'kernl serve' must be up
(or point elsewhere with --server <url> / KERNL_SERVER).

Example:
  kernl task list --project <project-id>

Run 'kernl task <subcommand> --help' for details on each.`,
	Subs: []commandMeta{
		{
			Name:    "list",
			Summary: "List the open tasks, optionally scoped to one project",
			Usage:   "kernl task list [--project <project-id>] [--status <status>] [--all] [--json]",
			Details: `Tasks that are done or closed are left out of the default listing, on
--json as well as on the human one: finished work outnumbers open work
several times over once a backlog has any history, and a list that shows
both is a list nobody reads. The omission is never silent: the count line
reports it, and under --json it goes to stderr so stdout stays a plain array.

closed is work that was called off rather than finished. Nothing moves into
it on its own, and it is left out of a project's progress entirely: counted
as done it would credit work nobody did, counted as outstanding it would
hold a finished project short of complete.

--all brings them back; --status done or --status closed lists only those.

{{flags}}`,
			Flags: []commandFlag{
				{Name: "--project", Value: "<id>", Description: "Only tasks belonging to that project"},
				{Name: "--status", Value: "<status>", Description: "Only tasks with that status: todo, in_progress, done or closed"},
				{Name: "--all", Description: "Keep every status, done and closed included"},
				{Name: "--json", Description: "Emit the API's task objects verbatim (camelCase), minus the ones the status filter dropped"},
			},
		},
		{
			Name:    "create",
			Summary: "Create a task",
			Usage:   "kernl task create <title> [--title <t>] [--project <id>] [--status <status>] [--description <text>] [--tags <a,b>] [--due <YYYY-MM-DD>] [--json]",
			Details: `The title comes from the positional argument or --title, never both. An
unquoted multi-word positional title is joined into one string; the success
line quotes what was stored, so a swallowed word is visible.

{{flags}}

Example:
  kernl task create "renew the domain" --project prj-1 --due 2026-08-01`,
			Flags: []commandFlag{
				{Name: "--title", Value: "<t>", Description: "The title, as an alternative to the positional form"},
				{Name: "--project", Value: "<id>", Description: "Attach the task to a project (creates the part_of edge)"},
				{Name: "--status", Value: "<status>", Description: "Initial status (server default when omitted)"},
				{Name: "--description", Value: "<text>", Description: "A sentence or two of context, mirrored into the companion note's frontmatter",
					Continuation: []string{"(the long-form material goes in the note's body)"}},
				{Name: "--tags", Value: "<a,b,c>", Description: "Comma-separated tags"},
				{Name: "--due", Value: "<YYYY-MM-DD>", Description: "Due date, a calendar day (no time, no timezone)"},
				{Name: "--json", Description: `Emit {"id"} on stdout`},
			},
		},
		{
			Name:    "set",
			Summary: "Change fields of an existing task",
			Usage:   "kernl task set <task-id> [--title <text>] [--description <text>] [--status <status>] [--tags <a,b>] [--due <YYYY-MM-DD>] [--json]",
			Details: `At least one field is required. Only the flags you pass are touched;
passing an empty value clears the field: --tags "" removes every tag and
--due "" removes the due date.

--description also rewrites the description in the frontmatter of the task's
companion note. That frontmatter belongs to kernl and is regenerated on every
sync, so edit the description here rather than in the file - a change typed
into the frontmatter is overwritten without warning. The body of the note,
below the frontmatter, is yours and is never touched.

{{flags}}`,
			Flags: []commandFlag{
				{Name: "--title", Value: "<text>", Description: "New title (cannot be empty). Does not rename the companion note"},
				{Name: "--description", Value: "<text>", Description: `Replace the description; "" clears it`},
				{Name: "--status", Value: "<status>", Description: "New status (cannot be empty)"},
				{Name: "--tags", Value: "<a,b,c>", Description: `Replace the tag set; "" clears it`},
				{Name: "--due", Value: "<YYYY-MM-DD>", Description: `Replace the due date; "" clears it`},
				{Name: "--json", Description: `Emit {"id","updated"} on stdout`},
			},
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
	status, hasStatus, rest, err := takeFlag("task list", rest, "--status")
	if err != nil {
		return err
	}
	all, rest := parseBoolFlag(rest, "--all")
	if err := rejectUnknownFlags("task list", rest); err != nil {
		return err
	}
	if len(rest) > 0 {
		return usagef("KERNL DISPATCH FAILURE: task list takes no positional arguments, got %q - run: kernl task list --help", rest[0])
	}
	if all && hasStatus {
		return usagef("KERNL DISPATCH FAILURE: task list got --all and --status %q together, and they contradict - --all keeps every status, --status keeps one. Run: kernl task list --help", status)
	}
	if hasStatus && !knownTaskStatus(status) {
		return usagef("KERNL DISPATCH FAILURE: task list got an unknown --status %q%s - valid: %s. Run: kernl task list --help",
			status, didYouMean(status, taskListStatuses), strings.Join(taskListStatuses, ", "))
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
	rows, err := decodeTaskRows(raw)
	if err != nil {
		return err
	}
	selection := selectTasks(rows, all, status, hasStatus)
	if asJSON {
		// The note goes to stderr, not into the document: stdout stays the
		// server's own array, and a machine reading it still gets told that
		// entries were dropped instead of having to re-run with --all to find out.
		if note := selection.hiddenFinishedNote(); note != "" {
			fmt.Fprintln(v.stderr(), note)
		}
		return emitTaskRows(v.stdout(), selection.kept)
	}
	return printTaskList(v.stdout(), selection)
}

// taskListStatuses is the vocabulary --status accepts, in board order. It reads
// the node package's constants instead of restating them, so a status cannot be
// legal in the graph and unknown to the filter at the same time.
var taskListStatuses = []string{nodes.TaskStatusTodo, nodes.TaskStatusInProgress, nodes.TaskStatusDone, nodes.TaskStatusClosed}

func knownTaskStatus(status string) bool {
	for _, s := range taskListStatuses {
		if s == status {
			return true
		}
	}
	return false
}

// taskRow pairs one task's bytes as the server sent them with the fields the CLI
// filters and prints on. Keeping the element verbatim is what lets --json stay
// the server's own document once the filter drops entries: re-encoding taskView
// would silently strip description, createdAt and updatedAt from a contract
// other tools read.
type taskRow struct {
	raw  json.RawMessage
	view taskView
}

func decodeTaskRows(raw json.RawMessage) ([]taskRow, error) {
	var elements []json.RawMessage
	if err := decodeInto(raw, "GET /api/tasks", &elements); err != nil {
		return nil, err
	}
	rows := make([]taskRow, 0, len(elements))
	for _, element := range elements {
		var view taskView
		if err := decodeInto(element, "GET /api/tasks", &view); err != nil {
			return nil, err
		}
		rows = append(rows, taskRow{raw: element, view: view})
	}
	return rows, nil
}

// taskSelection is what the filter did, not just what survived it. An empty
// result means something different depending on why it is empty, and a caller
// holding only the surviving rows cannot tell "nothing exists" from "nothing
// matched" - which is how a listing ends up telling someone their vault is empty
// while it holds three hundred tasks.
type taskSelection struct {
	kept           []taskRow
	hiddenFinished int
	// status is the --status value, empty when the flag was absent.
	status string
	// total is how many the server returned, before any filtering.
	total int
}

// selectTasks applies the status policy. The filtering is the client's own, not
// a query the server answers: the board and the web list need every status in
// one response, so hiding done is a listing decision, made where the listing is
// rendered. triageTasks already makes the same call for the same reason.
func selectTasks(rows []taskRow, all bool, status string, hasStatus bool) taskSelection {
	sel := taskSelection{kept: make([]taskRow, 0, len(rows)), total: len(rows)}
	if hasStatus {
		sel.status = status
	}
	for _, row := range rows {
		switch {
		case hasStatus:
			if row.view.Status == status {
				sel.kept = append(sel.kept, row)
			}
		case all:
			sel.kept = append(sel.kept, row)
		case row.view.Status == nodes.TaskStatusDone, row.view.Status == nodes.TaskStatusClosed:
			sel.hiddenFinished++
		default:
			sel.kept = append(sel.kept, row)
		}
	}
	return sel
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
		return usagef("KERNL DISPATCH FAILURE: task set needs at least one field to change - valid: --title, --description, --status, --tags, --due. Run: kernl task set --help")
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
			if err := emitJSON(v.stdout(), json.RawMessage(fmt.Sprintf(`{"id":%q,"deleted":false,"wouldDelete":true}`, id))); err != nil {
				return err
			}
			return refusedWithoutYes("task delete")
		}
		fmt.Fprintf(v.stdout(), "Would delete task %s and its companion note. Re-run with --yes to confirm.\n", id)
		return refusedWithoutYes("task delete")
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
// refusing both at once the way project create does - silently preferring one
// would hide a typo'd flag.
//
// Unquoted multi-word titles are the common shell slip, and joining them is what
// the caller meant, so the join stays as interactive forgiveness. It is safe to
// keep now that --title gives an unambiguous alternative and the joined title is
// echoed back on success; before that the join's only stated safety net  -
// "the title is echoed back anyway" - did not exist, since the verb printed the
// id alone.
func taskCreateTitle(title string, hasTitle bool, rest []string) (string, error) {
	if hasTitle && len(rest) > 0 {
		return "", usagef("KERNL DISPATCH FAILURE: task create got a title both positionally (%q) and via --title (%q) - pass only one",
			strings.Join(rest, " "), title)
	}
	if hasTitle {
		if strings.TrimSpace(title) == "" {
			return "", usagef(`KERNL DISPATCH FAILURE: task create got an empty --title - run: kernl task create --title "<title>"`)
		}
		return title, nil
	}
	if len(rest) == 0 {
		return "", usagef(`KERNL DISPATCH FAILURE: task create requires a title - run: kernl task create "<title>" [--project <id>]`)
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
		{"--description", "description"},
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
		return "", usagef("KERNL DISPATCH FAILURE: %s requires a task ID - run: kernl %s <task-id>. List them with: kernl task list", verb, verb)
	}
	if len(args) > 1 {
		return "", usagef("KERNL DISPATCH FAILURE: %s takes exactly one task ID, got %d (%s) - run: kernl %s --help",
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

// emitTaskRows re-assembles the array from the elements the server sent, so the
// only difference between this document and the response body is which entries
// are in it.
func emitTaskRows(w io.Writer, rows []taskRow) error {
	elements := make([]json.RawMessage, 0, len(rows))
	for _, row := range rows {
		elements = append(elements, row.raw)
	}
	body, err := json.Marshal(elements)
	if err != nil {
		return wrapLoud("encoding the filtered task list", err)
	}
	return emitJSON(w, body)
}

func printTaskList(w io.Writer, sel taskSelection) error {
	if len(sel.kept) == 0 {
		fmt.Fprintln(w, sel.emptyLine())
		return nil
	}
	for _, row := range sel.kept {
		t := row.view
		fmt.Fprintf(w, "%-24s [%-11s] %s%s\n", t.ID, t.Status, t.Title, taskAnnotations(t))
	}
	if note := sel.hiddenFinishedNote(); note != "" {
		fmt.Fprintf(w, "\n%d task(s), %s\n", len(sel.kept), note)
		return nil
	}
	fmt.Fprintf(w, "\n%d task(s)\n", len(sel.kept))
	return nil
}

// emptyLine answers the question the reader actually has, which is not "is the
// list empty" but "is there nothing, or did I filter it away". Reporting the
// total is the difference between the two, and it is the number a filtered
// empty result otherwise hides.
func (s taskSelection) emptyLine() string {
	switch {
	case s.status != "" && s.total > 0:
		return fmt.Sprintf("No tasks with status %q, out of %d in this listing. Widen it with: kernl task list --all", s.status, s.total)
	case s.hiddenFinished > 0:
		return fmt.Sprintf("No open tasks, and %d finished or called-off one(s) hidden. See them with: kernl task list --all", s.hiddenFinished)
	}
	return "No tasks. Create one with: kernl task create \"<title>\""
}

// hiddenFinishedNote is the honesty half of the default filter: a listing that
// drops rows without saying so reads as the whole truth about how much work
// exists. It names both terminal states rather than one, because a count
// labelled "done" over a pile of abandoned work is a number nobody can
// reconcile against the board.
func (s taskSelection) hiddenFinishedNote() string {
	if s.hiddenFinished == 0 {
		return ""
	}
	return fmt.Sprintf("%d done or closed hidden (--all keeps them, --status done|closed lists only those)", s.hiddenFinished)
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
