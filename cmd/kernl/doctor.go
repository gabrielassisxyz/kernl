package main

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"

	"github.com/gabrielassisxyz/kernl/internal/preflight"
)

func runDoctor(configPath string, args []string) error {
	var asJSON bool
	for _, arg := range args {
		switch arg {
		case "--json":
			asJSON = true
		default:
			return usagef("KERNL DISPATCH FAILURE: unknown doctor flag %q%s — valid: --json",
				arg, didYouMean(arg, []string{"--json"}))
		}
	}

	report := preflight.Run(preflight.Deps{
		LookPath:     preflight.LookPath,
		ConfigPath:   configPath,
		GoVersion:    runtime.Version(),
		Orchestrator: true,
	})

	if asJSON {
		if err := json.NewEncoder(os.Stdout).Encode(newDoctorReport(report)); err != nil {
			return err
		}
	} else {
		printReport(report)
	}

	if report.RequiredFailed() {
		return fmt.Errorf("KERNL DISPATCH FAILURE: one or more preflight checks failed — run: kernl doctor for details")
	}
	return nil
}

// doctorReport is the machine contract for `kernl doctor --json` (DIAGNOSE
// shape): overall verdict, per-check detail, and the single next action an
// agent should take when something is broken.
type doctorReport struct {
	OK                bool          `json:"ok"`
	Checks            []doctorCheck `json:"checks"`
	RecommendedAction string        `json:"recommendedAction"`
}

type doctorCheck struct {
	Name     string `json:"name"`
	OK       bool   `json:"ok"`
	Advisory bool   `json:"advisory"`
	Detail   string `json:"detail,omitempty"`
	Fix      string `json:"fix,omitempty"`
}

func newDoctorReport(r *preflight.Report) doctorReport {
	out := doctorReport{OK: !r.RequiredFailed(), Checks: make([]doctorCheck, 0)}
	for _, c := range r.Checks() {
		out.Checks = append(out.Checks, doctorCheck{
			Name: c.Name, OK: c.OK, Advisory: c.Advisory, Detail: c.Detail, Fix: c.Fix,
		})
		if out.RecommendedAction == "" && !c.OK && !c.Advisory && c.Fix != "" {
			out.RecommendedAction = c.Fix
		}
	}
	if out.OK {
		out.RecommendedAction = "all required checks passed"
	}
	return out
}

func printReport(r *preflight.Report) {
	for _, check := range []string{"bd", "opencode", "go", "config"} {
		c := r.Check(check)
		if c == nil {
			continue
		}
		switch {
		case c.OK:
			fmt.Printf("✓ %s\n", c.Name)
		case c.Advisory:
			fmt.Printf("⚠ %s — %s (advisory)\n", c.Name, c.Detail)
		default:
			fmt.Printf("✗ %s — %s\n", c.Name, c.Detail)
		}
		if !c.OK && c.Fix != "" {
			fmt.Printf("  Fix: %s\n", c.Fix)
		}
	}
}
