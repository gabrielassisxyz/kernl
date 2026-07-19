package main

import (
	"encoding/json"
	"io"
	"runtime"
)

// contractVersion identifies the shape of the capabilities/robot output.
// Bump it when a machine-facing field changes meaning or disappears.
const contractVersion = "1.0.0"

// capabilitiesOutput is the self-describing contract: everything an agent
// needs to drive the CLI without reading source — verbs, flags, env vars and
// the exit-code dictionary. Generated from commandTable so it cannot drift
// from dispatch or help.
type capabilitiesOutput struct {
	Tool            string              `json:"tool"`
	Version         string              `json:"version"`
	ContractVersion string              `json:"contractVersion"`
	Go              string              `json:"go"`
	Commands        []capabilityCommand `json:"commands"`
	GlobalFlags     []capabilityFlag    `json:"globalFlags"`
	EnvVars         []capabilityEnvVar  `json:"envVars"`
	ExitCodes       []capabilityExit    `json:"exitCodes"`
}

type capabilityCommand struct {
	Name        string              `json:"name"`
	Summary     string              `json:"summary"`
	Usage       string              `json:"usage"`
	Details     string              `json:"details,omitempty"`
	Subcommands []capabilityCommand `json:"subcommands,omitempty"`
}

type capabilityFlag struct {
	Name        string `json:"name"`
	Alias       string `json:"alias,omitempty"`
	Description string `json:"description"`
}

type capabilityEnvVar struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type capabilityExit struct {
	Code    int    `json:"code"`
	Meaning string `json:"meaning"`
}

var capabilityEnvVars = []capabilityEnvVar{
	{Name: "KERNL_SERVER", Description: "address of the running server the GUI-parity verbs call (e.g. http://127.0.0.1:8080); overridden by --server"},
	{Name: "KERNL_LOG_LEVEL", Description: "slog level for all verbs: debug, info (default), warn, error"},
	{Name: "KERNL_JSON_LOG_DIR", Description: "when set, structured JSON logs are also written to this directory"},
	{Name: "KERNL_LOG_ROOT", Description: "root directory for per-session dispatch forensics logs"},
	{Name: "FORCE_COLOR", Description: "force ANSI color even when stdout is not a TTY"},
	{Name: "NO_COLOR", Description: "disable ANSI color (https://no-color.org)"},
	{Name: "BD_BIN", Description: "override the bd binary used by the orchestrator backend"},
	{Name: "BD_DB", Description: "override the bd database path"},
	{Name: "KERNL_BD_LOCK_DIR", Description: "directory for bd CLI lock files (serializes concurrent bd calls)"},
	{Name: "KERNL_BD_COMMAND_TIMEOUT_MS", Description: "timeout in milliseconds for each bd CLI invocation"},
}

var capabilityExitCodes = []capabilityExit{
	{Code: 0, Meaning: "success"},
	{Code: 1, Meaning: "runtime/internal error (backend, config, network, agent run)"},
	{Code: 2, Meaning: "usage error (unknown verb/flag, missing argument, bad value)"},
}

var capabilityGlobalFlags = []capabilityFlag{
	{Name: "--config", Alias: "-c", Description: "path to kernl.yaml (default: kernl.yaml; accepts --config=path)"},
	{Name: "--port", Alias: "-p", Description: "server port (default: from kernl.yaml, or 8080)"},
	{Name: "--server", Description: "full address of the running server for GUI-parity verbs (e.g. http://127.0.0.1:8080); overrides --port and KERNL_SERVER"},
	{Name: "--no-orchestrator", Description: "serve only the GUI/graph/notes; do not require bd"},
	{Name: "--version", Alias: "-v", Description: "print version and build information"},
	{Name: "--help", Alias: "-h", Description: "show help for kernl or any verb/sub-verb"},
}

func runCapabilities(w io.Writer, args []string) error {
	for _, arg := range args {
		if arg != "--json" {
			return usagef("KERNL DISPATCH FAILURE: unknown capabilities flag %q%s — valid: --json (output is JSON either way)",
				arg, didYouMean(arg, []string{"--json"}))
		}
	}
	out := capabilitiesOutput{
		Tool:            "kernl",
		Version:         Version,
		ContractVersion: contractVersion,
		Go:              runtime.Version(),
		Commands:        capabilityCommands(commandTable),
		GlobalFlags:     capabilityGlobalFlags,
		EnvVars:         capabilityEnvVars,
		ExitCodes:       capabilityExitCodes,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func capabilityCommands(table []commandMeta) []capabilityCommand {
	out := make([]capabilityCommand, 0, len(table))
	for _, c := range table {
		out = append(out, capabilityCommand{
			Name:    c.Name,
			Summary: c.Summary,
			Usage:   c.Usage,
			// Details carries the per-command flag documentation and examples.
			// Emitting it puts the flag contract in the machine surface instead of
			// leaving it human-only in --help. (Structured per-flag data would need
			// a Flags field on commandMeta across every verb — a larger change.)
			Details:     c.Details,
			Subcommands: capabilityCommands(c.Subs),
		})
	}
	return out
}
