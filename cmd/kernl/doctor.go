package main

import (
	"fmt"
	"runtime"

	"github.com/gabrielassisxyz/kernl/internal/preflight"
)

func runDoctor(configPath string) error {
	report := preflight.Run(preflight.Deps{
		LookPath:   preflight.LookPath,
		ConfigPath: configPath,
		GoVersion:  runtime.Version(),
	})

	printReport(report)

	if !report.AllOK() {
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
		if c.OK {
			fmt.Printf("✓ %s\n", c.Name)
		} else {
			fmt.Printf("✗ %s — %s\n", c.Name, c.Detail)
			if c.Fix != "" {
				fmt.Printf("  Fix: %s\n", c.Fix)
			}
		}
	}
}
