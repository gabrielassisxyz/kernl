package main

import (
	"fmt"
	"runtime"

	"github.com/gabrielassisxyz/kernl/internal/preflight"
)

func runDoctor(configPath string) error {
	report := preflight.Run(preflight.Deps{
		LookPath:     preflight.LookPath,
		ConfigPath:   configPath,
		GoVersion:    runtime.Version(),
		Orchestrator: true,
	})

	printReport(report)

	if report.RequiredFailed() {
		return fmt.Errorf("KERNL DISPATCH FAILURE: one or more preflight checks failed")
	}
	return nil
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
