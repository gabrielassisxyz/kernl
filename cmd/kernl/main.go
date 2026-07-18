package main

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/logging"
)

// Build metadata, overridden at release time via goreleaser ldflags.
// Defaults apply to `go build`/`go run`.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

var (
	doctorFn   func(configPath string) error                        = runDoctor
	serveFn    func(configPath string, port int, noOrch bool) error = runServe
	epicFn     func(configPath string, args []string) error         = runEpic
	beadFn     func(configPath string, args []string) error         = runBead
	sweepFn    func(configPath string, args []string) error         = runSweep
	bookmarkFn func(configPath string, args []string) error         = runBookmark
	captureFn  func(configPath string, args []string) error         = runCapture
	planFn     func(configPath string, args []string) error         = runPlan
	helpFn     func() error                                         = printHelp
)

func main() {
	if err := Dispatch(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(exitCode(err))
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

func parsePort(args []string) (port int, rest []string) {
	for i := 0; i < len(args); i++ {
		if args[i] == "--port" || args[i] == "-p" {
			if i+1 < len(args) {
				p, err := strconv.Atoi(args[i+1])
				if err == nil {
					port = p
				}
				rest = append(rest, args[:i]...)
				rest = append(rest, args[i+2:]...)
				return
			}
		}
	}
	return 0, args
}

// parseBoolFlag strips a valueless flag (e.g. --no-orchestrator) from args and
// reports whether it was present.
func parseBoolFlag(args []string, name string) (present bool, rest []string) {
	for i := 0; i < len(args); i++ {
		if args[i] == name {
			rest = append(rest, args[:i]...)
			rest = append(rest, args[i+1:]...)
			return true, rest
		}
	}
	return false, args
}

func Dispatch(args []string) error {
	// Configure logging early so every subcommand gets the pretty terminal
	// handler (or JSON fallback) before anything else writes to slog.
	logLevel := os.Getenv("KERNL_LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}
	logging.Init(logLevel)

	if len(args) == 0 {
		return helpFn()
	}

	configPath, args := parseConfigPath(args)
	if configPath == "" {
		configPath = "kernl.yaml"
	}

	port, args := parsePort(args)
	noOrch, args := parseBoolFlag(args, "--no-orchestrator")

	// Help always wins: intercept --help/-h/help for any verb or sub-verb
	// BEFORE loading config or doing any work, so a help request can never
	// mutate state (capture used to store "--help" as a note, and sweep ran
	// a live tick).
	if topic, ok := helpTopic(args); ok {
		return printHelpFor(topic)
	}

	switch args[0] {
	case "serve":
		return serveFn(configPath, port, noOrch)
	case "doctor":
		return doctorFn(configPath)
	case "epic":
		return epicFn(configPath, args[1:])
	case "bead":
		return beadFn(configPath, args[1:])
	case "sweep":
		return sweepFn(configPath, args[1:])
	case "bookmark":
		return bookmarkFn(configPath, args[1:])
	case "capture":
		return captureFn(configPath, args[1:])
	case "plan":
		return planFn(configPath, args[1:])
	case "version", "--version", "-v":
		return printVersion()
	default:
		return usagef("KERNL DISPATCH FAILURE: unknown subcommand %q. Run: kernl --help", args[0])
	}
}

func printHelp() error {
	var b strings.Builder
	b.WriteString(`kernl — multi-agent orchestration runner

Usage:
  kernl [--config <path>] [--port <port>] <subcommand> [args...]

Subcommands:
`)
	for _, c := range commandTable {
		fmt.Fprintf(&b, "  %-12s %s\n", c.Name, c.Summary)
	}
	b.WriteString(`
Flags:
  --config, -c       Path to kernl.yaml (default: kernl.yaml)
  --port,  -p        Server port (default: from kernl.yaml, or 8080)
  --no-orchestrator  serve only the GUI/graph/notes; do not require bd
  --version, -v      Print version and build information
  --help,  -h        Show this help

Exit codes:
  0  success
  1  runtime/internal error (backend, config, network, agent run)
  2  usage error (unknown verb/flag, missing argument, bad value)

Run 'kernl <subcommand> --help' (or 'kernl help <subcommand>') for details.`)
	fmt.Println(b.String())
	return nil
}

func printVersion() error {
	fmt.Printf("kernl %s\ncommit: %s\nbuilt:  %s\ngo:     %s\n",
		Version, Commit, Date, runtime.Version())
	return nil
}
