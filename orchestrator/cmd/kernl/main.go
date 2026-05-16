package main

import (
	"fmt"
	"os"
)

var (
	doctorFn func(configPath string) error                 = runDoctor
	serveFn  func(configPath string) error                 = runServe
	epicFn   func(configPath string, args []string) error  = runEpic
	beadFn   func(configPath string, args []string) error  = runBead
	sweepFn  func(configPath string, args []string) error  = runSweep
	helpFn   func() error                                  = printHelp
)

func main() {
	if err := Dispatch(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseConfigPath(args []string) (configPath string, rest []string) {
	for i := 0; i < len(args); i++ {
		if args[i] == "--config" || args[i] == "-c" {
			if i+1 < len(args) {
				configPath = args[i+1]
				rest = append(rest, args[:i]...)
				rest = append(rest, args[i+2:]...)
				return
			}
		}
	}
	return "", args
}

func Dispatch(args []string) error {
	if len(args) == 0 {
		return helpFn()
	}

	configPath, args := parseConfigPath(args)
	if configPath == "" {
		configPath = "kernl.yaml"
	}

	switch args[0] {
	case "serve":
		return serveFn(configPath)
	case "doctor":
		return doctorFn(configPath)
	case "epic":
		return epicFn(configPath, args[1:])
	case "bead":
		return beadFn(configPath, args[1:])
	case "sweep":
		return sweepFn(configPath, args[1:])
	case "--help", "-h", "help":
		return helpFn()
	default:
		return fmt.Errorf("KERNL DISPATCH FAILURE: unknown subcommand %q. Run: kernl --help", args[0])
	}
}

func printHelp() error {
	fmt.Println(`kernl — multi-agent orchestration runner

Usage:
  kernl [--config <path>] <subcommand> [args...]

Subcommands:
  serve        Start the HTTP API server
  doctor       Run system checks (env, binaries, config)
  epic         Manage epics (bead graphs)
  bead         Manage individual beads
  sweep        Close epics whose PRs are merged in master

Flags:
  --config, -c Path to kernl.yaml (default: kernl.yaml)
  --help, -h   Show this help

Run 'kernl <subcommand> --help' for subcommand-specific help.`)
	return nil
}
