package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
	"time"
)

var approvalSubcommands = []string{"list", "resolve"}

// The two routes speak different action vocabularies because they resolve
// different registries: /api/approvals drives the orchestrator's approval
// registry (the GUI's gate buttons), while the terminal route answers an
// agent's in-session prompt, which additionally understands "remember this
// answer". Offering the union on both would let a caller send a word the
// target route cannot act on.
var (
	approvalGateActions     = []string{"approve", "reject"}
	approvalSessionActions  = []string{"accept", "always_approve", "decline"}
	approvalGrantingActions = map[string]bool{
		"approve": true, "accept": true, "always_approve": true,
	}
)

var approvalCommand = commandMeta{
	Name:    "approval",
	Summary: "See and resolve the gates waiting on a human",
	Usage:   "kernl approval <list|resolve> [args...]",
	Details: `Approvals are the points where the orchestrator stops and waits for a
person. These verbs call the same REST API the web GUI calls, so a server
must be running: start one with 'kernl serve', or point elsewhere with
--server <url> (env: KERNL_SERVER).

Run 'kernl approval <subcommand> --help' for details on each.`,
	Subs: []commandMeta{
		{
			Name:    "list",
			Summary: "Show what is waiting, for what, and since when",
			Usage:   "kernl approval list [--json]",
			Details: `Actionable requests are listed first, oldest first, each with the exact
command that resolves it.

{{flags}}`,
			Flags: []commandFlag{
				{Name: "--json", Description: "Emit the API's approval array verbatim (camelCase)"},
			},
		},
		{
			Name:    "resolve",
			Summary: "Answer one approval (approve/reject, or an in-session prompt)",
			Usage:   "kernl approval resolve <approval-id> --action <action> [--session <session-id>] [--yes] [--json]",
			Details: `Without --session the request goes to the orchestrator gate, whose actions
are: approve, reject.

With --session it answers the prompt an agent raised inside that terminal
session, whose actions are: accept, always_approve, decline. 'always_approve'
also silences future prompts of the same shape for that session.

Granting an approval (approve, accept, always_approve) requires --yes:
letting an agent proceed unreviewed is the one mistake this gate exists to
prevent. Declining does not, since it only stops work. Without --yes nothing
is sent to the server and the command exits 2 — a refused mutation is not a
success.

{{flags}}

Example:
  kernl approval list
  kernl approval resolve apr-7 --action approve --yes
  kernl approval resolve apr-7 --action decline --session sess-2`,
			Flags: []commandFlag{
				{Name: "--action", Value: "<action>", Description: "Required. What to answer"},
				{Name: "--session", Value: "<id>", Description: "Answer inside that terminal session instead"},
				{Name: "--yes", Description: "Confirm a granting action"},
				{Name: "--json", Description: `Emit {"id","resolved":true} (or the server's body)`},
			},
		},
	},
}

// approvalView mirrors the fields of the API's approval DTO that the
// human-readable listing prints. --json passes the server's own body through,
// so this struct never becomes a second wire contract.
type approvalView struct {
	ID               string   `json:"id"`
	Status           string   `json:"status"`
	CreatedAt        string   `json:"createdAt"`
	RepoPath         string   `json:"repoPath"`
	BeadID           string   `json:"beadId"`
	SessionID        string   `json:"sessionId"`
	Adapter          string   `json:"adapter"`
	ToolName         string   `json:"toolName"`
	SupportedActions []string `json:"supportedActions"`
	Actionable       bool     `json:"actionable"`
}

func runApproval(v verbContext, args []string) error {
	sub, rest, err := requireSub("approval", args, approvalSubcommands)
	if err != nil {
		return err
	}
	asJSON, rest := parseBoolFlag(rest, "--json")
	if sub == "list" {
		return runApprovalList(v, asJSON, rest)
	}
	return runApprovalResolve(v, asJSON, rest)
}

func runApprovalList(v verbContext, asJSON bool, args []string) error {
	if err := rejectUnknownFlags("approval list", args); err != nil {
		return err
	}
	if len(args) > 0 {
		return usagef("KERNL DISPATCH FAILURE: approval list takes no positional arguments, got %q — run: kernl approval list --help", args[0])
	}

	raw, err := requestApproval(v, func(ctx context.Context, c *apiClient) (json.RawMessage, error) {
		return c.get(ctx, "/api/approvals")
	})
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(v.stdout(), raw)
	}
	return printApprovalList(v.stdout(), raw)
}

func runApprovalResolve(v verbContext, asJSON bool, args []string) error {
	action, _, rest, err := takeFlag("approval resolve", args, "--action")
	if err != nil {
		return err
	}
	session, _, rest, err := takeFlag("approval resolve", rest, "--session")
	if err != nil {
		return err
	}
	confirmed, rest := parseBoolFlag(rest, "--yes")
	if err := rejectUnknownFlags("approval resolve", rest); err != nil {
		return err
	}
	id, err := singleApprovalID(rest)
	if err != nil {
		return err
	}
	if err := checkApprovalAction(action, session); err != nil {
		return err
	}

	// Preview without contacting the server at all: an unconfirmed grant must
	// not depend on the server being reachable to be safe.
	if approvalGrantingActions[action] && !confirmed {
		fmt.Fprintf(v.stdout(), "Would resolve approval %s with %q%s. Re-run with --yes to confirm.\n",
			id, action, approvalSessionSuffix(session))
		return refusedWithoutYes("approval resolve")
	}
	return sendApprovalAction(v, asJSON, id, session, action)
}

func sendApprovalAction(v verbContext, asJSON bool, id, session, action string) error {
	path := "/api/approvals/" + url.PathEscape(id) + "/actions"
	if session != "" {
		path = "/api/terminal/" + url.PathEscape(session) + "/approvals/" + url.PathEscape(id)
	}
	raw, err := requestApproval(v, func(ctx context.Context, c *apiClient) (json.RawMessage, error) {
		return c.post(ctx, path, map[string]any{"action": action})
	})
	if err != nil {
		return err
	}
	if asJSON {
		// Shared with the task verbs: the ack shape is about the CLI's --json
		// contract (never emit an empty document), not about tasks.
		return emitTaskAck(v.stdout(), raw, id, "resolved")
	}
	fmt.Fprintf(v.stdout(), "Resolved approval %s with %q%s\n", id, action, approvalSessionSuffix(session))
	return nil
}

// requestApproval builds the client only after the invocation has been
// validated, so a malformed command is diagnosed without needing a loadable
// config or a running server.
func requestApproval(v verbContext, call func(context.Context, *apiClient) (json.RawMessage, error)) (json.RawMessage, error) {
	c, err := v.client()
	if err != nil {
		return nil, err
	}
	return call(context.Background(), c)
}

// checkApprovalAction validates against the vocabulary of the route the flags
// selected, so a word that belongs to the other route is rejected here rather
// than silently accepted by a handler that cannot act on it.
func checkApprovalAction(action, session string) error {
	valid := approvalGateActions
	scope := "the orchestrator gate"
	if session != "" {
		valid = approvalSessionActions
		scope = "a terminal session (--session)"
	}
	if action == "" {
		return usagef("KERNL DISPATCH FAILURE: approval resolve requires --action — valid for %s: %s. Run: kernl approval resolve --help",
			scope, strings.Join(valid, ", "))
	}
	for _, a := range valid {
		if action == a {
			return nil
		}
	}
	return usagef("KERNL DISPATCH FAILURE: unknown approval action %q for %s%s — valid: %s. Run: kernl approval resolve --help",
		action, scope, didYouMean(action, valid), strings.Join(valid, ", "))
}

func singleApprovalID(args []string) (string, error) {
	if len(args) == 0 {
		return "", usagef("KERNL DISPATCH FAILURE: approval resolve requires an approval ID — run: kernl approval resolve <approval-id> --action <action>. List them with: kernl approval list")
	}
	if len(args) > 1 {
		return "", usagef("KERNL DISPATCH FAILURE: approval resolve takes exactly one approval ID, got %d (%s) — run: kernl approval resolve --help",
			len(args), strings.Join(args, ", "))
	}
	return args[0], nil
}

func approvalSessionSuffix(session string) string {
	if session == "" {
		return ""
	}
	return " in session " + session
}

func printApprovalList(w io.Writer, raw json.RawMessage) error {
	var approvals []approvalView
	if err := decodeInto(raw, "GET /api/approvals", &approvals); err != nil {
		return err
	}
	if len(approvals) == 0 {
		fmt.Fprintln(w, "Nothing waiting on you.")
		return nil
	}
	sortApprovals(approvals)

	waiting := 0
	for _, a := range approvals {
		if isApprovalWaiting(a) {
			waiting++
		}
		printApproval(w, a)
	}
	fmt.Fprintf(w, "%d approval(s), %d waiting on you\n", len(approvals), waiting)
	return nil
}

// sortApprovals puts what needs a decision at the top and the oldest of those
// first: the thing that has been blocked longest is the thing to look at.
func sortApprovals(approvals []approvalView) {
	sort.SliceStable(approvals, func(i, j int) bool {
		a, b := approvals[i], approvals[j]
		if isApprovalWaiting(a) != isApprovalWaiting(b) {
			return isApprovalWaiting(a)
		}
		return a.CreatedAt < b.CreatedAt
	})
}

func isApprovalWaiting(a approvalView) bool {
	return a.Actionable && a.Status == "pending"
}

func printApproval(w io.Writer, a approvalView) {
	marker := " "
	if isApprovalWaiting(a) {
		marker = "*"
	}
	fmt.Fprintf(w, "%s %-20s %-16s %s\n", marker, a.ID, a.Status, approvalAge(a.CreatedAt))
	fmt.Fprintf(w, "    asks for  %s\n", approvalSubject(a))
	if context := approvalContext(a); context != "" {
		fmt.Fprintf(w, "    context   %s\n", context)
	}
	if isApprovalWaiting(a) {
		fmt.Fprintf(w, "    resolve   %s\n", approvalResolveHint(a))
	}
	fmt.Fprintln(w)
}

func approvalSubject(a approvalView) string {
	if a.ToolName == "" {
		return "(the API reported no tool name)"
	}
	return a.ToolName
}

func approvalContext(a approvalView) string {
	var parts []string
	for _, p := range []struct{ label, value string }{
		{"bead", a.BeadID},
		{"session", a.SessionID},
		{"adapter", a.Adapter},
		{"repo", a.RepoPath},
	} {
		if p.value != "" {
			parts = append(parts, p.label+" "+p.value)
		}
	}
	return strings.Join(parts, "  ·  ")
}

// approvalResolveHint prints the command that answers this specific request,
// picking the route from the shape of the approval so the copied line is
// already correct for a session-scoped prompt.
func approvalResolveHint(a approvalView) string {
	grant, decline := "approve", "reject"
	scope := ""
	if isSessionScopedApproval(a) {
		grant, decline = "accept", "decline"
		scope = " --session " + a.SessionID
	}
	return fmt.Sprintf("kernl approval resolve %s --action %s%s --yes   (or --action %s)",
		a.ID, grant, scope, decline)
}

// isSessionScopedApproval reads the vocabulary the request itself advertises:
// a prompt raised inside a terminal session lists the session action words.
func isSessionScopedApproval(a approvalView) bool {
	if a.SessionID == "" {
		return false
	}
	for _, supported := range a.SupportedActions {
		for _, sessionAction := range approvalSessionActions {
			if supported == sessionAction {
				return true
			}
		}
	}
	return false
}

// approvalAge answers "since when" in the unit a human reads at a glance. An
// unparseable timestamp is shown verbatim rather than dropped: the server's
// own value is more useful than a blank, and less misleading than a guess.
func approvalAge(createdAt string) string {
	if createdAt == "" {
		return "age unknown"
	}
	t, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return createdAt
	}
	return humanizeApprovalAge(time.Since(t))
}

func humanizeApprovalAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours())/24)
	}
}
