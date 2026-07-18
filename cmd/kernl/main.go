package main

import (
	"encoding/json"
	"fmt"
	"io"
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

// parseStringFlag strips every occurrence of a global value flag, accepting
// both '--flag value' and '--flag=value'. When the flag repeats, the last
// value wins (the same semantics as epic run's --workflow, which used to
// disagree with --config's first-wins). A flag with no value fails loud.
func parseStringFlag(args []string, long, short string) (value string, rest []string, err error) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == long || a == short:
			if i+1 >= len(args) {
				return "", nil, usagef("KERNL DISPATCH FAILURE: %s requires a value — run: kernl --help", a)
			}
			value = args[i+1]
			i++
		case strings.HasPrefix(a, long+"="):
			value = strings.TrimPrefix(a, long+"=")
			if value == "" {
				return "", nil, usagef("KERNL DISPATCH FAILURE: %s requires a value — run: kernl --help", long)
			}
		default:
			rest = append(rest, a)
		}
	}
	return value, rest, nil
}

func parseConfigPath(args []string) (string, []string, error) {
	return parseStringFlag(args, "--config", "-c")
}

func parsePort(args []string) (int, []string, error) {
	raw, rest, err := parseStringFlag(args, "--port", "-p")
	if err != nil || raw == "" {
		return 0, rest, err
	}
	port, err := strconv.Atoi(raw)
	if err != nil {
		return 0, nil, usagef("KERNL DISPATCH FAILURE: --port needs an integer, got %q — example: kernl --port 8080 serve", raw)
	}
	return port, rest, nil
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

	configPath, args, err := parseConfigPath(args)
	if err != nil {
		return err
	}
	if configPath == "" {
		configPath = "kernl.yaml"
	}

	port, args, err := parsePort(args)
	if err != nil {
		return err
	}
	noOrch, args := parseBoolFlag(args, "--no-orchestrator")

	if len(args) == 0 {
		return helpFn()
	}

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
		return printVersion(os.Stdout, args[1:])
	default:
		if strings.HasPrefix(args[0], "-") {
			return usagef("KERNL DISPATCH FAILURE: unknown flag %q%s. Run: kernl --help",
				args[0], didYouMean(args[0], globalFlagNames))
		}
		return usagef("KERNL DISPATCH FAILURE: unknown subcommand %q%s. Run: kernl --help",
			args[0], didYouMean(args[0], append(commandNames(), "help")))
	}
}

// globalFlagNames are the flags the root parser understands, used for
// did-you-mean hints on unknown-flag errors.
var globalFlagNames = []string{
	"--config", "-c", "--port", "-p", "--no-orchestrator",
	"--version", "-v", "--help", "-h",
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

func printVersion(w io.Writer, args []string) error {
	var asJSON bool
	for _, arg := range args {
		switch arg {
		case "--json":
			asJSON = true
		default:
			return usagef("KERNL DISPATCH FAILURE: unknown version flag %q%s — valid: --json",
				arg, didYouMean(arg, []string{"--json"}))
		}
	}
	if asJSON {
		return json.NewEncoder(w).Encode(map[string]string{
			"version": Version,
			"commit":  Commit,
			"built":   Date,
			"go":      runtime.Version(),
		})
	}
	fmt.Fprintf(w, "kernl %s\ncommit: %s\nbuilt:  %s\ngo:     %s\n",
		Version, Commit, Date, runtime.Version())
	return nil
}
