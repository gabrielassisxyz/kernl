package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
)

// triage answers "what should I do now" in one round-trip.
//
// WHY this verb exists. Everything it reports was already reachable — inbox
// list, task list, ingest queue list, bead list, health all answer --json — but
// only as five separate calls whose results the caller then had to correlate.
// An agent asking the opening question of every session paid five round-trips
// and a join to get an answer the tool could assemble itself. Before this,
// `kernl status` answered "try: kernl epic list", sending someone who asked
// what to work on to an orchestrator listing.
//
// The ordering is the product decision: captures first (the inbox is where
// unprocessed input piles up and the thing most likely to be stale), then the
// work already moving, then what is free to start. Reading top to bottom is
// meant to be the order you would act in.
//
// A slice that cannot be read does NOT fail the command. A mega-command that
// dies because one route is down is worse than no mega-command: the caller
// loses the four slices that were fine. Each slice reports its own
// availability instead, and the exit code reflects only whether ANY slice
// answered — see triageReport.exitCode.

// triageSlice is one answerable question. Available slices carry items and a
// count; unavailable ones carry why, and the distinction is explicit rather
// than inferred from an empty list — "nothing pending" and "could not ask"
// must never look alike to a caller branching on the count.
type triageSlice struct {
	Available bool         `json:"available"`
	Reason    string       `json:"reason,omitempty"`
	Count     int          `json:"count"`
	Items     []triageItem `json:"items"`
	Command   string       `json:"command"`
}

type triageItem struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	State string `json:"state,omitempty"`
}

type triageReport struct {
	Captures  triageSlice  `json:"captures"`
	Ingest    triageSlice  `json:"ingest"`
	Running   triageSlice  `json:"running"`
	Ready     triageSlice  `json:"ready"`
	Tasks     triageSlice  `json:"tasks"`
	Approvals triageSlice  `json:"approvals"`
	Health    triageHealth `json:"health"`
}

type triageHealth struct {
	Available bool   `json:"available"`
	Status    string `json:"status"`
	Reason    string `json:"reason,omitempty"`
}

// triageItemLimit caps each slice. Triage is a decision aid, not a listing: the
// count tells you the size of the pile, the items are enough to recognise it,
// and the command is how you see the rest.
const triageItemLimit = 5

func runTriage(v verbContext, args []string) error {
	asJSON, rest := parseBoolFlag(args, "--json")
	if err := rejectUnknownFlags("triage", rest); err != nil {
		return err
	}
	if len(rest) > 0 {
		return usagef("KERNL DISPATCH FAILURE: triage takes no arguments, got %q — run: kernl triage --help", rest[0])
	}

	c, err := v.client()
	if err != nil {
		return err
	}
	// Resolve the address ONCE, before fanning out. The client resolves it
	// lazily on first use, so without this a single unresolvable server is
	// discovered independently by all six readers and reported six times — one
	// cause wearing six costumes, which buries the fact that they share one.
	// It is also the wrong diagnosis per section: the sections are fine.
	if _, err := c.base(); err != nil {
		return triageUnreachable(err)
	}

	report := collectTriage(context.Background(), c)
	if asJSON {
		body, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return wrapLoud("encoding the triage report", err)
		}
		if err := emitJSON(v.stdout(), json.RawMessage(body)); err != nil {
			return err
		}
	} else if err := printTriage(v.stdout(), report); err != nil {
		return err
	}
	// The report has already been written in both shapes; anything further is
	// exit code only, or the caller sees the failure twice.
	if err := report.exitCode(); err != nil {
		return reportedElsewhere(err)
	}
	return nil
}

// triageUnreachable explains a failure to even locate the server.
//
// WHY it names --server first. The generic config error says "run kernl from
// the directory containing kernl.yaml, or pass --config <path>" — advice that
// is useless here, because a triage run from an arbitrary directory has no
// kernl.yaml to point at and does not need one: it needs an address. A fix hint
// that omits the fix that works is worse than no hint, since it reads as
// authoritative and sends the caller down a dead end.
func triageUnreachable(err error) error {
	// Keep the diagnosis, drop the generic remedy riding on it: two "Fix:"
	// clauses in one message, the first of them wrong, is worse than one.
	cause := triageReason(err)
	if fix := strings.Index(cause, "Fix:"); fix >= 0 {
		cause = strings.TrimRight(strings.TrimSpace(cause[:fix]), " —")
	}
	return usagef("KERNL DISPATCH FAILURE: triage cannot tell where the server is — %s\n"+
		"Fix: pass --server <url> (e.g. --server http://127.0.0.1:8080), set KERNL_SERVER, "+
		"or run from a directory that has a kernl.yaml", cause)
}

// exitCode reports failure only when NOTHING could be read — which means the
// server is unreachable, not that one route is unhappy. A partial report is a
// successful triage: the caller got real answers plus a named gap.
func (r triageReport) exitCode() error {
	for _, s := range []bool{r.Captures.Available, r.Ingest.Available, r.Running.Available,
		r.Ready.Available, r.Tasks.Available, r.Approvals.Available, r.Health.Available} {
		if s {
			return nil
		}
	}
	return fmt.Errorf("KERNL DISPATCH FAILURE: triage could read nothing — is `kernl serve` running? Fix: start it, or point at another instance with --server <url>")
}

func collectTriage(ctx context.Context, c *apiClient) triageReport {
	var report triageReport
	var wg sync.WaitGroup
	// The six reads are independent, so paying for them serially would make the
	// one command that replaces five round-trips as slow as the five it replaces.
	for _, load := range []func(){
		func() { report.Captures = triageCaptures(ctx, c) },
		func() { report.Ingest = triageIngest(ctx, c) },
		func() { report.Tasks = triageTasks(ctx, c) },
		func() { report.Approvals = triageApprovals(ctx, c) },
		func() { report.Health = triageHealthCheck(ctx, c) },
		func() { report.Running, report.Ready = triageBeads(ctx, c) },
	} {
		wg.Add(1)
		go func(fn func()) { defer wg.Done(); fn() }(load)
	}
	wg.Wait()
	return report
}

func triageCaptures(ctx context.Context, c *apiClient) triageSlice {
	slice := triageSlice{Command: "kernl inbox list", Items: []triageItem{}}
	raw, err := c.get(ctx, "/api/inbox/pending")
	if err != nil {
		slice.Reason = triageReason(err)
		return slice
	}
	// Fields mirror inboxItemDTO (internal/api/inbox.go): the route serves a
	// derived title, falling back to the raw subtitle for a capture the
	// classifier has not titled yet. A capture with neither is still worth
	// counting — the id alone tells the caller something is queued.
	var rows []struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Subtitle string `json:"subtitle"`
	}
	if err := decodeInto(raw, "GET /api/inbox/pending", &rows); err != nil {
		slice.Reason = triageReason(err)
		return slice
	}
	slice.Available = true
	slice.Count = len(rows)
	for _, row := range rows {
		if len(slice.Items) == triageItemLimit {
			break
		}
		title := strings.TrimSpace(row.Title)
		if title == "" {
			title = strings.TrimSpace(row.Subtitle)
		}
		slice.Items = append(slice.Items, triageItem{ID: row.ID, Title: firstLine(title)})
	}
	return slice
}

func triageIngest(ctx context.Context, c *apiClient) triageSlice {
	slice := triageSlice{Command: "kernl ingest queue list", Items: []triageItem{}}
	raw, err := c.get(ctx, "/api/ingest/queue")
	if err != nil {
		slice.Reason = triageReason(err)
		return slice
	}
	// Fields mirror nodes.IngestReview — the tags are the contract in force,
	// and encoding/json's case-insensitive matching is exactly what let an
	// earlier spelling drift here unnoticed.
	var rows []struct {
		ID     string `json:"id"`
		Title  string `json:"title"`
		Action string `json:"action"`
	}
	if err := decodeInto(raw, "GET /api/ingest/queue", &rows); err != nil {
		slice.Reason = triageReason(err)
		return slice
	}
	slice.Available = true
	slice.Count = len(rows)
	for _, row := range rows {
		if len(slice.Items) == triageItemLimit {
			break
		}
		slice.Items = append(slice.Items, triageItem{ID: row.ID, Title: row.Title, State: row.Action})
	}
	return slice
}

func triageTasks(ctx context.Context, c *apiClient) triageSlice {
	slice := triageSlice{Command: "kernl task list", Items: []triageItem{}}
	raw, err := c.get(ctx, "/api/tasks")
	if err != nil {
		slice.Reason = triageReason(err)
		return slice
	}
	var rows []struct {
		ID     string `json:"id"`
		Title  string `json:"title"`
		Status string `json:"status"`
	}
	if err := decodeInto(raw, "GET /api/tasks", &rows); err != nil {
		slice.Reason = triageReason(err)
		return slice
	}
	slice.Available = true
	for _, row := range rows {
		if strings.EqualFold(row.Status, "done") || strings.EqualFold(row.Status, "closed") {
			continue
		}
		slice.Count++
		if len(slice.Items) == triageItemLimit {
			continue
		}
		slice.Items = append(slice.Items, triageItem{ID: row.ID, Title: row.Title, State: row.Status})
	}
	return slice
}

// triageApprovals is the reason this command waited for the approvals route to
// be settled. The endpoint answers 501 today, and a triage that rendered that
// as "0 pending" would tell the human the one thing they must never be told
// wrongly: that no judgment gate is waiting on them. Unavailable-with-a-reason
// is the honest answer, and it is why the slice carries Reason at all.
func triageApprovals(ctx context.Context, c *apiClient) triageSlice {
	slice := triageSlice{Command: "kernl approval list", Items: []triageItem{}}
	raw, err := c.get(ctx, "/api/approvals")
	if err != nil {
		slice.Reason = triageReason(err)
		return slice
	}
	var rows []struct {
		ID      string `json:"id"`
		Summary string `json:"summary"`
		State   string `json:"state"`
	}
	if err := decodeInto(raw, "GET /api/approvals", &rows); err != nil {
		slice.Reason = triageReason(err)
		return slice
	}
	slice.Available = true
	slice.Count = len(rows)
	for _, row := range rows {
		if len(slice.Items) == triageItemLimit {
			break
		}
		slice.Items = append(slice.Items, triageItem{ID: row.ID, Title: row.Summary, State: row.State})
	}
	return slice
}

// triageBeads splits the tracker into what is moving and what is free to pick
// up, because they answer different questions: running tells you whether to
// wait, ready tells you what to start.
//
// The split is derived from the state name rather than a tracker query: a
// `ready_for_*` state with nobody assigned is claimable, anything else that is
// not closed is in flight. GET /api/beads takes no filters, so this is the
// classification the CLI can make honestly from what the route returns.
func triageBeads(ctx context.Context, c *apiClient) (running, ready triageSlice) {
	running = triageSlice{Command: "kernl epic list", Items: []triageItem{}}
	ready = triageSlice{Command: "kernl bead list", Items: []triageItem{}}

	raw, err := c.get(ctx, "/api/beads")
	if err != nil {
		running.Reason, ready.Reason = triageReason(err), triageReason(err)
		return running, ready
	}
	var beads []beadView
	if err := decodeInto(raw, "GET /api/beads", &beads); err != nil {
		running.Reason, ready.Reason = triageReason(err), triageReason(err)
		return running, ready
	}
	running.Available, ready.Available = true, true

	// Highest priority first so a truncated slice shows the work that matters.
	sort.SliceStable(beads, func(i, j int) bool { return beads[i].Priority > beads[j].Priority })
	for _, b := range beads {
		if isClosedBeadState(b.State) {
			continue
		}
		item := triageItem{ID: b.ID, Title: b.Title, State: b.State}
		if strings.HasPrefix(b.State, "ready_for_") && b.Assignee == "" {
			ready.Count++
			if len(ready.Items) < triageItemLimit {
				ready.Items = append(ready.Items, item)
			}
			continue
		}
		running.Count++
		if len(running.Items) < triageItemLimit {
			running.Items = append(running.Items, item)
		}
	}
	return running, ready
}

func isClosedBeadState(state string) bool {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "closed", "done", "cancelled", "canceled":
		return true
	}
	return false
}

func triageHealthCheck(ctx context.Context, c *apiClient) triageHealth {
	raw, err := c.get(ctx, "/api/health")
	if err != nil {
		return triageHealth{Reason: triageReason(err)}
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := decodeInto(raw, "GET /api/health", &body); err != nil {
		return triageHealth{Reason: triageReason(err)}
	}
	return triageHealth{Available: true, Status: body.Status}
}

// triageReason compresses an error into one line a human can act on.
//
// The cut keeps the "Fix:" clause when there is one. Truncating a list is good
// manners; truncating an error removes the reason it was printed, and the half
// that gets cut is always the actionable half — the remedy comes last. So the
// remedy is preferred over the diagnosis when both will not fit, and the full
// text stays one command away, which is what the slice's Command field is for.
func triageReason(err error) string {
	msg := strings.TrimPrefix(err.Error(), "KERNL DISPATCH FAILURE: ")
	msg = strings.Join(strings.Fields(msg), " ")

	const budget = 200
	if len(msg) <= budget {
		return msg
	}
	if fix := strings.Index(msg, "Fix:"); fix >= 0 {
		remedy := msg[fix:]
		if len(remedy) >= budget {
			return truncateOnWord(remedy, budget)
		}
		// Keep the whole remedy, spending what is left on the diagnosis.
		return truncateOnWord(msg[:fix], budget-len(remedy)) + remedy
	}
	return truncateOnWord(msg, budget)
}

// truncateOnWord cuts at a word boundary so a flag or path is never left as a
// fragment the reader cannot act on ("--config <path-to-ker…").
func truncateOnWord(s string, budget int) string {
	if len(s) <= budget {
		return s
	}
	cut := s[:budget]
	if i := strings.LastIndexByte(cut, ' '); i > budget/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ") + "… "
}

func printTriage(w io.Writer, r triageReport) error {
	var b strings.Builder
	writeTriageSlice(&b, "captures to process", r.Captures)
	writeTriageSlice(&b, "ingest reviews", r.Ingest)
	writeTriageSlice(&b, "beads in flight", r.Running)
	writeTriageSlice(&b, "beads ready to start", r.Ready)
	writeTriageSlice(&b, "open tasks", r.Tasks)
	writeTriageSlice(&b, "approvals waiting", r.Approvals)

	if r.Health.Available {
		fmt.Fprintf(&b, "\nserver: %s\n", r.Health.Status)
	} else {
		fmt.Fprintf(&b, "\nserver: unavailable — %s\n", r.Health.Reason)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func writeTriageSlice(b *strings.Builder, label string, s triageSlice) {
	if !s.Available {
		fmt.Fprintf(b, "%s: unavailable — %s\n\n", label, s.Reason)
		return
	}
	if s.Count == 0 {
		fmt.Fprintf(b, "%s: none\n\n", label)
		return
	}
	fmt.Fprintf(b, "%s: %d\n", label, s.Count)
	for _, item := range s.Items {
		if item.State != "" {
			fmt.Fprintf(b, "  %-38s [%s] %s\n", item.ID, item.State, item.Title)
			continue
		}
		fmt.Fprintf(b, "  %-38s %s\n", item.ID, item.Title)
	}
	if s.Count > len(s.Items) {
		fmt.Fprintf(b, "  … %d more\n", s.Count-len(s.Items))
	}
	fmt.Fprintf(b, "  → %s\n\n", s.Command)
}
