package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/app"
)

// qualityColumnNote is printed under the human table and carried as a field
// in --json, so a reader of either surface sees why the report has no
// revert/fix-up column: the plan calls that "the rate at which the work it
// produced was later reverted or fixed up", sourced from Phase 5 (revert
// detection) and Phase 6 (fix-up attribution) - neither of which exists yet.
// A column of dashes standing in for a measurement that was never taken
// would read as "measured, and it's zero everywhere," which is worse than
// leaving the column out.
const qualityColumnNote = "revert/fix-up rate is not reported here - it needs the revert/fix-up tracking that later phases (revert detection, fix-up attribution) are meant to supply, and neither exists yet. The numbers below are speed and gate/review outcomes only, not a quality verdict."

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

	stats, err := app.AggregateAttemptStats(stateDir, stageFlag, cutoff)
	if err != nil {
		return err
	}

	if asJSON {
		return json.NewEncoder(w).Encode(newOrchestratorStatsOutput(stageFlag, sinceFlag, stats))
	}
	return printOrchestratorStatsTable(w, stats)
}

// reportNoAttemptsRecorded is the "nothing to report yet" answer, not a
// failure: a fresh install, or one where no bead has ever run, must not make
// this command look broken.
func reportNoAttemptsRecorded(w io.Writer, asJSON bool, stage, since string) error {
	const message = "no attempts recorded yet - nothing has run through the orchestrator's stage-attempt ledger"
	if asJSON {
		out := newOrchestratorStatsOutput(stage, since, nil)
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

func parseDayShorthand(raw string) (time.Duration, bool) {
	if !strings.HasSuffix(raw, "d") {
		return 0, false
	}
	days, err := strconv.ParseFloat(strings.TrimSuffix(raw, "d"), 64)
	if err != nil {
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
	Note    string                   `json:"qualityColumnNote"`
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
}

func newOrchestratorStatsOutput(stage, since string, stats []app.AgentAttemptStats) orchestratorStatsOutput {
	out := orchestratorStatsOutput{
		Stage:  stage,
		Since:  since,
		Agents: make([]orchestratorStatsAgent, 0, len(stats)),
		Note:   qualityColumnNote,
	}
	for _, s := range stats {
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
		})
	}
	return out
}

func printOrchestratorStatsTable(w io.Writer, stats []app.AgentAttemptStats) error {
	if len(stats) == 0 {
		fmt.Fprintln(w, "no attempts matched --stage/--since")
	} else {
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "AGENT\tATTEMPTS\tFIRST-PASS GATE\tREVIEW REJECT\tMEDIAN DURATION\tMEDIAN DIFF LINES")
		for _, s := range stats {
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
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, qualityColumnNote)
	return nil
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
