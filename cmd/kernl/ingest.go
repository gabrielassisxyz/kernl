package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

var ingestSubcommands = []string{"paste", "upload", "source", "trigger", "queue"}

var ingestQueueSubcommands = []string{"list", "resolve", "merge-plan"}

// ingestResolveActions maps a shell-friendly token onto the action string the
// API expects. The wire values contain a space ("Create Page"), which no one
// should have to quote correctly to discard a review.
//
// The token for the wire's "Skip" is "discard" because that is what it does: the
// review is deleted, permanently and with no undo. "skip" reads as "leave it for
// later", which is the opposite of the outcome.
var ingestResolveActions = map[string]string{
	"create-page": "Create Page",
	"update":      "Update",
	"discard":     "Skip",
}

var ingestCommand = commandMeta{
	Name:    "ingest",
	Summary: "Feed documents into the graph and triage what they extracted",
	Usage:   "kernl ingest <paste|upload|source|trigger|queue> [args...]",
	Details: `Ingest talks to a running 'kernl serve' and needs an LLM provider on the
server: extraction is an LLM pass. Without one every route answers 503 and
the CLI surfaces that verbatim — Fix: set llm.provider in kernl.yaml, then
restart the server.

Extraction runs in the background: paste, upload, source and trigger return
as soon as the content is staged. What they produced shows up in
'kernl ingest queue list'.

Run 'kernl ingest <subcommand> --help' for details on each.`,
	Subs: []commandMeta{
		{
			Name:    "paste",
			Summary: "Ingest text passed as an argument or piped on stdin",
			Usage:   "kernl ingest paste [--title <text>] [--json] [--] <text> | cat f.md | kernl ingest paste",
			Details: `Flags:
  --title <text>  Becomes a leading H1 so the extractor can anchor on it
  --json          Emit the API response verbatim

Everything after '--' is text, even when it looks like a flag.`,
		},
		{
			Name:    "upload",
			Summary: "Ingest a local .md or .txt file (sent as multipart/form-data)",
			Usage:   "kernl ingest upload <path> [--json]",
			Details: `The file is read locally and uploaded whole; the server rejects anything
that is not .md or .txt, is empty, is not UTF-8, or exceeds 2 MiB.`,
		},
		{
			Name:    "source",
			Summary: "Fetch a URL and ingest what it returns",
			Usage:   "kernl ingest source <url> [--kind <kind>] [--title <text>] [--json]",
			Details: `Flags:
  --kind <kind>   How the fetcher should read the URL (server decides by default)
  --title <text>  Override the title the fetcher derived
  --json          Emit {"sourceNodeId","title","kind"} verbatim

A bookmark node is created for the source, so the extracted notes stay
traceable to where they came from.`,
		},
		{
			Name:    "trigger",
			Summary: "Ingest a file already inside the vault, by server-side path",
			Usage:   "kernl ingest trigger <file-path> [--node <node-id>] [--json]",
			Details: `The path is resolved by the server, not by this shell: it must be readable
by the 'kernl serve' process. To send a local file's contents instead, use
'kernl ingest upload'.

Flags:
  --node <id>  Attach the extraction to an existing source node`,
		},
		{
			Name:    "queue",
			Summary: "Read and resolve the ingest review queue",
			Usage:   "kernl ingest queue <list|resolve|merge-plan> [args...]",
			Details: `list                  Pending reviews: id, proposed action, title
resolve <id>          Apply a review (--action create-page|update|discard)
merge-plan <id>       Ask the LLM which hunks an Update would add

Flags on resolve:
  --action <a>       create-page, update or discard — REQUIRED, no default.
                     'discard' deletes the review permanently; there is no undo.
  --target-note <id> With --action update: the note to merge into
  --json             Emit the API response verbatim

Hunk-by-hunk merge review is a GUI flow. 'resolve --action update' from the
CLI links the source to the target note and clears the review without
rewriting the note's body.`,
		},
	},
}

func runIngest(v verbContext, args []string) error {
	sub, rest, err := requireSub("ingest", args, ingestSubcommands)
	if err != nil {
		return err
	}
	// paste parses its own flags: its payload is free text that may legitimately
	// look like a flag, exactly as `kernl capture` handles it.
	if sub == "paste" {
		return ingestPaste(v, rest)
	}

	asJSON, rest := parseBoolFlag(rest, "--json")
	client, err := v.client()
	if err != nil {
		return err
	}
	ctx := context.Background()

	switch sub {
	case "upload":
		return ingestUpload(ctx, v, client, asJSON, rest)
	case "source":
		return ingestSource(ctx, v, client, asJSON, rest)
	case "trigger":
		return ingestTrigger(ctx, v, client, asJSON, rest)
	}
	return ingestQueue(ctx, v, client, asJSON, rest)
}

func ingestPaste(v verbContext, args []string) error {
	head, tail := splitAtSentinel(args)
	asJSON, head := parseBoolFlag(head, "--json")
	title, _, head, err := takeFlag("ingest paste", head, "--title")
	if err != nil {
		return err
	}
	if err := rejectUnknownFlags("ingest paste", head); err != nil {
		return err
	}
	if len(tail) > 0 {
		head = append(head, tail[1:]...)
	}

	text, err := ingestPasteText(strings.Join(head, " "))
	if err != nil {
		return err
	}
	client, err := v.client()
	if err != nil {
		return err
	}
	body := map[string]string{"text": text}
	if strings.TrimSpace(title) != "" {
		body["title"] = title
	}
	raw, err := client.post(context.Background(), "/api/ingest/paste", body)
	if err != nil {
		return err
	}
	return reportIngestAccepted(v, asJSON, raw, "Pasted content staged")
}

func ingestPasteText(fromArgs string) (string, error) {
	text := strings.TrimSpace(fromArgs)
	if text != "" {
		return text, nil
	}
	if stdinIsTerminal() {
		return "", usagef("KERNL DISPATCH FAILURE: ingest paste got no text and stdin is a terminal — pass it as an argument (kernl ingest paste \"<text>\") or pipe it in. Run: kernl ingest paste --help")
	}
	read, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", wrapLoud("reading pasted text from stdin", err)
	}
	text = strings.TrimSpace(string(read))
	if text == "" {
		return "", usagef("KERNL DISPATCH FAILURE: ingest paste got no text — pass it as an argument or pipe it on stdin. Run: kernl ingest paste --help")
	}
	return text, nil
}

func ingestUpload(ctx context.Context, v verbContext, c *apiClient, asJSON bool, args []string) error {
	path, err := singleIngestArg("upload", "a local file path", args)
	if err != nil {
		return err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return wrapLoud(fmt.Sprintf("reading %s for upload", path), err)
	}
	form, contentType, err := ingestUploadForm(filepath.Base(path), content)
	if err != nil {
		return err
	}
	raw, err := c.postMultipart(ctx, "/api/ingest/upload", contentType, form)
	if err != nil {
		return err
	}
	return reportIngestAccepted(v, asJSON, raw, "Uploaded "+filepath.Base(path))
}

// ingestUploadForm builds the multipart body the upload handler parses. The
// field name is "file" and the filename matters: the server decides whether the
// content is supported by its extension.
func ingestUploadForm(filename string, content []byte) ([]byte, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		return nil, "", wrapLoud("building the upload form", err)
	}
	if _, err := part.Write(content); err != nil {
		return nil, "", wrapLoud("writing the upload form", err)
	}
	if err := w.Close(); err != nil {
		return nil, "", wrapLoud("closing the upload form", err)
	}
	return buf.Bytes(), w.FormDataContentType(), nil
}

func ingestSource(ctx context.Context, v verbContext, c *apiClient, asJSON bool, args []string) error {
	flags, rest, err := takeFlags("ingest source", args, "--kind", "--title")
	if err != nil {
		return err
	}
	rawURL, err := singleIngestArg("source", "a URL", rest)
	if err != nil {
		return err
	}
	body := map[string]string{"url": rawURL}
	for flag, field := range map[string]string{"--kind": "kind", "--title": "title"} {
		if value := strings.TrimSpace(flags[flag]); value != "" {
			body[field] = value
		}
	}
	raw, err := c.post(ctx, "/api/ingest/source", body)
	if err != nil {
		return err
	}
	if asJSON {
		return emitIngestAck(v, raw)
	}
	var created struct {
		SourceNodeID string `json:"sourceNodeId"`
		Title        string `json:"title"`
		Kind         string `json:"kind"`
	}
	if err := decodeInto(raw, "POST /api/ingest/source", &created); err != nil {
		return err
	}
	_, err = fmt.Fprintf(v.stdout(), "Fetched %q (%s) as %s. Extraction runs in the background: kernl ingest queue list\n",
		created.Title, created.Kind, created.SourceNodeID)
	return err
}

func ingestTrigger(ctx context.Context, v verbContext, c *apiClient, asJSON bool, args []string) error {
	node, _, rest, err := takeFlag("ingest trigger", args, "--node")
	if err != nil {
		return err
	}
	path, err := singleIngestArg("trigger", "a file path the server can read", rest)
	if err != nil {
		return err
	}
	// snake_case, unlike the rest of the API: this handler predates the
	// camelCase contract and still reads file_path / node_id.
	body := map[string]string{"file_path": path}
	if strings.TrimSpace(node) != "" {
		body["node_id"] = node
	}
	raw, err := c.post(ctx, "/api/ingest/trigger", body)
	if err != nil {
		return err
	}
	return reportIngestAccepted(v, asJSON, raw, "Triggered ingest of "+path)
}

func ingestQueue(ctx context.Context, v verbContext, c *apiClient, asJSON bool, args []string) error {
	sub, rest, err := requireSub("ingest queue", args, ingestQueueSubcommands)
	if err != nil {
		return err
	}
	switch sub {
	case "list":
		return ingestQueueList(ctx, v, c, asJSON, rest)
	case "resolve":
		return ingestQueueResolve(ctx, v, c, asJSON, rest)
	}
	return ingestQueueMergePlan(ctx, v, c, asJSON, rest)
}

func ingestQueueList(ctx context.Context, v verbContext, c *apiClient, asJSON bool, args []string) error {
	if err := rejectUnknownFlags("ingest queue list", args); err != nil {
		return err
	}
	if len(args) > 0 {
		return usagef("KERNL DISPATCH FAILURE: ingest queue list takes no arguments, got %q — run: kernl ingest queue list --help", args[0])
	}
	raw, err := c.get(ctx, "/api/ingest/queue")
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(v.stdout(), raw)
	}
	return printIngestQueue(v, raw)
}

// printIngestQueue decodes the subset of the review DTO the listing prints.
// The route used to serialize nodes.IngestReview with no json tags, which put
// Go field names on the wire; the struct carries camelCase tags now, and
// TestIngestQueueJSONContract pins that. encoding/json matches case-insensitively,
// so the old spelling kept working here and hid the drift — these tags name the
// contract that is actually in force.
func printIngestQueue(v verbContext, raw json.RawMessage) error {
	var reviews []struct {
		ID     string `json:"id"`
		Title  string `json:"title"`
		Action string `json:"action"`
	}
	if err := decodeInto(raw, "GET /api/ingest/queue", &reviews); err != nil {
		return err
	}
	if len(reviews) == 0 {
		_, err := fmt.Fprintln(v.stdout(), "No pending ingest reviews.")
		return err
	}
	for _, r := range reviews {
		if _, err := fmt.Fprintf(v.stdout(), "%s  %-12s %s\n", r.ID, r.Action, r.Title); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(v.stdout(), "\n%d review(s)\n", len(reviews))
	return err
}

func ingestQueueResolve(ctx context.Context, v verbContext, c *apiClient, asJSON bool, args []string) error {
	flags, rest, err := takeFlags("ingest queue resolve", args, "--action", "--target-note")
	if err != nil {
		return err
	}
	id, err := singleIngestArg("queue resolve", "a review id", rest)
	if err != nil {
		return err
	}
	action, err := ingestResolveAction(flags["--action"])
	if err != nil {
		return err
	}
	body := map[string]any{"action": action}
	if target := strings.TrimSpace(flags["--target-note"]); target != "" {
		body["targetNoteId"] = target
	}
	raw, err := c.post(ctx, "/api/ingest/queue/"+url.PathEscape(id)+"/resolve", body)
	if err != nil {
		return err
	}
	if asJSON {
		return emitIngestAck(v, raw)
	}
	_, err = fmt.Fprintf(v.stdout(), "Resolved review %s as %s.\n", id, action)
	return err
}

// ingestResolveAction requires --action. It used to default to Skip, mirroring
// the handler, which meant a forgotten flag silently deleted the review. The API
// now rejects an absent action too, so both ends agree that resolving is always
// an explicit choice.
func ingestResolveAction(raw string) (string, error) {
	valid := []string{"create-page", "update", "discard"}
	token := strings.TrimSpace(raw)
	if token == "" {
		return "", usagef("KERNL DISPATCH FAILURE: ingest queue resolve requires --action — valid: %s (discard deletes the review permanently). Run: kernl ingest queue --help",
			strings.Join(valid, ", "))
	}
	if action, ok := ingestResolveActions[token]; ok {
		return action, nil
	}
	return "", usagef("KERNL DISPATCH FAILURE: unknown --action %q%s — valid: %s. Run: kernl ingest queue --help",
		token, didYouMean(token, valid), strings.Join(valid, ", "))
}

func ingestQueueMergePlan(ctx context.Context, v verbContext, c *apiClient, asJSON bool, args []string) error {
	id, err := singleIngestArg("queue merge-plan", "a review id", args)
	if err != nil {
		return err
	}
	route := "/api/ingest/queue/" + url.PathEscape(id) + "/merge-plan"
	raw, err := c.post(ctx, route, nil)
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(v.stdout(), raw)
	}
	var plan struct {
		TargetNoteID string `json:"targetNoteId"`
		TargetTitle  string `json:"targetTitle"`
		Hunks        []struct {
			Text string `json:"text"`
		} `json:"hunks"`
	}
	if err := decodeInto(raw, "POST "+route, &plan); err != nil {
		return err
	}
	if plan.TargetNoteID == "" {
		_, err := fmt.Fprintln(v.stdout(), "No merge target: resolve this review with --action create-page.")
		return err
	}
	_, err = fmt.Fprintf(v.stdout(), "%s (%s): %d hunk(s) would be added\n",
		plan.TargetTitle, plan.TargetNoteID, len(plan.Hunks))
	return err
}

// singleIngestArg pulls the one positional an ingest verb needs, rejecting both
// a missing one and extra positionals that would otherwise be ignored silently.
func singleIngestArg(sub, what string, args []string) (string, error) {
	if err := rejectUnknownFlags("ingest "+sub, args); err != nil {
		return "", err
	}
	if len(args) == 0 {
		return "", usagef("KERNL DISPATCH FAILURE: ingest %s requires %s — run: kernl ingest %s --help", sub, what, sub)
	}
	if len(args) > 1 {
		return "", usagef("KERNL DISPATCH FAILURE: ingest %s takes exactly one argument, got %d (%s) — run: kernl ingest %s --help",
			sub, len(args), strings.Join(args, ", "), sub)
	}
	return args[0], nil
}

// reportIngestAccepted covers the routes that answer 202 with no body: there is
// nothing to print but the fact that the work was staged, and --json must still
// hand a script a parseable document.
func reportIngestAccepted(v verbContext, asJSON bool, raw json.RawMessage, done string) error {
	if asJSON {
		return emitIngestAck(v, raw)
	}
	_, err := fmt.Fprintf(v.stdout(), "%s. Extraction runs in the background: kernl ingest queue list\n", done)
	return err
}

func emitIngestAck(v verbContext, raw json.RawMessage) error {
	if len(bytes.TrimSpace(raw)) > 0 {
		return emitJSON(v.stdout(), raw)
	}
	ack, err := json.Marshal(map[string]string{"status": "accepted"})
	if err != nil {
		return wrapLoud("encoding the ingest acknowledgement", err)
	}
	return emitJSON(v.stdout(), ack)
}

// postMultipart sends a pre-encoded multipart body, reusing the client's
// transport and its status/exit-code mapping — apiClient.post would JSON-encode
// the form and send it as a quoted string.
func (c *apiClient) postMultipart(ctx context.Context, path, contentType string, form []byte) (json.RawMessage, error) {
	base, err := c.base()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path, bytes.NewReader(form))
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
