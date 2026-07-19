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

var projectCommand = commandMeta{
	Name:    "project",
	Summary: "Manage projects (list, create, edit, delete)",
	Usage:   "kernl project <list|create|set|delete> [args...]",
	Details: `Projects are graph nodes with a companion note in the vault. These verbs
call the same REST API the web GUI calls, so a server must be running:
start one with 'kernl serve', or point elsewhere with --server <url>
(env: KERNL_SERVER).

Run 'kernl project <subcommand> --help' for details on each.`,
	Subs: []commandMeta{
		{
			Name:    "list",
			Summary: "List projects with status and task counts",
			Usage:   "kernl project list [--json]",
			Details: `Flags:
  --json  Emit the API's [{"id","title","description","status","tags",
          "createdAt","updatedAt","taskCount","doneCount"}] on stdout`,
		},
		{
			Name:    "create",
			Summary: "Create a project (and its companion note)",
			Usage:   "kernl project create [--json] [--description <text>] [--status <s>] [--tags <a,b>] <title>",
			Details: `The title is required; give it positionally or with --title.

Flags:
  --title <text>        Title, when you prefer a flag over the positional arg
  --description <text>  Free-text description
  --status <status>     Initial status (server-side default when omitted)
  --tags <a,b,c>        Comma-separated tags
  --json                Emit {"id"} on stdout

Example:
  kernl project create --tags home,infra "Rebuild the homelab backups"`,
		},
		{
			Name:    "set",
			Summary: "Edit a project's title, description, status or tags",
			Usage:   "kernl project set [--json] [--title <t>] [--description <d>] [--status <s>] [--tags <a,b>] <id>",
			Details: `Only the fields you pass are touched; at least one is required. Passing an
empty value clears the field ('--tags ""' removes every tag), which is why
omitting a flag and passing it empty are different things.

Flags:
  --title <text>        New title (may not be empty)
  --description <text>  New description ("" clears it)
  --status <status>     New status (may not be empty)
  --tags <a,b,c>        Replace the tag list ("" clears it)
  --json                Emit {"id","updated":true} on stdout

Example:
  kernl project set --status done fkq3v9`,
		},
		{
			Name:    "delete",
			Summary: "Delete a project and its companion note (requires --yes)",
			Usage:   "kernl project delete [--json] <id> --yes",
			Details: `Destructive and not undoable: it removes the project node AND deletes the
companion note file from the vault. Tasks are not cascaded — they keep their
projectId and render as unassigned.

Without --yes nothing is deleted: the project that would go is printed and
the command exits 0.

Flags:
  --yes   Actually delete
  --json  Emit {"id","deleted":true} (or the preview) on stdout`,
		},
	},
}

var projectSubs = []string{"list", "create", "set", "delete"}

func runProject(v verbContext, args []string) error {
	sub, rest, err := requireSub("project", args, projectSubs)
	if err != nil {
		return err
	}
	asJSON, rest := parseBoolFlag(rest, "--json")

	client, err := v.client()
	if err != nil {
		return err
	}
	switch sub {
	case "list":
		return projectList(v, client, asJSON, rest)
	case "create":
		return projectCreate(v, client, asJSON, rest)
	case "set":
		return projectSet(v, client, asJSON, rest)
	}
	return projectDelete(v, client, asJSON, rest)
}

// projectRow is the subset of the API's project DTO the human-readable output
// needs. Decoding a subset keeps the CLI from breaking when the DTO grows.
type projectRow struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	TaskCount int    `json:"taskCount"`
	DoneCount int    `json:"doneCount"`
}

func projectList(v verbContext, c *apiClient, asJSON bool, args []string) error {
	if err := rejectUnknownFlags("project list", args); err != nil {
		return err
	}
	if len(args) > 0 {
		return usagef("KERNL DISPATCH FAILURE: project list takes no arguments, got %q — run: kernl project list [--json]", args[0])
	}

	raw, err := c.get(context.Background(), "/api/projects")
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(v.stdout(), raw)
	}

	var projects []projectRow
	if err := decodeInto(raw, "GET /api/projects", &projects); err != nil {
		return err
	}
	if len(projects) == 0 {
		fmt.Fprintln(v.stdout(), "No projects yet — create one with: kernl project create <title>")
		return nil
	}
	for _, p := range projects {
		fmt.Fprintf(v.stdout(), "%s  %-12s %d/%d  %s\n", p.ID, p.Status, p.DoneCount, p.TaskCount, p.Title)
	}
	return nil
}

func projectCreate(v verbContext, c *apiClient, asJSON bool, args []string) error {
	fields, rest, err := takeProjectFields(args)
	if err != nil {
		return err
	}
	if err := rejectUnknownFlags("project create", rest); err != nil {
		return err
	}
	title, err := projectCreateTitle(fields, rest)
	if err != nil {
		return err
	}

	body := fields.patchBody()
	body["title"] = title
	raw, err := c.post(context.Background(), "/api/projects", body)
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(v.stdout(), raw)
	}

	var created struct {
		ID string `json:"id"`
	}
	if err := decodeInto(raw, "POST /api/projects", &created); err != nil {
		return err
	}
	fmt.Fprintf(v.stdout(), "Created project %s — %s\n", created.ID, title)
	return nil
}

// createTitle resolves the title from the positional argument or --title,
// refusing both at once: silently preferring one would hide a typo'd flag.
func projectCreateTitle(fields projectFields, rest []string) (string, error) {
	if len(rest) > 1 {
		return "", usagef("KERNL DISPATCH FAILURE: project create takes one title, got %d arguments — quote it: kernl project create \"%s\"", len(rest), strings.Join(rest, " "))
	}
	if len(rest) == 1 && fields.hasTitle {
		return "", usagef("KERNL DISPATCH FAILURE: project create got a title both positionally (%q) and via --title (%q) — pass only one", rest[0], fields.title)
	}
	if len(rest) == 1 {
		return rest[0], nil
	}
	if fields.hasTitle && strings.TrimSpace(fields.title) != "" {
		return fields.title, nil
	}
	return "", usagef("KERNL DISPATCH FAILURE: project create requires a title — run: kernl project create \"My project\"")
}

func projectSet(v verbContext, c *apiClient, asJSON bool, args []string) error {
	fields, rest, err := takeProjectFields(args)
	if err != nil {
		return err
	}
	if err := rejectUnknownFlags("project set", rest); err != nil {
		return err
	}
	id, err := projectSingleID("project set", "kernl project set --status done <id>", rest)
	if err != nil {
		return err
	}

	body := fields.patchBody()
	if len(body) == 0 {
		return usagef("KERNL DISPATCH FAILURE: project set %s changes nothing — pass at least one of --title, --description, --status, --tags", id)
	}
	raw, err := c.patch(context.Background(), "/api/projects/"+url.PathEscape(id), body)
	if err != nil {
		return err
	}
	return emitProjectMutation(v.stdout(), asJSON, raw, id, "updated", fmt.Sprintf("Updated project %s", id))
}

func projectDelete(v verbContext, c *apiClient, asJSON bool, args []string) error {
	confirmed, rest := parseBoolFlag(args, "--yes")
	if err := rejectUnknownFlags("project delete", rest); err != nil {
		return err
	}
	id, err := projectSingleID("project delete", "kernl project delete <id> --yes", rest)
	if err != nil {
		return err
	}
	if !confirmed {
		return previewProjectDelete(v, c, asJSON, id)
	}

	raw, err := c.delete(context.Background(), "/api/projects/"+url.PathEscape(id))
	if err != nil {
		return err
	}
	human := fmt.Sprintf("Deleted project %s and its companion note", id)
	return emitProjectMutation(v.stdout(), asJSON, raw, id, "deleted", human)
}

// previewProjectDelete is the unconfirmed path: it reads, never writes, and
// names the project so the operator can see what --yes would destroy.
func previewProjectDelete(v verbContext, c *apiClient, asJSON bool, id string) error {
	raw, err := c.get(context.Background(), "/api/projects")
	if err != nil {
		return err
	}
	var projects []projectRow
	if err := decodeInto(raw, "GET /api/projects", &projects); err != nil {
		return err
	}
	for _, p := range projects {
		if p.ID != id {
			continue
		}
		if asJSON {
			return emitJSON(v.stdout(), json.RawMessage(fmt.Sprintf(`{"id":%q,"title":%q,"deleted":false,"wouldDelete":true}`, p.ID, p.Title)))
		}
		fmt.Fprintf(v.stdout(), "Would delete project %s — %s, plus its companion note. Re-run with --yes.\n", p.ID, p.Title)
		return nil
	}
	return usagef("KERNL DISPATCH FAILURE: no project with id %q — list them with: kernl project list", id)
}

// projectFields carries the four editable attributes with a "was it given"
// flag each, because a PATCH must tell "leave this alone" from "clear it".
type projectFields struct {
	title, description, status                   string
	tags                                         []string
	hasTitle, hasDescription, hasStatus, hasTags bool
}

func takeProjectFields(args []string) (projectFields, []string, error) {
	var f projectFields
	var err error
	if f.title, f.hasTitle, args, err = takeFlag(args, "--title"); err != nil {
		return f, nil, err
	}
	if f.description, f.hasDescription, args, err = takeFlag(args, "--description"); err != nil {
		return f, nil, err
	}
	if f.status, f.hasStatus, args, err = takeFlag(args, "--status"); err != nil {
		return f, nil, err
	}
	var rawTags string
	if rawTags, f.hasTags, args, err = takeFlag(args, "--tags"); err != nil {
		return f, nil, err
	}
	if f.hasTags {
		f.tags = splitProjectTags(rawTags)
	}
	return f, args, nil
}

// patchBody emits only the fields that were actually passed, matching the
// server's pointer-field semantics.
func (f projectFields) patchBody() map[string]any {
	body := map[string]any{}
	if f.hasTitle {
		body["title"] = f.title
	}
	if f.hasDescription {
		body["description"] = f.description
	}
	if f.hasStatus {
		body["status"] = f.status
	}
	if f.hasTags {
		body["tags"] = f.tags
	}
	return body
}

// splitTags returns a non-nil slice so an empty --tags marshals as [] and
// clears the list rather than encoding as null (which the server ignores).
func splitProjectTags(raw string) []string {
	tags := []string{}
	for _, t := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(t); trimmed != "" {
			tags = append(tags, trimmed)
		}
	}
	return tags
}

func projectSingleID(verb, example string, rest []string) (string, error) {
	if len(rest) == 0 {
		return "", usagef("KERNL DISPATCH FAILURE: %s requires a project id — run: %s", verb, example)
	}
	if len(rest) > 1 {
		return "", usagef("KERNL DISPATCH FAILURE: %s takes one project id, got %d: %s — run: %s", verb, len(rest), strings.Join(rest, " "), example)
	}
	return rest[0], nil
}

// emitMutation reports a route that answers 204 with an empty body. There is
// nothing to pass through, so --json gets a minimal confirmation object
// instead of a blank line — an agent parsing stdout needs valid JSON.
func emitProjectMutation(w io.Writer, asJSON bool, raw json.RawMessage, id, action, human string) error {
	if !asJSON {
		fmt.Fprintln(w, human)
		return nil
	}
	if len(bytes.TrimSpace(raw)) > 0 {
		return emitJSON(w, raw)
	}
	return emitJSON(w, json.RawMessage(fmt.Sprintf(`{"id":%q,%q:true}`, id, action)))
}
