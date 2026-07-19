package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
)

var settingsSections = []string{"llm", "vault", "inbox", "runtime"}

var settingsCommand = commandMeta{
	Name:    "settings",
	Summary: "Read and write kernl.yaml through the running server",
	Usage:   "kernl settings <get|set> [args...]",
	Details: `The same settings page the GUI exposes, from the shell. A server must be
running ('kernl serve'), or point elsewhere with --server <url> (env:
KERNL_SERVER).

Every write lands in the kernl.yaml the server was started with — nothing
is applied to the process that is already running. Fields you change stay
listed under "restart pending" until the server is restarted.

Run 'kernl settings <subcommand> --help' for details on each.`,
	Subs: []commandMeta{
		{
			Name:    "get",
			Summary: "Show the saved settings and which keys await a restart",
			Usage:   "kernl settings get [--json]",
			Details: `Reads the config file back from disk, so it shows what is persisted, not
what the running process holds. The LLM API key is never returned by the
server — only whether one is set.

Flags:
  --json  Emit the API's settings object verbatim (camelCase)`,
		},
		{
			Name:    "set",
			Summary: "Update one section of kernl.yaml (llm, vault, inbox, runtime)",
			Usage:   "kernl settings set <llm|vault|inbox|runtime> [--field <value>...] [--json]",
			Details: `Each section is written as a whole, so the current values are read first
and only the flags you pass are changed. At least one flag is required.

Saved to kernl.yaml, NOT applied to the running server: the process keeps
the values it booted with until you restart it. The output lists the keys
still waiting on that restart.

llm      --provider <openai|anthropic|ollama|noop>
         --model <name>            --endpoint <http(s) url>
         --api-key <key>           (never echoed back; omit to keep the stored one)
vault    --root <absolute dir>     (must already exist)
         --coalesce-window-ms <n>  --move-window-ms <n>
         --rescan-interval-sec <n>
inbox    --auto-prep <true|false>  --da-subdir <relative path inside the vault>
runtime  --server-port <1-65535>   --worktree-root <absolute path>
         --max-concurrent-beads <1-64>
         --run-state-path <absolute path>
         --stage-retry-attempts <0-10>
         --sweep-interval-sec <n>  --pr-stale-warn-days <n>
         --sweep-failure-limit <n> --sweep-backoff-minutes <5,15,60>

Flags:
  --json  Emit the API's full settings object after the write

Example:
  kernl settings set llm --provider openai --model gpt-4o-mini`,
		},
	},
}

func runSettings(v verbContext, args []string) error {
	sub, rest, err := requireSub("settings", args, []string{"get", "set"})
	if err != nil {
		return err
	}
	asJSON, rest := parseBoolFlag(rest, "--json")

	client, err := v.client()
	if err != nil {
		return err
	}
	if sub == "get" {
		return settingsGet(v, client, asJSON, rest)
	}
	return settingsSet(v, client, asJSON, rest)
}

// settingsSnapshot mirrors the subset of the settings DTO the CLI needs: the
// current values (to merge a partial edit onto) and the restart-pending list.
type settingsSnapshot struct {
	ConfigPath     string                 `json:"configPath"`
	Writable       bool                   `json:"writable"`
	RestartPending []string               `json:"restartPending"`
	LLM            settingsLLMView        `json:"llm"`
	Vault          settingsVaultSection   `json:"vault"`
	Inbox          settingsInboxSection   `json:"inbox"`
	Runtime        settingsRuntimeSection `json:"runtime"`
}

// settingsLLMView has no key field on purpose: the server reports only whether
// a credential exists, so the CLI has nothing to leak.
type settingsLLMView struct {
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	Endpoint  string `json:"endpoint"`
	APIKeySet bool   `json:"apiKeySet"`
}

type settingsVaultSection struct {
	Root              string `json:"root"`
	CoalesceWindowMs  int    `json:"coalesceWindowMs"`
	MoveWindowMs      int    `json:"moveWindowMs"`
	RescanIntervalSec int    `json:"rescanIntervalSec"`
}

type settingsInboxSection struct {
	AutoPrep bool   `json:"autoPrep"`
	DASubdir string `json:"daSubdir"`
}

type settingsRuntimeSection struct {
	ServerPort          int    `json:"serverPort"`
	WorktreeRoot        string `json:"worktreeRoot"`
	MaxConcurrentBeads  int    `json:"maxConcurrentBeads"`
	RunStatePath        string `json:"runStatePath"`
	StageRetryAttempts  int    `json:"stageRetryAttempts"`
	SweepIntervalSec    int    `json:"sweepIntervalSec"`
	PRStaleWarnDays     int    `json:"prStaleWarnDays"`
	SweepFailureLimit   int    `json:"sweepFailureLimit"`
	SweepBackoffMinutes []int  `json:"sweepBackoffMinutes"`
}

// settingsLLMBody is the PUT payload. APIKey is a pointer so an omitted
// --api-key leaves the stored credential alone instead of clearing it.
type settingsLLMBody struct {
	Provider string  `json:"provider"`
	Model    string  `json:"model"`
	Endpoint string  `json:"endpoint"`
	APIKey   *string `json:"apiKey,omitempty"`
}

func settingsGet(v verbContext, c *apiClient, asJSON bool, args []string) error {
	if err := rejectUnknownFlags("settings get", args); err != nil {
		return err
	}
	if len(args) > 0 {
		return usagef("KERNL DISPATCH FAILURE: settings get takes no arguments, got %q — run: kernl settings get [--json]", args[0])
	}

	raw, err := c.get(context.Background(), "/api/settings")
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(v.stdout(), raw)
	}
	var snap settingsSnapshot
	if err := decodeInto(raw, "GET /api/settings", &snap); err != nil {
		return err
	}
	printSettingsHeader(v.stdout(), snap)
	for _, section := range settingsSections {
		printSettingsSection(v.stdout(), snap, section)
	}
	return nil
}

func settingsSet(v verbContext, c *apiClient, asJSON bool, args []string) error {
	section, rest, err := requireSub("settings set", args, settingsSections)
	if err != nil {
		return err
	}
	snap, err := readSettings(c)
	if err != nil {
		return err
	}
	body, err := mergeSettingsSection(section, snap, rest)
	if err != nil {
		return err
	}

	raw, err := c.put(context.Background(), "/api/settings/"+section, body)
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(v.stdout(), raw)
	}
	var updated settingsSnapshot
	if err := decodeInto(raw, "PUT /api/settings/"+section, &updated); err != nil {
		return err
	}
	printSettingsSection(v.stdout(), updated, section)
	printSaveNotice(v.stdout(), updated)
	return nil
}

func readSettings(c *apiClient) (settingsSnapshot, error) {
	var snap settingsSnapshot
	raw, err := c.get(context.Background(), "/api/settings")
	if err != nil {
		return snap, err
	}
	err = decodeInto(raw, "GET /api/settings", &snap)
	return snap, err
}

const settingsSetVerb = "settings set" // every merge path below is reached only through this verb

func mergeSettingsSection(section string, snap settingsSnapshot, args []string) (any, error) {
	switch section {
	case "llm":
		return mergeLLMSection(snap.LLM, args)
	case "vault":
		return mergeVaultSection(snap.Vault, args)
	case "inbox":
		return mergeInboxSection(snap.Inbox, args)
	}
	return mergeRuntimeSection(snap.Runtime, args)
}

func mergeLLMSection(current settingsLLMView, args []string) (any, error) {
	body := settingsLLMBody{Provider: current.Provider, Model: current.Model, Endpoint: current.Endpoint}
	changed, err := applyStringFlags(&args, []stringFlagTarget{
		{"--provider", &body.Provider},
		{"--model", &body.Model},
		{"--endpoint", &body.Endpoint},
	})
	if err != nil {
		return nil, err
	}
	key, hasKey, args, err := takeFlag(settingsSetVerb, args, "--api-key")
	if err != nil {
		return nil, err
	}
	if hasKey {
		body.APIKey, changed = &key, true
	}
	if err := finishSettingsFlags("llm", changed, args); err != nil {
		return nil, err
	}
	return body, nil
}

func mergeVaultSection(current settingsVaultSection, args []string) (any, error) {
	body := current
	changedStrings, err := applyStringFlags(&args, []stringFlagTarget{{"--root", &body.Root}})
	if err != nil {
		return nil, err
	}
	changedInts, err := applyIntFlags(&args, []intFlagTarget{
		{"--coalesce-window-ms", &body.CoalesceWindowMs},
		{"--move-window-ms", &body.MoveWindowMs},
		{"--rescan-interval-sec", &body.RescanIntervalSec},
	})
	if err != nil {
		return nil, err
	}
	if err := finishSettingsFlags("vault", changedStrings || changedInts, args); err != nil {
		return nil, err
	}
	return body, nil
}

func mergeInboxSection(current settingsInboxSection, args []string) (any, error) {
	body := current
	changedStrings, err := applyStringFlags(&args, []stringFlagTarget{{"--da-subdir", &body.DASubdir}})
	if err != nil {
		return nil, err
	}
	raw, hasAutoPrep, args, err := takeFlag(settingsSetVerb, args, "--auto-prep")
	if err != nil {
		return nil, err
	}
	if hasAutoPrep {
		parsed, convErr := strconv.ParseBool(strings.TrimSpace(raw))
		if convErr != nil {
			return nil, usagef("KERNL DISPATCH FAILURE: --auto-prep takes true or false, got %q — run: kernl settings set inbox --auto-prep true", raw)
		}
		body.AutoPrep = parsed
	}
	if err := finishSettingsFlags("inbox", changedStrings || hasAutoPrep, args); err != nil {
		return nil, err
	}
	return body, nil
}

func mergeRuntimeSection(current settingsRuntimeSection, args []string) (any, error) {
	body := current
	changedStrings, err := applyStringFlags(&args, []stringFlagTarget{
		{"--worktree-root", &body.WorktreeRoot},
		{"--run-state-path", &body.RunStatePath},
	})
	if err != nil {
		return nil, err
	}
	changedInts, err := applyIntFlags(&args, []intFlagTarget{
		{"--server-port", &body.ServerPort},
		{"--max-concurrent-beads", &body.MaxConcurrentBeads},
		{"--stage-retry-attempts", &body.StageRetryAttempts},
		{"--sweep-interval-sec", &body.SweepIntervalSec},
		{"--pr-stale-warn-days", &body.PRStaleWarnDays},
		{"--sweep-failure-limit", &body.SweepFailureLimit},
	})
	if err != nil {
		return nil, err
	}
	backoff, hasBackoff, args, err := takeFlag(settingsSetVerb, args, "--sweep-backoff-minutes")
	if err != nil {
		return nil, err
	}
	if hasBackoff {
		if body.SweepBackoffMinutes, err = parseBackoffMinutes(backoff); err != nil {
			return nil, err
		}
	}
	if err := finishSettingsFlags("runtime", changedStrings || changedInts || hasBackoff, args); err != nil {
		return nil, err
	}
	return body, nil
}

type stringFlagTarget struct {
	flag   string
	target *string
}

type intFlagTarget struct {
	flag   string
	target *int
}

// applyStringFlags overwrites a target only when its flag was actually given,
// which is what makes a partial edit safe against a whole-section PUT.
func applyStringFlags(args *[]string, targets []stringFlagTarget) (bool, error) {
	changed := false
	for _, t := range targets {
		value, present, rest, err := takeFlag(settingsSetVerb, *args, t.flag)
		if err != nil {
			return false, err
		}
		*args = rest
		if present {
			*t.target, changed = value, true
		}
	}
	return changed, nil
}

func applyIntFlags(args *[]string, targets []intFlagTarget) (bool, error) {
	changed := false
	for _, t := range targets {
		raw, present, rest, err := takeFlag(settingsSetVerb, *args, t.flag)
		if err != nil {
			return false, err
		}
		*args = rest
		if !present {
			continue
		}
		value, convErr := strconv.Atoi(strings.TrimSpace(raw))
		if convErr != nil {
			return false, usagef("KERNL DISPATCH FAILURE: %s takes a whole number, got %q — run: kernl settings set --help", t.flag, raw)
		}
		*t.target, changed = value, true
	}
	return changed, nil
}

func parseBackoffMinutes(raw string) ([]int, error) {
	steps := []int{}
	for _, part := range strings.Split(raw, ",") {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		value, err := strconv.Atoi(trimmed)
		if err != nil {
			return nil, usagef("KERNL DISPATCH FAILURE: --sweep-backoff-minutes takes comma-separated whole minutes, got %q — example: --sweep-backoff-minutes 5,15,60", raw)
		}
		steps = append(steps, value)
	}
	if len(steps) == 0 {
		return nil, usagef("KERNL DISPATCH FAILURE: --sweep-backoff-minutes needs at least one step — example: --sweep-backoff-minutes 5,15,60")
	}
	return steps, nil
}

// finishSettingsFlags rejects leftovers and refuses a no-op write: a PUT that
// changes nothing would still rewrite the file and reorder it for no reason.
func finishSettingsFlags(section string, changed bool, rest []string) error {
	if err := rejectUnknownFlags("settings set "+section, rest); err != nil {
		return err
	}
	if len(rest) > 0 {
		return usagef("KERNL DISPATCH FAILURE: settings set %s takes no positional arguments, got %q — run: kernl settings set --help", section, rest[0])
	}
	if !changed {
		return usagef("KERNL DISPATCH FAILURE: settings set %s changes nothing — pass at least one field flag. Run: kernl settings set --help", section)
	}
	return nil
}

func printSettingsHeader(w io.Writer, snap settingsSnapshot) {
	if !snap.Writable || snap.ConfigPath == "" {
		fmt.Fprintln(w, "Config file: none — this server was started without one, so settings cannot be saved")
		return
	}
	fmt.Fprintf(w, "Config file: %s\n", snap.ConfigPath)
	printSaveNotice(w, snap)
}

func printSaveNotice(w io.Writer, snap settingsSnapshot) {
	if len(snap.RestartPending) == 0 {
		fmt.Fprintln(w, "Saved file and running server agree.")
		return
	}
	fmt.Fprintf(w, "Saved to the config file but NOT active yet — restart 'kernl serve' to apply: %s\n",
		strings.Join(snap.RestartPending, ", "))
}

func printSettingsSection(w io.Writer, snap settingsSnapshot, section string) {
	switch section {
	case "llm":
		fmt.Fprintf(w, "llm      provider=%s model=%s endpoint=%s apiKey=%s\n",
			orUnset(snap.LLM.Provider), orUnset(snap.LLM.Model), orUnset(snap.LLM.Endpoint), keyState(snap.LLM.APIKeySet))
	case "vault":
		fmt.Fprintf(w, "vault    root=%s coalesceWindowMs=%d moveWindowMs=%d rescanIntervalSec=%d\n",
			orUnset(snap.Vault.Root), snap.Vault.CoalesceWindowMs, snap.Vault.MoveWindowMs, snap.Vault.RescanIntervalSec)
	case "inbox":
		fmt.Fprintf(w, "inbox    autoPrep=%t daSubdir=%s\n", snap.Inbox.AutoPrep, orUnset(snap.Inbox.DASubdir))
	default:
		r := snap.Runtime
		fmt.Fprintf(w, "runtime  serverPort=%d worktreeRoot=%s maxConcurrentBeads=%d runStatePath=%s\n",
			r.ServerPort, orUnset(r.WorktreeRoot), r.MaxConcurrentBeads, orUnset(r.RunStatePath))
		fmt.Fprintf(w, "         stageRetryAttempts=%d sweepIntervalSec=%d prStaleWarnDays=%d sweepFailureLimit=%d backoffMinutes=%s\n",
			r.StageRetryAttempts, r.SweepIntervalSec, r.PRStaleWarnDays, r.SweepFailureLimit, joinMinutes(r.SweepBackoffMinutes))
	}
}

func orUnset(value string) string {
	if strings.TrimSpace(value) == "" {
		return "(unset)"
	}
	return value
}

// keyState is the only thing ever printed about the credential — the key value
// itself must not reach stdout, a log, or a screenshot.
func keyState(set bool) string {
	if set {
		return "set"
	}
	return "(unset)"
}

func joinMinutes(values []int) string {
	if len(values) == 0 {
		return "(unset)"
	}
	parts := make([]string, 0, len(values))
	for _, v := range values {
		parts = append(parts, strconv.Itoa(v))
	}
	return strings.Join(parts, ",")
}
