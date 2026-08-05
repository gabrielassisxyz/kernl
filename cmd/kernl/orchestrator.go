package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/app"
)

// qualityColumnNote explains what this report still cannot say about quality.
// Fix-up attribution now exists at the attempt level - the rework table below
// charges an attempt that redoes rejected work to the agent that redid it -
// so the note no longer disclaims that half. Revert detection
// does not exist: nothing here notices work that shipped and was reverted
// afterwards, which is the one outcome that would contradict a clean
// first-pass rate. A column of dashes standing in for a measurement that was
// never taken would read as "measured, and it's zero everywhere," which is
// worse than leaving the column out.
//
// The epic-level fix-up count stays out for a different reason: fix-up beads
// are labelled in the tracker, not recorded in this ledger, so counting them
// here would mean this command reading a second, unrelated source.
//
// The wording names what it qualifies ("this report") instead of leaning on
// print order, because it is read from two positions that disagree about
// where "above" and "below" point: printed ahead of the human table, and
// carried as a JSON field that sits beside the data rather than around it.
const qualityColumnNote = "revert rate is not part of this report - nothing here detects work that shipped and was reverted later. This report covers speed, gate/review outcomes, and the rework a review rejection caused; the epic-level fix-up bead count lives in the tracker, not in this ledger."

var orchestratorSubcommands = []string{"stats", "backfill-rework"}

var orchestratorCommand = commandMeta{
	Name:    "orchestrator",
	Summary: "Compare agents by their recorded stage-attempt history",
	Usage:   "kernl orchestrator stats [--stage <stage>] [--since <duration>] [--json]",
	Details: `Reads every epic's stage-attempt ledger (written by the orchestrator as it
dispatches beads) and reports, per agent: how many attempts it made, its
first-pass gate rate, its review-rejection rate, and its median duration
and diff size. This is the answer to "which agent should implement" from
recorded history instead of a hunch.

A second table reports rework: the attempts that redo work a reviewer
rejected, what share of an agent's work they were, how often the redo
passed, and what it cost. Rework is charged to whoever redid the work,
never to the reviewer that rejected it - that reviewer's own behaviour is
the review-rejection rate above. An attempt that merely follows a rejection
without redoing anything (a review re-running on unchanged work, a stage
running for the first time) is not counted.

A third table reports stopped beads: the ones whose last attempt failed its
gate, so the run got no further and a person has to look. That is the cost
that leaves a day rather than a quota, and it is broken down by what
actually failed - a bead rejected twice and one that never produced a commit
are different problems with different fixes. It is a snapshot: a bead that
runs again stops counting, and one repaired by hand outside kernl leaves no
row, so it undercounts, most for the oldest runs.

Every rate and median is nil (printed as "-") when the ledger never
measured it for that group - never a fabricated zero. The row alongside it
says how many observations the number covers, which is what makes a rate
over sixteen beads legible next to one over three.

This command only reports. It never edits kernl.yaml or reweights a pool -
that stays a human decision made with kernl settings.

Run 'kernl orchestrator stats --help' for its flags.`,
	Subs: []commandMeta{
		{
			Name:    "stats",
			Summary: "Per-agent attempt counts, gate/review rates and medians",
			Usage:   "kernl orchestrator stats [--stage <stage>] [--since <duration>] [--json]",
			Details: `--since accepts a Go duration (720h) or the day shorthand (30d); any other
value is refused rather than silently treated as "all time".

When nothing has ever run through the ledger, this prints that plainly and
exits 0 - it is not an error.

{{flags}}`,
			Flags: []commandFlag{
				{Name: "--stage", Value: "<stage>", Description: "Only rows for this workflow stage (e.g. implementation)"},
				{Name: "--since", Value: "<duration>", Description: "Only rows started at or after now minus this - 720h or 30d"},
				{Name: "--json", Description: "Emit the same numbers as one camelCase document, null where unmeasured"},
			},
		},
		{
			Name:    "backfill-rework",
			Summary: "Recompute causedBy across every recorded attempt",
			Usage:   "kernl orchestrator backfill-rework [--dry-run] [--yes] [--json]",
			Details: `causedBy - the review artifact whose rejection sent a bead back - is
derived once, when the attempt is recorded. Rows written while that
derivation matched the wrong gate reason kept the wrong answer, and no
reader can tell them apart from rows written since. The rework numbers in
'orchestrator stats' therefore describe a mixture of two rules until this
has run.

Reports what would change and writes nothing unless you pass --yes. A
rewrite keeps every unchanged row's exact bytes, so the resulting diff is
the rows it decided about and nothing else, and it holds the same lock the
orchestrator takes to append - a run in progress waits rather than
interleaving.

{{flags}}`,
			Flags: []commandFlag{
				{Name: "--dry-run", Description: "Report what would change and write nothing (the default)"},
				{Name: "--yes", Description: "Actually rewrite the ledgers"},
				{Name: "--json", Description: "Emit {\"applied\",\"ledgersScanned\",\"rowsScanned\",\"ledgers\":[...]}"},
			},
		},
	},
}

func runOrchestrator(w io.Writer, args []string) error {
	sub, rest, err := requireSub("orchestrator", args, orchestratorSubcommands)
	if err != nil {
		return err
	}
	if sub == "backfill-rework" {
		return runOrchestratorBackfillRework(w, rest)
	}
	return runOrchestratorStats(w, rest)
}

// orchestratorBackfillOutput is `kernl orchestrator backfill-rework --json`.
// Ledgers carries only the files with something to change, the same list the
// human output prints, so the two surfaces cannot disagree about the work.
type orchestratorBackfillOutput struct {
	Applied        bool                       `json:"applied"`
	LedgersScanned int                        `json:"ledgersScanned"`
	RowsScanned    int                        `json:"rowsScanned"`
	Ledgers        []orchestratorBackfillFile `json:"ledgers"`
}

type orchestratorBackfillFile struct {
	Path            string `json:"path"`
	Marked          int    `json:"marked"`
	Unmarked        int    `json:"unmarked"`
	DanglingDropped bool   `json:"danglingDropped"`
}

func runOrchestratorBackfillRework(w io.Writer, args []string) error {
	dryRun, rest := parseBoolFlag(args, "--dry-run")
	confirmed, rest := parseBoolFlag(rest, "--yes")
	asJSON, rest := parseBoolFlag(rest, "--json")
	if err := rejectUnknownFlags("orchestrator backfill-rework", rest); err != nil {
		return err
	}
	if len(rest) > 0 {
		return usagef("KERNL DISPATCH FAILURE: orchestrator backfill-rework takes no positional arguments, got %q - run: kernl orchestrator backfill-rework --help", rest[0])
	}
	// Passing both is a contradiction about whether to write, and picking a
	// winner would mean guessing which one the operator meant on a command
	// that rewrites the ledger.
	if dryRun && confirmed {
		return usagef("KERNL DISPATCH FAILURE: orchestrator backfill-rework was given both --dry-run and --yes, which ask for opposite things - run: kernl orchestrator backfill-rework --yes to rewrite, or with neither flag to see what would change")
	}

	stateDir, err := app.DefaultStateDir()
	if err != nil {
		return err
	}
	result, err := app.BackfillCausedBy(stateDir, confirmed)
	if err != nil {
		return err
	}

	if asJSON {
		out := orchestratorBackfillOutput{
			Applied:        result.Applied,
			LedgersScanned: result.LedgersScanned,
			RowsScanned:    result.RowsScanned,
			Ledgers:        make([]orchestratorBackfillFile, 0, len(result.Ledgers)),
		}
		for _, l := range result.Ledgers {
			out.Ledgers = append(out.Ledgers, orchestratorBackfillFile{
				Path: l.Path, Marked: l.Marked, Unmarked: l.Unmarked, DanglingDropped: l.DanglingDropped,
			})
		}
		return json.NewEncoder(w).Encode(out)
	}

	if len(result.Ledgers) == 0 {
		fmt.Fprintf(w, "every recorded attempt already agrees with the current rule (%d row(s) across %d ledger(s)) - nothing to do\n", result.RowsScanned, result.LedgersScanned)
		return nil
	}

	marked, unmarked := 0, 0
	for _, l := range result.Ledgers {
		marked += l.Marked
		unmarked += l.Unmarked
		dangling := ""
		if l.DanglingDropped {
			dangling = ", and an incomplete trailing row to trim"
		}
		fmt.Fprintf(w, "%s: %d row(s) to mark as rework, %d to clear%s\n", l.Path, l.Marked, l.Unmarked, dangling)
	}
	fmt.Fprintf(w, "\n%d row(s) across %d ledger(s): %d gained a rejection they always had, %d lost one they never had\n",
		result.RowsScanned, result.LedgersScanned, marked, unmarked)
	if !result.Applied {
		fmt.Fprintln(w, "nothing was written - re-run with --yes to apply")
	}
	return nil
}

func runOrchestratorStats(w io.Writer, args []string) error {
	stageFlag, _, rest, err := takeFlag("orchestrator stats", args, "--stage")
	if err != nil {
		return err
	}
	sinceFlag, hasSince, rest, err := takeFlag("orchestrator stats", rest, "--since")
	if err != nil {
		return err
	}
	asJSON, rest := parseBoolFlag(rest, "--json")
	if err := rejectUnknownFlags("orchestrator stats", rest); err != nil {
		return err
	}
	if len(rest) > 0 {
		return usagef("KERNL DISPATCH FAILURE: orchestrator stats takes no positional arguments, got %q - run: kernl orchestrator stats --help", rest[0])
	}

	var cutoff time.Time
	if hasSince {
		d, err := parseSinceDuration(sinceFlag)
		if err != nil {
			return err
		}
		cutoff = time.Now().Add(-d)
	}

	stateDir, err := app.DefaultStateDir()
	if err != nil {
		return err
	}

	paths, err := app.AttemptLedgerPaths(stateDir)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return reportNoAttemptsRecorded(w, asJSON, stageFlag, sinceFlag)
	}

	result, err := app.AggregateAttemptStats(stateDir, stageFlag, cutoff)
	if err != nil {
		return err
	}

	if asJSON {
		return json.NewEncoder(w).Encode(newOrchestratorStatsOutput(stageFlag, sinceFlag, result))
	}
	return printOrchestratorStatsTable(w, result)
}

// reportNoAttemptsRecorded is the "nothing to report yet" answer, not a
// failure: a fresh install, or one where no bead has ever run, must not make
// this command look broken.
func reportNoAttemptsRecorded(w io.Writer, asJSON bool, stage, since string) error {
	const message = "no attempts recorded yet - nothing has run through the orchestrator's stage-attempt ledger"
	if asJSON {
		out := newOrchestratorStatsOutput(stage, since, app.AttemptStatsResult{})
		out.Message = message
		return json.NewEncoder(w).Encode(out)
	}
	fmt.Fprintln(w, message)
	return nil
}

// parseSinceDuration accepts a Go duration string (720h) or the day
// shorthand the plan's own example uses (30d). Anything else is refused
// loudly: silently falling back to "all time" would answer a different
// question than the one asked.
func parseSinceDuration(raw string) (time.Duration, error) {
	if d, ok := parseDayShorthand(raw); ok {
		return d, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, usagef("KERNL DISPATCH FAILURE: --since %q is not a valid duration - Fix: use a Go duration like 720h, or the day shorthand like 30d", raw)
	}
	return d, nil
}

// parseDayShorthand accepts only a finite, non-negative day count.
// strconv.ParseFloat itself accepts "NaN" and "Inf"/"-Inf" as valid floats,
// and a bare negative ("-30") parses cleanly too - none of those are a
// sensible lookback window, and time.Now().Add(-NaN) or a negative cutoff
// (which means "started after N days from now") would run successfully
// against a cutoff that matches nothing or everything, silently. Returning
// false here for any of those falls through to time.ParseDuration, which
// rejects "d" as a unit and produces the same loud usage error every other
// malformed --since value gets.
func parseDayShorthand(raw string) (time.Duration, bool) {
	if !strings.HasSuffix(raw, "d") {
		return 0, false
	}
	days, err := strconv.ParseFloat(strings.TrimSuffix(raw, "d"), 64)
	if err != nil || math.IsNaN(days) || math.IsInf(days, 0) || days < 0 {
		return 0, false
	}
	return time.Duration(days * float64(24*time.Hour)), true
}

// orchestratorStatsOutput is the machine contract for
// `kernl orchestrator stats --json`: one camelCase document, null wherever a
// number was never measured (AGENTS.md §3: REST/CLI JSON is camelCase).
type orchestratorStatsOutput struct {
	Stage   string                   `json:"stage,omitempty"`
	Since   string                   `json:"since,omitempty"`
	Message string                   `json:"message,omitempty"`
	Agents  []orchestratorStatsAgent `json:"agents"`
	// DanglingLedgers names ledger files whose most recent row was dropped
	// because it was still mid-write when this command read it (see
	// app.AttemptStatsResult) - the affected attempt is missing from every
	// count above, not merely unmeasured, so a caller must not read a clean
	// document as a complete one without checking this field.
	DanglingLedgers []string `json:"danglingLedgers,omitempty"`
	Note            string   `json:"qualityColumnNote"`
}

type orchestratorStatsAgent struct {
	AgentID                     string         `json:"agentId"`
	Attempts                    int            `json:"attempts"`
	FirstPassGateRate           *float64       `json:"firstPassGateRate"`
	FirstPassGateObservations   int            `json:"firstPassGateObservations"`
	ReviewRejectionRate         *float64       `json:"reviewRejectionRate"`
	ReviewRejectionObservations int            `json:"reviewRejectionObservations"`
	MedianDurationMs            *float64       `json:"medianDurationMs"`
	DurationObservations        int            `json:"durationObservations"`
	MedianDiffLines             *float64       `json:"medianDiffLines"`
	DiffObservations            int            `json:"diffObservations"`
	ReworkAttempts              int            `json:"reworkAttempts"`
	ReworkRate                  *float64       `json:"reworkRate"`
	ReworkGatePassRate          *float64       `json:"reworkGatePassRate"`
	ReworkGateObservations      int            `json:"reworkGateObservations"`
	ReworkDurationMs            int64          `json:"reworkDurationMs"`
	ReworkOutputTokens          *int64         `json:"reworkOutputTokens"`
	ReworkTokenObservations     int            `json:"reworkTokenObservations"`
	ReworkCostUSD               *float64       `json:"reworkCostUSD"`
	ReworkCostObservations      int            `json:"reworkCostObservations"`
	BeadsStopped                int            `json:"beadsStopped"`
	BeadsImplemented            int            `json:"beadsImplemented"`
	BeadsStoppedRate            *float64       `json:"beadsStoppedRate"`
	StoppedByReason             map[string]int `json:"stoppedByReason"`
}

func newOrchestratorStatsOutput(stage, since string, result app.AttemptStatsResult) orchestratorStatsOutput {
	out := orchestratorStatsOutput{
		Stage:           stage,
		Since:           since,
		Agents:          make([]orchestratorStatsAgent, 0, len(result.Agents)),
		DanglingLedgers: result.DanglingLedgers,
		Note:            qualityColumnNote,
	}
	for _, s := range result.Agents {
		out.Agents = append(out.Agents, orchestratorStatsAgent{
			AgentID:                     s.AgentID,
			Attempts:                    s.Attempts,
			FirstPassGateRate:           s.FirstPassGateRate,
			FirstPassGateObservations:   s.FirstPassGateObservations,
			ReviewRejectionRate:         s.ReviewRejectionRate,
			ReviewRejectionObservations: s.ReviewRejectionObservations,
			MedianDurationMs:            s.MedianDurationMs,
			DurationObservations:        s.DurationObservations,
			MedianDiffLines:             s.MedianDiffLines,
			DiffObservations:            s.DiffObservations,
			ReworkAttempts:              s.ReworkAttempts,
			ReworkRate:                  s.ReworkRate,
			ReworkGatePassRate:          s.ReworkGatePassRate,
			ReworkGateObservations:      s.ReworkGateObservations,
			ReworkDurationMs:            s.ReworkDurationMs,
			ReworkOutputTokens:          s.ReworkOutputTokens,
			ReworkTokenObservations:     s.ReworkTokenObservations,
			ReworkCostUSD:               s.ReworkCostUSD,
			ReworkCostObservations:      s.ReworkCostObservations,
			BeadsStopped:                s.BeadsStopped,
			BeadsImplemented:            s.BeadsImplemented,
			BeadsStoppedRate:            s.BeadsStoppedRate,
			StoppedByReason:             s.StoppedByReason,
		})
	}
	return out
}

func printOrchestratorStatsTable(w io.Writer, result app.AttemptStatsResult) error {
	for _, path := range result.DanglingLedgers {
		fmt.Fprintf(w, "warning: %s had an incomplete trailing row (an interrupted write) - that attempt was not counted\n", path)
	}
	if len(result.DanglingLedgers) > 0 {
		fmt.Fprintln(w)
	}

	fmt.Fprintln(w, qualityColumnNote)
	fmt.Fprintln(w)

	if len(result.Agents) == 0 {
		fmt.Fprintln(w, "no attempts matched --stage/--since")
	} else {
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "AGENT\tATTEMPTS\tFIRST-PASS GATE\tREVIEW REJECT\tMEDIAN DURATION\tMEDIAN DIFF LINES")
		for _, s := range result.Agents {
			fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s\t%s\n",
				s.AgentID, s.Attempts,
				formatRate(s.FirstPassGateRate, s.FirstPassGateObservations),
				formatRate(s.ReviewRejectionRate, s.ReviewRejectionObservations),
				formatMedianDuration(s.MedianDurationMs, s.DurationObservations),
				formatMedianDiff(s.MedianDiffLines, s.DiffObservations),
			)
		}
		if err := tw.Flush(); err != nil {
			return fmt.Errorf("KERNL DISPATCH FAILURE: writing orchestrator stats table: %w", err)
		}
		if err := printReworkTable(w, result); err != nil {
			return err
		}
		if err := printStoppedTable(w, result); err != nil {
			return err
		}
	}
	return nil
}

// printStoppedTable reports the beads that ended on a failed gate: the cost
// that is not paid in quota. Rework is an agent fixing its own work; a stop is
// a person's day, and an agent that is cheaper per token and stops more often
// looks better in every other column on this report while being worse to live
// with.
//
// The breakdown by reason is printed beside the rate rather than folded into
// it, because the two failures it separates are not the same problem: a bead
// rejected twice produced work that was judged and sent back, while one that
// never emitted a commit marker produced nothing on its first and only
// attempt. Reading the total alone turns the second into evidence about the
// first.
func printStoppedTable(w io.Writer, result app.AttemptStatsResult) error {
	total := 0
	for _, s := range result.Agents {
		total += s.BeadsStopped
	}

	fmt.Fprintln(w)
	if total == 0 {
		fmt.Fprintln(w, "stopped: every bead in this window ended on a gate that passed")
		return nil
	}

	fmt.Fprintln(w, "stopped - beads whose last attempt failed its gate, so the run got no")
	fmt.Fprintln(w, "further and a person has to look. Charged to whoever produced the work:")
	fmt.Fprintln(w)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "AGENT\tSTOPPED\tOF BEADS WORKED\tWHY")
	for _, s := range result.Agents {
		if s.BeadsStopped == 0 {
			continue
		}
		fmt.Fprintf(tw, "%s\t%d\t%s\t%s\n",
			s.AgentID, s.BeadsStopped,
			formatRate(s.BeadsStoppedRate, s.BeadsImplemented),
			formatStopReasons(s.StoppedByReason),
		)
	}
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: writing orchestrator stopped table: %w", err)
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "A stop is what the ledger can see now: a bead that runs again stops counting,")
	fmt.Fprintln(w, "and one repaired by hand outside kernl leaves no row at all, so this")
	fmt.Fprintln(w, "undercounts - most for the oldest runs. Read the rates as directional and")
	fmt.Fprintln(w, "look at the n= before quoting one.")
	return nil
}

// formatStopReasons renders the breakdown deterministically - map order would
// otherwise shuffle two reports of the same data and make them impossible to
// diff, the same reason configuredBinaries sorts its agent ids.
func formatStopReasons(byReason map[string]int) string {
	if len(byReason) == 0 {
		return "-"
	}
	reasons := make([]string, 0, len(byReason))
	for reason := range byReason {
		reasons = append(reasons, reason)
	}
	sort.Slice(reasons, func(i, j int) bool {
		if byReason[reasons[i]] != byReason[reasons[j]] {
			return byReason[reasons[i]] > byReason[reasons[j]]
		}
		return reasons[i] < reasons[j]
	})
	parts := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		parts = append(parts, fmt.Sprintf("%s %d", reason, byReason[reason]))
	}
	return strings.Join(parts, ", ")
}

// printReworkTable reports what redoing rejected work cost, in a table of its
// own rather than as six more columns on the one above - which would already
// be past the width of a terminal, and would put "how this agent normally
// performs" and "what its rejections cost" on one line where neither is
// readable.
//
// A window with no rework prints one line saying so instead of an empty
// table: "no rows" and "no rework happened" are the same picture otherwise,
// and only the second is good news.
func printReworkTable(w io.Writer, result app.AttemptStatsResult) error {
	total := 0
	for _, s := range result.Agents {
		total += s.ReworkAttempts
	}

	fmt.Fprintln(w)
	if total == 0 {
		fmt.Fprintln(w, "rework: no attempt in this window redid work a review rejected")
		return nil
	}

	fmt.Fprintln(w, "rework - attempts that redo work a reviewer rejected,")
	fmt.Fprintln(w, "charged to the agent that redid the work (not to the reviewer that rejected it):")
	fmt.Fprintln(w)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "AGENT\tREWORK\tSHARE OF ATTEMPTS\tRECOVERED\tTIME SPENT\tOUTPUT TOKENS\tCOST")
	for _, s := range result.Agents {
		if s.ReworkAttempts == 0 {
			continue
		}
		fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s\t%s\t%s\n",
			s.AgentID, s.ReworkAttempts,
			formatRate(s.ReworkRate, s.Attempts),
			formatRate(s.ReworkGatePassRate, s.ReworkGateObservations),
			(time.Duration(s.ReworkDurationMs) * time.Millisecond).Round(time.Second).String(),
			formatReworkTokens(s.ReworkOutputTokens, s.ReworkTokenObservations),
			formatReworkCost(s.ReworkCostUSD, s.ReworkCostObservations),
		)
	}
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: writing orchestrator rework table: %w", err)
	}
	return nil
}

// formatReworkTokens and formatReworkCost print "-" for a sum nobody
// measured. A dialect that reports no usage (codex reports neither model nor
// cost) leaves the sum nil, and printing 0 there would read as rework that
// was free rather than rework nobody priced.
func formatReworkTokens(tokens *int64, n int) string {
	if tokens == nil {
		return fmt.Sprintf("-  (n=%d)", n)
	}
	return fmt.Sprintf("%d  (n=%d)", *tokens, n)
}

func formatReworkCost(cost *float64, n int) string {
	if cost == nil {
		return fmt.Sprintf("-  (n=%d)", n)
	}
	return fmt.Sprintf("$%.4f  (n=%d)", *cost, n)
}

func formatRate(rate *float64, n int) string {
	if rate == nil {
		return fmt.Sprintf("-  (n=%d)", n)
	}
	return fmt.Sprintf("%.0f%%  (n=%d)", *rate*100, n)
}

func formatMedianDuration(ms *float64, n int) string {
	if ms == nil {
		return fmt.Sprintf("-  (n=%d)", n)
	}
	d := time.Duration(*ms * float64(time.Millisecond)).Round(time.Second)
	return fmt.Sprintf("%s  (n=%d)", d, n)
}

func formatMedianDiff(lines *float64, n int) string {
	if lines == nil {
		return fmt.Sprintf("-  (n=%d)", n)
	}
	return fmt.Sprintf("%.1f  (n=%d)", *lines, n)
}
