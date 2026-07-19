package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

var inboxBatchSubcommands = []string{"analyze", "preview", "apply"}

var inboxBatchCommand = commandMeta{
	Name:    "batch",
	Summary: "Split one pasted export into many captures (analyze, preview, apply)",
	Usage:   "kernl inbox batch <analyze|preview|apply> [--file <path>] [--split <mode>] [--source <s>] [--title <text>] [--json]",
	Details: `Reads the text from --file, or from stdin when the flag is omitted.

  analyze  Deterministic split plus LLM enrichment (a title, merges to consider)
  preview  Deterministic split only, no LLM in the path
  apply    Create the captures; requires --yes (without it you get a preview)

Flags:
  --file <path>   Local file to read instead of stdin
  --split <mode>  auto (default), whatsapp, lines, blocks, divider, markdown, semantic
  --source <s>    whatsapp or text; the server infers it when omitted
  --title <text>  Context title for the batch
  --yes           On apply only: actually create the captures
  --json          Emit the API response verbatim

The pasted text is sent exactly as read. Capture bodies are rebuilt
server-side from that text, so neither this CLI nor a model behind it can
rewrite what you captured; merge decisions are a GUI review flow.`,
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
			return emitJSON(v.stdout(), json.RawMessage(env))
		}
		return emitJSON(v.stdout(), raw)
	}
	if sub == "apply" && confirmed {
		return printInboxBatchResult(v.stdout(), raw, route)
	}
	return printInboxBatchSplit(v.stdout(), raw, route, sub == "apply")
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
