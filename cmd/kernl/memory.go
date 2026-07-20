package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
)

var memorySubcommands = []string{"topics", "claims", "telos", "add-claim", "refute"}

var memoryCommand = commandMeta{
	Name:    "memory",
	Summary: "Inspect and edit what the DA remembers (claims and telos)",
	Usage:   "kernl memory <topics|claims|telos|add-claim|refute> [args...]",
	Details: `Talks to a running server over the REST API, so 'kernl serve' must be up
(or point elsewhere with --server <url> / KERNL_SERVER).

Memory has two halves: claims — statements the DA learned or you asserted,
retrieved by relevance — and telos, the notes tagged 'telos' that are always
injected into the DA's context.

Run 'kernl memory <subcommand> --help' for details on each.`,
	Subs: []commandMeta{
		{
			Name:    "topics",
			Summary: "List the subjects that still have at least one active claim",
			Usage:   "kernl memory topics [--json]",
			Details: `A subject whose every claim was refuted is not listed.

{{flags}}`,
			Flags: []commandFlag{
				{Name: "--json", Description: `Emit {"topics":[...]} verbatim on stdout`},
			},
		},
		{
			Name:    "claims",
			Summary: "List the active claims of one topic",
			Usage:   "kernl memory claims --topic <topic> [--json]",
			Details: `Refuted claims are filtered out by the server. --topic is required:
the route has no "all topics" mode.

{{flags}}`,
			Flags: []commandFlag{
				{Name: "--topic", Value: "<topic>", Description: "Which subject to read (list them with: kernl memory topics)"},
				{Name: "--json", Description: `Emit {"claims":[...]} verbatim on stdout`},
			},
		},
		{
			Name:    "telos",
			Summary: "Show the always-injected telos notes and their context footprint",
			Usage:   "kernl memory telos [--json]",
			Details: `Prints every note tagged 'telos' plus the size of the block the chat
engine actually injects, so you can see when it is being truncated.

{{flags}}`,
			Flags: []commandFlag{
				{Name: "--json", Description: `Emit {"notes":[...],"injection":{...}} verbatim on stdout`},
			},
		},
		{
			Name:    "add-claim",
			Summary: "Assert a claim yourself, without going through the DA",
			Usage:   "kernl memory add-claim --subject <topic> <statement> [--json]",
			Details: `The claim is stored with user provenance, so it is distinguishable
from one the DA learned.

{{flags}}

Example:
  kernl memory add-claim --subject deploys "releases are cut from tags, never from master"`,
			Flags: []commandFlag{
				{Name: "--subject", Value: "<topic>", Description: "The subject the claim belongs to (required)"},
				{Name: "--json", Description: `Emit {"id"} on stdout`},
			},
		},
		{
			Name:    "refute",
			Summary: "Refute a claim so it stops being retrieved",
			Usage:   "kernl memory refute <claim-id> [--reason <text>] [--json]",
			Details: `The claim is not deleted: a refutation node is recorded against it and
the read paths stop returning it.

{{flags}}`,
			Flags: []commandFlag{
				{Name: "--reason", Value: "<text>", Description: "Why it is wrong (kept with the refutation)"},
				{Name: "--json", Description: `Emit {"status","id"} verbatim on stdout`},
			},
		},
	},
}

// memoryClaimView is the subset of the claim DTO the readable listing prints.
// The route once serialized Go field names (Statement, Subject, …) instead of
// the camelCase the rest of the API emits. nodes.MemoryClaim carries json tags
// now and TestMemoryClaimsJSONContract pins them, so these tags name the live
// contract rather than tolerating two.
type memoryClaimView struct {
	ID         string  `json:"id"`
	Statement  string  `json:"statement"`
	Subject    string  `json:"subject"`
	Source     string  `json:"source"`
	Confidence float64 `json:"confidence"`
}

type memoryTelosNoteView struct {
	Title string `json:"title"`
	Path  string `json:"path"`
}

func runMemory(v verbContext, args []string) error {
	sub, rest, err := requireSub("memory", args, memorySubcommands)
	if err != nil {
		return err
	}
	asJSON, rest := parseBoolFlag(rest, "--json")
	switch sub {
	case "topics":
		return runMemoryTopics(v, asJSON, rest)
	case "claims":
		return runMemoryClaims(v, asJSON, rest)
	case "telos":
		return runMemoryTelos(v, asJSON, rest)
	case "add-claim":
		return runMemoryAddClaim(v, asJSON, rest)
	default:
		return runMemoryRefute(v, asJSON, rest)
	}
}

func runMemoryTopics(v verbContext, asJSON bool, args []string) error {
	if err := noMemoryArgs("memory topics", args); err != nil {
		return err
	}
	raw, err := requestMemory(v, func(ctx context.Context, c *apiClient) (json.RawMessage, error) {
		return c.get(ctx, "/api/memory/topics")
	})
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(v.stdout(), raw)
	}
	var payload struct {
		Topics []string `json:"topics"`
	}
	if err := decodeInto(raw, "GET /api/memory/topics", &payload); err != nil {
		return err
	}
	if len(payload.Topics) == 0 {
		fmt.Fprintln(v.stdout(), `No topics with active claims. Add one with: kernl memory add-claim --subject "<topic>" "<statement>"`)
		return nil
	}
	for _, t := range payload.Topics {
		fmt.Fprintln(v.stdout(), t)
	}
	fmt.Fprintf(v.stdout(), "\n%d topic(s)\n", len(payload.Topics))
	return nil
}

func runMemoryClaims(v verbContext, asJSON bool, args []string) error {
	topic, present, rest, err := takeFlag("memory claims", args, "--topic")
	if err != nil {
		return err
	}
	if err := noMemoryArgs("memory claims", rest); err != nil {
		return err
	}
	if !present || strings.TrimSpace(topic) == "" {
		return usagef("KERNL DISPATCH FAILURE: memory claims requires --topic — the route has no all-topics mode. List them with: kernl memory topics")
	}

	raw, err := requestMemory(v, func(ctx context.Context, c *apiClient) (json.RawMessage, error) {
		return c.get(ctx, "/api/memory/claims?topic="+url.QueryEscape(topic))
	})
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(v.stdout(), raw)
	}
	return printMemoryClaims(v.stdout(), raw, topic)
}

func runMemoryTelos(v verbContext, asJSON bool, args []string) error {
	if err := noMemoryArgs("memory telos", args); err != nil {
		return err
	}
	raw, err := requestMemory(v, func(ctx context.Context, c *apiClient) (json.RawMessage, error) {
		return c.get(ctx, "/api/memory/telos")
	})
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(v.stdout(), raw)
	}
	return printMemoryTelos(v.stdout(), raw)
}

func runMemoryAddClaim(v verbContext, asJSON bool, args []string) error {
	subject, _, rest, err := takeFlag("memory add-claim", args, "--subject")
	if err != nil {
		return err
	}
	if err := rejectUnknownFlags("memory add-claim", rest); err != nil {
		return err
	}
	if strings.TrimSpace(subject) == "" {
		return usagef(`KERNL DISPATCH FAILURE: memory add-claim requires --subject — run: kernl memory add-claim --subject "<topic>" "<statement>"`)
	}
	if len(rest) == 0 {
		return usagef(`KERNL DISPATCH FAILURE: memory add-claim requires a statement — run: kernl memory add-claim --subject "<topic>" "<statement>"`)
	}
	// An unquoted multi-word statement is the common shell slip, and joining
	// it is unambiguously what the caller meant.
	body := map[string]any{"subject": subject, "statement": strings.Join(rest, " ")}

	raw, err := requestMemory(v, func(ctx context.Context, c *apiClient) (json.RawMessage, error) {
		return c.post(ctx, "/api/memory/claims", body)
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
	if err := decodeInto(raw, "POST /api/memory/claims", &created); err != nil {
		return err
	}
	statement, _ := body["statement"].(string)
	fmt.Fprintln(v.stdout(), createdLine("Added claim", statement, fmt.Sprintf("under %q", subject), created.ID))
	return nil
}

func runMemoryRefute(v verbContext, asJSON bool, args []string) error {
	reason, _, rest, err := takeFlag("memory refute", args, "--reason")
	if err != nil {
		return err
	}
	if err := rejectUnknownFlags("memory refute", rest); err != nil {
		return err
	}
	id, err := singleMemoryClaimID("memory refute", rest)
	if err != nil {
		return err
	}

	raw, err := requestMemory(v, func(ctx context.Context, c *apiClient) (json.RawMessage, error) {
		return c.post(ctx, "/api/memory/claims/"+url.PathEscape(id)+"/refute", map[string]any{"reason": reason})
	})
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(v.stdout(), raw)
	}
	fmt.Fprintf(v.stdout(), "Refuted claim %s\n", id)
	return nil
}

// requestMemory builds the client only after the invocation has been validated,
// so a malformed command is diagnosed without needing a running server.
func requestMemory(v verbContext, call func(context.Context, *apiClient) (json.RawMessage, error)) (json.RawMessage, error) {
	c, err := v.client()
	if err != nil {
		return nil, err
	}
	return call(context.Background(), c)
}

func noMemoryArgs(verb string, args []string) error {
	if err := rejectUnknownFlags(verb, args); err != nil {
		return err
	}
	if len(args) > 0 {
		return usagef("KERNL DISPATCH FAILURE: %s takes no positional arguments, got %q — run: kernl %s --help", verb, args[0], verb)
	}
	return nil
}

func singleMemoryClaimID(verb string, args []string) (string, error) {
	if len(args) == 0 {
		return "", usagef("KERNL DISPATCH FAILURE: %s requires a claim ID — run: kernl %s <claim-id>. Find one with: kernl memory claims --topic <topic>", verb, verb)
	}
	if len(args) > 1 {
		return "", usagef("KERNL DISPATCH FAILURE: %s takes exactly one claim ID, got %d (%s) — run: kernl %s --help",
			verb, len(args), strings.Join(args, ", "), verb)
	}
	return args[0], nil
}

func printMemoryClaims(w io.Writer, raw json.RawMessage, topic string) error {
	var payload struct {
		Claims []memoryClaimView `json:"claims"`
	}
	if err := decodeInto(raw, "GET /api/memory/claims", &payload); err != nil {
		return err
	}
	if len(payload.Claims) == 0 {
		fmt.Fprintf(w, "No active claims under %q. List the subjects that have some with: kernl memory topics\n", topic)
		return nil
	}
	for _, c := range payload.Claims {
		fmt.Fprintf(w, "%-38s %s%s\n", c.ID, c.Statement, memoryClaimAnnotations(c))
	}
	fmt.Fprintf(w, "\n%d claim(s) under %s\n", len(payload.Claims), topic)
	return nil
}

func memoryClaimAnnotations(c memoryClaimView) string {
	var parts []string
	if c.Source != "" {
		parts = append(parts, "via "+c.Source)
	}
	if c.Confidence > 0 {
		parts = append(parts, fmt.Sprintf("confidence %.2f", c.Confidence))
	}
	if len(parts) == 0 {
		return ""
	}
	return "  (" + strings.Join(parts, ", ") + ")"
}

func printMemoryTelos(w io.Writer, raw json.RawMessage) error {
	var payload struct {
		Notes     []memoryTelosNoteView `json:"notes"`
		Injection struct {
			Bytes     int  `json:"bytes"`
			CapBytes  int  `json:"capBytes"`
			Truncated bool `json:"truncated"`
		} `json:"injection"`
	}
	if err := decodeInto(raw, "GET /api/memory/telos", &payload); err != nil {
		return err
	}
	if len(payload.Notes) == 0 {
		fmt.Fprintln(w, "No telos notes. Tag a vault note with 'telos' to make the DA always see it.")
		return nil
	}
	for _, n := range payload.Notes {
		fmt.Fprintf(w, "%-40s %s\n", n.Title, n.Path)
	}
	warning := ""
	if payload.Injection.Truncated {
		warning = " (TRUNCATED — the DA is not seeing all of it)"
	}
	fmt.Fprintf(w, "\n%d note(s), injecting %d/%d bytes%s\n",
		len(payload.Notes), payload.Injection.Bytes, payload.Injection.CapBytes, warning)
	return nil
}
