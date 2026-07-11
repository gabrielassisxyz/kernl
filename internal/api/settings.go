package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/vault"
)

// llmProviders are the providers the chat registry can actually construct.
var llmProviders = []string{"openai", "anthropic", "ollama", "noop"}

type settingsResponse struct {
	ConfigPath string `json:"configPath"`
	// Writable reports whether this process knows which file it loaded. A config
	// built in memory (tests, embedded harnesses) has nowhere to write back to.
	Writable bool `json:"writable"`
	// RestartPending lists the dotted keys whose saved value no longer matches the
	// value this process is running with. The UI surfaces these instead of
	// pretending a saved field already took effect.
	RestartPending []string             `json:"restartPending"`
	LLM            llmSettings          `json:"llm"`
	Vault          vaultSettings        `json:"vault"`
	Inbox          inboxSettings        `json:"inbox"`
	Runtime        runtimeSettings      `json:"runtime"`
	Prompts        []readOnlySettingRow `json:"prompts"`
	Agents         []readOnlySettingRow `json:"agents"`
}

type llmSettings struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Endpoint string `json:"endpoint"`
	// APIKeySet reports whether a credential exists. The key itself is never
	// serialized: the settings page must be safe to screenshot.
	APIKeySet bool `json:"apiKeySet"`
}

type vaultSettings struct {
	Root              string `json:"root"`
	CoalesceWindowMs  int    `json:"coalesceWindowMs"`
	MoveWindowMs      int    `json:"moveWindowMs"`
	RescanIntervalSec int    `json:"rescanIntervalSec"`
}

type inboxSettings struct {
	AutoPrep bool   `json:"autoPrep"`
	DASubdir string `json:"daSubdir"`
}

type runtimeSettings struct {
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

type readOnlySettingRow struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Value       string `json:"value"`
	Source      string `json:"source"`
	Reason      string `json:"reason"`
}

// llmUpdate uses a pointer for the credential so the UI can leave it untouched
// (omitted) without having to echo the key back to the server.
type llmUpdate struct {
	Provider string  `json:"provider"`
	Model    string  `json:"model"`
	Endpoint string  `json:"endpoint"`
	APIKey   *string `json:"apiKey"`
}

func RegisterSettingsRoutes(mux *http.ServeMux, a *app.App) {
	mux.HandleFunc("GET /api/settings", settingsHandler(a))
	mux.HandleFunc("PUT /api/settings/llm", updateLLMHandler(a))
	mux.HandleFunc("PUT /api/settings/vault", updateVaultHandler(a))
	mux.HandleFunc("PUT /api/settings/inbox", updateInboxHandler(a))
	mux.HandleFunc("PUT /api/settings/runtime", updateRuntimeHandler(a))
}

func settingsHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if a.Config == nil {
			writeError(w, http.StatusServiceUnavailable, "settings require a loaded config")
			return
		}

		saved, err := savedConfig(a)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(buildSettings(a.ConfigPath, saved, a.Config))
	}
}

// savedConfig re-reads the config from disk so the page shows what is persisted,
// not what this process happens to be holding. They diverge on purpose after a
// write, and that divergence is what "restart to apply" is built from.
//
// serve applies the vault defaults into the running config after boot, so the
// re-read has to go through the same normalization. Skipping it would make an
// untouched config report its own defaults as unsaved changes.
func savedConfig(a *app.App) (*config.Config, error) {
	if a.ConfigPath == "" {
		return a.Config, nil
	}
	saved, err := config.Load(a.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("re-reading %s: %w", a.ConfigPath, err)
	}
	vault.ApplyDefaults(&saved.Vault)
	return saved, nil
}

func buildSettings(configPath string, saved, running *config.Config) settingsResponse {
	return settingsResponse{
		ConfigPath:     configPath,
		Writable:       configPath != "",
		RestartPending: restartPending(saved, running),
		LLM: llmSettings{
			Provider:  saved.LLM.Provider,
			Model:     saved.LLM.Model,
			Endpoint:  saved.LLM.Endpoint,
			APIKeySet: saved.LLM.APIKey != "",
		},
		Vault: vaultSettings{
			Root:              saved.Vault.Root,
			CoalesceWindowMs:  saved.Vault.CoalesceWindowMs,
			MoveWindowMs:      saved.Vault.MoveWindowMs,
			RescanIntervalSec: saved.Vault.RescanIntervalSec,
		},
		Inbox: inboxSettings{
			AutoPrep: saved.Inbox.AutoPrep,
			DASubdir: saved.Inbox.DASubdir,
		},
		Runtime: runtimeSettings{
			ServerPort:          saved.Server.Port,
			WorktreeRoot:        saved.Orchestrator.WorktreeRoot,
			MaxConcurrentBeads:  saved.Orchestrator.MaxConcurrentBeads,
			RunStatePath:        saved.Orchestrator.RunStatePath,
			StageRetryAttempts:  saved.Orchestrator.StageRetryAttempts,
			SweepIntervalSec:    saved.Sweep.AutoIntervalSeconds,
			PRStaleWarnDays:     saved.Sweep.PRStaleWarnDays,
			SweepFailureLimit:   saved.Sweep.FailureThreshold,
			SweepBackoffMinutes: saved.Sweep.BackoffMinutes,
		},
		Prompts: promptRows(),
		Agents:  agentRows(saved),
	}
}

// restartPending compares the saved config against the one this process booted
// with, key by key, and returns the dotted keys that differ.
func restartPending(saved, running *config.Config) []string {
	pending := []string{}
	compare := []struct {
		key            string
		saved, running any
	}{
		{"llm.provider", saved.LLM.Provider, running.LLM.Provider},
		{"llm.model", saved.LLM.Model, running.LLM.Model},
		{"llm.endpoint", saved.LLM.Endpoint, running.LLM.Endpoint},
		{"llm.api_key", saved.LLM.APIKey != "", running.LLM.APIKey != ""},
		{"vault.root", saved.Vault.Root, running.Vault.Root},
		{"vault.coalesceWindowMs", saved.Vault.CoalesceWindowMs, running.Vault.CoalesceWindowMs},
		{"vault.moveWindowMs", saved.Vault.MoveWindowMs, running.Vault.MoveWindowMs},
		{"vault.rescanIntervalSec", saved.Vault.RescanIntervalSec, running.Vault.RescanIntervalSec},
		{"inbox.auto_prep", saved.Inbox.AutoPrep, running.Inbox.AutoPrep},
		{"inbox.da_subdir", saved.Inbox.DASubdir, running.Inbox.DASubdir},
		{"server.port", saved.Server.Port, running.Server.Port},
		{"orchestrator.worktreeRoot", saved.Orchestrator.WorktreeRoot, running.Orchestrator.WorktreeRoot},
		{"orchestrator.maxConcurrentBeads", saved.Orchestrator.MaxConcurrentBeads, running.Orchestrator.MaxConcurrentBeads},
		{"orchestrator.runStatePath", saved.Orchestrator.RunStatePath, running.Orchestrator.RunStatePath},
		{"orchestrator.stageRetryAttempts", saved.Orchestrator.StageRetryAttempts, running.Orchestrator.StageRetryAttempts},
		{"sweep.auto_interval_seconds", saved.Sweep.AutoIntervalSeconds, running.Sweep.AutoIntervalSeconds},
		{"sweep.pr_stale_warn_days", saved.Sweep.PRStaleWarnDays, running.Sweep.PRStaleWarnDays},
		{"sweep.failure_threshold", saved.Sweep.FailureThreshold, running.Sweep.FailureThreshold},
		{"sweep.backoff_minutes", saved.Sweep.BackoffMinutes, running.Sweep.BackoffMinutes},
	}

	for _, field := range compare {
		if !reflect.DeepEqual(field.saved, field.running) {
			pending = append(pending, field.key)
		}
	}
	return pending
}

func promptRows() []readOnlySettingRow {
	return []readOnlySettingRow{
		{
			Key:         "prompts.notes.applyInstruction",
			Label:       "Note edit instruction",
			Description: "Prompt used when applying an AI edit instruction to a note body.",
			Value:       "Built in",
			Source:      "code",
			Reason:      "Lives in Go. Editing it needs a prompt store, not a config field.",
		},
		{
			Key:         "prompts.inbox.classifier",
			Label:       "Inbox classifier",
			Description: "Prompt family used to classify captures and suggest next actions.",
			Value:       "Built in",
			Source:      "code",
			Reason:      "Lives in Go. Editing it needs a prompt store, not a config field.",
		},
		{
			Key:         "prompts.inbox.prep",
			Label:       "Inbox prep",
			Description: "Prompt family used to prepare DA-authored brief notes from captures.",
			Value:       "Built in",
			Source:      "code",
			Reason:      "Lives in Go. Editing it needs a prompt store, not a config field.",
		},
	}
}

func agentRows(cfg *config.Config) []readOnlySettingRow {
	rows := []readOnlySettingRow{
		{
			Key:         "settings.agents",
			Label:       "Agents",
			Description: "CLI agents Kernl can dispatch work to.",
			Value:       joinOrNone(sortedKeys(cfg.Settings.Agents)),
			Source:      "yaml",
			Reason:      "Nested command, args, and env. Editing it safely needs its own editor.",
		},
		{
			Key:         "settings.pools",
			Label:       "Pools",
			Description: "Weighted dispatch pools built from the agents above.",
			Value:       joinOrNone(sortedKeys(cfg.Settings.Pools)),
			Source:      "yaml",
			Reason:      "Nested weighted lists. Editing it safely needs its own editor.",
		},
		{
			Key:         "settings.actions.take",
			Label:       "Take action agent",
			Description: "Agent used for take-loop sessions.",
			Value:       orNone(cfg.Settings.Actions.Take),
			Source:      "yaml",
			Reason:      "Bound to the agent list above.",
		},
		{
			Key:         "settings.actions.scene",
			Label:       "Scene action agent",
			Description: "Agent used for scene sessions.",
			Value:       orNone(cfg.Settings.Actions.Scene),
			Source:      "yaml",
			Reason:      "Bound to the agent list above.",
		},
		{
			Key:         "settings.actions.scopeRefinement",
			Label:       "Scope refinement agent",
			Description: "Agent used to refine bead scope.",
			Value:       orNone(cfg.Settings.Actions.ScopeRefinement),
			Source:      "yaml",
			Reason:      "Bound to the agent list above.",
		},
		{
			Key:         "settings.actions.staleGrooming",
			Label:       "Stale grooming agent",
			Description: "Agent used for stale-work grooming.",
			Value:       orNone(cfg.Settings.Actions.StaleGrooming),
			Source:      "yaml",
			Reason:      "Bound to the agent list above.",
		},
		{
			Key:         "settings.defaults.interactiveSessionTimeoutMinutes",
			Label:       "Interactive session timeout",
			Description: "How long a session waiting on a human stays alive.",
			Value:       fmt.Sprintf("%d minutes", cfg.Settings.Defaults.InteractiveSessionTimeoutMinutes),
			Source:      "yaml",
			Reason:      "Bound to the agent list above.",
		},
	}
	return rows
}

func updateLLMHandler(a *app.App) http.HandlerFunc {
	return writeHandler(a, func(body []byte, current *config.Config) ([]config.Update, error) {
		var update llmUpdate
		if err := json.Unmarshal(body, &update); err != nil {
			return nil, badRequest("request body is not valid JSON")
		}

		provider := strings.TrimSpace(update.Provider)
		model := strings.TrimSpace(update.Model)
		endpoint := strings.TrimSpace(update.Endpoint)

		if provider != "" && !contains(llmProviders, provider) {
			return nil, badRequest(fmt.Sprintf("unknown provider %q — pick one of %s", provider, strings.Join(llmProviders, ", ")))
		}
		if provider != "" && model == "" {
			return nil, badRequest("a model is required when a provider is set")
		}
		if endpoint != "" {
			parsed, err := url.Parse(endpoint)
			if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
				return nil, badRequest("endpoint must be an absolute http:// or https:// URL")
			}
		}

		updates := []config.Update{
			{Path: []string{"llm", "provider"}, Value: provider},
			{Path: []string{"llm", "model"}, Value: model},
			{Path: []string{"llm", "endpoint"}, Value: endpoint},
		}
		// A nil APIKey means "leave the stored credential alone", which is what the
		// UI sends whenever the user did not retype the key.
		if update.APIKey != nil {
			updates = append(updates, config.Update{Path: []string{"llm", "api_key"}, Value: strings.TrimSpace(*update.APIKey)})
		}
		return updates, nil
	})
}

func updateVaultHandler(a *app.App) http.HandlerFunc {
	return writeHandler(a, func(body []byte, current *config.Config) ([]config.Update, error) {
		var update vaultSettings
		if err := json.Unmarshal(body, &update); err != nil {
			return nil, badRequest("request body is not valid JSON")
		}

		root := strings.TrimSpace(update.Root)
		if root != "" {
			if !filepath.IsAbs(root) {
				return nil, badRequest("vault root must be an absolute path")
			}
			info, err := os.Stat(root)
			if err != nil || !info.IsDir() {
				return nil, badRequest(fmt.Sprintf("vault root %s is not an existing directory", root))
			}
		}
		if err := requireNonNegative("coalesce window", update.CoalesceWindowMs); err != nil {
			return nil, err
		}
		if err := requireNonNegative("move window", update.MoveWindowMs); err != nil {
			return nil, err
		}
		if err := requireNonNegative("rescan interval", update.RescanIntervalSec); err != nil {
			return nil, err
		}

		return []config.Update{
			{Path: []string{"vault", "root"}, Value: root},
			{Path: []string{"vault", "coalesceWindowMs"}, Value: update.CoalesceWindowMs},
			{Path: []string{"vault", "moveWindowMs"}, Value: update.MoveWindowMs},
			{Path: []string{"vault", "rescanIntervalSec"}, Value: update.RescanIntervalSec},
		}, nil
	})
}

func updateInboxHandler(a *app.App) http.HandlerFunc {
	return writeHandler(a, func(body []byte, current *config.Config) ([]config.Update, error) {
		var update inboxSettings
		if err := json.Unmarshal(body, &update); err != nil {
			return nil, badRequest("request body is not valid JSON")
		}

		subdir := strings.TrimSpace(update.DASubdir)
		if subdir == "" {
			return nil, badRequest("DA subdirectory is required")
		}
		// The subdir is joined onto the vault root to write DA-authored notes, so an
		// absolute or climbing path would let the inbox write outside the vault.
		if filepath.IsAbs(subdir) || strings.Contains(subdir, "..") {
			return nil, badRequest("DA subdirectory must be a relative path inside the vault")
		}

		return []config.Update{
			{Path: []string{"inbox", "auto_prep"}, Value: update.AutoPrep},
			{Path: []string{"inbox", "da_subdir"}, Value: subdir},
		}, nil
	})
}

func updateRuntimeHandler(a *app.App) http.HandlerFunc {
	return writeHandler(a, func(body []byte, current *config.Config) ([]config.Update, error) {
		var update runtimeSettings
		if err := json.Unmarshal(body, &update); err != nil {
			return nil, badRequest("request body is not valid JSON")
		}

		if update.ServerPort < 1 || update.ServerPort > 65535 {
			return nil, badRequest("server port must be between 1 and 65535")
		}
		if update.MaxConcurrentBeads < 1 || update.MaxConcurrentBeads > 64 {
			return nil, badRequest("max concurrent beads must be between 1 and 64")
		}
		if update.StageRetryAttempts < 0 || update.StageRetryAttempts > 10 {
			return nil, badRequest("stage retry attempts must be between 0 and 10")
		}
		if err := requireAbsolute("worktree root", update.WorktreeRoot); err != nil {
			return nil, err
		}
		if err := requireAbsolute("run-state path", update.RunStatePath); err != nil {
			return nil, err
		}
		if err := requireNonNegative("sweep interval", update.SweepIntervalSec); err != nil {
			return nil, err
		}
		if err := requireNonNegative("PR stale warning", update.PRStaleWarnDays); err != nil {
			return nil, err
		}
		if err := requireNonNegative("failure threshold", update.SweepFailureLimit); err != nil {
			return nil, err
		}
		if len(update.SweepBackoffMinutes) == 0 {
			return nil, badRequest("the backoff schedule needs at least one step")
		}
		for _, minutes := range update.SweepBackoffMinutes {
			if minutes <= 0 {
				return nil, badRequest("every backoff step must be a positive number of minutes")
			}
		}

		return []config.Update{
			{Path: []string{"server", "port"}, Value: update.ServerPort},
			{Path: []string{"orchestrator", "worktreeRoot"}, Value: strings.TrimSpace(update.WorktreeRoot)},
			{Path: []string{"orchestrator", "maxConcurrentBeads"}, Value: update.MaxConcurrentBeads},
			{Path: []string{"orchestrator", "runStatePath"}, Value: strings.TrimSpace(update.RunStatePath)},
			{Path: []string{"orchestrator", "stageRetryAttempts"}, Value: update.StageRetryAttempts},
			{Path: []string{"sweep", "auto_interval_seconds"}, Value: update.SweepIntervalSec},
			{Path: []string{"sweep", "pr_stale_warn_days"}, Value: update.PRStaleWarnDays},
			{Path: []string{"sweep", "failure_threshold"}, Value: update.SweepFailureLimit},
			{Path: []string{"sweep", "backoff_minutes"}, Value: update.SweepBackoffMinutes},
		}, nil
	})
}

// writeHandler is the shared shape of every settings write: guard that a file
// exists to write to, validate the body into a whitelisted update set, apply it,
// then answer with the freshly re-read settings so the UI never has to guess what
// landed on disk.
func writeHandler(a *app.App, validate func(body []byte, current *config.Config) ([]config.Update, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.Config == nil {
			writeError(w, http.StatusServiceUnavailable, "settings require a loaded config")
			return
		}
		if a.ConfigPath == "" {
			writeError(w, http.StatusConflict, "this process was started without a config file, so settings cannot be saved")
			return
		}

		body, err := readLimitedBody(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		updates, err := validate(body, a.Config)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		if err := config.Apply(a.ConfigPath, updates); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		saved, err := savedConfig(a)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(buildSettings(a.ConfigPath, saved, a.Config))
	}
}

func readLimitedBody(r *http.Request) ([]byte, error) {
	decoded, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil {
		return nil, fmt.Errorf("could not read request body")
	}
	return decoded, nil
}

type validationError struct{ msg string }

func (e validationError) Error() string { return e.msg }

func badRequest(msg string) error { return validationError{msg: msg} }

func requireNonNegative(label string, value int) error {
	if value < 0 {
		return badRequest(fmt.Sprintf("%s cannot be negative", label))
	}
	return nil
}

func requireAbsolute(label, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return badRequest(fmt.Sprintf("%s is required", label))
	}
	if !filepath.IsAbs(trimmed) {
		return badRequest(fmt.Sprintf("%s must be an absolute path", label))
	}
	return nil
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func orNone(value string) string {
	if strings.TrimSpace(value) == "" {
		return "Not set"
	}
	return value
}

func joinOrNone(values []string) string {
	if len(values) == 0 {
		return "None"
	}
	return strings.Join(values, ", ")
}

func sortedKeys[T any](items map[string]T) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
