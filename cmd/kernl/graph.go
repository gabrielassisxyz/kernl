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

var graphSubcommands = []string{"nodes", "search", "related", "briefing", "edges"}

var graphCommand = commandMeta{
	Name:    "graph",
	Summary: "Read the knowledge graph (nodes, edges, search, relatedness)",
	Usage:   "kernl graph <nodes|search|related|briefing|edges> [args...]",
	Details: `Every subcommand is read-only. They talk to a running server over the
REST API, so 'kernl serve' must be up (or point elsewhere with
--server <url> / KERNL_SERVER).

Run 'kernl graph <subcommand> --help' for details on each.`,
	Subs: []commandMeta{
		{
			Name:    "nodes",
			Summary: "List every live node in the graph",
			Usage:   "kernl graph nodes [--json]",
			Details: `The whole graph, tombstones excluded. The route takes no filters, so
narrow it with 'kernl graph search' rather than expecting flags here.

{{flags}}`,
			Flags: []commandFlag{
				{Name: "--json", Description: `Emit [{"id","title","type"}] verbatim on stdout`},
			},
		},
		{
			Name:    "search",
			Summary: "Prefix-search node titles",
			Usage:   "kernl graph search <query> [--type <node-type>] [--limit <n>] [--json]",
			Details: `Matches title prefixes (this is the editor's wikilink autocomplete),
not full-text over bodies.

{{flags}}

Example:
  kernl graph search backup --type note --limit 5`,
			Flags: []commandFlag{
				{Name: "--type", Value: "<node-type>", Description: "Restrict to one node type, e.g. note, task, project"},
				{Name: "--limit", Value: "<n>", Description: "Maximum hits, 1-50 (server default: 10; it clamps at 50)"},
				{Name: "--json", Description: `Emit [{"id","title","type"}] verbatim on stdout`},
			},
		},
		{
			Name:    "related",
			Summary: "Show the nodes a node is related to",
			Usage:   "kernl graph related <node-id> [--limit <n>] [--json]",
			Details: `Computed relevance, not stored edges — use 'kernl graph edges' for the
connections as persisted.

{{flags}}`,
			Flags: []commandFlag{
				{Name: "--limit", Value: "<n>", Description: "Maximum results (server default: 10)"},
				{Name: "--json", Description: `Emit [{"id","title","type"}] verbatim on stdout`},
			},
		},
		{
			Name:    "briefing",
			Summary: "Print the DA briefing note attached to a node",
			Usage:   "kernl graph briefing <node-id> [--json]",
			Details: `A node the DA never briefed is a normal answer, not a broken
invocation: it exits 0 with "No briefing for <id> yet." (or {"briefing":null}
under --json), so branch on the body/JSON, never on the exit code.

{{flags}}`,
			Flags: []commandFlag{
				{Name: "--json", Description: `Emit {"id","title","body"} verbatim on stdout, or {"briefing":null}`,
					Continuation: []string{"when the node has no briefing yet"}},
			},
		},
		{
			Name:    "edges",
			Summary: "List every stored edge between live nodes",
			Usage:   "kernl graph edges [--json]",
			Details: `The edges table as persisted — the source of truth for checking that
connections are actually being made.

{{flags}}`,
			Flags: []commandFlag{
				{Name: "--json", Description: `Emit [{"id","src","dst","label"}] verbatim on stdout`},
			},
		},
	},
}

// graphNodeView is the shape both /api/nodes and the two node-listing routes
// answer with; --json passes the server's body through, so this stays a
// display-only subset.
type graphNodeView struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Type  string `json:"type"`
}

type graphEdgeView struct {
	ID    string `json:"id"`
	Src   string `json:"src"`
	Dst   string `json:"dst"`
	Label string `json:"label"`
}

func runGraph(v verbContext, args []string) error {
	sub, rest, err := requireSub("graph", args, graphSubcommands)
	if err != nil {
		return err
	}
	asJSON, rest := parseBoolFlag(rest, "--json")
	switch sub {
	case "nodes":
		return runGraphNodes(v, asJSON, rest)
	case "search":
		return runGraphSearch(v, asJSON, rest)
	case "related":
		return runGraphRelated(v, asJSON, rest)
	case "briefing":
		return runGraphBriefing(v, asJSON, rest)
	default:
		return runGraphEdges(v, asJSON, rest)
	}
}

func runGraphNodes(v verbContext, asJSON bool, args []string) error {
	if err := noGraphArgs("graph nodes", args); err != nil {
		return err
	}
	raw, err := requestGraph(v, "/api/nodes")
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(v.stdout(), raw)
	}
	return printGraphNodes(v.stdout(), raw, "GET /api/nodes",
		"The graph is empty. Put something in it with: kernl capture \"<text>\"")
}

func runGraphSearch(v verbContext, asJSON bool, args []string) error {
	nodeType, _, rest, err := takeFlag("graph search", args, "--type")
	if err != nil {
		return err
	}
	limit, rest, err := takeGraphLimit(rest, "graph search")
	if err != nil {
		return err
	}
	if err := rejectUnknownFlags("graph search", rest); err != nil {
		return err
	}
	if len(rest) == 0 {
		return usagef("KERNL DISPATCH FAILURE: graph search requires a query — run: kernl graph search <query> [--type <node-type>]")
	}

	query := url.Values{"q": {strings.Join(rest, " ")}}
	if nodeType != "" {
		query.Set("type", nodeType)
	}
	if limit != "" {
		query.Set("limit", limit)
	}
	raw, err := requestGraph(v, "/api/nodes/search?"+query.Encode())
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(v.stdout(), raw)
	}
	return printGraphNodes(v.stdout(), raw, "GET /api/nodes/search",
		"No matches. Search is over title prefixes only — try a shorter prefix.")
}

func runGraphRelated(v verbContext, asJSON bool, args []string) error {
	limit, rest, err := takeGraphLimit(args, "graph related")
	if err != nil {
		return err
	}
	if err := rejectUnknownFlags("graph related", rest); err != nil {
		return err
	}
	id, err := singleGraphNodeID("graph related", rest)
	if err != nil {
		return err
	}

	path := "/api/nodes/" + url.PathEscape(id) + "/related"
	if limit != "" {
		path += "?limit=" + url.QueryEscape(limit)
	}
	raw, err := requestGraph(v, path)
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(v.stdout(), raw)
	}
	return printGraphNodes(v.stdout(), raw, "GET /api/nodes/{id}/related",
		"Nothing related yet. Relatedness needs tags, links or shared context to work from.")
}

func runGraphBriefing(v verbContext, asJSON bool, args []string) error {
	if err := rejectUnknownFlags("graph briefing", args); err != nil {
		return err
	}
	id, err := singleGraphNodeID("graph briefing", args)
	if err != nil {
		return err
	}
	// A node with no briefing yet is a normal answer to a fair question, not a
	// mis-invocation, so it exits 0 with an explicit "none" rather than the
	// exit 2 a bare 404 would produce.
	c, err := v.client()
	if err != nil {
		return err
	}
	raw, found, err := c.getOptional(context.Background(), "/api/nodes/"+url.PathEscape(id)+"/briefing")
	if err != nil {
		return err
	}
	if !found {
		if asJSON {
			_, err := fmt.Fprintln(v.stdout(), `{"briefing":null}`)
			return err
		}
		fmt.Fprintf(v.stdout(), "No briefing for %s yet.\n", id)
		return nil
	}
	if asJSON {
		return emitJSON(v.stdout(), raw)
	}
	var note struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if err := decodeInto(raw, "GET /api/nodes/{id}/briefing", &note); err != nil {
		return err
	}
	fmt.Fprintf(v.stdout(), "%s\n\n%s\n", note.Title, note.Body)
	return nil
}

func runGraphEdges(v verbContext, asJSON bool, args []string) error {
	if err := noGraphArgs("graph edges", args); err != nil {
		return err
	}
	raw, err := requestGraph(v, "/api/edges")
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(v.stdout(), raw)
	}
	var edges []graphEdgeView
	if err := decodeInto(raw, "GET /api/edges", &edges); err != nil {
		return err
	}
	if len(edges) == 0 {
		fmt.Fprintln(v.stdout(), "No edges. Nothing in the graph is connected yet.")
		return nil
	}
	for _, e := range edges {
		fmt.Fprintf(v.stdout(), "%-38s --%s--> %s\n", e.Src, e.Label, e.Dst)
	}
	fmt.Fprintf(v.stdout(), "\n%d edge(s)\n", len(edges))
	return nil
}

// requestGraph builds the client only after the invocation has been validated,
// so a malformed command is diagnosed without needing a running server.
func requestGraph(v verbContext, path string) (json.RawMessage, error) {
	c, err := v.client()
	if err != nil {
		return nil, err
	}
	return c.get(context.Background(), path)
}

// takeGraphLimit validates --limit here rather than letting the handlers ignore
// an unparseable value: silently serving the default is exactly the kind of
// quiet wrong answer a scripted caller cannot detect.
func takeGraphLimit(args []string, verb string) (string, []string, error) {
	raw, present, rest, err := takeFlag(verb, args, "--limit")
	if err != nil {
		return "", nil, err
	}
	if !present {
		return "", rest, nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 1 {
		return "", nil, usagef("KERNL DISPATCH FAILURE: %s --limit must be a positive integer, got %q — run: kernl %s --help", verb, raw, verb)
	}
	return strconv.Itoa(n), rest, nil
}

func noGraphArgs(verb string, args []string) error {
	if err := rejectUnknownFlags(verb, args); err != nil {
		return err
	}
	if len(args) > 0 {
		return usagef("KERNL DISPATCH FAILURE: %s takes no positional arguments, got %q — run: kernl %s --help", verb, args[0], verb)
	}
	return nil
}

func singleGraphNodeID(verb string, args []string) (string, error) {
	if len(args) == 0 {
		return "", usagef("KERNL DISPATCH FAILURE: %s requires a node ID — run: kernl %s <node-id>. Find one with: kernl graph search <query>", verb, verb)
	}
	if len(args) > 1 {
		return "", usagef("KERNL DISPATCH FAILURE: %s takes exactly one node ID, got %d (%s) — run: kernl %s --help",
			verb, len(args), strings.Join(args, ", "), verb)
	}
	return args[0], nil
}

func printGraphNodes(w io.Writer, raw json.RawMessage, route, emptyHint string) error {
	var nodes []graphNodeView
	if err := decodeInto(raw, route, &nodes); err != nil {
		return err
	}
	if len(nodes) == 0 {
		fmt.Fprintln(w, emptyHint)
		return nil
	}
	for _, n := range nodes {
		fmt.Fprintf(w, "%-38s [%-12s] %s\n", n.ID, n.Type, n.Title)
	}
	fmt.Fprintf(w, "\n%d node(s)\n", len(nodes))
	return nil
}
