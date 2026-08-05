package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/app"
)

// qualityColumnNote explains what this report still cannot say about quality.
// Fix-up attribution now exists at the attempt level - the rework table below
// charges every attempt that followed a deliberate rejection to the agent
// that redid it - so the note no longer disclaims that half. Revert detection
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

var orchestratorSubcommands = []string{"stats"}

var orchestratorCommand = commandMeta{
	Name:    "orchestrator",
	Summary: "Compare agents by their recorded stage-attempt history",
	Usage:   "kernl orchestrator stats [--stage <stage>] [--since <duration>] [--json]",
	Details: `Reads every epic's stage-attempt ledger (written by the orchestrator as it
dispatches beads) and reports, per agent: how many attempts it made, its
first-pass gate rate, its review-rejection rate, and its median duration
and diff size. This is the answer to "which agent should implement" from
recorded history instead of a hunch.

A second table reports rework: the attempts that exist only because a
reviewer rejected the previous one, what share of an agent's work they
were, how often the redo passed, and what it cost. Rework is charged to
whoever redid the work, never to the reviewer that rejected it - that
reviewer's own behaviour is the review-rejection rate above.

Every rate and median is nil (printed as "-") when the ledger never
measured it for that group - never a fabricated zero. The row alongside it
says how many observations the number covers.

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
	},
}

func runOrchestrator(w io.Writer, args []string) error {
	// requireSub narrows to orchestratorSubcommands, whose only member is
	// "stats" - there is nothing else to branch on yet.
	_, rest, err := requireSub("orchestrator", args, orchestratorSubcommands)
	if err != nil {
		return err
	}
	return runOrchestratorStats(w, rest)
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
	AgentID                     string   `json:"agentId"`
	Attempts                    int      `json:"attempts"`
	FirstPassGateRate           *float64 `json:"firstPassGateRate"`
	FirstPassGateObservations   int      `json:"firstPassGateObservations"`
	ReviewRejectionRate         *float64 `json:"reviewRejectionRate"`
	ReviewRejectionObservations int      `json:"reviewRejectionObservations"`
	MedianDurationMs            *float64 `json:"medianDurationMs"`
	DurationObservations        int      `json:"durationObservations"`
	MedianDiffLines             *float64 `json:"medianDiffLines"`
	DiffObservations            int      `json:"diffObservations"`
	ReworkAttempts              int      `json:"reworkAttempts"`
	ReworkRate                  *float64 `json:"reworkRate"`
	ReworkGatePassRate          *float64 `json:"reworkGatePassRate"`
	ReworkGateObservations      int      `json:"reworkGateObservations"`
	ReworkDurationMs            int64    `json:"reworkDurationMs"`
	ReworkOutputTokens          *int64   `json:"reworkOutputTokens"`
	ReworkTokenObservations     int      `json:"reworkTokenObservations"`
	ReworkCostUSD               *float64 `json:"reworkCostUSD"`
	ReworkCostObservations      int      `json:"reworkCostObservations"`
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
	}
	return nil
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
		fmt.Fprintln(w, "rework: no attempt in this window followed a review rejection")
		return nil
	}

	fmt.Fprintln(w, "rework - attempts that exist because a reviewer rejected the previous one,")
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
