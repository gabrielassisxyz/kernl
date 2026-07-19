package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"
)

var noteSubs = []string{"list", "read", "write", "delete", "tags", "suggest", "apply-hunks"}

var noteCommand = commandMeta{
	Name:    "note",
	Summary: "Read and write vault notes (the same files the editor edits)",
	Usage:   "kernl note <list|read|write|delete|tags|suggest|apply-hunks> [args...]",
	Details: `Every path is vault-relative (e.g. "projects/kernl.md"); the server
resolves it against vault.root and rejects anything that escapes it.

Run 'kernl note <subcommand> --help' for details on each.`,
	Subs: []commandMeta{
		{
			Name:    "list",
			Summary: "List notes with their graph node id, type and title",
			Usage:   "kernl note list [--files] [--json]",
			Details: `Flags:
  --files  List the .md files on disk instead (files the graph has not
           indexed yet show up here and nowhere else)
  --json   Emit the API response verbatim on stdout`,
		},
		{
			Name:    "read",
			Summary: "Print a note's contents",
			Usage:   "kernl note read <path> [--json]",
			Details: `Writes the file bytes to stdout unchanged, so it pipes.

Flags:
  --json  Emit {"path","content"} instead — the route itself returns
          text/plain, so there is no server JSON to pass through`,
		},
		{
			Name:    "write",
			Summary: "Create or overwrite a note from a file or stdin",
			Usage:   "kernl note write <path> [--file <local-path>] [--json]",
			Details: `The body comes from --file, or from stdin when --file is omitted.
Writing a .md note the server does not know yet gets a node id injected
into its frontmatter.

Flags:
  --file <local-path>  Read the body from a local file instead of stdin
  --json               Emit {"status":"saved"} on stdout

Examples:
  kernl note write inbox/idea.md --file ./draft.md
  echo "# Title" | kernl note write inbox/idea.md`,
		},
		{
			Name:    "delete",
			Summary: "Move a note to the trash (requires --yes)",
			Usage:   "kernl note delete <path> --yes [--json]",
			Details: `Destructive: the file goes to the system trash and the vault watcher
reconciles its graph node away. Without --yes this prints what would be
deleted and exits 0.

Flags:
  --yes   Actually delete
  --json  Emit {"path","status"} on stdout`,
		},
		{
			Name:    "tags",
			Summary: "List every note tag with the files carrying it",
			Usage:   "kernl note tags [--json]",
			Details: `Flags:
  --json  Emit {"<tag>":{"files":[...]}} on stdout`,
		},
		{
			Name:    "suggest",
			Summary: "Ask the DA for edit hunks on a note (never writes)",
			Usage:   "kernl note suggest <path> --instruction <text> [--json]",
			Details: `Returns line-aligned hunks to review; nothing is written until they are
fed back through 'kernl note apply-hunks'. Needs an llm section in
kernl.yaml — without one the server answers 503.

Flags:
  --instruction <text>  What to change (required)
  --json                Emit {"hunks":[{"id","from","to","content"}]}`,
		},
		{
			Name:    "apply-hunks",
			Summary: "Apply hunks from 'note suggest' to a note",
			Usage:   "kernl note apply-hunks <path> [--hunks <local-path>] [--json]",
			Details: `Reads a JSON array of hunks — either the "hunks" array of a suggest
response, or the whole response object — from --hunks or stdin, and
applies exactly those. Pass only the hunks that were reviewed.

Flags:
  --hunks <local-path>  Read the hunks from a local file instead of stdin
  --json                Emit {"status","last_modified"} on stdout

Example:
  kernl note suggest daily.md --instruction "tighten the intro" --json > h.json
  kernl note apply-hunks daily.md --hunks h.json`,
		},
	},
}

func runNote(v verbContext, args []string) error {
	sub, rest, err := requireSub("note", args, noteSubs)
	if err != nil {
		return err
	}
	asJSON, rest := parseBoolFlag(rest, "--json")
	client, err := v.client()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	switch sub {
	case "list":
		return runNoteList(ctx, client, v.stdout(), asJSON, rest)
	case "read":
		return runNoteRead(ctx, client, v.stdout(), asJSON, rest)
	case "write":
		return runNoteWrite(ctx, client, v.stdout(), asJSON, rest)
	case "delete":
		return runNoteDelete(ctx, client, v.stdout(), asJSON, rest)
	case "tags":
		return runNoteTags(ctx, client, v.stdout(), asJSON, rest)
	case "suggest":
		return runNoteSuggest(ctx, client, v.stdout(), asJSON, rest)
	default:
		return runNoteApplyHunks(ctx, client, v.stdout(), asJSON, rest)
	}
}

// notePathArg consumes the one vault-relative path every file verb needs. It
// runs after each verb has taken its own flags, so anything flag-shaped left
// here is a typo and must fail rather than be read as a path.
func notePathArg(sub string, args []string) (string, error) {
	if err := rejectUnknownFlags("note "+sub, args); err != nil {
		return "", err
	}
	if len(args) == 0 {
		return "", usagef("KERNL DISPATCH FAILURE: note %s requires a vault-relative note path — example: kernl note %s projects/kernl.md", sub, sub)
	}
	if len(args) > 1 {
		return "", usagef("KERNL DISPATCH FAILURE: note %s takes exactly one path, got %d (%s) — quote paths containing spaces", sub, len(args), strings.Join(args, ", "))
	}
	return args[0], nil
}

func noteFileRoute(path string) string {
	return "/api/vault/file?" + url.Values{"path": {path}}.Encode()
}

func rejectNoteArgs(sub string, args []string) error {
	if err := rejectUnknownFlags("note "+sub, args); err != nil {
		return err
	}
	if len(args) > 0 {
		return usagef("KERNL DISPATCH FAILURE: note %s takes no arguments, got %q — run: kernl note %s --help", sub, args[0], sub)
	}
	return nil
}

func runNoteList(ctx context.Context, c *apiClient, out io.Writer, asJSON bool, args []string) error {
	onlyFiles, args := parseBoolFlag(args, "--files")
	if err := rejectNoteArgs("list", args); err != nil {
		return err
	}
	if onlyFiles {
		return runNoteListFiles(ctx, c, out, asJSON)
	}
	raw, err := c.get(ctx, "/api/vault/notes")
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(out, raw)
	}
	// The route answers with a bare JSON array, not an envelope object.
	var notes []struct {
		Path  string `json:"path"`
		ID    string `json:"id"`
		Type  string `json:"type"`
		Title string `json:"title"`
	}
	if err := decodeInto(raw, "GET /api/vault/notes", &notes); err != nil {
		return err
	}
	if len(notes) == 0 {
		_, err := fmt.Fprintln(out, "No notes indexed. Try: kernl note list --files")
		return err
	}
	for _, n := range notes {
		if _, err := fmt.Fprintf(out, "%s\t%s\t%s\n", n.Path, n.Type, n.Title); err != nil {
			return err
		}
	}
	return nil
}

func runNoteListFiles(ctx context.Context, c *apiClient, out io.Writer, asJSON bool) error {
	raw, err := c.get(ctx, "/api/vault/list")
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(out, raw)
	}
	var payload struct {
		Files []string `json:"files"`
	}
	if err := decodeInto(raw, "GET /api/vault/list", &payload); err != nil {
		return err
	}
	for _, f := range payload.Files {
		if _, err := fmt.Fprintln(out, f); err != nil {
			return err
		}
	}
	return nil
}

func runNoteRead(ctx context.Context, c *apiClient, out io.Writer, asJSON bool, args []string) error {
	path, err := notePathArg("read", args)
	if err != nil {
		return err
	}
	raw, err := c.get(ctx, noteFileRoute(path))
	if err != nil {
		return err
	}
	if asJSON {
		// This route returns text/plain, so there is no server JSON to pass
		// through — the CLI builds the envelope rather than emitting a
		// bare blob that no --json consumer could parse.
		return json.NewEncoder(out).Encode(map[string]string{"path": path, "content": string(raw)})
	}
	// Byte-exact, no added newline: `kernl note read x.md > x.md` must round-trip.
	_, err = out.Write(raw)
	return err
}

func runNoteWrite(ctx context.Context, c *apiClient, out io.Writer, asJSON bool, args []string) error {
	source, hasSource, args, err := takeFlag("note write", args, "--file")
	if err != nil {
		return err
	}
	path, err := notePathArg("write", args)
	if err != nil {
		return err
	}
	body, err := readNoteBody("write", "--file", source, hasSource)
	if err != nil {
		return err
	}
	raw, err := c.postRaw(ctx, noteFileRoute(path), "text/markdown", body)
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(out, raw)
	}
	_, err = fmt.Fprintf(out, "Wrote %s (%d bytes).\n", path, len(body))
	return err
}

// readNoteBody takes the payload from a local file, or from stdin when the
// flag was omitted — the same "arg or stdin" shape `kernl capture` uses.
func readNoteBody(sub, flag, source string, hasSource bool) ([]byte, error) {
	if hasSource {
		if strings.TrimSpace(source) == "" {
			return nil, usagef("KERNL DISPATCH FAILURE: note %s %s requires a readable local file path — example: kernl note %s <path> %s ./body.md", sub, flag, sub, flag)
		}
		body, err := os.ReadFile(source)
		if err != nil {
			return nil, wrapLoud(fmt.Sprintf("reading %s from %s", flag, source), err)
		}
		return body, nil
	}
	if stdinIsTerminal() {
		return nil, usagef("KERNL DISPATCH FAILURE: note %s reads its body from stdin, but stdin is a terminal — pipe content in (echo ... | kernl note %s <path>) or pass %s <local-path>", sub, sub, flag)
	}
	body, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, wrapLoud("reading the note body from stdin", err)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, usagef("KERNL DISPATCH FAILURE: note %s got an empty body — pipe content on stdin or pass %s <local-path>", sub, flag)
	}
	return body, nil
}

func runNoteDelete(ctx context.Context, c *apiClient, out io.Writer, asJSON bool, args []string) error {
	confirmed, args := parseBoolFlag(args, "--yes")
	path, err := notePathArg("delete", args)
	if err != nil {
		return err
	}
	if !confirmed {
		if asJSON {
			return json.NewEncoder(out).Encode(map[string]string{"path": path, "status": "dry-run"})
		}
		_, err := fmt.Fprintf(out, "Would delete %s. Re-run with --yes to delete it.\n", path)
		return err
	}
	raw, err := c.delete(ctx, noteFileRoute(path))
	if err != nil {
		return err
	}
	if asJSON {
		// A 204 carries no body; emit the CLI's own confirmation instead.
		if len(bytes.TrimSpace(raw)) == 0 {
			return json.NewEncoder(out).Encode(map[string]string{"path": path, "status": "deleted"})
		}
		return emitJSON(out, raw)
	}
	_, err = fmt.Fprintf(out, "Deleted %s (moved to trash).\n", path)
	return err
}

func runNoteTags(ctx context.Context, c *apiClient, out io.Writer, asJSON bool, args []string) error {
	if err := rejectNoteArgs("tags", args); err != nil {
		return err
	}
	raw, err := c.get(ctx, "/api/notes/tags")
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(out, raw)
	}
	var tree map[string]struct {
		Files []string `json:"files"`
	}
	if err := decodeInto(raw, "GET /api/notes/tags", &tree); err != nil {
		return err
	}
	for _, tag := range sortedKeys(tree) {
		if _, err := fmt.Fprintf(out, "%s\t%d\n", tag, len(tree[tag].Files)); err != nil {
			return err
		}
	}
	return nil
}

func runNoteSuggest(ctx context.Context, c *apiClient, out io.Writer, asJSON bool, args []string) error {
	instruction, hasInstruction, args, err := takeFlag("note suggest", args, "--instruction")
	if err != nil {
		return err
	}
	path, err := notePathArg("suggest", args)
	if err != nil {
		return err
	}
	if !hasInstruction || strings.TrimSpace(instruction) == "" {
		return usagef("KERNL DISPATCH FAILURE: note suggest requires --instruction <text> — example: kernl note suggest %s --instruction \"tighten the intro\"", path)
	}
	raw, err := c.post(ctx, "/api/notes/suggest", map[string]string{"path": path, "instruction": instruction})
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(out, raw)
	}
	var payload struct {
		Hunks []struct {
			ID      string `json:"id"`
			From    int    `json:"from"`
			To      int    `json:"to"`
			Content string `json:"content"`
		} `json:"hunks"`
	}
	if err := decodeInto(raw, "POST /api/notes/suggest", &payload); err != nil {
		return err
	}
	if len(payload.Hunks) == 0 {
		_, err := fmt.Fprintln(out, "No changes suggested.")
		return err
	}
	for _, h := range payload.Hunks {
		if _, err := fmt.Fprintf(out, "%s [%d:%d]\n%s\n", h.ID, h.From, h.To, h.Content); err != nil {
			return err
		}
	}
	return nil
}

func runNoteApplyHunks(ctx context.Context, c *apiClient, out io.Writer, asJSON bool, args []string) error {
	source, hasSource, args, err := takeFlag("note apply-hunks", args, "--hunks")
	if err != nil {
		return err
	}
	path, err := notePathArg("apply-hunks", args)
	if err != nil {
		return err
	}
	body, err := readNoteBody("apply-hunks", "--hunks", source, hasSource)
	if err != nil {
		return err
	}
	hunks, err := parseHunks(body)
	if err != nil {
		return err
	}
	raw, err := c.post(ctx, "/api/notes/apply-hunks", map[string]any{"path": path, "hunks": hunks})
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(out, raw)
	}
	_, err = fmt.Fprintf(out, "Applied %d hunk(s) to %s.\n", len(hunks), path)
	return err
}

// parseHunks accepts either a bare hunks array or a whole suggest response, so
// `note suggest --json > f` can be fed straight back in without hand-editing.
func parseHunks(body []byte) ([]json.RawMessage, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var hunks []json.RawMessage
		if err := json.Unmarshal(trimmed, &hunks); err != nil {
			return nil, usagef("KERNL DISPATCH FAILURE: the hunks input is not a JSON array of hunks: %v", err)
		}
		return requireHunks(hunks)
	}
	var envelope struct {
		Hunks []json.RawMessage `json:"hunks"`
	}
	if err := json.Unmarshal(trimmed, &envelope); err != nil {
		return nil, usagef("KERNL DISPATCH FAILURE: the hunks input is neither a JSON array nor a {\"hunks\":[...]} object: %v", err)
	}
	return requireHunks(envelope.Hunks)
}

func requireHunks(hunks []json.RawMessage) ([]json.RawMessage, error) {
	if len(hunks) == 0 {
		return nil, usagef("KERNL DISPATCH FAILURE: no hunks to apply — pass the hunks you accepted from 'kernl note suggest <path> --instruction <text> --json'")
	}
	return hunks, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// postRaw sends a body the server wants verbatim (a note is markdown, not
// JSON), reusing the client's transport and its status/exit-code mapping —
// apiClient.post would JSON-encode the markdown and write a quoted string.
func (c *apiClient) postRaw(ctx context.Context, path, contentType string, body []byte) (json.RawMessage, error) {
	base, err := c.base()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path, bytes.NewReader(body))
	if err != nil {
		return nil, wrapLoud("building request", err)
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, c.unreachable(err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, wrapLoud("reading response", err)
	}
	if resp.StatusCode >= 400 {
		return nil, httpStatusError(http.MethodPost, path, resp.StatusCode, raw)
	}
	return raw, nil
}
