package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// Derived from inboxBatchCommand.Subs — see the note on ingestQueueSubcommands.
var inboxBatchSubcommands = subNames(&inboxBatchCommand)

var inboxBatchCommand = commandMeta{
	Name:    "batch",
	Summary: "Split one pasted export into many captures (analyze, preview, apply)",
	Usage:   "kernl inbox batch <analyze|preview|apply> [--file <path>] [--split <mode>] [--source <s>] [--title <text>] [--json]",
	Details: `Reads the text from --file, or from stdin when the flag is omitted.

{{flags}}

The pasted text is sent exactly as read. Capture bodies are rebuilt
server-side from that text, so neither this CLI nor a model behind it can
rewrite what you captured; merge decisions are a GUI review flow.

Run 'kernl inbox batch <subcommand> --help' for details on each.`,
	FlagsHeading: "Flags (all three subcommands):",
	Flags: []commandFlag{
		{Name: "--file", Value: "<path>", Description: "Local file to read instead of stdin"},
		{Name: "--split", Value: "<mode>", Description: "auto (default), whatsapp, lines, blocks, divider, markdown, semantic"},
		{Name: "--source", Value: "<s>", Description: "whatsapp or text; the server infers it when omitted"},
		{Name: "--title", Value: "<text>", Description: "Context title for the batch"},
		{Name: "--json", Description: "Emit the API response verbatim"},
	},
	Subs: []commandMeta{
		{
			Name:    "analyze",
			Summary: "Split the text and enrich it with an LLM (a title, merges to consider)",
			Usage:   "kernl inbox batch analyze [--file <path>] [--split <mode>] [--source <s>] [--title <text>] [--json]",
			Details: `Needs an LLM provider on the server; 'preview' is the same split without one.
Writes nothing — it only reports how the text would divide.`,
		},
		{
			Name:    "preview",
			Summary: "Split the text deterministically, with no LLM in the path",
			Usage:   "kernl inbox batch preview [--file <path>] [--split <mode>] [--source <s>] [--title <text>] [--json]",
			Details: `Writes nothing. Use it to check a --split mode before applying it.`,
		},
		{
			Name:    "apply",
			Summary: "Create the captures (requires --yes)",
			Usage:   "kernl inbox batch apply [--file <path>] [--split <mode>] [--source <s>] [--title <text>] --yes [--json]",
			Details: `Without --yes nothing is created: the request is answered by the preview
route and the output says so, including under --json, which wraps the preview
in {"applied":false,"wouldApply":true,...}.

{{flags}}`,
			Flags: []commandFlag{
				{Name: "--yes", Description: "Actually create the captures"},
			},
		},
	},
}

func inboxBatch(ctx context.Context, v verbContext, c *apiClient, asJSON bool, args []string) error {
	sub, rest, err := requireSub("inbox batch", args, inboxBatchSubcommands)
	if err != nil {
		return err
	}
	confirmed, rest := parseBoolFlag(rest, "--yes")
	body, err := inboxBatchBody(sub, rest)
	if err != nil {
		return err
	}
	// An unconfirmed apply is answered with the preview route: it is the same
	// question ("what would this create?") and it writes nothing.
	route := "/api/inbox/batch"
	if sub != "apply" {
		route += "/" + sub
	} else if !confirmed {
		route += "/preview"
	}

	raw, err := c.post(ctx, route, body)
	if err != nil {
		return err
	}
	unconfirmedApply := sub == "apply" && !confirmed
	if asJSON {
		if unconfirmedApply {
			// `apply` without --yes is answered by the preview route and creates
			// nothing. Human output says so; the JSON must too, or a --json caller
			// reads a dry-run as a completed apply. Wrap the preview in a envelope
			// that states, unmistakably, that nothing was created.
			env := fmt.Sprintf(`{"applied":false,"wouldApply":true,"preview":%s}`, raw)
			if err := emitJSON(v.stdout(), json.RawMessage(env)); err != nil {
				return err
			}
			return refusedWithoutYes("inbox batch apply")
		}
		return emitJSON(v.stdout(), raw)
	}
	if sub == "apply" && confirmed {
		return printInboxBatchResult(v.stdout(), raw, route)
	}
	if err := printInboxBatchSplit(v.stdout(), raw, route, unconfirmedApply); err != nil {
		return err
	}
	// analyze and preview are read-only and end here at exit 0; only apply
	// reached this line by refusing to act.
	if unconfirmedApply {
		return refusedWithoutYes("inbox batch apply")
	}
	return nil
}

// inboxBatchBody carries only what the API accepts: the text as read and the
// split parameters. It never sends rawSegments or finalSegments — those are a
// reviewed split, and nothing here has reviewed anything.
func inboxBatchBody(sub string, args []string) (map[string]string, error) {
	flags, rest, err := takeFlags("inbox batch", args, "--file", "--split", "--source", "--title")
	if err != nil {
		return nil, err
	}
	if err := rejectUnknownFlags("inbox batch "+sub, rest); err != nil {
		return nil, err
	}
	if len(rest) > 0 {
		return nil, usagef("KERNL DISPATCH FAILURE: inbox batch %s takes no positional arguments, got %q — pass the text with --file or on stdin. Run: kernl inbox batch --help", sub, rest[0])
	}
	text, err := readInboxBatchText(sub, flags["--file"])
	if err != nil {
		return nil, err
	}
	body := map[string]string{"text": text}
	for flag, field := range map[string]string{
		"--split":  "splitMode",
		"--source": "source",
		"--title":  "contextTitle",
	} {
		if value := strings.TrimSpace(flags[flag]); value != "" {
			body[field] = value
		}
	}
	return body, nil
}

func readInboxBatchText(sub, path string) (string, error) {
	if strings.TrimSpace(path) != "" {
		content, err := os.ReadFile(path)
		if err != nil {
			return "", wrapLoud("reading the batch text from "+path, err)
		}
		return string(content), nil
	}
	if stdinIsTerminal() {
		return "", usagef("KERNL DISPATCH FAILURE: inbox batch %s got no text and stdin is a terminal — pass --file <path> or pipe the export in. Run: kernl inbox batch --help", sub)
	}
	read, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", wrapLoud("reading the batch text from stdin", err)
	}
	if strings.TrimSpace(string(read)) == "" {
		return "", usagef("KERNL DISPATCH FAILURE: inbox batch %s got no text — pass --file <path> or pipe the export on stdin. Run: kernl inbox batch --help", sub)
	}
	return string(read), nil
}

func printInboxBatchSplit(w io.Writer, raw json.RawMessage, route string, unconfirmed bool) error {
	var split struct {
		Source                string `json:"source"`
		Separator             string `json:"separator"`
		SuggestedContextTitle string `json:"suggestedContextTitle"`
		Segments              []struct {
			Sequence int    `json:"sequence"`
			Body     string `json:"body"`
		} `json:"segments"`
	}
	if err := decodeInto(raw, "POST "+route, &split); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "%s / %s  %s\n", split.Source, split.Separator, split.SuggestedContextTitle); err != nil {
		return err
	}
	for _, seg := range split.Segments {
		if _, err := fmt.Fprintf(w, "  %d  %s\n", seg.Sequence, firstLine(seg.Body)); err != nil {
			return err
		}
	}
	if unconfirmed {
		_, err := fmt.Fprintf(w, "\n%d capture(s) would be created. Re-run with --yes to confirm.\n", len(split.Segments))
		return err
	}
	_, err := fmt.Fprintf(w, "\n%d segment(s)\n", len(split.Segments))
	return err
}

func printInboxBatchResult(w io.Writer, raw json.RawMessage, route string) error {
	var result struct {
		BatchID string   `json:"batchId"`
		IDs     []string `json:"ids"`
	}
	if err := decodeInto(raw, "POST "+route, &result); err != nil {
		return err
	}
	// A batch has no title, so what it is worth leading with is its size — the
	// one number the caller can check. The id follows in parentheses, per the
	// shape createdLine documents.
	_, err := fmt.Fprintf(w, "Created batch of %d capture(s) (%s). Inspect it with: kernl inbox batch-log %s\n",
		len(result.IDs), result.BatchID, result.BatchID)
	return err
}
