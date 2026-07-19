package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// epicAPISubcommands documents the read-only half of `epic`; help.go splices it
// into the `epic` entry next to list/run/merge/abort.
var epicAPISubcommands = []commandMeta{
	{
		Name:    "events",
		Summary: "Print an epic's orchestration events",
		Usage:   "kernl epic events <epic-id> [--follow] [--limit <n>] [--timeout <dur>] [--json]",
		Details: `The route is an SSE stream, not a paged list: it replays the hub's buffer
for the epic and then stays open. So by default this drains the replay
buffer and exits once the stream falls quiet — what a script wants — and
--follow keeps it open instead, like 'tail -f'.

Flags:
  --follow          Keep streaming as events arrive (exit with Ctrl-C)
  --limit <n>       Stop after n events
  --timeout <dur>   Give up after a duration (e.g. 30s, 2m)
  --json            One compact JSON event per line (NDJSON — a stream has
                    no last element to close an array with)`,
	},
	{
		Name:    "sessions",
		Summary: "List the agent sessions the server has open",
		Usage:   "kernl epic sessions <epic-id> [--json]",
		Details: `The epic id is required by the route but the server answers with every
active session in the process, not only that epic's — the session manager
does not index by epic.

  --json  Emit the API's session array verbatim`,
	},
}

// epicEventsIdleWindow is how long the drain waits after the last event before
// deciding the replay buffer is exhausted. The stream never signals "end of
// buffer", so quiet is the only available end marker.
const epicEventsIdleWindow = 750 * time.Millisecond

// epicEventView mirrors the fields of an epic event worth reading on a
// terminal; --json passes the server's own line through instead.
type epicEventView struct {
	Type      string `json:"type"`
	BeadID    string `json:"beadId"`
	SessionID string `json:"sessionId"`
	Detail    string `json:"detail"`
	Time      int64  `json:"time"`
}

type epicSessionView struct {
	SessionID string `json:"sessionId"`
	BeadID    string `json:"beadId"`
	Exited    bool   `json:"exited"`
}

func runEpicAPI(v verbContext, args []string) error {
	if len(args) == 0 {
		return usagef("KERNL DISPATCH FAILURE: epic requires a subcommand — run: kernl epic --help")
	}
	asJSON, rest := parseBoolFlag(args[1:], "--json")
	if args[0] == "sessions" {
		return runEpicSessions(v, asJSON, rest)
	}
	return runEpicEvents(v, asJSON, rest)
}

func runEpicSessions(v verbContext, asJSON bool, args []string) error {
	if err := rejectUnknownFlags("epic sessions", args); err != nil {
		return err
	}
	id, err := singleEpicID("epic sessions", args)
	if err != nil {
		return err
	}
	raw, err := epicAPICall(v, func(ctx context.Context, c *apiClient) (json.RawMessage, error) {
		return c.get(ctx, "/api/epics/"+url.PathEscape(id)+"/sessions")
	})
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(v.stdout(), raw)
	}
	return printEpicSessions(v.stdout(), raw)
}

func runEpicEvents(v verbContext, asJSON bool, args []string) error {
	opts, rest, err := parseEpicEventsOptions(args)
	if err != nil {
		return err
	}
	if err := rejectUnknownFlags("epic events", rest); err != nil {
		return err
	}
	id, err := singleEpicID("epic events", rest)
	if err != nil {
		return err
	}
	c, err := v.client()
	if err != nil {
		return err
	}
	return streamEpicEvents(v, c, id, asJSON, opts)
}

// epicEventsOptions holds the three ways the caller can bound an open stream.
type epicEventsOptions struct {
	follow  bool
	limit   int
	timeout time.Duration
}

func parseEpicEventsOptions(args []string) (epicEventsOptions, []string, error) {
	opts := epicEventsOptions{}
	follow, rest := parseBoolFlag(args, "--follow")
	opts.follow = follow

	limit, present, rest, err := takeFlag(rest, "--limit")
	if err != nil {
		return opts, nil, err
	}
	if present {
		n, convErr := strconv.Atoi(strings.TrimSpace(limit))
		if convErr != nil || n <= 0 {
			return opts, nil, usagef("KERNL DISPATCH FAILURE: --limit requires a positive integer, got %q — example: --limit 20", limit)
		}
		opts.limit = n
	}

	timeout, present, rest, err := takeFlag(rest, "--timeout")
	if err != nil {
		return opts, nil, err
	}
	if present {
		d, convErr := time.ParseDuration(strings.TrimSpace(timeout))
		if convErr != nil || d <= 0 {
			return opts, nil, usagef("KERNL DISPATCH FAILURE: --timeout requires a positive duration, got %q — example: --timeout 30s", timeout)
		}
		opts.timeout = d
	}
	return opts, rest, nil
}

// streamEpicEvents opens the SSE route and prints events until whichever bound
// applies is reached. It does not go through apiClient.request: that reads the
// whole body before returning and caps every call at 60s, neither of which a
// stream survives.
func streamEpicEvents(v verbContext, c *apiClient, id string, asJSON bool, opts epicEventsOptions) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if opts.timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, opts.timeout)
		defer cancel()
	}

	path := "/api/epics/" + url.PathEscape(id) + "/events"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return wrapLoud("building request", err)
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return c.unreachable(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return httpStatusError(http.MethodGet, path, resp.StatusCode, body)
	}
	return consumeEpicEventStream(v, resp.Body, asJSON, opts)
}

// consumeEpicEventStream prints SSE data frames until the caller's bound is
// hit. Reading happens in a goroutine because the drain has to give up on a
// silent stream, and a blocked Read cannot be selected on.
func consumeEpicEventStream(v verbContext, body io.Reader, asJSON bool, opts epicEventsOptions) error {
	frames := make(chan []byte)
	// done unblocks the reader when we stop early (--limit, idle drain), so the
	// goroutine cannot be left parked on a send nobody will receive.
	done := make(chan struct{})
	defer close(done)
	go scanSSEFrames(body, frames, done)

	idle := time.NewTimer(epicEventsIdleWindow)
	defer idle.Stop()
	seen := 0
	for {
		select {
		case frame, ok := <-frames:
			if !ok {
				return epicEventsSummary(v, asJSON, seen)
			}
			if err := printEpicEvent(v.stdout(), frame, asJSON); err != nil {
				return err
			}
			seen++
			if opts.limit > 0 && seen >= opts.limit {
				return nil
			}
			idle.Reset(epicEventsIdleWindow)
		case <-idle.C:
			if opts.follow {
				idle.Reset(epicEventsIdleWindow)
				continue
			}
			return epicEventsSummary(v, asJSON, seen)
		}
	}
}

func scanSSEFrames(body io.Reader, frames chan<- []byte, done <-chan struct{}) {
	defer close(frames)
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		raw := scanner.Text()
		// An SSE frame carries its payload on a "data:" line; the blank
		// separators and any comment lines are not events.
		line := strings.TrimPrefix(raw, "data: ")
		if line == raw || strings.TrimSpace(line) == "" {
			continue
		}
		select {
		case frames <- []byte(line):
		case <-done:
			return
		}
	}
}

// epicEventsSummary keeps the human output honest about an empty stream; --json
// stays silent so an NDJSON consumer never receives a prose line.
func epicEventsSummary(v verbContext, asJSON bool, seen int) error {
	if asJSON || seen > 0 {
		return nil
	}
	fmt.Fprintln(v.stdout(), "No events for this epic. It may not have run yet — start it with: kernl epic run <epic-id>")
	return nil
}

func printEpicEvent(w io.Writer, frame []byte, asJSON bool) error {
	if asJSON {
		_, err := fmt.Fprintln(w, string(frame))
		return err
	}
	var e epicEventView
	if err := decodeInto(frame, "GET /api/epics/{id}/events", &e); err != nil {
		return err
	}
	fmt.Fprintf(w, "%-19s %-18s %s%s\n", epicEventTime(e.Time), e.Type, e.BeadID, epicEventDetail(e))
	return nil
}

// epicEventTime renders the hub's epoch stamp; a zero stamp means the event
// carried none, and printing 1970 would read as a real timestamp.
func epicEventTime(stamp int64) string {
	if stamp == 0 {
		return "-"
	}
	return time.Unix(stamp, 0).Format("2006-01-02 15:04:05")
}

func epicEventDetail(e epicEventView) string {
	var parts []string
	if e.Detail != "" {
		parts = append(parts, e.Detail)
	}
	if e.SessionID != "" {
		parts = append(parts, "session "+e.SessionID)
	}
	if len(parts) == 0 {
		return ""
	}
	return "  (" + strings.Join(parts, ", ") + ")"
}

func printEpicSessions(w io.Writer, raw json.RawMessage) error {
	var sessions []epicSessionView
	if err := decodeInto(raw, "GET /api/epics/{id}/sessions", &sessions); err != nil {
		return err
	}
	if len(sessions) == 0 {
		fmt.Fprintln(w, "No active agent sessions.")
		return nil
	}
	for _, s := range sessions {
		fmt.Fprintf(w, "%-28s bead %-20s %s\n", s.SessionID, s.BeadID, epicSessionState(s.Exited))
	}
	fmt.Fprintf(w, "\n%d session(s)\n", len(sessions))
	return nil
}

func epicSessionState(exited bool) string {
	if exited {
		return "exited"
	}
	return "running"
}

func epicAPICall(v verbContext, call func(context.Context, *apiClient) (json.RawMessage, error)) (json.RawMessage, error) {
	c, err := v.client()
	if err != nil {
		return nil, err
	}
	return call(context.Background(), c)
}

func singleEpicID(verb string, args []string) (string, error) {
	if len(args) == 0 {
		return "", usagef("KERNL DISPATCH FAILURE: %s requires an epic ID — run: kernl %s <epic-id>. List them with: kernl epic list", verb, verb)
	}
	if len(args) > 1 {
		return "", usagef("KERNL DISPATCH FAILURE: %s takes exactly one epic ID, got %d (%s) — run: kernl %s --help",
			verb, len(args), strings.Join(args, ", "), verb)
	}
	return args[0], nil
}
