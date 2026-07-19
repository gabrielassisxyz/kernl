package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
)

var sessionSubcommands = []string{"nudge", "nudge-prompts"}

// The presets the server substitutes templates for. Validated here so a typo
// is named as a typo instead of silently falling back to the generic prompt.
var sessionNudgePresets = []string{"generic", "advance_status"}

var sessionCommand = commandMeta{
	Name:    "session",
	Summary: "Poke a running agent session",
	Usage:   "kernl session <nudge|nudge-prompts> [args...]",
	Details: `A session is one agent run driving one bead. These verbs call the same REST
API the web GUI calls, so a server must be running: start one with
'kernl serve', or point elsewhere with --server <url> (env: KERNL_SERVER).

Live session output is an SSE stream and has no CLI verb; read it in the GUI
or with 'curl -N <server>/api/sessions/<id>/events'.

Run 'kernl session <subcommand> --help' for details on each.`,
	Subs: []commandMeta{
		{
			Name:    "nudge",
			Summary: "Send a prompt into a session that has gone quiet",
			Usage:   "kernl session nudge <session-id> [--preset <preset>] [--prompt <text>] [--json]",
			Details: `Delivers a message to the agent already running in that session. Use it when
a run stalls, or to push it to record its status.

Flags:
  --preset <preset>  generic (default, server-side) or advance_status
  --prompt <text>    Send this exact text instead of the preset's template
  --json             Emit the API's {"status","sessionId"} on stdout

A session that is mid-turn answers 409 and a session with no agent attached
answers 422; both exit 2, since the fix is to retry or pick another session.

Example:
  kernl session nudge sess-2 --preset advance_status`,
		},
		{
			Name:    "nudge-prompts",
			Summary: "Show the prompt text each preset would send",
			Usage:   "kernl session nudge-prompts <session-id> [--json]",
			Details: `Prints the templates already substituted for this session's bead and repo,
so you can read what 'session nudge' would send before sending it — or copy
one, edit it, and pass it back with --prompt.

Flags:
  --json  Emit the API's {"beadId","opencodeSessionId","running","generic",
          "advance_status"} on stdout`,
		},
	},
}

// sessionPromptsView mirrors the fields of GET /api/sessions/{id}/nudge-prompts
// that the human-readable rendering prints. --json passes the server's own body
// through, so this struct never becomes a second wire contract.
type sessionPromptsView struct {
	BeadID            string `json:"beadId"`
	OpencodeSessionID string `json:"opencodeSessionId"`
	Running           bool   `json:"running"`
	Generic           string `json:"generic"`
	AdvanceStatus     string `json:"advance_status"`
}

func runSession(v verbContext, args []string) error {
	sub, rest, err := requireSub("session", args, sessionSubcommands)
	if err != nil {
		return err
	}
	asJSON, rest := parseBoolFlag(rest, "--json")
	if sub == "nudge" {
		return runSessionNudge(v, asJSON, rest)
	}
	return runSessionNudgePrompts(v, asJSON, rest)
}

func runSessionNudge(v verbContext, asJSON bool, args []string) error {
	body, rest, err := sessionNudgeBody("session nudge", args)
	if err != nil {
		return err
	}
	if err := rejectUnknownFlags("session nudge", rest); err != nil {
		return err
	}
	id, err := singleSessionID("session nudge", rest)
	if err != nil {
		return err
	}

	raw, err := requestSession(v, func(ctx context.Context, c *apiClient) (json.RawMessage, error) {
		return c.post(ctx, "/api/sessions/"+url.PathEscape(id)+"/nudge", body)
	})
	if err != nil {
		return err
	}
	if asJSON {
		return emitTaskAck(v.stdout(), raw, id, "nudged")
	}
	return printSessionNudge(v.stdout(), raw, id)
}

func runSessionNudgePrompts(v verbContext, asJSON bool, args []string) error {
	if err := rejectUnknownFlags("session nudge-prompts", args); err != nil {
		return err
	}
	id, err := singleSessionID("session nudge-prompts", args)
	if err != nil {
		return err
	}

	raw, err := requestSession(v, func(ctx context.Context, c *apiClient) (json.RawMessage, error) {
		return c.get(ctx, "/api/sessions/"+url.PathEscape(id)+"/nudge-prompts")
	})
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(v.stdout(), raw)
	}
	return printSessionPrompts(v.stdout(), raw, id)
}

// requestSession builds the client only after the invocation has been
// validated, so a malformed command is diagnosed without needing a loadable
// config or a running server.
func requestSession(v verbContext, call func(context.Context, *apiClient) (json.RawMessage, error)) (json.RawMessage, error) {
	c, err := v.client()
	if err != nil {
		return nil, err
	}
	return call(context.Background(), c)
}

// sessionNudgeBody maps the nudge flags onto the POST payload and returns the
// leftover positional args. Only flags the caller passed are included: the
// handler owns the default preset, and sending an empty one would override it.
func sessionNudgeBody(verb string, args []string) (map[string]any, []string, error) {
	body := map[string]any{}
	preset, presetGiven, rest, err := takeFlag(verb, args, "--preset")
	if err != nil {
		return nil, nil, err
	}
	prompt, promptGiven, rest, err := takeFlag(verb, rest, "--prompt")
	if err != nil {
		return nil, nil, err
	}
	if presetGiven {
		if err := checkNudgePreset(preset); err != nil {
			return nil, nil, err
		}
		body["preset"] = preset
	}
	if promptGiven {
		if strings.TrimSpace(prompt) == "" {
			return nil, nil, usagef("KERNL DISPATCH FAILURE: --prompt needs text to send — omit it to use the preset's template. Run: kernl session nudge --help")
		}
		body["prompt"] = prompt
	}
	return body, rest, nil
}

func checkNudgePreset(preset string) error {
	for _, p := range sessionNudgePresets {
		if preset == p {
			return nil
		}
	}
	return usagef("KERNL DISPATCH FAILURE: unknown nudge preset %q%s — valid: %s. Run: kernl session nudge --help",
		preset, didYouMean(preset, sessionNudgePresets), strings.Join(sessionNudgePresets, ", "))
}

func singleSessionID(verb string, args []string) (string, error) {
	if len(args) == 0 {
		return "", usagef("KERNL DISPATCH FAILURE: %s requires a session ID — run: kernl %s <session-id>. Find one with: kernl epic sessions <epic-id>", verb, verb)
	}
	if len(args) > 1 {
		return "", usagef("KERNL DISPATCH FAILURE: %s takes exactly one session ID, got %d (%s) — run: kernl %s --help",
			verb, len(args), strings.Join(args, ", "), verb)
	}
	return args[0], nil
}

func printSessionNudge(w io.Writer, raw json.RawMessage, id string) error {
	var ack struct {
		Status string `json:"status"`
	}
	if err := decodeInto(raw, "POST /api/sessions/{id}/nudge", &ack); err != nil {
		return err
	}
	if ack.Status == "" {
		fmt.Fprintf(w, "Nudged session %s\n", id)
		return nil
	}
	fmt.Fprintf(w, "Nudged session %s: %s\n", id, ack.Status)
	return nil
}

func printSessionPrompts(w io.Writer, raw json.RawMessage, id string) error {
	var view sessionPromptsView
	if err := decodeInto(raw, "GET /api/sessions/{id}/nudge-prompts", &view); err != nil {
		return err
	}
	fmt.Fprintf(w, "session %s  ·  %s\n\n", id, sessionPromptsHeader(view))
	printNamedPrompt(w, "generic", view.Generic)
	printNamedPrompt(w, "advance_status", view.AdvanceStatus)
	return nil
}

func sessionPromptsHeader(view sessionPromptsView) string {
	parts := []string{"idle"}
	if view.Running {
		// A running session rejects a nudge with 409, so say so up front.
		parts = []string{"running (a nudge will be refused until it finishes)"}
	}
	if view.BeadID != "" {
		parts = append(parts, "bead "+view.BeadID)
	}
	if view.OpencodeSessionID != "" {
		parts = append(parts, "agent session "+view.OpencodeSessionID)
	}
	return strings.Join(parts, "  ·  ")
}

func printNamedPrompt(w io.Writer, name, prompt string) {
	if prompt == "" {
		fmt.Fprintf(w, "%s:\n  (the API returned no template)\n\n", name)
		return
	}
	fmt.Fprintf(w, "%s:\n", name)
	for _, line := range strings.Split(strings.TrimRight(prompt, "\n"), "\n") {
		fmt.Fprintf(w, "  %s\n", line)
	}
	fmt.Fprintln(w)
}
